package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandsOutstandingWorkflowOutcome(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			ctx := context.Background()
			repo, event, advance := seedRetentionPromptSubmission(t, engine, "errands-outstanding")
			stepTime := time.Date(2025, 6, 7, 8, 10, 0, 0, time.UTC)
			seedRetentionWorkflowPolicy(t, repo, event, "errands-outstanding", "step-current", stepTime)
			if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
				`UPDATE task_sessions SET state = ? WHERE id = ?`), models.TaskSessionStateWaitingForInput, event.SessionID); err != nil {
				t.Fatal(err)
			}
			effect := retentionWorkflowOutcome(event, retentionPromptTurnID(t, event),
				"errands_outstanding", "wf-errands-outstanding", "step-current", stepTime)
			wrong := *effect
			wrong.SubmissionID = "unrelated-submission"
			committed, err := repo.RecordAgentDeliveryWorkflowOutcome(ctx, retentionTaskIDForEvent(event), &wrong)
			if err != nil || committed {
				t.Fatalf("mismatched outstanding receipt = %v, %v", committed, err)
			}
			committed, err = repo.RecordAgentDeliveryWorkflowOutcome(ctx, retentionTaskIDForEvent(event), effect)
			if err != nil || !committed {
				t.Fatalf("exact outstanding receipt = %v, %v", committed, err)
			}
			completed, err := repo.CompleteAgentDeliveryTerminalSettlement(ctx, terminalRetentionEffectKey(event), time.Time{})
			if err != nil || !completed {
				t.Fatalf("outstanding terminal settlement = %v, %v", completed, err)
			}
			var stepID, state string
			if err := repo.db.QueryRowxContext(ctx, repo.db.Rebind(`
				SELECT t.workflow_step_id, s.state FROM tasks t
				JOIN task_sessions s ON s.task_id = t.id WHERE s.id = ?`), event.SessionID).Scan(&stepID, &state); err != nil {
				t.Fatal(err)
			}
			if stepID != "step-current" || state != string(models.TaskSessionStateWaitingForInput) {
				t.Fatalf("outstanding parent moved: step=%s state=%s", stepID, state)
			}
			advance(26 * time.Hour)
			var pruned int
			for attempt := 0; attempt < 3; attempt++ {
				batch, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 10})
				if err != nil {
					t.Fatal(err)
				}
				pruned += batch.PromptPayloadsPruned
			}
			if pruned != 1 {
				t.Fatalf("outstanding prompt payloads pruned = %d, want 1", pruned)
			}
		})
	}
}
