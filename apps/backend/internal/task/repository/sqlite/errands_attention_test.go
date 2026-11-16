package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// AC-TASKS-ERRANDS-002.4-.5: recovered follow-up work keeps capacity despite an older result.
func TestErrandRecoveryAttentionCapacity(t *testing.T) {
	exerciseErrandRecoveryAttentionCapacity(t, newRepoForSessionTests(t))
}
func TestPostgresErrandRecoveryAttentionCapacity(t *testing.T) {
	exerciseErrandRecoveryAttentionCapacity(t, newPostgresErrandRepo(t))
}
func seedRecoveryAttention(t *testing.T, repo *Repository) (*models.Errand, *models.TaskSession) {
	row, effect := seedExactSettlement(t, repo)
	require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	require.NoError(t, repo.CompleteTurn(t.Context(), row.FirstTurnID))
	session, err := repo.GetTaskSession(t.Context(), row.ID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	session.Metadata = map[string]interface{}{}
	session.Metadata[models.SessionMetaKeyAgentDeliveryRecovery] = models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryRecovered, Revision: 1, SessionID: row.ID,
		IncarnationID: session.QueueIncarnationID, SubmissionID: row.SubmissionID,
		HarnessGeneration: 1, AgentExecutionID: "original", StreamID: "original-stream",
	}
	require.NoError(t, repo.UpdateTaskSession(t.Context(), session))
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), session.ID, models.SessionMetaKeyAgentDeliveryRecovery, session.Metadata[models.SessionMetaKeyAgentDeliveryRecovery]))
	_, err = repo.db.Exec(repo.db.Rebind(`INSERT INTO task_session_turns
 (id, task_id, task_session_id, started_at, created_at, updated_at, metadata)
 VALUES ('older-result', ?, ?, ?, ?, ?, '{}')`), row.TaskID, row.ID, errandScenarioBase.Add(-time.Hour), errandScenarioBase, errandScenarioBase)
	require.NoError(t, err)
	row.FirstTurnID, row.SubmissionID = "older-result", "older-submission"
	row.LastOutcome = models.ErrandOutcomeDone
	row.ResultTurnID, row.LatestSettledTurnID = "older-result", "older-result"
	row.ResultState, row.ResultKind = models.ErrandResultSuperseded, models.ErrandResultOutput
	row.ResultText = "preserved output"
	require.NoError(t, repo.UpdateErrand(t.Context(), row))
	return row, session
}

func exerciseErrandRecoveryAttentionCapacity(t *testing.T, repo *Repository) {
	row, _ := seedRecoveryAttention(t, repo)
	count, err := repo.CountLiveErrandsByTask(t.Context(), row.TaskID)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	summary, err := repo.BatchErrandsSummariesByTaskIDs(t.Context(), []string{row.TaskID})
	require.NoError(t, err)
	require.Equal(t, count, summary[row.TaskID].Live)
	stored, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, row.ResultText, stored.ResultText)
	require.Equal(t, row.ResultRevision, stored.ResultRevision)
	require.Equal(t, row.ResultState, stored.ResultState)
}

func TestErrandRecoveryAttentionEvidence(t *testing.T) {
	exerciseRecoveryAttentionEvidence(t, newRepoForSessionTests)
}
func TestPostgresErrandRecoveryAttentionEvidence(t *testing.T) {
	exerciseRecoveryAttentionEvidence(t, newPostgresErrandRepo)
}
func exerciseRecoveryAttentionEvidence(t *testing.T, create func(*testing.T) *Repository) {
	cases := recoveryAttentionCases()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := create(t)
			row, session := seedRecoveryAttention(t, r)
			if tc.change != nil {
				tc.change(t, r, row, session)
			}
			require.NoError(t, r.UpdateTaskSession(t.Context(), session))
			require.NoError(t, r.SetSessionMetadataKey(t.Context(), session.ID, models.SessionMetaKeyAgentDeliveryRecovery, session.Metadata[models.SessionMetaKeyAgentDeliveryRecovery]))
			require.NoError(t, r.UpdateErrand(t.Context(), row))
			observed, err := r.ErrandRecoveryAttention(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, tc.want, observed)
		})
	}
}
func recoveryChange(change func(*models.AgentDeliveryRecovery)) func(*testing.T, *Repository, *models.Errand, *models.TaskSession) {
	return func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
		v := s.Metadata[models.SessionMetaKeyAgentDeliveryRecovery].(models.AgentDeliveryRecovery)
		change(&v)
		s.Metadata[models.SessionMetaKeyAgentDeliveryRecovery] = v
	}
}

type recoveryAttentionCase struct {
	name   string
	change func(*testing.T, *Repository, *models.Errand, *models.TaskSession)
	want   bool
}

func recoveryAttentionCases() []recoveryAttentionCase {
	return []recoveryAttentionCase{
		{"retained identity without new assessment", nil, true},
		{"awaiting flag absent", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) { e.AwaitingResult = false }, true},
		{"legacy initial reservation", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			e.FirstTurnID = "exact-A"
			e.SubmissionID = "prompt:exact-A"
			e.ResultTurnID = ""
			e.LastOutcome = ""
			_, err := r.db.Exec(`DELETE FROM agent_delivery_effects WHERE effect_key='exact-settlement-A'`)
			require.NoError(t, err)
		}, true},
		{"startup", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			s.State = models.TaskSessionStateStarting
		}, false},
		{"active", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			s.State = models.TaskSessionStateRunning
		}, false},
		{"failed", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			s.State = models.TaskSessionStateFailed
		}, false},
		{"cancelled", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			e.Lifecycle = models.ErrandLifecycleCancelled
			e.CancelSource = models.ErrandCancelSourceUser
		}, false},
		{"no recovery", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			delete(s.Metadata, models.SessionMetaKeyAgentDeliveryRecovery)
		}, false},
		{"reconnecting", recoveryChange(func(v *models.AgentDeliveryRecovery) { v.Phase = models.AgentDeliveryRecoveryReconnecting }), false},
		{"stale incarnation", recoveryChange(func(v *models.AgentDeliveryRecovery) { v.IncarnationID = "stale" }), false},
		{"other submission", recoveryChange(func(v *models.AgentDeliveryRecovery) { v.SubmissionID = "other" }), false},
		{"invalid revision", recoveryChange(func(v *models.AgentDeliveryRecovery) { v.Revision = 0 }), false},
		{"open turn", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			_, err := r.db.Exec(`UPDATE task_session_turns SET completed_at=NULL WHERE id='exact-A'`)
			require.NoError(t, err)
		}, false},
		{"lifecycle only", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			_, err := r.db.Exec(`UPDATE task_session_turns SET metadata='{"lifecycle_only":true}' WHERE id='exact-A'`)
			require.NoError(t, err)
		}, false},
		{"newer result", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			_, err := r.db.Exec(r.db.Rebind(`UPDATE task_session_turns SET started_at=? WHERE id='older-result'`), errandScenarioBase.Add(time.Hour))
			require.NoError(t, err)
		}, false},
		{"receipt complete", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			_, err := r.db.Exec(r.db.Rebind(`UPDATE agent_delivery_effects SET state='completed',outcome='parent:interrupted',completed_at=? WHERE effect_key='exact-settlement-A'`), errandScenarioBase)
			require.NoError(t, err)
		}, false},
		{"ambiguous exact receipts", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			_, err := r.db.Exec(r.db.Rebind(`INSERT INTO agent_delivery_effects
 (effect_key,stream_id,sequence,effect_type,state,session_id,incarnation_id,harness_generation,submission_id,turn_id,outcome,created_at,completed_at)
 SELECT 'duplicate',stream_id,sequence,effect_type,'completed',session_id,incarnation_id,harness_generation,submission_id,turn_id,'parent:done',created_at,?
 FROM agent_delivery_effects WHERE effect_key='exact-settlement-A'`), errandScenarioBase)
			require.NoError(t, err)
		}, true},
		{"terminal reconciliation", func(t *testing.T, r *Repository, e *models.Errand, s *models.TaskSession) {
			_, err := r.db.Exec(`UPDATE agent_delivery_effects SET outcome='parent:done' WHERE effect_key='exact-settlement-A'`)
			require.NoError(t, err)
		}, false},
	}
}

func TestErrandRecoveryAttentionProjectedTerminal(t *testing.T) {
	exerciseAttentionTerminal(t, newRepoForSessionTests)
}
func TestPostgresErrandRecoveryAttentionProjectedTerminal(t *testing.T) {
	exerciseAttentionTerminal(t, newPostgresErrandRepo)
}
func exerciseAttentionTerminal(t *testing.T, create func(*testing.T) *Repository) {
	cases := []struct {
		name, state                  string
		generation                   int
		projected, conflicting, want bool
	}{
		{"resolved", "block_resolved", 1, false, false, false},
		{"completed", "completed", 1, false, false, false},
		{"pending projected", "pending", 1, true, false, false},
		{"pending unprojected", "pending", 1, false, false, true},
		{"stale generation", "completed", 2, false, false, true},
		{"conflicting outcomes", "completed", 1, false, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := create(t)
			row, _ := seedRecoveryAttention(t, repo)
			_, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO agent_delivery_effects
 (effect_key,stream_id,sequence,effect_type,state,session_id,incarnation_id,harness_generation,submission_id,turn_id,outcome,created_at)
 SELECT 'projected-terminal',stream_id,1,?,?,session_id,incarnation_id,?,submission_id,turn_id,'completed',created_at
 FROM agent_delivery_effects WHERE effect_key='exact-settlement-A'`), models.DeliveryTerminalEffectType, tc.state, tc.generation)
			require.NoError(t, err)
			if tc.projected {
				_, err = repo.db.Exec(`INSERT INTO agent_delivery_cursors (stream_id,session_id,incarnation_id,harness_generation,projected_sequence,updated_at)
 SELECT stream_id,session_id,incarnation_id,harness_generation,sequence,created_at FROM agent_delivery_effects WHERE effect_key='projected-terminal'`)
				require.NoError(t, err)
			}
			if tc.conflicting {
				_, err = repo.db.Exec(`INSERT INTO agent_delivery_effects
 (effect_key,stream_id,sequence,effect_type,state,session_id,incarnation_id,harness_generation,submission_id,turn_id,outcome,created_at)
 SELECT 'conflicting-terminal',stream_id,2,effect_type,state,session_id,incarnation_id,harness_generation,submission_id,turn_id,'failed',created_at
 FROM agent_delivery_effects WHERE effect_key='projected-terminal'`)
				require.NoError(t, err)
			}
			attention, err := repo.ErrandRecoveryAttention(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, tc.want, attention)
		})
	}
}
