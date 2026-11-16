package orchestrator

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-003.1
// Independently proven terminal evidence isolates historical convergence;
// generic abandonment does not authorize interruption settlement.
func TestErrandRestartAbandonmentSettlesExactInterruptedTurn(t *testing.T) {
	for _, later := range []bool{false, true} {
		name := "exact_terminal_only"
		if later {
			name = "later_unclassified_turn"
		}
		t.Run(name, func(t *testing.T) {
			f := newQueueSettlementFixture(t, false)
			require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), f.child.ID, false))
			row, err := f.repo.GetErrand(t.Context(), f.child.ID)
			require.NoError(t, err)
			effect := &models.AgentDeliveryEffect{EffectKey: "recovery-interruption", EffectType: "errand.turn_settlement",
				SessionID: row.ID, IncarnationID: f.child.QueueIncarnationID, HarnessGeneration: 1,
				SubmissionID: row.SubmissionID, TurnID: row.FirstTurnID, Outcome: "parent", State: "pending"}
			require.NoError(t, f.repo.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
			f.svc.abandonOpenTurnsOnStartup(t.Context(), row.ID, "restart regression")
			require.NoError(t, f.repo.UpdateTaskSessionState(t.Context(), row.ID, models.TaskSessionStateWaitingForInput, ""))
			// The incident lacks this retained exact terminal proof and stays fenced.
			_, err = f.repo.DB().Exec(`INSERT INTO agent_delivery_effects
    (effect_key, stream_id, sequence, effect_type, state, session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome, created_at)
    VALUES ('recovery-terminal', 'retained-stream', 7, ?, ?, ?, ?, 1, ?, ?, ?, ?)`,
				models.DeliveryTerminalEffectType, models.DeliveryEffectBlockResolved, row.ID, f.child.QueueIncarnationID,
				row.SubmissionID, row.FirstTurnID, models.DeliverySubmissionCancelled, row.CreatedAt)
			require.NoError(t, err)
			if later {
				// Seed canonicalDeliveryTurnTx's persisted shape, which bypasses normal
				// successor admission when projecting retained events.
				at := time.Now().UTC().Add(time.Second)
				_, err = f.repo.DB().Exec(`INSERT INTO task_session_turns
     (id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at)
     VALUES ('agent-delivery-turn-recovery-14', ?, ?, ?, ?, '{}', ?, ?)`, row.ID, row.TaskID, at, at, at, at)
				require.NoError(t, err)
			}
			f.errands.StopWorkers()
			recovered, _ := newErrandServiceOnRealStack(t, f.errandLaunchFixture)
			recovered.SetTurnReader(settlementTranscript{f.repo})
			// Later uncertainty may keep admission fenced without suppressing a
			// preceding independently proven decision.
			reconcileErr := recovered.ReconcileTerminalDecisions(t.Context(), row.ID)
			if later {
				require.ErrorIs(t, reconcileErr, db.ErrErrandSettlementPending)
			} else {
				require.NoError(t, reconcileErr)
			}
			after, err := f.repo.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			if later {
				require.Equal(t, row, after, "historical receipt must preserve the entire Errand row")
				var state string
				require.NoError(t, f.repo.DB().QueryRow(`SELECT state FROM agent_delivery_effects WHERE effect_key = 'recovery-interruption'`).Scan(&state))
				require.Equal(t, models.DeliveryEffectCompleted, state)
				require.ErrorIs(t, f.repo.CreateTurn(t.Context(), &models.Turn{ID: "blocked-successor", TaskID: row.TaskID, TaskSessionID: row.ID, StartedAt: time.Now()}), db.ErrErrandSettlementPending)
			} else {
				require.Equal(t, row.FirstTurnID, after.LatestSettledTurnID)
				require.Equal(t, models.ErrandOutcomeFailed, after.LastOutcome)
				require.False(t, after.AwaitingResult)
				session, err := f.repo.GetTaskSession(t.Context(), row.ID)
				require.NoError(t, err)
				require.Equal(t, models.ErrandStatusFailed, errands.ProjectStatus(after, session, false))
				// @covers AC-TASKS-ERRANDS-002.7
				session.AgentProfileID = "native-profile"
				session.DownstreamACPSessionID = "retained-native"
				require.NoError(t, f.repo.UpdateTaskSession(t.Context(), session))
				committed, err := f.repo.CommitHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{
					SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1,
					NativeSessionID: "retained-native", CreationReason: "initial", CreatedAt: time.Now().UTC(), CommittedAt: time.Now().UTC(),
				}, 0)
				require.NoError(t, err)
				require.True(t, committed)
				f.manager.resolveProfileInfo = &executor.AgentProfileInfo{NativeSessionResume: true}
				require.NoError(t, f.repo.DeleteExecutorRunningBySessionID(t.Context(), session.ID))
				candidate, err := f.svc.loadResumeSessionCandidate(t.Context(), row.TaskID, row.ID, executor.ResumeOptions{}, nil)
				require.NoError(t, err, "exact interruption and retained native identity permit missing executor restoration")
				require.Equal(t, "retained-native", candidate.options.RequiredNativeConversationID)
				require.False(t, candidate.options.AllowCompletedSessionResume)
				f.manager.resolveProfileInfo.NativeSessionResume = false
				_, err = f.svc.loadResumeSessionCandidate(t.Context(), row.TaskID, row.ID, executor.ResumeOptions{}, nil)
				require.Error(t, err, "unsupported provider stays fenced")
				f.manager.resolveProfileInfo.NativeSessionResume = true
				session.DownstreamACPSessionID = "different-native"
				require.NoError(t, f.repo.UpdateTaskSession(t.Context(), session))
				_, err = f.svc.loadResumeSessionCandidate(t.Context(), row.TaskID, row.ID, executor.ResumeOptions{}, nil)
				require.Error(t, err, "conflicting native identities stay fenced")
				session.DownstreamACPSessionID = "retained-native"
				require.NoError(t, f.repo.UpdateTaskSession(t.Context(), session))
				require.NoError(t, f.repo.CreateTurn(t.Context(), &models.Turn{ID: "unresolved-later", TaskID: row.TaskID, TaskSessionID: row.ID, StartedAt: time.Now().UTC()}))
				_, err = f.svc.loadResumeSessionCandidate(t.Context(), row.TaskID, row.ID, executor.ResumeOptions{}, nil)
				require.Error(t, err, "later open work cannot restore through the interrupted exception")

			}
			require.Zero(t, f.taskRepo.stateWrites[f.taskID])
			select {
			case <-f.manager.finished:
				t.Fatal("recovery replayed a provider prompt")
			default:
			}
		})
	}
}
