package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func unsentOwnershipFixture(t *testing.T, engine string) (*Repository, *models.AgentDeliverySubmission) {
	t.Helper()
	repo := newRetentionTestRepo(t, engine)
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "unsent-task", WorkspaceID: retentionTestWorkspaceID(t, repo), Title: "Unsent ownership"}))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "unsent-session", TaskID: "unsent-task", QueueIncarnationID: "unsent-incarnation"}))
	row := &models.AgentDeliverySubmission{
		ID: "prompt:owned", SessionID: "unsent-session", IncarnationID: "unsent-incarnation",
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "owned", PayloadHash: "owned-hash",
		Payload: []byte("original instruction"), State: models.DeliverySubmissionDispatching,
	}
	created, err := repo.PrepareAgentDeliverySubmission(t.Context(), row)
	require.NoError(t, err)
	require.True(t, created)
	return repo, row
}

// @covers AC-PLATFORM-UNDELIVERED-PROMPT-001.1 AC-PLATFORM-DELIVERY-RETENTION-001.3
func TestUnsentPromptSettlementOwnership(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			for _, scenario := range []string{"exact", "id", "session", "incarnation", "harness", "owner", "hash", "attempt", "terminal inbox", "terminal effect", "already failed"} {
				t.Run(scenario, func(t *testing.T) {
					repo, row := unsentOwnershipFixture(t, engine)
					expected := *row
					switch scenario {
					case "id":
						expected.ID = "other"
					case "session":
						expected.SessionID = "other"
					case "incarnation":
						expected.IncarnationID = "other"
					case "harness":
						expected.HarnessGeneration++
					case "owner":
						expected.OwnerGeneration++
					case "hash":
						expected.PayloadHash = "other"
					case "attempt":
						expected.DispatchAttemptID = "other"
					case "terminal inbox":
						inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &models.AgentDeliveryEvent{
							SessionID: row.SessionID, IncarnationID: row.IncarnationID, HarnessGeneration: 1,
							StreamID: "terminal-stream", Sequence: 1, SubmissionID: row.ID, EventType: "complete",
							Payload: []byte(`{"type":"complete"}`), Terminal: true,
						}, 1)
						require.NoError(t, err)
						require.True(t, inserted)
					case "terminal effect":
						_, err := repo.PutAgentDeliveryEffect(t.Context(), &models.AgentDeliveryEffect{
							EffectKey: "terminal", EffectType: "agent_delivery.terminal_settlement", State: models.DeliveryEffectPending,
							SessionID: row.SessionID, IncarnationID: row.IncarnationID, HarnessGeneration: 1,
							StreamID: "terminal-stream", Sequence: 1, SubmissionID: row.ID,
						})
						require.NoError(t, err)
					case "already failed":
						_, err := repo.TransitionAgentDeliverySubmission(t.Context(), row.ID, row.State, models.DeliverySubmissionFailed, "failed", time.Now().UTC())
						require.NoError(t, err)
					}
					changed, err := repo.FailUnsentAgentDeliverySubmission(t.Context(), &expected, time.Now().UTC())
					require.NoError(t, err)
					require.Equal(t, scenario == "exact", changed, "only the exact uncontradicted dispatch reservation may settle")
					stored, err := repo.GetAgentDeliverySubmissionMetadata(t.Context(), row.ID)
					require.NoError(t, err)
					proved, err := repo.AgentDeliverySubmissionWasUnsent(t.Context(), &expected)
					require.NoError(t, err)
					require.Equal(t, scenario == "exact", proved)
					if scenario == "exact" {
						require.NotNil(t, stored.FullySettledAt)
					} else {
						require.Nil(t, stored.FullySettledAt)
					}
				})
			}
		})
	}
}

func TestUnsentPromptReceiptSurvivesPayloadRetention(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, row := unsentOwnershipFixture(t, engine)
			changed, err := repo.FailUnsentAgentDeliverySubmission(t.Context(), row, time.Now().UTC().Add(-3*time.Hour))
			require.NoError(t, err)
			require.True(t, changed)
			pruned := 0
			for range 2 {
				batch, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{
					MaxCandidates: 100, WorkTimeout: 5 * time.Second,
				})
				require.NoError(t, err)
				pruned += batch.PromptPayloadsPruned
			}
			require.Equal(t, 1, pruned)
			proved, err := repo.AgentDeliverySubmissionWasUnsent(t.Context(), row)
			require.NoError(t, err)
			require.True(t, proved, "retained metadata must still prove the same captured owner")
			wrongAttempt := *row
			wrongAttempt.DispatchAttemptID = "successor"
			proved, err = repo.AgentDeliverySubmissionWasUnsent(t.Context(), &wrongAttempt)
			require.NoError(t, err)
			require.False(t, proved)
			_, err = repo.PutAgentDeliveryEffect(t.Context(), &models.AgentDeliveryEffect{
				EffectKey: "late-terminal", EffectType: models.DeliveryTerminalEffectType, State: models.DeliveryEffectPending,
				SessionID: row.SessionID, IncarnationID: row.IncarnationID, HarnessGeneration: 1,
				StreamID: "terminal-stream", Sequence: 1, SubmissionID: row.ID,
			})
			require.NoError(t, err)
			proved, err = repo.AgentDeliverySubmissionWasUnsent(t.Context(), row)
			require.NoError(t, err)
			require.False(t, proved, "a late contradictory terminal receipt must fence refusal too")
		})
	}
}
