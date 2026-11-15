package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type errandLaunchRecoveryState string

const (
	errandLaunchRecoveryAbsent   errandLaunchRecoveryState = "absent"
	errandLaunchRecoveryPrepared errandLaunchRecoveryState = "prepared"
	errandLaunchRecoveryAdmitted errandLaunchRecoveryState = "admitted"
	errandLaunchRecoverySettled  errandLaunchRecoveryState = "settled"
	errandLaunchRecoveryBlocked  errandLaunchRecoveryState = "blocked"
)

type errandLaunchRuntimeEvidence struct {
	ExecutionID   string
	TaskID        string
	SessionID     string
	IncarnationID string
	TurnID        string
	SubmissionID  string
}

// Absence flags require completed, authoritative reads. RuntimeAbsent includes
// persisted inventory and pending authenticated restore, not just an empty
// process-local lookup. ReadError retains any failed evidence read. Submission
// is payload-free GetAgentDeliverySubmissionMetadata output. Runtime is a
// snapshot of an already restored execution and its current prompt receipt.
type errandLaunchRecoveryEvidence struct {
	Reservation        *models.Errand
	Task               *models.Task
	Parent             *models.TaskSession
	Session            *models.TaskSession
	SessionAbsent      bool
	ReservedTurn       *models.Turn
	ReservedTurnAbsent bool
	CurrentTurn        *models.Turn
	CurrentTurnAbsent  bool
	Submission         *models.AgentDeliverySubmission
	SubmissionAbsent   bool
	Runtime            *errandLaunchRuntimeEvidence
	RuntimeAbsent      bool
	ReadError          error
}

// classifyErrandLaunchRecovery performs no I/O and grants no runtime action.
// In particular, admitted includes uncertain effects and never permits replay.
func classifyErrandLaunchRecovery(e errandLaunchRecoveryEvidence) (errandLaunchRecoveryState, error) {
	if err := e.validate(); err != nil {
		return errandLaunchRecoveryBlocked, err
	}
	if e.Session == nil {
		return errandLaunchRecoveryAbsent, nil
	}
	authority, found, err := models.LoadErrandLaunchAuthority(e.Session.Metadata)
	if err != nil {
		return errandLaunchRecoveryBlocked, err
	}
	if found && *authority != e.reservedAuthority() {
		return blockedErrandLaunch("stored provenance differs from reservation")
	}
	terminal, err := e.submissionTerminal()
	if err != nil {
		return errandLaunchRecoveryBlocked, err
	}
	if !found && !e.legacyExactProof(terminal) {
		return blockedErrandLaunch("legacy session lacks independent complete launch proof")
	}
	if terminal {
		return errandLaunchRecoverySettled, nil
	}
	if e.Runtime != nil || e.submissionAdmitted() {
		return errandLaunchRecoveryAdmitted, nil
	}
	if !found || !e.preparedWithoutEffect() {
		return blockedErrandLaunch("reserved launch has no proved unattempted preparation")
	}
	return errandLaunchRecoveryPrepared, nil
}

func blockedErrandLaunch(reason string) (errandLaunchRecoveryState, error) {
	return errandLaunchRecoveryBlocked, fmt.Errorf("reserved errand launch blocked: %s", reason)
}

func (e errandLaunchRecoveryEvidence) validate() error {
	if e.ReadError != nil {
		return fmt.Errorf("read reserved errand launch evidence: %w", e.ReadError)
	}
	if err := e.validateReads(); err != nil {
		return err
	}
	if err := e.validateOwner(); err != nil {
		return err
	}
	if e.Session == nil {
		if e.Reservation.Lifecycle != models.ErrandLifecycleQueued || e.ReservedTurn != nil ||
			e.CurrentTurn != nil || e.Submission != nil || e.Runtime != nil {
			return errors.New("absent errand session has conflicting launch effects")
		}
		return nil
	}
	if err := e.validateSessionAndTurns(); err != nil {
		return err
	}
	return e.validateRuntime()
}

func (e errandLaunchRecoveryEvidence) validateReads() error {
	for _, source := range []struct {
		name    string
		present bool
		absent  bool
	}{
		{"session", e.Session != nil, e.SessionAbsent},
		{"reserved turn", e.ReservedTurn != nil, e.ReservedTurnAbsent},
		{"current turn", e.CurrentTurn != nil, e.CurrentTurnAbsent},
		{"submission", e.Submission != nil, e.SubmissionAbsent},
		{"runtime", e.Runtime != nil, e.RuntimeAbsent},
	} {
		if source.present == source.absent {
			return fmt.Errorf("reserved errand %s presence is unknown or contradictory", source.name)
		}
	}
	return nil
}

func (e errandLaunchRecoveryEvidence) reservedAuthority() models.ErrandLaunchAuthority {
	row := e.Reservation
	return models.ErrandLaunchAuthority{Version: models.ErrandLaunchAuthorityVersion,
		TaskID: row.TaskID, SessionID: row.ID, ParentSessionID: row.ParentSessionID,
		FirstTurnID: row.FirstTurnID, SubmissionID: row.SubmissionID, AgentProfileID: row.AgentProfileID}
}

func (e errandLaunchRecoveryEvidence) validateOwner() error {
	row := e.Reservation
	if row == nil {
		return models.ErrInvalidErrandLaunchAuthority
	}
	if err := e.reservedAuthority().Validate(); err != nil {
		return err
	}
	if row.Lifecycle != models.ErrandLifecycleQueued && row.Lifecycle != models.ErrandLifecycleLaunched {
		return errors.New("errand reservation is not runnable")
	}
	if e.Task == nil || e.Task.ID != row.TaskID || e.Task.ArchivedAt != nil {
		return errors.New("errand task owner is missing, different or archived")
	}
	return validateErrandLaunchParent(row, e.Parent)
}

func validateErrandLaunchParent(row *models.Errand, parent *models.TaskSession) error {
	if parent == nil || parent.ID != row.ParentSessionID || parent.TaskID != row.TaskID ||
		parent.Role != "" || parent.ParentSessionID != "" || parent.IsPassthrough {
		return errors.New("errand parent owner is unverified")
	}
	switch parent.State {
	case models.TaskSessionStateCreated, models.TaskSessionStateStarting, models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput, models.TaskSessionStateIdle:
		return nil
	default:
		return errors.New("errand parent owner is not live")
	}
}

func (e errandLaunchRecoveryEvidence) validateSessionAndTurns() error {
	row, session := e.Reservation, e.Session
	if session.ID != row.ID || session.TaskID != row.TaskID || session.ParentSessionID != row.ParentSessionID ||
		session.Role != models.SessionRoleErrand || session.IsPrimary || session.IsPassthrough ||
		strings.TrimSpace(session.QueueIncarnationID) == "" {
		return errors.New("canonical errand session identity differs or lacks an incarnation")
	}
	for _, turn := range []*models.Turn{e.ReservedTurn, e.CurrentTurn} {
		if !errandLaunchTurnMatches(row, turn) {
			return errors.New("errand turn evidence differs from the reserved first turn")
		}
	}
	if e.ReservedTurn == nil && (e.CurrentTurn != nil || e.Submission != nil || e.Runtime != nil) {
		return errors.New("reserved errand turn is missing despite observed launch effects")
	}
	return e.validateTurnAdmission()
}

func errandLaunchTurnMatches(row *models.Errand, turn *models.Turn) bool {
	return turn == nil || (turn.ID == row.FirstTurnID && turn.TaskID == row.TaskID && turn.TaskSessionID == row.ID &&
		turn.Metadata[models.TurnMetaKeyLifecycleOnly] != true)
}

func (e errandLaunchRecoveryEvidence) validateTurnAdmission() error {
	if e.ReservedTurn == nil {
		return nil
	}
	raw, found := e.ReservedTurn.Metadata[errandAdmissionTurnMetadataKey]
	if !found {
		return nil
	}
	marker, ok := raw.(map[string]interface{})
	submissionID, _ := marker["submission_id"].(string)
	if !ok || submissionID == "" || normalizeDeliverySubmissionID(submissionID) != normalizeDeliverySubmissionID(e.Reservation.SubmissionID) ||
		marker["sender_session_id"] != e.Reservation.ParentSessionID {
		return errors.New("reserved turn admission contradicts launch ownership")
	}
	switch marker["state"] {
	case string(models.ErrandAdmissionReserved), string(models.ErrandAdmissionAccepted), string(models.ErrandAdmissionUnknown):
		return nil
	default:
		return errors.New("reserved turn admission is refused or unreadable")
	}
}

func (e errandLaunchRecoveryEvidence) validateRuntime() error {
	runtime := e.Runtime
	if runtime == nil {
		return nil
	}
	row := e.Reservation
	if strings.TrimSpace(runtime.ExecutionID) == "" || runtime.TaskID != row.TaskID || runtime.SessionID != row.ID ||
		runtime.IncarnationID != e.Session.QueueIncarnationID || runtime.TurnID != row.FirstTurnID ||
		runtime.SubmissionID != normalizeDeliverySubmissionID(row.SubmissionID) {
		return errors.New("restored errand runtime lacks the complete reserved prompt receipt")
	}
	return nil
}

func (e errandLaunchRecoveryEvidence) submissionTerminal() (bool, error) {
	submission := e.Submission
	if submission == nil {
		return false, nil
	}
	if submission.ID != normalizeDeliverySubmissionID(e.Reservation.SubmissionID) || submission.SessionID != e.Session.ID ||
		submission.IncarnationID != e.Session.QueueIncarnationID {
		return false, errors.New("durable submission differs from reserved launch identity")
	}
	terminal := submission.TerminalTurnID != ""
	if terminal && (submission.TerminalTurnID != e.Reservation.FirstTurnID || e.ReservedTurn.CompletedAt == nil) {
		return false, errors.New("durable submission lacks matching terminal turn proof")
	}
	switch submission.State {
	case models.DeliverySubmissionCompleted, models.DeliverySubmissionFailed, models.DeliverySubmissionCancelled:
		if !terminal {
			return false, errors.New("terminal submission lacks a reserved turn link")
		}
	case models.DeliverySubmissionInterruptedUnknown:
		// Uncertainty remains admitted unless the exact terminal turn is proved.
	case models.DeliverySubmissionPrepared, models.DeliverySubmissionAccepted, models.DeliverySubmissionDispatching:
		if terminal {
			return false, errors.New("nonterminal submission contains contradictory terminal proof")
		}
	default:
		return false, errors.New("reserved submission state is unreadable")
	}
	return terminal, nil
}

func (e errandLaunchRecoveryEvidence) submissionAdmitted() bool {
	if e.Submission == nil {
		return false
	}
	switch e.Submission.State {
	case models.DeliverySubmissionAccepted, models.DeliverySubmissionDispatching, models.DeliverySubmissionInterruptedUnknown:
		return true
	default:
		return false
	}
}

func (e errandLaunchRecoveryEvidence) legacyExactProof(terminal bool) bool {
	if e.ReservedTurn == nil || e.ReservedTurn.ExecutionProfileID != e.Reservation.AgentProfileID {
		return false
	}
	if terminal || e.Runtime != nil {
		return true
	}
	marker, _ := e.ReservedTurn.Metadata[errandAdmissionTurnMetadataKey].(map[string]interface{})
	return e.submissionAdmitted() && (marker["state"] == string(models.ErrandAdmissionAccepted) ||
		marker["state"] == string(models.ErrandAdmissionUnknown))
}

func (e errandLaunchRecoveryEvidence) preparedWithoutEffect() bool {
	session := e.Session
	if !e.RuntimeAbsent || session.State != models.TaskSessionStateCreated || session.AgentExecutionID != "" || session.DownstreamACPSessionID != "" {
		return false
	}
	if e.Submission != nil && (e.Submission.State != models.DeliverySubmissionPrepared || e.Submission.DispatchAttemptID != "") {
		return false
	}
	initial, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	if err != nil || (found && initial.State != models.InitialPromptSubmissionPending) {
		return false
	}
	return errandLaunchTurnUnattempted(e.ReservedTurn)
}

func errandLaunchTurnUnattempted(turn *models.Turn) bool {
	if turn == nil {
		return true
	}
	pending, pendingOK := turn.Metadata[models.TurnMetaKeyPromptDispatchPending].(bool)
	attempted, attemptedExists := turn.Metadata[models.TurnMetaKeyPromptDispatchAttempted]
	_, admissionExists := turn.Metadata[errandAdmissionTurnMetadataKey]
	return turn.CompletedAt == nil && pendingOK && pending && (!attemptedExists || attempted == false) && !admissionExists
}

type errandLaunchEvidenceStore interface {
	errandRowReader
	GetLatestTurnBySessionID(context.Context, string) (*models.Turn, error)
	GetAgentDeliverySubmissionMetadata(context.Context, string) (*models.AgentDeliverySubmission, error)
}

func (s *Service) inspectErrandLaunch(ctx context.Context, req errands.LaunchRequest) (errandLaunchRecoveryEvidence, errandLaunchRecoveryState, error) {
	e := errandLaunchRecoveryEvidence{}
	if err := s.validateErrandLaunchReservation(ctx, req, false); err != nil {
		return e, errandLaunchRecoveryBlocked, err
	}
	guard, err := s.lockErrandLaunchAdmission(ctx, req, false)
	if err != nil {
		return e, errandLaunchRecoveryBlocked, err
	}
	defer guard.release()
	return s.readErrandLaunchEvidence(ctx, req)
}

func (s *Service) readErrandLaunchEvidence(ctx context.Context, req errands.LaunchRequest) (errandLaunchRecoveryEvidence, errandLaunchRecoveryState, error) {
	e := errandLaunchRecoveryEvidence{}
	var err error
	store, ok := s.repo.(errandLaunchEvidenceStore)
	if !ok || s.turnService == nil || s.agentManager == nil {
		return e, errandLaunchRecoveryBlocked, errors.New("reserved errand evidence readers are unavailable")
	}
	e.Reservation, err = store.GetErrand(ctx, req.ErrandID)
	e.ReadError = errors.Join(e.ReadError, err)
	e.Task, err = s.repo.GetTask(ctx, req.TaskID)
	e.ReadError = errors.Join(e.ReadError, err)
	e.Parent, err = s.repo.GetTaskSession(ctx, req.ParentSessionID)
	e.ReadError = errors.Join(e.ReadError, err)
	e.Session, err = s.repo.GetTaskSession(ctx, req.ErrandID)
	e.SessionAbsent = errors.Is(err, models.ErrTaskSessionNotFound)
	if !e.SessionAbsent {
		e.ReadError = errors.Join(e.ReadError, err)
	}
	e.ReservedTurn, err = s.turnService.GetTurn(ctx, req.TurnID)
	e.ReservedTurnAbsent = errors.Is(err, sql.ErrNoRows)
	if !e.ReservedTurnAbsent {
		e.ReadError = errors.Join(e.ReadError, err)
	}
	e.CurrentTurn, e.CurrentTurnAbsent, err = s.readErrandLaunchCurrentTurn(ctx, store, req)
	e.ReadError = errors.Join(e.ReadError, err)
	e.Submission, err = store.GetAgentDeliverySubmissionMetadata(ctx, normalizeDeliverySubmissionID(req.SubmissionID))
	e.SubmissionAbsent = errors.Is(err, repoerrors.ErrAgentDeliverySubmissionNotFound)
	if !e.SubmissionAbsent {
		e.ReadError = errors.Join(e.ReadError, err)
	}
	e.Runtime, e.RuntimeAbsent, err = s.readErrandLaunchRuntime(ctx, req.ErrandID)
	e.ReadError = errors.Join(e.ReadError, err)
	state, err := classifyErrandLaunchRecovery(e)
	return e, state, err
}

// Active authority includes attempted unpublished prompts that visible
// history omits. Both reads must complete before absence is known.
func (s *Service) readErrandLaunchCurrentTurn(ctx context.Context, store errandLaunchEvidenceStore, req errands.LaunchRequest) (*models.Turn, bool, error) {
	active, activeErr := s.turnService.GetActiveTurn(ctx, req.ErrandID)
	latest, latestErr := store.GetLatestTurnBySessionID(ctx, req.ErrandID)
	err := errors.Join(activeErr, latestErr)
	if latest != nil && latest.ID != req.TurnID {
		err = errors.Join(err, errors.New("reserved errand launch has different latest turn authority"))
	}
	if reserved := s.reservedPromptTurnID(req.ErrandID); reserved != "" && reserved != req.TurnID {
		err = errors.Join(err, errors.New("reserved errand launch has another live turn reservation"))
	}
	if active != nil {
		return active, false, err
	}
	return latest, err == nil && latest == nil, err
}

func (s *Service) readErrandLaunchRuntime(ctx context.Context, sessionID string) (*errandLaunchRuntimeEvidence, bool, error) {
	id, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	if errors.Is(err, agentruntime.ErrNoExecutionForSession) && id == "" {
		absenceErr := s.confirmErrandLaunchRuntimeAbsent(ctx, sessionID)
		return nil, absenceErr == nil, absenceErr
	}
	if err != nil {
		return nil, false, err
	}
	reader, ok := s.agentManager.(interface {
		GetExecution(string) (*agentruntime.AgentExecution, bool)
	})
	if !ok {
		return nil, false, errors.New("reserved errand runtime snapshots are unavailable")
	}
	execution, ok := reader.GetExecution(id)
	if !ok || execution == nil || id == "" {
		return nil, false, errors.New("reserved errand runtime lookup lacks an execution snapshot")
	}
	metadata := execution.MetadataSnapshot()
	turnID, _ := metadata["prompt_turn_id"].(string)
	submissionID, _ := metadata["initial_delivery_submission_id"].(string)
	return &errandLaunchRuntimeEvidence{ExecutionID: execution.ID, TaskID: execution.TaskID,
		SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID,
		TurnID: turnID, SubmissionID: submissionID}, false, nil
}

func (s *Service) confirmErrandLaunchRuntimeAbsent(ctx context.Context, sessionID string) error {
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read reserved runtime inventory: %w", err)
	}
	if running == nil {
		return errors.New("reserved runtime inventory returned no absence witness")
	}
	if s.hasPendingRemoteRecovery(sessionID, running.AgentExecutionID) {
		return errors.New("reserved runtime authenticated recovery is pending")
	}
	if s.hasErrandRuntimeAbsenceWitness(sessionID, running) {
		return nil
	}
	return errors.New("reserved runtime absence is unconfirmed by persisted inventory")
}

// Callers must exclude missing inventory and pending authenticated recovery.
func (s *Service) hasErrandRuntimeAbsenceWitness(sessionID string, running *models.ExecutorRunning) bool {
	if claim, ok := s.executionTeardownClaimFor(sessionID, running.AgentExecutionID); ok && claim.cleanupCompleted {
		return true
	}
	return (running.Status == models.ExecutorRunningStatusStopped && running.LocalPID == 0) ||
		s.rowLiveness(running) == models.ProcessLivenessDead
}

func recoveredErrandLaunch(e errandLaunchRecoveryEvidence, state errandLaunchRecoveryState) (errands.LaunchResult, *LaunchSessionResponse, error) {
	disposition := errands.LaunchAdmitted
	if state == errandLaunchRecoverySettled {
		disposition = errands.LaunchSettled
	}
	row := e.Reservation
	response := &LaunchSessionResponse{Success: true, TaskID: row.TaskID, SessionID: row.ID,
		AgentProfileID: row.AgentProfileID, State: string(e.Session.State)}
	if e.Runtime != nil {
		response.AgentExecutionID = e.Runtime.ExecutionID
	}
	return errands.LaunchResult{Disposition: disposition, SessionID: row.ID,
		TurnID: row.FirstTurnID, SubmissionID: row.SubmissionID}, response, nil
}

func (s *Service) blockReservedErrandLaunch(ctx context.Context, req errands.LaunchRequest, e errandLaunchRecoveryEvidence, cause error) error {
	block := &sessionRecoveryRequiredError{Block: &models.SessionRecoveryBlock{
		Reason: "errand_launch_unverified", DeliverySubmissionID: normalizeDeliverySubmissionID(req.SubmissionID), ConsumerReference: "errand_launch"}}
	var retained *sessionRecoveryRequiredError
	if errors.As(cause, &retained) && retained.Block != nil && retained.Block.SessionID == req.ErrandID {
		block = retained
	} else if e.Reservation != nil && errandTeardownSessionMatches(e.Reservation, e.Session) {
		cause = errors.Join(cause, s.recordSessionRecoveryBlock(ctx, req.ErrandID, "errand_launch", block))
	}
	s.logger.Warn("reserved errand launch blocked", zap.String("task_id", req.TaskID),
		zap.String("session_id", req.ErrandID), zap.String("parent_session_id", req.ParentSessionID),
		zap.String("turn_id", req.TurnID), zap.String("submission_id", req.SubmissionID), zap.Error(cause))
	return errors.Join(block, cause)
}
