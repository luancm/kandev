package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ListOwnedErrandsAwaitingSettlement selects completed work on the current
// execution. The terminal receipt is verified before an outcome is written.
func (r *Repository) ListOwnedErrandsAwaitingSettlement(ctx context.Context) ([]*models.Errand, error) {
	return r.listErrands(ctx, `lifecycle='launched' AND EXISTS (
		SELECT 1 FROM errand_execution_generations g
		JOIN agent_delivery_submissions s ON s.session_id=('errand:' || g.errand_id) AND s.dispatch_attempt_id=g.execution_id
		JOIN agent_delivery_effects a ON a.effect_key=? || s.id AND a.effect_type=?
		JOIN errand_turns t ON t.errand_id=g.errand_id AND t.id=a.turn_id
		WHERE g.errand_id=task_session_errands.id AND g.active_submission_id=''
		 AND g.generation=(SELECT MAX(latest.generation) FROM errand_execution_generations latest WHERE latest.errand_id=g.errand_id)
		 AND t.completed_at IS NOT NULL AND t.id<>task_session_errands.latest_settled_turn_id
		 AND (t.id<>task_session_errands.first_turn_id OR task_session_errands.awaiting_result)
		 AND s.state IN ('completed','failed','cancelled')) ORDER BY task_id,created_at,id`,
		errandPromptAdmissionEffect+":", errandPromptAdmissionEffect)
}

// ReadOwnedErrandFirstTerminal returns the current execution's projected
// terminal receipt for the reserved first turn.
func (r *Repository) ReadOwnedErrandFirstTerminal(ctx context.Context, errandID string) (models.ErrandTeardownTarget, models.DeliverySubmissionState, error) {
	return readOwnedErrandTerminal(ctx, r.ro, errandID, true)
}

func (r *Repository) ReadOwnedErrandLatestTerminal(ctx context.Context, errandID string) (models.ErrandTeardownTarget, models.DeliverySubmissionState, error) {
	return readOwnedErrandTerminal(ctx, r.ro, errandID, false)
}

// HasSettledErrandDeliverySubmission checks the projected terminal effect
// independently of parent delivery and runtime cleanup.
func (r *Repository) HasSettledErrandDeliverySubmission(ctx context.Context, submission *models.AgentDeliverySubmission, streamID, turnID string) (bool, error) {
	if submission == nil || !validDeliveryTerminalOutcome(submission.State) || streamID == "" || turnID == "" {
		return false, nil
	}
	errandID, owned := errandDeliveryIdentity(submission.SessionID)
	if !owned || errandID == "" {
		return false, nil
	}
	var settled bool
	err := r.ro.GetContext(ctx, &settled, r.ro.Rebind(`SELECT EXISTS (
		SELECT 1 FROM agent_delivery_effects
		WHERE effect_type=? AND session_id=? AND incarnation_id=? AND harness_generation=?
		  AND submission_id=? AND stream_id=? AND turn_id=? AND outcome=? AND sequence>0
		  AND state IN (?,?)
	)`), models.DeliveryTerminalEffectType, submission.SessionID, submission.IncarnationID, submission.HarnessGeneration,
		submission.ID, streamID, turnID, submission.State, models.DeliveryEffectBlockResolved, models.DeliveryEffectCompleted)
	return settled, err
}

type errandTerminalReader interface {
	Rebind(string) string
	QueryRowxContext(context.Context, string, ...interface{}) *sqlx.Row
}

func readOwnedErrandTerminal(ctx context.Context, reader errandTerminalReader, errandID string, firstOnly bool) (models.ErrandTeardownTarget, models.DeliverySubmissionState, error) {
	var attempt, taskID, turnID, submissionID string
	var generation, promptGeneration uint64
	var outcome models.DeliverySubmissionState
	err := reader.QueryRowxContext(ctx, reader.Rebind(`SELECT a.outcome,e.task_id,t.id,s.id,g.generation,g.prompt_generation,s.state
		FROM task_session_errands e
		JOIN errand_execution_generations g ON g.errand_id=e.id
		JOIN agent_delivery_submissions s ON s.session_id=('errand:' || e.id) AND s.dispatch_attempt_id=g.execution_id
		JOIN agent_delivery_effects a ON a.effect_key=? || s.id AND a.effect_type=? AND a.session_id=s.session_id
		JOIN errand_turns t ON t.errand_id=e.id AND t.id=a.turn_id
		WHERE e.id=? AND t.completed_at IS NOT NULL AND g.active_submission_id=''
		  AND (NOT ? OR (t.id=e.first_turn_id AND s.id=e.submission_id))
		  AND g.generation=(SELECT MAX(latest.generation) FROM errand_execution_generations latest WHERE latest.errand_id=e.id)
		  AND s.state IN ('completed','failed','cancelled')
		  AND EXISTS (SELECT 1 FROM agent_delivery_effects terminal
		    WHERE terminal.effect_type=? AND terminal.state='block_resolved' AND terminal.sequence>0
		      AND terminal.session_id=a.session_id AND terminal.stream_id=a.stream_id
		      AND terminal.incarnation_id=a.incarnation_id AND terminal.harness_generation=a.harness_generation
		      AND terminal.submission_id=s.id AND terminal.turn_id=t.id AND terminal.outcome=s.state)
		ORDER BY t.completed_at DESC,t.id DESC LIMIT 1`),
		errandPromptAdmissionEffect+":", errandPromptAdmissionEffect, errandID, firstOnly, models.DeliveryTerminalEffectType).
		Scan(&attempt, &taskID, &turnID, &submissionID, &generation, &promptGeneration, &outcome)
	if err != nil {
		return models.ErrandTeardownTarget{}, "", err
	}
	var proof errandPromptProof
	if err := json.Unmarshal([]byte(attempt), &proof); err != nil {
		return models.ErrandTeardownTarget{}, "", err
	}
	if proof.ErrandID != errandID || proof.TurnID != turnID || proof.SubmissionID != submissionID ||
		proof.Generation != generation || proof.PromptGeneration != promptGeneration {
		return models.ErrandTeardownTarget{}, "", models.ErrErrandVersionConflict
	}
	return models.ErrandTeardownTarget{
		TaskID: taskID, ErrandID: errandID, OwnerGeneration: generation, ExecutionID: proof.ExecutionID,
		TurnID: turnID, SubmissionID: submissionID, PromptGeneration: promptGeneration,
	}, outcome, nil
}

func validateOwnedCompletionTarget(ctx context.Context, tx *sqlx.Tx, current, next *models.Errand) error {
	target, outcome, err := readOwnedErrandTerminal(ctx, tx, current.ID, false)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrErrandVersionConflict
	}
	if err != nil {
		return err
	}
	intent := next.TeardownIntent
	if current.Lifecycle != models.ErrandLifecycleLaunched || next.Lifecycle != current.Lifecycle ||
		intent.State != models.ErrandTeardownIntentPending || intent.Target != target || next.LatestSettledTurnID != target.TurnID {
		return models.ErrInvalidErrand
	}
	parentInitiated, err := validateOwnedResultOrigin(ctx, tx, current, target)
	if err != nil {
		return err
	}
	if !validOwnedCompletionResult(current, next, target, outcome, parentInitiated) {
		return models.ErrInvalidErrand
	}
	if current.TeardownIntent != nil {
		return validateCompletionIntentReplacement(current.TeardownIntent, intent)
	}
	return nil
}

func validateOwnedResultOrigin(ctx context.Context, tx *sqlx.Tx, row *models.Errand, target models.ErrandTeardownTarget) (bool, error) {
	if target.TurnID == row.FirstTurnID {
		return true, nil
	}
	state, err := readErrandAdmission(ctx, tx, target.ParentAdmission(row.ParentSessionID))
	if err != nil || state == "" {
		return false, err
	}
	if state != string(models.ErrandAdmissionAccepted) {
		return false, models.ErrErrandVersionConflict
	}
	return true, nil
}

func validOwnedCompletionResult(current, next *models.Errand, target models.ErrandTeardownTarget, outcome models.DeliverySubmissionState, parentInitiated bool) bool {
	if parentInitiated {
		return next.ResultTurnID == target.TurnID && !next.AwaitingResult && next.ResultState == models.ErrandResultPending &&
			(outcome == models.DeliverySubmissionCompleted) == (next.LastOutcome == models.ErrandOutcomeDone)
	}
	return next.ResultState == current.ResultState && next.ResultTurnID == current.ResultTurnID &&
		next.ResultText == current.ResultText && next.ResultKind == current.ResultKind && next.ResultTruncated == current.ResultTruncated &&
		next.AwaitingResult == current.AwaitingResult && next.LastOutcome == current.LastOutcome &&
		next.DeliveredRoute == current.DeliveredRoute && next.DeliveryToken == current.DeliveryToken
}

// GetErrandLastAssistantMessage reads the final visible assistant text
// without loading the prompt or tool output.
func (r *Repository) GetErrandLastAssistantMessage(ctx context.Context, errandID, turnID string) (*models.Message, error) {
	driver := r.ro.DriverName()
	query := `SELECT ` + errandMessageColumns + `
		FROM errand_messages m JOIN task_session_errands e ON e.id=m.errand_id
		WHERE m.errand_id=? AND m.turn_id=? AND ` + dialect.JSONExtract(driver, "m.payload", "author_type") + `=?
		AND COALESCE(` + dialect.JSONExtract(driver, "m.payload", "type") + `,?)=?
		ORDER BY m.created_at DESC,m.id DESC LIMIT 1`
	return scanErrandMessage(r.ro.QueryRowContext(ctx, r.ro.Rebind(query), errandID, turnID,
		models.MessageAuthorAgent, models.MessageTypeMessage, models.MessageTypeMessage))
}
