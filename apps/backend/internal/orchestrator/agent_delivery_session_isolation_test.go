package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.7
func TestDeliverySessionIsolationSettlesNewerTerminalWhileReportingOldRefusal(t *testing.T) {
	f := newDeliveryWorkflowOutcomeFixture(t, "isolation-sweep", "")
	old := seedUnresolvedSessionHistory(t, f, false)
	if err := f.service.recordNotRequiredDeliveryWorkflowOutcome(f.ctx, f.taskID, f.effect); err != nil {
		t.Fatal(err)
	}
	if err := f.service.reconcileAgentDeliverySettlements(f.ctx, f.session.ID); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
		t.Fatalf("session inventory must still report unresolved history: %v", err)
	}
	assertIsolatedTerminalSettlement(t, f, old)
}

func TestDeliverySessionIsolationRetriesExactTerminal(t *testing.T) {
	retry := newRetryTerminalFixture(t)
	retry.settle()
	if _, err := retry.repo.DB().ExecContext(t.Context(), `UPDATE tasks SET workflow_step_id = '' WHERE id = ?`, retry.session.TaskID); err != nil {
		t.Fatal(err)
	}
	event := persistedTerminalEvent(retry.session.ID, retry.recovery.IncarnationID, 1,
		retry.recovery.SubmissionID, retry.recovery.StreamID, "complete")
	recordSettlementWorkflowOutcome(t, retry.repo, t.Context(), event)
	effect, err := retry.repo.GetAgentDeliveryEffect(t.Context(), "agent_delivery.terminal:"+event.StreamID+":1")
	if err != nil {
		t.Fatal(err)
	}
	f := &deliveryWorkflowOutcomeFixture{ctx: t.Context(), repo: retry.repo, service: retry.service,
		taskID: retry.session.TaskID, session: retry.session, effect: effect}
	old := seedUnresolvedSessionHistory(t, f, false)
	for range 2 {
		response, err := f.service.RetrySessionDelivery(f.ctx, f.taskID, f.session.ID)
		if err != nil || response.Outcome != SessionDeliveryRecoverySettled {
			t.Fatalf("retry current proven terminal = %+v, %v", response, err)
		}
	}
	assertIsolatedTerminalSettlement(t, f, old)
	recovery := retry.recovery
	recovery.SubmissionID, recovery.StreamID = old.SubmissionID, old.StreamID
	response, err := f.service.retryTerminalSessionDelivery(f.ctx, f.session, recovery, nil)
	if err != nil || response.Outcome != SessionDeliveryRecoveryBlocked {
		t.Fatalf("retry unresolved historical terminal = %+v, %v; want blocked", response, err)
	}
}

func TestDeliverySessionIsolationSettlesOwnedCancellation(t *testing.T) {
	cancelled := seedOrchestratorSettledSessionState(t, "isolated-cancel", false)
	session, err := cancelled.repo.GetTaskSession(t.Context(), cancelled.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := cancelled.repo.GetAgentDeliveryEffect(t.Context(), "agent_delivery.terminal:"+cancelled.streamID+":1")
	if err != nil {
		t.Fatal(err)
	}
	f := &deliveryWorkflowOutcomeFixture{ctx: t.Context(), repo: cancelled.repo, service: cancelled.service,
		taskID: cancelled.taskID, session: session, effect: effect}
	old := seedUnresolvedSessionHistory(t, f, false)
	controller := &settlingErrandController{}
	f.service.SetErrandController(controller)
	f.service.settleCancelledErrandTurn(f.ctx, f.session.ID, effect.TurnID, effect.SubmissionID)
	if len(controller.cancellations) != 1 || controller.cancellations[0].submissionID != effect.SubmissionID {
		t.Fatalf("owned cancellation blocked by unrelated history: %+v", controller.cancellations)
	}
	assertIsolatedTerminalSettlement(t, f, old)
}

func TestDeliverySessionIsolationAutoRunsQueuedSuccessor(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprintf("historical_conflict_%t", conflict), func(t *testing.T) {
			f := newDeliveryWorkflowOutcomeFixture(t, "isolation-queue", "")
			old := seedUnresolvedSessionHistory(t, f, conflict)
			seedExecutorRunning(t, f.repo, f.session.ID, f.taskID, "isolation-execution")
			observed := make(chan error, 2)
			var calls atomic.Int32
			manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: f.repo}
			manager.promptAgentFunc = func(ctx context.Context, _ string, text string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
				calls.Add(1)
				submission, err := f.repo.GetAgentDeliverySubmissionMetadata(ctx, f.effect.SubmissionID)
				if err == nil && (submission.FullySettledAt == nil || text != "independent successor") {
					err = fmt.Errorf("successor reached provider before exact settlement or with changed input: %+v, %q", submission, text)
				}
				observed <- err
				return &executor.PromptResult{}, nil
			}
			f.service.agentManager = manager
			f.service.executor = withTestCapabilities(executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{}))
			if _, err := f.service.messageQueue.QueueMessage(f.ctx, f.session.ID, f.taskID, "independent successor", "", "user", false, nil); err != nil {
				t.Fatal(err)
			}
			f.service.handleAgentReady(f.ctx, watcher.AgentEventData{
				TaskID: f.taskID, SessionID: f.session.ID, AgentExecutionID: "isolation-execution", TurnID: f.effect.TurnID,
				DeliveryStreamID: f.effect.StreamID, DeliveryIncarnationID: f.effect.IncarnationID,
				DeliverySubmissionID: f.effect.SubmissionID, DeliveryHarnessGeneration: uint64(f.effect.HarnessGeneration), DeliverySequence: uint64(f.effect.Sequence),
			})
			select {
			case err := <-observed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("historical terminal prevented proven successor from reaching provider")
			}
			assertIsolatedTerminalSettlement(t, f, old)
			status := f.service.messageQueue.GetStatus(f.ctx, f.session.ID)
			if status.Count != 0 || calls.Load() != 1 {
				t.Fatalf("queue count=%d provider calls=%d", status.Count, calls.Load())
			}
		})
	}
}

func seedUnresolvedSessionHistory(t *testing.T, f *deliveryWorkflowOutcomeFixture, conflict bool) models.AgentDeliveryEvent {
	t.Helper()
	id := "historical-" + f.effect.SubmissionID
	if _, err := f.repo.PrepareAgentDeliverySubmission(f.ctx, &models.AgentDeliverySubmission{
		ID: id, SessionID: f.session.ID, IncarnationID: f.effect.IncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "historical-hash", Payload: []byte("historical input"), State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatal(err)
	}
	old := persistedTerminalEvent(f.session.ID, f.effect.IncarnationID, 1, id, "historical-stream", "complete")
	old.Payload = []byte(`{"type":"complete","session_id":"historical-native","prompt_generation":1,"data":{"stop_reason":"end_turn"}}`)
	if conflict {
		old.Payload = []byte(`{"type":"complete","session_id":"historical-native","turn_id":"original-historical-turn","prompt_generation":1}`)
	}
	projectOrchestratorDeliveryEvent(t, f.repo, f.ctx, old)
	seedIsolationTranscriptTurns(t, f)
	if err := f.repo.CreateMessage(f.ctx, &models.Message{ID: "historical-output", TaskSessionID: f.session.ID,
		TaskID: f.taskID, TurnID: "historical-transcript-turn", AuthorType: models.MessageAuthorAgent, Content: "preserved historical answer"}); err != nil {
		t.Fatal(err)
	}
	turnID := ""
	if conflict {
		turnID = "unproven-conflicting-turn"
	}
	if _, err := f.repo.DB().ExecContext(f.ctx,
		`UPDATE agent_delivery_effects SET state = 'block_resolved', turn_id = ?, created_at = ? WHERE stream_id = ?`,
		turnID, time.Now().UTC().Add(-time.Hour), old.StreamID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.DB().ExecContext(f.ctx,
		`UPDATE agent_delivery_submissions SET state = 'completed', outcome = 'completed' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	return old
}

func assertIsolatedTerminalSettlement(t *testing.T, f *deliveryWorkflowOutcomeFixture, old models.AgentDeliveryEvent) {
	t.Helper()
	current, err := f.repo.GetAgentDeliverySubmissionMetadata(f.ctx, f.effect.SubmissionID)
	if err != nil || current.FullySettledAt == nil {
		t.Fatalf("newer proven completion remains unsettled: %+v, %v", current, err)
	}
	pending, err := f.repo.ListPendingAgentDeliverySettlements(f.ctx, f.session.ID)
	if err != nil || len(pending) != 1 || pending[0].SubmissionID != old.SubmissionID || pending[0].State != models.DeliveryEffectBlockResolved {
		t.Fatalf("historical effect was changed or newer terminal was retained: %+v, %v", pending, err)
	}
	expectedTurn := ""
	if eventTurnID(old) != "" {
		expectedTurn = "unproven-conflicting-turn"
	}
	if pending[0].TurnID != expectedTurn {
		t.Fatalf("historical attribution changed: %q", pending[0].TurnID)
	}
	message, err := f.repo.GetMessage(f.ctx, "historical-output")
	if err != nil || message.Content != "preserved historical answer" {
		t.Fatalf("saved conversation changed: %+v, %v", message, err)
	}
	var payload []byte
	if err := f.repo.DB().QueryRowContext(f.ctx, `SELECT payload FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`, old.StreamID, old.Sequence).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if string(payload) != string(old.Payload) {
		t.Fatal("historical transcript evidence changed")
	}
	legacy, err := f.repo.GetAgentDeliverySubmissionMetadata(f.ctx, old.SubmissionID)
	if err != nil || legacy.FullySettledAt != nil {
		t.Fatalf("unresolved history was certified as settled: %+v, %v", legacy, err)
	}
}

func seedIsolationTranscriptTurns(t *testing.T, f *deliveryWorkflowOutcomeFixture) {
	t.Helper()
	oldTime := time.Now().UTC().Add(-time.Hour)
	if err := f.repo.CreateTurn(f.ctx, &models.Turn{ID: "historical-transcript-turn", TaskSessionID: f.session.ID,
		TaskID: f.taskID, StartedAt: oldTime, CompletedAt: &oldTime}); err != nil {
		t.Fatal(err)
	}
	_, err := f.repo.GetTurn(f.ctx, f.effect.TurnID)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC()
		err = f.repo.CreateTurn(f.ctx, &models.Turn{ID: f.effect.TurnID, TaskSessionID: f.session.ID,
			TaskID: f.taskID, StartedAt: now, CompletedAt: &now})
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeliverySessionIsolationReadyDoesNotBypassCurrentTerminalFailure(t *testing.T) {
	for _, failure := range []string{"identity", "storage"} {
		t.Run(failure, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newDeliveryWorkflowOutcomeFixture(t, "current-failure", "")
				seedExecutorRunning(t, f.repo, f.session.ID, f.taskID, "isolation-execution")
				if failure == "identity" {
					if _, err := f.repo.DB().ExecContext(f.ctx, `UPDATE agent_delivery_effects SET turn_id = 'conflicting-turn' WHERE effect_type = ?`, models.DeliveryTerminalEffectType); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := f.repo.DB().ExecContext(f.ctx, `CREATE TRIGGER refuse_current_settlement BEFORE UPDATE ON agent_delivery_submissions BEGIN SELECT RAISE(ABORT, 'current settlement storage failure'); END`); err != nil {
						t.Fatal(err)
					}
				}
				var calls atomic.Int32
				manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: f.repo}
				manager.promptAgentFunc = func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
					calls.Add(1)
					return &executor.PromptResult{}, nil
				}
				f.service.agentManager = manager
				f.service.executor = withTestCapabilities(executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{}))
				queued, err := f.service.messageQueue.QueueMessage(f.ctx, f.session.ID, f.taskID, "held successor", "", "user", false, nil)
				if err != nil {
					t.Fatal(err)
				}
				f.service.handleAgentReady(f.ctx, watcher.AgentEventData{
					TaskID: f.taskID, SessionID: f.session.ID, AgentExecutionID: "isolation-execution", TurnID: f.effect.TurnID,
					DeliveryStreamID: f.effect.StreamID, DeliveryIncarnationID: f.effect.IncarnationID,
					DeliverySubmissionID: f.effect.SubmissionID, DeliveryHarnessGeneration: uint64(f.effect.HarnessGeneration), DeliverySequence: uint64(f.effect.Sequence),
				})
				synctest.Wait()
				status := f.service.messageQueue.GetStatus(f.ctx, f.session.ID)
				if calls.Load() != 0 || status.Count != 1 || status.Entries[0].ID != queued.ID {
					t.Fatalf("current terminal refusal was bypassed: provider calls=%d queue=%+v", calls.Load(), status)
				}
			})
		})
	}
}
