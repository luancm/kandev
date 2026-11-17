package sqlite

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type errandGenerationStore interface {
	ReserveErrandExecutionGeneration(context.Context, string, uint64) (*models.ErrandExecutionDescriptor, error)
	BindErrandExecution(context.Context, string, uint64, string) (bool, error)
	GetErrandExecutionDescriptor(context.Context, string) (*models.ErrandExecutionDescriptor, error)
}

func TestErrandExecutionGeneration(t *testing.T) {
	exerciseErrandExecutionGeneration(t, newRepoForSessionTests(t))
}
func TestPostgresErrandExecutionGeneration(t *testing.T) {
	exerciseErrandExecutionGeneration(t, newPostgresErrandRepo(t))
}
func exerciseErrandExecutionGeneration(t *testing.T, repo *Repository) {
	t.Helper()
	store, ok := any(repo).(errandGenerationStore)
	require.True(t, ok, "repository must provide Errand execution generation ownership")
	ctx := context.Background()
	task := newErrandTask(t, repo, "generation")
	mustCreateErrand(t, repo, sampleErrand("errand-generation", task, "parent", errandScenarioBase))
	first, err := store.ReserveErrandExecutionGeneration(ctx, "errand-generation", 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, first.Generation)
	require.Equal(t, task, first.TaskID)
	bound, err := store.BindErrandExecution(ctx, first.ErrandID, first.Generation, "execution-1")
	require.NoError(t, err)
	require.True(t, bound)
	_, err = store.ReserveErrandExecutionGeneration(ctx, first.ErrandID, 0)
	require.Error(t, err)
	second, err := store.ReserveErrandExecutionGeneration(ctx, first.ErrandID, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, second.Generation)
	bound, err = store.BindErrandExecution(ctx, first.ErrandID, first.Generation, "stale")
	require.NoError(t, err)
	require.False(t, bound)
	current, err := store.GetErrandExecutionDescriptor(ctx, first.ErrandID)
	require.NoError(t, err)
	require.EqualValues(t, 2, current.Generation)
	require.Empty(t, current.ExecutionID)
	third, err := store.ReserveErrandExecutionGeneration(ctx, first.ErrandID, 2)
	require.NoError(t, err)
	bound, err = store.BindErrandExecution(ctx, first.ErrandID, second.Generation, "stale-unbound")
	require.NoError(t, err)
	require.False(t, bound)
	current, err = store.GetErrandExecutionDescriptor(ctx, first.ErrandID)
	require.NoError(t, err)
	require.Equal(t, third.Generation, current.Generation)
	require.Empty(t, current.ExecutionID)
	_, err = store.ReserveErrandExecutionGeneration(ctx, first.ErrandID, math.MaxInt64)
	require.Error(t, err)
	bound, err = store.BindErrandExecution(ctx, first.ErrandID, math.MaxUint64, "overflow")
	require.Error(t, err)
	require.False(t, bound)
	var sessions int
	require.NoError(t, repo.db.GetContext(ctx, &sessions, repo.db.Rebind(`SELECT COUNT(*) FROM task_sessions WHERE task_id=?`), task))
	require.Zero(t, sessions)
}

func TestErrandExecutionGenerationRetainsReceiptOwner(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	task := newErrandTask(t, repo, "generation-retention")
	mustCreateErrand(t, repo, sampleErrand("errand-retention", task, "parent", errandScenarioBase))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "errand-retention", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "errand-retention", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{ErrandID: "errand-retention", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream-retention"}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`INSERT INTO agent_delivery_cursors (stream_id,session_id,incarnation_id,harness_generation,updated_at) VALUES (?,?,?,?,?)`), "stream-retention", "errand:errand-retention", "incarnation", 1, errandScenarioBase)
	require.NoError(t, err)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	candidates, _, _, err := repo.loadAgentDeliveryOrphanCandidatesTx(ctx, tx, "", 100)
	require.NoError(t, err)
	for _, candidate := range candidates {
		require.NotEqual(t, "errand:errand-retention", candidate.sessionID)
	}
}

func TestPostgresErrandBindingSerializesWithGenerationReservation(t *testing.T) {
	repo := newPostgresErrandRepo(t)
	repo.db.SetMaxOpenConns(4)
	ctx := context.Background()
	task := newErrandTask(t, repo, "generation-lock")
	mustCreateErrand(t, repo, sampleErrand("errand-generation-lock", task, "parent", errandScenarioBase))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "errand-generation-lock", 0)
	require.NoError(t, err)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, repo.lockTaskRowInTx(ctx, tx, task))
	blocked, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	_, err = repo.BindErrandExecution(blocked, "errand-generation-lock", 1, "execution")
	require.Error(t, err, "binding must wait for generation reservation's task lock")
}

type errandAdmissionStore interface {
	PrepareErrandDeliverySubmission(context.Context, *models.ErrandDeliveryAdmission) (bool, error)
}

func TestErrandDeliveryAdmissionAtomic(t *testing.T) {
	exerciseErrandDeliveryAdmission(t, newRepoForSessionTests(t), true)
}
func TestPostgresErrandDeliveryAdmission(t *testing.T) {
	exerciseErrandDeliveryAdmission(t, newPostgresErrandRepo(t), false)
}
func exerciseErrandDeliveryAdmission(t *testing.T, repo *Repository, injectFailure bool) {
	t.Helper()
	store, ok := any(repo).(errandAdmissionStore)
	require.True(t, ok, "Errand admission must be independent of task sessions")
	ctx := context.Background()
	task := newErrandTask(t, repo, "delivery-admission")
	mustCreateErrand(t, repo, sampleErrand("admission", task, "parent", errandScenarioBase))
	generation, err := repo.ReserveErrandExecutionGeneration(ctx, "admission", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "admission", generation.Generation, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.CreateErrandTurn(ctx, "admission", &models.Turn{ID: "turn", TaskID: task}))
	binding := &models.ErrandHarnessBinding{ErrandID: "admission", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, StreamID: "stream"}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	admission := &models.ErrandDeliveryAdmission{ErrandID: "admission", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 3, StreamID: "stream", PromptGeneration: 2, TurnID: "turn", Submission: &models.AgentDeliverySubmission{ID: "submission", PayloadHash: journal.SubmissionHash([]byte("payload")), Payload: []byte("payload")}}
	if injectFailure {
		_, err = repo.db.Exec(`CREATE TRIGGER reject_errand_submission BEFORE INSERT ON agent_delivery_submissions BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
		require.NoError(t, err)
		_, err = store.PrepareErrandDeliverySubmission(ctx, admission)
		require.Error(t, err)
		var effects int
		require.NoError(t, repo.db.GetContext(ctx, &effects, `SELECT COUNT(*) FROM agent_delivery_effects`))
		require.Zero(t, effects)
		var active string
		require.NoError(t, repo.db.GetContext(ctx, &active, `SELECT active_submission_id FROM errand_execution_generations WHERE errand_id='admission'`))
		require.Empty(t, active)
		_, err = repo.db.Exec(`DROP TRIGGER reject_errand_submission`)
		require.NoError(t, err)
	}
	created, err := store.PrepareErrandDeliverySubmission(ctx, admission)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, repo.CompleteErrandHarnessNativeSession(ctx, binding, "native"))
	created, err = store.PrepareErrandDeliverySubmission(ctx, admission)
	require.NoError(t, err)
	require.False(t, created)
	conflicting := *admission
	conflicting.TurnID = "missing"
	_, err = store.PrepareErrandDeliverySubmission(ctx, &conflicting)
	require.Error(t, err)
	var count int
	require.NoError(t, repo.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM agent_delivery_submissions`))
	require.Equal(t, 1, count)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	candidates, _, _, err := repo.loadAgentDeliveryOrphanCandidatesTx(ctx, tx, "", 100)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	for _, candidate := range candidates {
		require.NotEqual(t, "errand:admission", candidate.sessionID)
	}
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE errand_execution_generations SET active_submission_id='', active_turn_id='' WHERE errand_id=?`), "admission")
	require.NoError(t, err)
	_, err = store.PrepareErrandDeliverySubmission(ctx, admission)
	require.Error(t, err, "settled admission cannot be redispatched")
	fresh := *admission
	fresh.Submission = &models.AgentDeliverySubmission{ID: "next-submission", Payload: []byte("next"), PayloadHash: journal.SubmissionHash([]byte("next"))}
	_, err = store.PrepareErrandDeliverySubmission(ctx, &fresh)
	require.Error(t, err, "fresh admission must advance prompt generation")
	fresh.PromptGeneration++
	created, err = store.PrepareErrandDeliverySubmission(ctx, &fresh)
	require.NoError(t, err)
	require.True(t, created)
}

type errandHarnessStore interface {
	AppendErrandHarnessBinding(context.Context, *models.ErrandHarnessBinding, uint64) error
	CompleteErrandHarnessNativeSession(context.Context, *models.ErrandHarnessBinding, string) error
}

func TestErrandHarnessBinding(t *testing.T) {
	exerciseErrandHarnessBinding(t, newRepoForSessionTests(t))
}
func TestPostgresErrandHarnessBinding(t *testing.T) {
	exerciseErrandHarnessBinding(t, newPostgresErrandRepo(t))
}
func exerciseErrandHarnessBinding(t *testing.T, repo *Repository) {
	t.Helper()
	store, ok := any(repo).(errandHarnessStore)
	require.True(t, ok, "immutable Errand harness binding required")
	ctx := context.Background()
	task := newErrandTask(t, repo, "harness-binding")
	mustCreateErrand(t, repo, sampleErrand("binding", task, "parent", errandScenarioBase))
	generation, err := repo.ReserveErrandExecutionGeneration(ctx, "binding", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "binding", generation.Generation, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	first := &models.ErrandHarnessBinding{ErrandID: "binding", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream"}
	require.NoError(t, store.AppendErrandHarnessBinding(ctx, first, 0))
	require.EqualValues(t, 1, first.Sequence)
	require.NoError(t, store.AppendErrandHarnessBinding(ctx, first, 0))
	require.NoError(t, store.CompleteErrandHarnessNativeSession(ctx, first, "native"))
	require.NoError(t, store.AppendErrandHarnessBinding(ctx, first, 0))
	require.NoError(t, store.CompleteErrandHarnessNativeSession(ctx, first, "native"))
	require.Error(t, store.CompleteErrandHarnessNativeSession(ctx, first, "different"))
	first.NativeSessionID = "native"
	duplicate := *first
	duplicate.StreamID = "other"
	require.Error(t, store.AppendErrandHarnessBinding(ctx, &duplicate, 1))
	second := *first
	second.IncarnationID = "new-incarnation"
	second.StreamID = "new-stream"
	require.NoError(t, store.AppendErrandHarnessBinding(ctx, &second, 1))
	require.EqualValues(t, 2, second.Sequence)
	_, err = repo.ReserveErrandExecutionGeneration(ctx, "binding", 1)
	require.NoError(t, err)
	stale := second
	stale.StreamID = "stale"
	stale.IncarnationID = "stale"
	require.Error(t, store.AppendErrandHarnessBinding(ctx, &stale, 2))
	var count int
	require.NoError(t, repo.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM errand_harness_bindings`))
	require.Equal(t, 2, count)
}

func TestPostgresErrandBindingFencesOrphanScheduling(t *testing.T) {
	repo := newPostgresErrandRepo(t)
	repo.db.SetMaxOpenConns(4)
	ctx := context.Background()
	task := newErrandTask(t, repo, "orphan-race")
	mustCreateErrand(t, repo, sampleErrand("orphan-race", task, "parent", errandScenarioBase))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "orphan-race", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "orphan-race", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`INSERT INTO agent_delivery_cursors (stream_id,session_id,incarnation_id,harness_generation,updated_at) VALUES (?,?,?,?,?)`), "stream-race", "errand:orphan-race", "incarnation", 1, errandScenarioBase)
	require.NoError(t, err)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	candidates, _, _, err := repo.loadAgentDeliveryOrphanCandidatesTx(ctx, tx, "", 100)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	binding := &models.ErrandHarnessBinding{ErrandID: "orphan-race", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream-race"}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	var result models.AgentDeliveryOrphanBatchResult
	require.NoError(t, repo.scheduleAgentDeliveryOrphansTx(ctx, tx, candidates, &result))
	require.Zero(t, result.JobsScheduled)
	require.NoError(t, tx.Commit())
	tx, err = repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, repo.insertAgentDeliveryRetirementJobsTx(ctx, tx, []agentDeliverySessionIdentity{{sessionID: "errand:orphan-race", incarnationID: "incarnation"}}))
	require.NoError(t, tx.Commit())
	next := *binding
	next.IncarnationID = "next"
	next.StreamID = "next"
	require.Error(t, repo.AppendErrandHarnessBinding(ctx, &next, 1))
	retirement, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, retirement.Suspended)
	var count int
	require.NoError(t, repo.db.GetContext(ctx, &count, repo.db.Rebind(`SELECT COUNT(*) FROM agent_delivery_cursors WHERE stream_id=?`), "stream-race"))
	require.Equal(t, 1, count)
}
