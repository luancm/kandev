package orchestrator

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// @covers AC-PLATFORM-UNDELIVERED-PROMPT-001.4 AC-PLATFORM-DELIVERY-RETENTION-001.3
func TestErrandUnsentAdmissionRecoveryReopensAfterRetention(t *testing.T) {
	for _, proof := range []bool{true, false} {
		t.Run(map[bool]string{true: "committed receipt", false: "arbitrary failed row"}[proof], func(t *testing.T) {
			f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
			ctx := t.Context()
			a := fixtureAdmission(f)
			_, err := f.svc.reserveErrandParentDispatch(ctx, f.session, errands.ParentPromptOrigin{
				SenderSessionID: f.parent, SubmissionID: "admission-submission",
			}, a.TurnID, "admission-submission", f.session.AgentExecutionID)
			require.NoError(t, err)
			delivery, err := f.svc.prepareAgentDeliverySubmission(ctx, f.session, f.session.AgentExecutionID,
				"retained instruction", nil, f.options(messagequeue.DeliveryProtocolV1, true))
			require.NoError(t, err)
			at := time.Now().UTC().Add(-3 * time.Hour)
			if proof {
				changed, err := f.repo.FailUnsentAgentDeliverySubmission(ctx, delivery.expected, at)
				require.NoError(t, err)
				require.True(t, changed)
			} else {
				_, err := f.repo.TransitionAgentDeliverySubmission(ctx, a.SubmissionID, models.DeliverySubmissionDispatching, models.DeliverySubmissionFailed, "failed", at)
				require.NoError(t, err)
			}
			pruned := 0
			for range 2 {
				batch, err := f.repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{MaxCandidates: 100, WorkTimeout: 5 * time.Second})
				require.NoError(t, err)
				pruned += batch.PromptPayloadsPruned
			}
			require.Equal(t, map[bool]int{true: 1, false: 0}[proof], pruned)
			var sequence int
			var name, path string
			require.NoError(t, f.repo.DB().QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path))
			f.errands.StopWorkers()
			require.NoError(t, f.svc.StopEventWatcher())
			require.NoError(t, f.repo.DB().Close())
			require.Error(t, f.repo.DB().Ping())

			raw, err := kandevdb.OpenSQLite(path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, raw.Close()) })
			opened := sqlx.NewDb(raw, "sqlite3")
			repo := sqliterepo.NewWithInitializedDB(opened, opened, nil)
			manager := &mockAgentManager{repoForExecutionLookup: repo}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			turnBus := bus.NewMemoryEventBus(testLogger())
			t.Cleanup(turnBus.Close)
			svc.SetTurnService(taskservice.NewService(taskservice.Repos{
				Tasks: repo, TaskRepos: repo, Workspaces: repo, Workflows: repo, Messages: repo, Turns: repo,
				Sessions: repo, Executors: repo, TaskEnvironments: repo,
			}, turnBus, testLogger(), taskservice.RepositoryDiscoveryConfig{}))
			adapter := NewErrandAdapter(svc)
			service, err := errands.New(errands.Dependencies{
				Store: repo, Launcher: adapter, Stopper: adapter, MCP: adapter,
				Profiles: errandProfileStub{}, Events: &errandEventRecorder{}, Logger: testLogger(),
			})
			require.NoError(t, err)
			svc.SetErrandController(service)
			require.NoError(t, svc.reconcileErrandParentAdmissions(ctx))
			reservation, err := repo.ReserveErrandParentAdmission(ctx, a)
			require.NoError(t, err)
			expected := models.ErrandAdmissionUnknown
			if proof {
				expected = models.ErrandAdmissionRefused
			}
			require.Equal(t, expected, reservation.State)
			retained, err := repo.GetAgentDeliverySubmissionMetadata(ctx, a.SubmissionID)
			require.NoError(t, err)
			require.Equal(t, delivery.expected.PayloadHash, retained.PayloadHash)
			require.Equal(t, delivery.expected.DispatchAttemptID, retained.DispatchAttemptID)
			row, err := repo.GetErrand(ctx, a.ErrandID)
			require.NoError(t, err)
			require.Equal(t, models.ErrandResultPending, row.ResultState)
			require.Equal(t, f.row.ResultRevision, row.ResultRevision)
			require.Zero(t, row.ParentAdmissionGeneration)
			turn, err := repo.GetTurn(ctx, a.TurnID)
			require.NoError(t, err)
			require.Nil(t, turn.CompletedAt, "receipt recovery does not invent a terminal work turn")
			require.Empty(t, manager.capturedPrompts)
		})
	}
}

func TestErrandUnsentAdmissionRecoveryKeepsConflictingOwner(t *testing.T) {
	for _, scenario := range []string{"turn", "execution", "incarnation", "generation", "sender", "attempt", "terminal", "cancelled", "accepted"} {
		t.Run(scenario, func(t *testing.T) {
			f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
			ctx := t.Context()
			a := fixtureAdmission(f)
			_, err := f.svc.reserveErrandParentDispatch(ctx, f.session, errands.ParentPromptOrigin{
				SenderSessionID: f.parent, SubmissionID: "admission-submission",
			}, a.TurnID, "admission-submission", f.session.AgentExecutionID)
			require.NoError(t, err)
			delivery, err := f.svc.prepareAgentDeliverySubmission(ctx, f.session, f.session.AgentExecutionID,
				"unsent instruction", nil, f.options(messagequeue.DeliveryProtocolV1, true))
			require.NoError(t, err)
			changed, err := f.repo.FailUnsentAgentDeliverySubmission(ctx, delivery.expected, time.Now().UTC())
			require.NoError(t, err)
			require.True(t, changed)
			switch scenario {
			case "turn":
				now := time.Now().UTC().Add(time.Second)
				require.NoError(t, f.repo.CreateTurn(ctx, &models.Turn{ID: "later-turn", TaskSessionID: f.session.ID, TaskID: f.taskID, StartedAt: now, CreatedAt: now, UpdatedAt: now}))
			case "execution":
				seedExecutorRunning(t, f.repo, f.session.ID, f.taskID, "successor-execution")
			case "incarnation":
				_, err := f.repo.DB().ExecContext(ctx, "UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?", "successor-incarnation", f.session.ID)
				require.NoError(t, err)
			case "generation":
				require.NoError(t, f.repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
					SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID, Generation: 2,
					NativeSessionID: "successor-native", CreationReason: "native_started",
				}))
			case "sender":
				a.SenderSessionID = "other-parent"
			case "attempt":
				_, err := f.repo.DB().ExecContext(ctx, "UPDATE agent_delivery_submissions SET dispatch_attempt_id = ? WHERE id = ?", "other-attempt", a.SubmissionID)
				require.NoError(t, err)
			case "terminal":
				_, err := f.repo.PutAgentDeliveryEffect(ctx, &models.AgentDeliveryEffect{
					EffectKey: "contradictory-terminal", EffectType: models.DeliveryTerminalEffectType, State: models.DeliveryEffectPending,
					SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID, HarnessGeneration: 1,
					StreamID: "terminal-stream", Sequence: 1, SubmissionID: a.SubmissionID,
				})
				require.NoError(t, err)
			case "cancelled":
				row, err := f.repo.GetErrand(ctx, f.row.ID)
				require.NoError(t, err)
				row.Lifecycle = models.ErrandLifecycleCancelled
				require.NoError(t, f.repo.UpdateErrand(ctx, row))
			case "accepted":
				require.NoError(t, f.svc.recordErrandAdmissionEvidence(ctx, a, models.ErrandAdmissionAccepted))
			}
			state, err := f.svc.ReadErrandParentAdmissionEvidence(ctx, a)
			require.NoError(t, err)
			if scenario == "accepted" {
				require.Equal(t, models.ErrandAdmissionAccepted, state, "positive acceptance wins over a contradictory local receipt")
			} else {
				require.Equal(t, models.ErrandAdmissionUnknown, state)
			}
			require.Empty(t, f.manager.capturedPrompts)
		})
	}
}
