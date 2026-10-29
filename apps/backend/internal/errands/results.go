package errands

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	// resultByteLimit is the maximum UTF-8 size of one stored result.
	resultByteLimit = 16 * 1024
	// truncationNote states that the transcript still holds the cut text.
	truncationNote = "\n\n[truncated; the full transcript is readable with get_task_conversation_kandev]"
	// cancellationNotice is the result a user cancel or delete leaves behind.
	cancellationNotice = "The errand was cancelled."
	// failureNotice is the result when a failed turn has no summary of its own.
	failureNotice         = "The errand failed before it produced a result."
	interruptedTurnNotice = "The errand was interrupted before it finished."
)

// TurnSettlement is one settled errand turn. Outcome is done or failed.
// FailureSummary is the sanitized failure text for a failed turn.
type TurnSettlement struct {
	SessionID      string
	TurnID         string
	SubmissionID   string
	Outcome        models.ErrandOutcome
	FailureSummary string
	Interrupted    bool
}

// OnErrandTurnSettled records a settled errand turn. A session that is not an
// errand is ignored. Capture of a parent-initiated turn is idempotent on the
// result turn id.
func (s *Service) OnErrandTurnSettled(ctx context.Context, settlement TurnSettlement) error {
	return s.onErrandTurnSettled(ctx, settlement, "")
}

func (s *Service) onErrandTurnSettled(ctx context.Context, settlement TurnSettlement, reconciliationAction string) error {
	if settlement.SessionID == "" || settlement.TurnID == "" {
		return nil
	}
	errand, err := s.store.GetErrand(ctx, settlement.SessionID)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load errand for settlement: %w", err)
	}
	unlock := s.parentLocks.lock(errand.ParentSessionID)
	row, changed, settleErr := s.settleTurn(ctx, errand, settlement)
	unlock()
	if settleErr != nil {
		return settleErr
	}
	if changed {
		if reconciliationAction != "" {
			s.logReconciled(row, reconciliationAction)
		}
		s.afterResultWrite(ctx, row)
	}
	return nil
}

// OnErrandTurnCancelled settles the exact turn observed by cancellation. A
// terminal turn that already won the race keeps its real outcome; otherwise a
// parent-origin turn becomes an interrupted failure and a user-origin turn
// keeps the errand's prior settled outcome.
func (s *Service) OnErrandTurnCancelled(ctx context.Context, sessionID, turnID, submissionID string) error {
	if sessionID == "" || turnID == "" {
		return nil
	}
	errand, err := s.store.GetErrand(ctx, sessionID)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load errand before cancellation settlement: %w", err)
	}
	if turnID == errand.FirstTurnID {
		if submissionID == "" {
			submissionID = errand.SubmissionID
		}
		if submissionID != errand.SubmissionID {
			return nil
		}
	}
	outcome := models.ErrandOutcomeFailed
	interrupted := true
	if s.turns != nil {
		latestID, latest, err := s.turns.LatestTurn(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("read current errand turn before cancellation settlement: %w", err)
		}
		if latestID != "" && latestID != turnID {
			return nil
		}
		if latest.Completed {
			outcome = models.ErrandOutcomeDone
			interrupted = false
		}
	}
	return s.OnErrandTurnSettled(ctx, TurnSettlement{
		SessionID: sessionID, TurnID: turnID, SubmissionID: submissionID,
		Outcome: outcome, FailureSummary: interruptedTurnNotice, Interrupted: interrupted,
	})
}

// OnParentPromptAdmitted marks that the errand's parent admitted a prompt.
// The sender must be that parent. An undelivered result is superseded so it
// is never delivered after the new turn starts.
func (s *Service) OnParentPromptAdmitted(ctx context.Context, errandSessionID, senderSessionID string) error {
	if errandSessionID == "" || senderSessionID == "" {
		return nil
	}
	errand, err := s.store.GetErrand(ctx, errandSessionID)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load errand for parent prompt: %w", err)
	}
	if errand.ParentSessionID != senderSessionID {
		return nil
	}
	unlock := s.parentLocks.lock(errand.ParentSessionID)
	defer unlock()
	row, changed, err := s.update(ctx, errand.ID, applyParentAdmission)
	if err != nil {
		return fmt.Errorf("record parent prompt: %w", err)
	}
	if changed {
		s.publish(ctx, row)
	}
	return nil
}

// SendResult captures the latest settled turn as a pending result when the
// send-result rule allows it.
func (s *Service) SendResult(ctx context.Context, errandID string) (*Snapshot, error) {
	errand, err := s.store.GetErrand(ctx, errandID)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil, notFound()
	}
	if err != nil {
		return nil, fmt.Errorf("load errand: %w", err)
	}
	status, err := s.Project(ctx, errand)
	if err != nil {
		return nil, err
	}
	if !errand.CanSendResult(status) {
		return nil, newError(ReasonResultNotEligible, "errand result cannot be sent", nil)
	}
	view, err := s.readTurn(ctx, errand.ID, errand.LatestSettledTurnID)
	if err != nil {
		return nil, err
	}
	text, truncated := boundResult(view.LastAgentText)
	unlock := s.parentLocks.lock(errand.ParentSessionID)
	row, _, err := s.update(ctx, errand.ID, func(e *models.Errand) bool {
		if !e.CanSendResult(status) {
			return false
		}
		writePendingResult(e, text, truncated, e.LatestSettledTurnID, models.ErrandResultOutput, s.clock.Now())
		return true
	})
	unlock()
	if err != nil {
		return nil, fmt.Errorf("send errand result: %w", err)
	}
	projected, err := s.Project(ctx, row)
	if err != nil {
		return nil, err
	}
	if !sentResult(row, errand.LatestSettledTurnID) {
		return nil, newError(ReasonResultNotEligible, "errand result cannot be sent", nil)
	}
	s.afterResultWrite(ctx, row)
	return &Snapshot{Errand: row, Status: projected}, nil
}

func sentResult(row *models.Errand, turnID string) bool {
	return row != nil && row.ResultState == models.ErrandResultPending && row.ResultTurnID == turnID
}

func (s *Service) settleTurn(
	ctx context.Context, errand *models.Errand, settlement TurnSettlement,
) (*models.Errand, bool, error) {
	if s.turns != nil {
		latestID, _, err := s.turns.LatestTurn(ctx, errand.ID)
		if err != nil {
			return nil, false, fmt.Errorf("verify current errand turn: %w", err)
		}
		if latestID != "" && latestID != settlement.TurnID {
			return errand, false, nil
		}
	}
	if settlement.TurnID == errand.FirstTurnID && settlement.SubmissionID != "" && settlement.SubmissionID != errand.SubmissionID {
		return errand, false, nil
	}
	view, err := s.readTurn(ctx, errand.ID, settlement.TurnID)
	if err != nil {
		return nil, false, err
	}
	parentInitiated := parentInitiatedTurn(errand, settlement.TurnID, view)
	alreadyCaptured := errand.ResultTurnID == settlement.TurnID && resultAlreadyCaptured(errand.ResultState)
	text, truncated, kind := resultOf(settlement, view, parentInitiated)
	now := s.clock.Now()
	row, changed, err := s.update(ctx, errand.ID, func(e *models.Errand) bool {
		return applySettlement(e, settlement, parentInitiated, text, truncated, kind, now)
	})
	if err != nil {
		return nil, false, fmt.Errorf("record errand settlement: %w", err)
	}
	if changed && parentInitiated && !alreadyCaptured {
		status := models.ErrandStatusDone
		if settlement.Outcome == models.ErrandOutcomeFailed {
			status = models.ErrandStatusFailed
		}
		countSettled(status)
		s.logger.Info("errand.settled", s.errandFields(row, "status", string(status))...)
	}
	return row, changed, nil
}

func parentInitiatedTurn(errand *models.Errand, turnID string, view TurnView) bool {
	if turnID == errand.FirstTurnID {
		return true
	}
	return view.EarliestUserSender != "" && view.EarliestUserSender == errand.ParentSessionID
}

func (s *Service) readTurn(ctx context.Context, sessionID, turnID string) (TurnView, error) {
	if s.turns == nil || turnID == "" {
		return TurnView{}, nil
	}
	view, err := s.turns.ReadTurn(ctx, sessionID, turnID)
	if err != nil {
		return TurnView{}, fmt.Errorf("read errand turn %s: %w", turnID, err)
	}
	return view, nil
}

func applyParentAdmission(e *models.Errand) bool {
	if e.Lifecycle == models.ErrandLifecycleCancelled {
		return false
	}
	changed := !e.AwaitingResult
	e.AwaitingResult = true
	if e.ResultState == models.ErrandResultPending {
		e.ResultState = models.ErrandResultSuperseded
		changed = true
	}
	return changed
}

func applySettlement(
	e *models.Errand, settlement TurnSettlement, parentInitiated bool,
	text string, truncated bool, kind models.ErrandResultKind, now time.Time,
) bool {
	changed := e.LatestSettledTurnID != settlement.TurnID
	e.LatestSettledTurnID = settlement.TurnID
	if !parentInitiated {
		return changed
	}
	if e.AwaitingResult {
		e.AwaitingResult = false
		changed = true
	}
	if e.ResultTurnID == settlement.TurnID && resultAlreadyCaptured(e.ResultState) {
		return changed
	}
	writePendingResult(e, text, truncated, settlement.TurnID, kind, now)
	if settlement.Outcome == models.ErrandOutcomeDone || settlement.Outcome == models.ErrandOutcomeFailed {
		e.LastOutcome = settlement.Outcome
	}
	return true
}

func resultAlreadyCaptured(state models.ErrandResultState) bool {
	return state == models.ErrandResultPending || state == models.ErrandResultDelivered
}

func writePendingResult(
	e *models.Errand, text string, truncated bool, turnID string,
	kind models.ErrandResultKind, now time.Time,
) {
	e.ResultText = text
	e.ResultTruncated = truncated
	e.ResultTurnID = turnID
	e.ResultKind = kind
	e.ResultState = models.ErrandResultPending
	e.DeliveredRoute = ""
	e.DeliveryToken = ""
	e.DeliveredAt = nil
	e.SettledAt = &now
}

func resultOf(settlement TurnSettlement, view TurnView, parentInitiated bool) (string, bool, models.ErrandResultKind) {
	if !parentInitiated {
		return "", false, ""
	}
	if settlement.Interrupted {
		summary := settlement.FailureSummary
		if summary == "" {
			summary = interruptedTurnNotice
		}
		text, truncated := boundResult(summary)
		return text, truncated, models.ErrandResultInterrupted
	}
	if settlement.Outcome == models.ErrandOutcomeFailed {
		summary := settlement.FailureSummary
		if summary == "" {
			summary = view.LastAgentText
		}
		if summary == "" {
			summary = failureNotice
		}
		text, truncated := boundResult(summary)
		return text, truncated, models.ErrandResultFailure
	}
	text, truncated := boundResult(view.LastAgentText)
	return text, truncated, models.ErrandResultOutput
}

// boundResult keeps a result inside 16 KiB and says when the transcript continues.
func boundResult(text string) (string, bool) {
	text = sysprompt.StripTags(text)
	if len(text) <= resultByteLimit {
		return text, false
	}
	budget := resultByteLimit - len(truncationNote)
	return cutBytes(text, budget) + truncationNote, true
}

func (s *Service) afterResultWrite(ctx context.Context, row *models.Errand) {
	if row == nil {
		return
	}
	s.publish(ctx, row)
	s.wakeAwaits(row.ParentSessionID)
	if row.ResultState == models.ErrandResultPending {
		s.deliverPending(ctx, row.ParentSessionID)
	}
}
