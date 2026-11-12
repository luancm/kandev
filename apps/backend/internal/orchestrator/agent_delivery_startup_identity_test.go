package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
func TestDeliveryStartupRetainsUnattributedTerminalAndSettlesIndependentSession(t *testing.T) {
	for _, state := range []string{models.DeliveryEffectPending, models.DeliveryEffectBlockResolved} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			service, repo := newServiceWithRealRepo(t)
			legacy := seedStartupTerminal(t, repo, "legacy", false)
			if _, err := repo.DB().ExecContext(ctx,
				`UPDATE agent_delivery_effects SET state = ? WHERE stream_id = ?`, state, legacy.StreamID); err != nil {
				t.Fatal(err)
			}
			// Older versions could settle the submission without retaining a turn admission.
			if _, err := repo.DB().ExecContext(ctx,
				`UPDATE agent_delivery_submissions SET state = 'completed', outcome = 'completed' WHERE id = ?`, legacy.SubmissionID); err != nil {
				t.Fatal(err)
			}
			healthy := seedStartupTerminal(t, repo, "healthy", true)
			recordSettlementWorkflowOutcome(t, repo, ctx, healthy)
			turns := &housekeepingTurns{}
			service.SetTurnService(turns)
			if err := service.PrepareStartup(ctx); err != nil {
				t.Fatalf("prepare startup blocked by historical identity: %v", err)
			}
			if !service.startupPrepared || turns.reconciliations != 1 || service.running {
				t.Fatal("startup did not finish housekeeping without starting producers")
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := service.reconcileDurableQueueStateOnStartup(ctx); err != nil {
					t.Fatalf("startup attempt %d blocked by historical identity: %v", attempt, err)
				}
			}
			pending, err := repo.ListPendingAgentDeliverySettlements(ctx, "")
			if err != nil || len(pending) != 1 || pending[0].SubmissionID != legacy.SubmissionID ||
				pending[0].State != state || pending[0].TurnID != "" {
				t.Fatalf("pending effects = %+v, %v; want untouched legacy effect only", pending, err)
			}
			submission, err := repo.GetAgentDeliverySubmission(ctx, legacy.SubmissionID)
			if err != nil || submission.State != models.DeliverySubmissionCompleted || submission.FullySettledAt != nil {
				t.Fatalf("legacy submission = %+v, %v; must not authorize full settlement", submission, err)
			}
			var payload []byte
			if err := repo.DB().QueryRowContext(ctx,
				`SELECT payload FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`, legacy.StreamID, legacy.Sequence).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			if string(payload) != string(legacy.Payload) {
				t.Fatalf("retained terminal payload changed: %s", payload)
			}
			if err := service.reconcileAgentDeliverySettlements(ctx, legacy.SessionID); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
				t.Fatalf("session-scoped settlement = %v; want identity refusal", err)
			}
			if err := service.settleQueuedDeliveryPredecessor(ctx, "task-legacy", legacy.SessionID, pending[0]); !errors.Is(err, repoerrors.ErrAgentDeliveryIdentityMismatch) {
				t.Fatalf("queued successor settlement = %v; want identity refusal", err)
			}
		})
	}
}

func TestDeliveryStartupStillFailsOnSettlementStorageError(t *testing.T) {
	ctx := context.Background()
	service, repo := newServiceWithRealRepo(t)
	event := seedStartupTerminal(t, repo, "storage-failure", true)
	recordSettlementWorkflowOutcome(t, repo, ctx, event)
	if _, err := repo.DB().ExecContext(ctx, `CREATE TRIGGER reject_startup_settlement
		BEFORE UPDATE ON agent_delivery_submissions BEGIN SELECT RAISE(ABORT, 'startup settlement write failed'); END`); err != nil {
		t.Fatal(err)
	}
	if err := service.reconcileDurableQueueStateOnStartup(ctx); err == nil || !strings.Contains(err.Error(), "startup settlement write failed") {
		t.Fatalf("startup storage failure = %v; want real write failure propagated", err)
	}
}

func seedStartupTerminal(t *testing.T, repo *tasksqlite.Repository, suffix string, withTurn bool) models.AgentDeliveryEvent {
	t.Helper()
	ctx := context.Background()
	sessionID := "session-" + suffix
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-" + suffix, Title: "Startup recovery"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: "task-" + suffix, QueueIncarnationID: sessionID,
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	submissionID := "prompt:initial:" + sessionID
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: sessionID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash-" + suffix,
		Payload: []byte("retained input"), State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatal(err)
	}
	event := persistedTerminalEvent(sessionID, sessionID, 1, submissionID, "stream-"+suffix, "complete")
	if !withTurn {
		event.Payload = []byte(`{"type":"complete","session_id":"native-session","prompt_generation":1,"data":{"stop_reason":"end_turn"}}`)
	}
	projectOrchestratorDeliveryEvent(t, repo, ctx, event)
	return event
}
