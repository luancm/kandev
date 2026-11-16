package orchestrator

import (
	"context"
	"expvar"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Reads the committed canonical transcript, including the exact requested turn.
type settlementTranscript struct{ repo *sqliterepo.Repository }

func (r settlementTranscript) ReadTurn(ctx context.Context, sessionID, turnID string) (errands.TurnView, error) {
	turn, err := r.repo.GetTurn(ctx, turnID)
	if err != nil || turn.TaskSessionID != sessionID {
		return errands.TurnView{}, err
	}
	messages, err := r.repo.ListMessagesByTurnID(ctx, turnID)
	view := errands.TurnView{Completed: turn.CompletedAt != nil}
	for _, m := range messages {
		if m.AuthorType == models.MessageAuthorAgent && (m.Type == "" || m.Type == models.MessageTypeMessage) {
			view.LastAgentText = m.Content
		}
		if m.AuthorType == models.MessageAuthorUser && view.EarliestUserSender == "" {
			view.EarliestUserSender, _ = m.Metadata["sender_session_id"].(string)
		}
	}
	return view, err
}

func (r settlementTranscript) LatestTurn(ctx context.Context, sessionID string) (string, errands.TurnView, error) {
	turns, err := r.repo.ListTurnsBySession(ctx, sessionID)
	if err != nil || len(turns) == 0 {
		return "", errands.TurnView{}, err
	}
	id := turns[len(turns)-1].ID
	view, err := r.ReadTurn(ctx, sessionID, id)
	return id, view, err
}

type settlementDispatchManager struct {
	*retentionPromptAgentManager
	finished   chan struct{}
	finishOnce sync.Once
}

func (m *settlementDispatchManager) finish() { m.finishOnce.Do(func() { close(m.finished) }) }

func (m *settlementDispatchManager) PromptAgentWithAdmissionCallback(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool,
	before func(context.Context, string, []v1.MessageAttachment) error, accepted func(),
) (*executor.PromptResult, error) {
	defer m.finish()
	return m.mockAgentManager.PromptAgentWithAdmissionCallback(ctx, executionID, prompt, attachments, dispatchOnly, before, accepted)
}

func (m *settlementDispatchManager) PromptAgentWithDispatchCallback(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, accepted func(),
) (*executor.PromptResult, error) {
	defer m.finish()
	return m.mockAgentManager.PromptAgentWithDispatchCallback(ctx, executionID, prompt, attachments, dispatchOnly, accepted)
}

func (m *settlementDispatchManager) PromptAgentWithAdmissionCallbackAndSubmissionID(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool,
	before func(context.Context, string, []v1.MessageAttachment) error, accepted func(), submissionID string,
) (*executor.PromptResult, error) {
	defer m.finish()
	return m.retentionPromptAgentManager.PromptAgentWithAdmissionCallbackAndSubmissionID(
		ctx, executionID, prompt, attachments, dispatchOnly, before, accepted, submissionID)
}

func (m *settlementDispatchManager) PromptAgentWithDispatchCallbackAndSubmissionID(
	ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool,
	accepted func(), submissionID string,
) (*executor.PromptResult, error) {
	defer m.finish()
	return m.retentionPromptAgentManager.PromptAgentWithDispatchCallbackAndSubmissionID(
		ctx, executionID, prompt, attachments, dispatchOnly, accepted, submissionID)
}

type queueSettlementFixture struct {
	*errandLaunchFixture
	errands    *errands.Service
	child      *models.TaskSession
	manager    *settlementDispatchManager
	entry      *messagequeue.QueuedMessage
	workerDone chan struct{}
	sendNow    bool
}

func newQueueSettlementFixture(t *testing.T, parentSource bool) *queueSettlementFixture {
	t.Helper()
	f := newErrandLaunchFixture(t)
	f.svc.SetMessageCreator(&repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: f.repo})
	f.svc.SetTransientRetryMessageService(f.svc.turnService.(TransientRetryMessageService))
	service, _ := newErrandServiceOnRealStack(t, f)
	service.SetTurnReader(settlementTranscript{f.repo})
	now := time.Now().UTC()
	child := &models.TaskSession{ID: "settlement-child", TaskID: f.taskID, ParentSessionID: f.parent,
		Role: models.SessionRoleErrand, State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now}
	require.NoError(t, f.repo.CreateTaskSession(t.Context(), child))
	require.NoError(t, f.repo.CreateErrand(t.Context(), &models.Errand{ID: child.ID, TaskID: f.taskID,
		ParentSessionID: f.parent, Title: "capture", Objective: "capture A", AgentProfileID: "errand-profile",
		FirstTurnID: "settlement-A", SubmissionID: "prompt:errand:settlement-A", Lifecycle: models.ErrandLifecycleLaunched,
		AwaitingResult: true, ResultState: models.ErrandResultNone, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, f.repo.CreateTurn(t.Context(), &models.Turn{ID: "settlement-A", TaskID: f.taskID,
		TaskSessionID: child.ID, StartedAt: now, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, f.repo.CreateMessage(t.Context(), &models.Message{ID: "A-output", TaskID: f.taskID,
		TaskSessionID: child.ID, TurnID: "settlement-A", AuthorType: models.MessageAuthorAgent,
		Content: "canonical result A", CreatedAt: now}))
	seedExecutorRunning(t, f.repo, child.ID, f.taskID, "settlement-execution")
	manager := &settlementDispatchManager{retentionPromptAgentManager: &retentionPromptAgentManager{
		durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
			mockAgentManager: &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: f.repo},
			capability:       agentruntime.DurableDeliveryCapability{},
		}}, finished: make(chan struct{})}
	f.svc.agentManager = manager
	f.svc.executor = withTestExecutorActor(executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{}))
	database := sqlx.NewDb(f.repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	f.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	workerDone := make(chan struct{})
	var workerOnce sync.Once
	f.svc.onQueuedMessageExecutionComplete = func() { workerOnce.Do(func() { close(workerDone) }) }
	by, metadata := messagequeue.QueuedByUser, map[string]interface{}{}
	if parentSource {
		by, metadata[errands.ParentPromptSenderMetadataKey] = messagequeue.QueuedByAgent, f.parent
	}
	entry, err := f.svc.messageQueue.QueueMessageWithMetadata(t.Context(), child.ID, f.taskID, "successor B", "", by, false, nil, metadata)
	require.NoError(t, err)
	require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), child.ID, true))
	return &queueSettlementFixture{errandLaunchFixture: f, errands: service, child: child, manager: manager, entry: entry, workerDone: workerDone}
}

func (f *queueSettlementFixture) ready(t *testing.T) {
	t.Helper()
	f.svc.handleAgentReady(t.Context(), watcher.AgentEventData{TaskID: f.taskID, SessionID: f.child.ID,
		AgentExecutionID: "settlement-execution", TurnID: "settlement-A"})
}

func (f *queueSettlementFixture) awaitDispatch(t *testing.T) {
	t.Helper()
	select {
	case <-f.manager.finished:
	case <-time.After(10 * time.Second):
		session, _ := f.repo.GetTaskSession(t.Context(), f.child.ID)
		turns, _ := f.repo.ListTurnsBySession(t.Context(), f.child.ID)
		t.Fatalf("successor did not finish provider admission: state=%v turns=%+v queue=%+v", session.State, turns, f.svc.messageQueue.GetStatus(t.Context(), f.child.ID))
	}
	if f.sendNow {
		f.svc.stopSendNowWorkers()
		return
	}
	select {
	case <-f.workerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("queued worker did not release its reservation")
	}
}

// @covers AC-TASKS-ERRANDS-003.1 AC-TASKS-ERRANDS-003.3 AC-TASKS-ERRANDS-006.5
func TestErrandReadyCapturesResultBeforeQueuedUserTurn(t *testing.T) {
	f := newQueueSettlementFixture(t, false)
	settlements := func() int64 {
		counter := expvar.Get("errands_settled_total").(*expvar.Map).Get("status=done")
		if counter == nil {
			return 0
		}
		value, err := strconv.ParseInt(counter.String(), 10, 64)
		require.NoError(t, err)
		return value
	}
	before := settlements()
	observed := make(chan *models.Errand, 1)
	f.manager.promptAgentFunc = func(ctx context.Context, _, _ string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		row, err := f.repo.GetErrand(ctx, f.child.ID)
		observed <- row
		return &executor.PromptResult{}, err
	}
	f.ready(t)
	f.awaitDispatch(t)
	row := <-observed
	require.Equal(t, "canonical result A", row.ResultText, "capture must commit before B reaches the provider")
	require.Equal(t, "settlement-A", row.ResultTurnID)
	require.Equal(t, "settlement-A", row.LatestSettledTurnID)
	require.Equal(t, models.ErrandOutcomeDone, row.LastOutcome)
	require.Equal(t, models.ErrandResultPending, row.ResultState)
	require.False(t, row.AwaitingResult)
	require.Zero(t, f.taskRepo.stateWrites[f.taskID])
	b, err := f.repo.GetActiveTurnBySessionID(t.Context(), f.child.ID)
	require.NoError(t, err)
	require.NotNil(t, b)
	f.svc.handleAgentReady(t.Context(), watcher.AgentEventData{TaskID: f.taskID, SessionID: f.child.ID,
		AgentExecutionID: "settlement-execution", TurnID: b.ID})
	after, err := f.repo.GetErrand(t.Context(), f.child.ID)
	require.NoError(t, err)
	require.Equal(t, "canonical result A", after.ResultText)
	require.Equal(t, "settlement-A", after.ResultTurnID)
	require.Equal(t, b.ID, after.LatestSettledTurnID)
	require.Equal(t, models.ErrandOutcomeDone, after.LastOutcome)
	require.False(t, after.AwaitingResult)
	require.Zero(t, f.taskRepo.stateWrites[f.taskID])
	require.Equal(t, before+1, settlements(), "one exact parent result, with no extra settlement for the user successor")
}

// @covers AC-TASKS-ERRANDS-003.3 AC-TASKS-ERRANDS-012.3
func TestErrandSettlementFailureParksSuccessorUntilDurableRetry(t *testing.T) {
	for _, retry := range []string{"readiness", "fifo", "send_now"} {
		t.Run(retry, func(t *testing.T) {
			f := newQueueSettlementFixture(t, false)
			_, err := f.repo.DB().Exec(`CREATE TRIGGER fail_exact_capture BEFORE UPDATE ON task_session_errands
				WHEN NEW.latest_settled_turn_id = 'settlement-A' BEGIN SELECT RAISE(ABORT, 'capture fault'); END`)
			require.NoError(t, err)
			f.ready(t)
			status := f.svc.messageQueue.GetStatus(t.Context(), f.child.ID)
			require.Len(t, status.Entries, 1, "failed capture must leave B unclaimed")
			require.Equal(t, f.entry.ID, status.Entries[0].ID)
			session, err := f.repo.GetTaskSession(t.Context(), f.child.ID)
			require.NoError(t, err)
			require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
			require.Zero(t, f.taskRepo.stateWrites[f.taskID])
			select {
			case <-f.manager.finished:
				t.Fatal("capture fault admitted B")
			default:
			}
			_, err = f.repo.DB().Exec(`DROP TRIGGER fail_exact_capture`)
			require.NoError(t, err)
			f.errands.StopWorkers()
			f.errands, _ = newErrandServiceOnRealStack(t, f.errandLaunchFixture)
			f.errands.SetTurnReader(settlementTranscript{f.repo})
			switch retry {
			case "readiness":
				f.svc.reconcileErrandsOnStartup(t.Context())
				f.svc.handleAgentBootReady(t.Context(), watcher.AgentEventData{TaskID: f.taskID, SessionID: f.child.ID, AgentExecutionID: "settlement-execution"})
			case "fifo":
				dispatched, err := f.svc.DrainQueuedMessage(t.Context(), f.child.ID)
				require.NoError(t, err)
				require.True(t, dispatched)
			case "send_now":
				f.sendNow = true
				_, err := f.svc.SendQueuedNow(t.Context(), f.child.ID, "all", "")
				require.NoError(t, err)
				t.Cleanup(f.svc.stopSendNowWorkers)
			}
			f.awaitDispatch(t)
			row, err := f.repo.GetErrand(t.Context(), f.child.ID)
			require.NoError(t, err)
			require.Equal(t, "canonical result A", row.ResultText)
			require.Equal(t, "settlement-A", row.LatestSettledTurnID)
		})
	}
}

// @covers AC-TASKS-ERRANDS-006.1 AC-TASKS-ERRANDS-006.5
func TestParentOutstandingQueueDrainDoesNotWriteReview(t *testing.T) {
	f := newQueueSettlementFixture(t, false)
	parent, err := f.repo.GetTaskSession(t.Context(), f.parent)
	require.NoError(t, err)
	parent.State = models.TaskSessionStateRunning
	require.NoError(t, f.repo.UpdateTaskSession(t.Context(), parent))
	now := time.Now().UTC()
	require.NoError(t, f.repo.CreateTurn(t.Context(), &models.Turn{ID: "parent-A", TaskID: f.taskID, TaskSessionID: f.parent, StartedAt: now}))
	seedExecutorRunning(t, f.repo, f.parent, f.taskID, "parent-execution")
	_, err = f.svc.messageQueue.QueueMessage(t.Context(), f.parent, f.taskID, "parent successor", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	f.svc.handleAgentReady(t.Context(), watcher.AgentEventData{TaskID: f.taskID, SessionID: f.parent, AgentExecutionID: "parent-execution", TurnID: "parent-A"})
	f.awaitDispatch(t)
	f.taskRepo.mu.Lock()
	history := append([]v1.TaskState(nil), f.taskRepo.stateHistory[f.taskID]...)
	f.taskRepo.mu.Unlock()
	require.NotContains(t, history, v1.TaskStateReview, "known outstanding decision must survive the real queue drain")
}

// @covers AC-TASKS-ERRANDS-003.4 AC-TASKS-ERRANDS-005.1
func TestErrandReadyQueuedParentTurnSupersedesCapturedResult(t *testing.T) {
	f := newQueueSettlementFixture(t, true)
	f.ready(t)
	f.awaitDispatch(t)
	row, err := f.repo.GetErrand(t.Context(), f.child.ID)
	require.NoError(t, err)
	require.Equal(t, "canonical result A", row.ResultText)
	require.Equal(t, "settlement-A", row.LatestSettledTurnID)
	require.Equal(t, models.ErrandResultSuperseded, row.ResultState)
	require.True(t, row.AwaitingResult)
	require.EqualValues(t, 1, row.ParentAdmissionGeneration)
	require.Zero(t, f.taskRepo.stateWrites[f.taskID])
}
