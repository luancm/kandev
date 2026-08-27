package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type pendingMoveEntryStore struct {
	mu      sync.Mutex
	entries map[string]*workflowmove.Entry
}

type workflowMovePhaseClaim struct {
	expected workflowmove.EntryPhase
	next     workflowmove.EntryPhase
	target   string
}

type recordingWorkflowMoveStore struct {
	workflowmove.LifecycleStore

	mu            sync.Mutex
	claims        []workflowMovePhaseClaim
	finalizeCalls int
}

func (s *recordingWorkflowMoveStore) ClaimPhase(
	ctx context.Context,
	id string,
	expected workflowmove.EntryPhase,
	next workflowmove.EntryPhase,
	targetSessionID string,
) (bool, error) {
	claimed, err := s.LifecycleStore.ClaimPhase(ctx, id, expected, next, targetSessionID)
	if claimed {
		s.mu.Lock()
		s.claims = append(s.claims, workflowMovePhaseClaim{
			expected: expected,
			next:     next,
			target:   targetSessionID,
		})
		s.mu.Unlock()
	}
	return claimed, err
}

func (s *recordingWorkflowMoveStore) Finalize(ctx context.Context, taskID, moveID string) error {
	s.mu.Lock()
	s.finalizeCalls++
	s.mu.Unlock()
	return s.LifecycleStore.Finalize(ctx, taskID, moveID)
}

func (s *recordingWorkflowMoveStore) successfulClaims() []workflowMovePhaseClaim {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]workflowMovePhaseClaim(nil), s.claims...)
}

func (s *recordingWorkflowMoveStore) finalizeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finalizeCalls
}

type failingPendingMoveStepGetter struct {
	WorkflowStepGetter
	targetID string
	err      error
}

type retryingPendingMoveStepGetter struct {
	WorkflowStepGetter
	targetID  string
	failures  int
	mu        sync.Mutex
	calls     int
	lookupErr error
}

type retryingPendingMoveCommitter struct {
	sessionExecutorStore
	delegate pendingWorkflowMoveCommitter
	err      error
	failures int
	mu       sync.Mutex
	calls    int
}

func (r *retryingPendingMoveCommitter) CommitPendingWorkflowMove(
	ctx context.Context,
	sessionID, moveID, taskID, fromWorkflowID, fromStepID, workflowID, workflowStepID string,
	limit int,
	state *v1.TaskState,
) (*models.Task, *workflowmove.EntryOptions, bool, bool, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call <= r.failures {
		return nil, nil, false, false, r.err
	}
	return r.delegate.CommitPendingWorkflowMove(
		ctx, sessionID, moveID, taskID, fromWorkflowID, fromStepID,
		workflowID, workflowStepID, limit, state,
	)
}

func (r *retryingPendingMoveCommitter) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *retryingPendingMoveCommitter) WorkflowMoveTransactionOwner() *sql.DB {
	owner, _ := r.sessionExecutorStore.(workflowMoveTaskTransactionOwner)
	if owner == nil {
		return nil
	}
	return owner.WorkflowMoveTransactionOwner()
}

type retryingPendingMoveQueueRepository struct {
	messagequeue.Repository
	err      error
	failures int
	mu       sync.Mutex
	calls    int
}

func (r *retryingPendingMoveQueueRepository) GetPendingMove(ctx context.Context, sessionID string) (*messagequeue.PendingMove, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call <= r.failures {
		return nil, r.err
	}
	return r.Repository.GetPendingMove(ctx, sessionID)
}

func (*retryingPendingMoveQueueRepository) UsesTaskTransactionHandoff() {}

func (r *retryingPendingMoveQueueRepository) WorkflowMoveTransactionOwner() *sql.DB {
	owner, _ := r.Repository.(interface{ WorkflowMoveTransactionOwner() *sql.DB })
	if owner == nil {
		return nil
	}
	return owner.WorkflowMoveTransactionOwner()
}

func (r *retryingPendingMoveQueueRepository) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (g *retryingPendingMoveStepGetter) GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if stepID != g.targetID {
		return g.WorkflowStepGetter.GetStep(ctx, stepID)
	}
	g.mu.Lock()
	g.calls++
	call := g.calls
	g.mu.Unlock()
	if call <= g.failures {
		return nil, g.lookupErr
	}
	return g.WorkflowStepGetter.GetStep(ctx, stepID)
}

func (g *retryingPendingMoveStepGetter) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

type failingSessionTransferRepository struct {
	messagequeue.Repository
	err error
}

func (r failingSessionTransferRepository) TransferSession(context.Context, string, string) error {
	return r.err
}

func (g *failingPendingMoveStepGetter) GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if stepID == g.targetID {
		return nil, g.err
	}
	return g.WorkflowStepGetter.GetStep(ctx, stepID)
}

func newPendingMoveEntryStore() *pendingMoveEntryStore {
	return &pendingMoveEntryStore{entries: make(map[string]*workflowmove.Entry)}
}

func (s *pendingMoveEntryStore) Save(_ context.Context, entry *workflowmove.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *entry
	s.entries[entry.ID] = &copy
	return nil
}

func (s *pendingMoveEntryStore) Load(_ context.Context, id string) (*workflowmove.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[id]
	if entry == nil {
		return nil, nil
	}
	copy := *entry
	return &copy, nil
}

func (s *pendingMoveEntryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[id]; !ok {
		return workflowmove.ErrEntryNotFound
	}
	delete(s.entries, id)
	return nil
}

// TestPendingMove_ReviewToInProgress_OneTransitionOnly reproduces the production bug
// observed at task a99d863e ("buggy fibo"): a QA agent calls move_task_kandev to send
// the task back to "In Progress" with a hand-off prompt, but the deferred-move flow
// triggers spurious additional transitions and the task ends up at "Reviewed" instead.
//
// Workflow (simplified, matches the user's actual setup):
//
//	[In Progress] --on_turn_complete-->  [In Review] --on_turn_complete-->  [Reviewed]
//	  on_enter: auto_start_agent              on_enter: auto_start_agent
//	  profile-impl                            profile-review
//
// Both on_turn_complete rules are unconditional — any agent.ready event triggers
// a transition. That's the workflow author's choice, but the orchestrator must
// not feed it spurious ready events. The deferred-move feature must produce
// exactly one transition: "In Review" → "In Progress". Anything else (e.g.
// "In Progress" → "In Review", or worse, "In Review" → "Reviewed" via a stale
// ready) is the bug.
//
// Scenario the test sets up:
//   - Task is currently at "In Review" (the QA step).
//   - Two sessions exist: an "In Progress" session (profile-impl, completed earlier
//     when the workflow first transitioned to Review) and an "In Review" session
//     (profile-review, currently RUNNING, primary).
//   - QA called move_task_kandev mid-turn → handleMoveTask set a PendingMove
//     pointing at "In Progress" and queued the legacy hand-off prompt.
//   - QA's turn ends → agent.ready fires → handleAgentReady is invoked.
//
// Expected outcome:
//   - Task workflow_step_id == "In Progress" step ID.
//   - The "In Progress" session is the primary (revived from COMPLETED).
//   - The "In Review" session is COMPLETED.
//   - No subsequent transition fires.
//
// The test deliberately stubs PromptAgent / LaunchAgent so we don't need a real
// agent process. The bug we're chasing is in the orchestrator's transition
// logic, not in the executor — so an executor that returns success deterministically
// is sufficient to expose multiple transitions if they occur.
func TestPendingMove_ReviewToInProgress_OneTransitionOnly(t *testing.T) {
	sc := buildPendingMoveScenario(t)

	// Snapshot the workflow_step_id history by sampling at intervals. We expect
	// exactly one change: stepInReviewID → stepInProgressID. Anything else
	// (e.g. stepInProgressID → stepInReviewID right after, or skipping ahead
	// to stepReviewedID) means the bug has fired.
	historyDone, stepHistory := sc.startStepHistorySampler(t, 2*time.Second)

	// Fire the QA session's agent.ready — this is what handleAgentReady receives
	// when MarkReady is called from handleCompleteEventMarkState after the QA
	// agent's turn ends.
	sc.svc.handleAgentReady(sc.ctx, watcher.AgentEventData{
		TaskID:           "task-1",
		SessionID:        sc.reviewSessionID,
		AgentExecutionID: "ae-review",
		AgentProfileID:   profileReview,
	})

	// Give the async processStepExitAndEnter goroutine time to complete.
	// Then drain the history collector.
	time.Sleep(1 * time.Second)
	<-historyDone

	sc.assertOneTransitionToInProgress(t, *stepHistory)
}

func TestPendingMove_OutOfTerminalStepReopensCompletedTask(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	sc.stepGetter.steps[stepReviewedID].Name = "Done"

	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = stepReviewedID
	task.State = v1.TaskStateCompleted
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatalf("seed terminal task state: %v", err)
	}

	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
	})

	task, err = sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load moved task: %v", err)
	}
	if task.WorkflowStepID != stepInProgressID {
		t.Fatalf("workflow_step_id = %q, want %q", task.WorkflowStepID, stepInProgressID)
	}
	if task.State != v1.TaskStateTODO {
		t.Fatalf("state = %q, want TODO after pending move out of terminal step", task.State)
	}

	// This is the real production applyPendingMove path (spec.md:518-519):
	// unlike TestApplyTransitionPreservesOuterCallerTrigger, which presets
	// mcp_deferred_move on ctx and so never reaches this call site at all,
	// this drives the actual deferred-move flow end to end and must prove
	// the ledger row it writes carries the trigger the scenario claims.
	rows := stepTransitionRowsForTaskOrchestrator(t, sc.repo, "task-1")
	if len(rows) == 0 {
		t.Fatalf("expected at least one ledger row for task-1")
	}
	last := rows[len(rows)-1]
	if last.trigger != string(steptelemetry.TriggerMCPDeferredMove) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerMCPDeferredMove)
	}
	if last.actorKind != string(steptelemetry.ActorSystem) {
		t.Fatalf("actor_kind = %q, want %q when sender session is absent", last.actorKind, steptelemetry.ActorSystem)
	}
	if last.actorID != nil || last.sessionID != nil {
		t.Fatalf("actor/session IDs = %v/%v, want NULL/NULL when sender session is absent", last.actorID, last.sessionID)
	}
}

func TestPendingMove_DoesNotReplayAfterStaleSnapshotRestored(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	queuedAt := time.Date(2026, 8, 17, 2, 23, 48, 0, time.UTC)
	firstMove := &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
		QueuedAt:       queuedAt,
	}

	// Consume the deferred move once, as handleAgentReady does at turn end.
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, firstMove)
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task after first move: %v", err)
	}
	if task.WorkflowStepID != stepInProgressID {
		t.Fatalf("workflow_step_id after first move = %q, want %q", task.WorkflowStepID, stepInProgressID)
	}

	// Simulate a legitimate later return to Review, followed by a stale queue
	// rollback restoring the original already-consumed legacy pending move. The
	// restored row predates move_id, so applyPendingMove must derive the same
	// stable identity from the persisted legacy fields instead of relying on the
	// mutated firstMove pointer above.
	task.WorkflowStepID = stepInReviewID
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatalf("return task to review: %v", err)
	}
	staleRestoredMove := &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
		QueuedAt:       queuedAt,
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, staleRestoredMove)

	task, err = sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task after stale replay: %v", err)
	}
	if task.WorkflowStepID != stepInReviewID {
		t.Fatalf("workflow_step_id after stale replay = %q, want %q", task.WorkflowStepID, stepInReviewID)
	}
}

func TestPendingMove_EqualTargetRecordsAppliedMoveID(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = stepInProgressID
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatalf("move task to target step: %v", err)
	}

	const moveID = "move-equal-target"
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		MoveID:         moveID,
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
	})

	updated, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	applied, ok := updated.Metadata[models.MetaKeyAppliedDeferredMoves].(map[string]interface{})
	if !ok || applied[moveID] != true {
		t.Fatalf("applied deferred moves = %#v, want %q", updated.Metadata[models.MetaKeyAppliedDeferredMoves], moveID)
	}
}

func TestPendingMove_DuplicateRemovesOnlyMatchingHandoffPrompt(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}

	const moveID = "move-duplicate"
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.Metadata = map[string]interface{}{
		models.MetaKeyAppliedDeferredMoves: map[string]interface{}{moveID: true},
	}
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatalf("mark deferred move as applied: %v", err)
	}
	if _, ok := sc.svc.messageQueue.TakeQueued(sc.ctx, sc.reviewSessionID); !ok {
		t.Fatalf("remove scenario handoff prompt")
	}

	if _, err := sc.svc.messageQueue.QueueMessageWithMetadata(
		sc.ctx, sc.reviewSessionID, "task-1", "stale handoff", "",
		messagequeue.QueuedByMoveTask, false, nil,
		map[string]interface{}{messagequeue.MetadataDeferredMoveID: moveID},
	); err != nil {
		t.Fatalf("queue stale handoff: %v", err)
	}
	if _, err := sc.svc.messageQueue.QueueMessage(
		sc.ctx, sc.reviewSessionID, "task-1", "unrelated prompt", "",
		messagequeue.QueuedByUser, false, nil,
	); err != nil {
		t.Fatalf("queue unrelated prompt: %v", err)
	}

	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		MoveID:         moveID,
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
	})

	status := sc.svc.messageQueue.GetStatus(sc.ctx, sc.reviewSessionID)
	if len(status.Entries) != 1 || status.Entries[0].Content != "unrelated prompt" {
		t.Fatalf("remaining queue entries = %#v, want only unrelated prompt", status.Entries)
	}
}

func TestPendingMove_DropsForeignWorkflowStepWithoutMovingTask(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	sc.stepGetter.steps["foreign-step"] = &wfmodels.WorkflowStep{
		ID:         "foreign-step",
		WorkflowID: "wf-other",
		Name:       "Foreign",
		Position:   1,
	}

	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: "foreign-step",
	})

	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.WorkflowStepID != stepInReviewID {
		t.Fatalf("workflow_step_id = %q, want unchanged %q", task.WorkflowStepID, stepInReviewID)
	}
	session, err = sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("reload review session: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want unchanged RUNNING", session.State)
	}

	// Regression: the workflow-mismatch drop must clean up the legacy hand-off
	// prompt queued before EntryOptions was introduced. Without this cleanup,
	// that prompt (authored for the foreign-workflow target step) would still be
	// sitting in the queue and could be misdelivered on a future turn.
	if status := sc.svc.messageQueue.GetStatus(sc.ctx, sc.reviewSessionID); status.Count != 0 {
		t.Fatalf("queued message count = %d, want 0 after workflow-mismatch drop", status.Count)
	}
}

func TestPendingMoveWithEntryOptionsPreservesUnrelatedQueuedMessageOnFailure(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	sc.stepGetter.steps["foreign-step"] = &wfmodels.WorkflowStep{
		ID: "foreign-step", WorkflowID: "wf-other", Name: "Foreign", Position: 1,
	}
	if _, err := sc.svc.messageQueue.CancelAll(sc.ctx, sc.reviewSessionID); err != nil {
		t.Fatalf("clear seeded queue: %v", err)
	}
	if _, err := sc.svc.messageQueue.QueueMessage(sc.ctx, sc.reviewSessionID, "task-1", "user follow-up", "", messagequeue.QueuedByUser, false, nil); err != nil {
		t.Fatalf("queue unrelated message: %v", err)
	}

	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: "foreign-step",
		EntryOptions:   &workflowmove.EntryOptions{Instructions: "handoff"},
	})

	status := sc.svc.messageQueue.GetStatus(sc.ctx, sc.reviewSessionID)
	if status.Count != 1 || status.Entries[0].Content != "user follow-up" {
		t.Fatalf("queue after failed move = %+v, want unrelated message preserved", status.Entries)
	}
}

func TestPendingMoveWithEntryOptionsSurvivesWIPQueueUntilPromotion(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	sc.stepGetter.steps[stepInProgressID].AgentProfileID = profileReview
	sc.stepGetter.steps[stepInProgressID].WIPLimit = 1
	entryStore := newPendingMoveEntryStore()
	sc.svc.SetMoveEntryStore(entryStore)
	sc.agentMgr.isAgentRunning = true
	sc.agentMgr.isAgentReadyFn = func(context.Context, string) bool { return true }
	messages := &mockMessageCreator{}
	sc.svc.messageCreator = messages
	seedExecutorRunning(t, sc.repo, sc.reviewSessionID, "task-1", "ae-review")

	if err := sc.repo.CreateTask(sc.ctx, &models.Task{
		ID: "progress-occupant", WorkspaceID: "ws1", WorkflowID: "wf1",
		WorkflowStepID: stepInProgressID, State: v1.TaskStateTODO, WIPAdmitted: true,
	}); err != nil {
		t.Fatalf("create WIP occupant: %v", err)
	}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load source session: %v", err)
	}
	moveID := "deferred-options-move"
	options := &workflowmove.EntryOptions{
		ResetContext:   true,
		Instructions:   "Create the PR ready for review, not as a draft.",
		AgentProfileID: profileReview,
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
		MoveID:         moveID,
		EntryOptions:   options,
	})

	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("reload queued task: %v", err)
	}
	if task.WorkflowStepID != stepInProgressID || task.WIPAdmitted || task.QueuedForStepID != stepInProgressID {
		t.Fatalf("queued task placement = step=%q admitted=%v queue=%q", task.WorkflowStepID, task.WIPAdmitted, task.QueuedForStepID)
	}
	if got := queuedMoveEntryID(task); got != moveID {
		t.Fatalf("queued move id = %q, want %q", got, moveID)
	}
	if entry, loadErr := entryStore.Load(sc.ctx, moveID); loadErr != nil || entry == nil || entry.Options != *options {
		t.Fatalf("private entry = %+v err=%v, want options retained while queued", entry, loadErr)
	}

	// The source-exit barrier runs asynchronously after the transition. Wait
	// for its durable completion before making the destination appear admitted.
	deadline := time.Now().Add(time.Second)
	for {
		task, err = sc.repo.GetTask(sc.ctx, "task-1")
		if err != nil {
			t.Fatalf("reload source-exit state: %v", err)
		}
		if queuedMoveExitCompleted(task) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("queued source-exit barrier did not complete: %#v", task.Metadata)
		}
		time.Sleep(time.Millisecond)
	}
	// Reload after the asynchronous source-exit barrier so the promotion write
	// preserves its durable completion marker instead of overwriting it with
	// the pre-barrier task snapshot.
	task, err = sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("reload task before promotion: %v", err)
	}

	// Duplicate task.moved deliveries for a queued move must not consume the
	// private options or run destination entry before WIP admission. The source
	// exit has already completed, so these are real replay deliveries rather
	// than a second attempt to perform the source barrier.
	queuedMoveEvent := watcher.TaskMovedEventData{
		TaskID:          "task-1",
		FromStepID:      stepInReviewID,
		ToStepID:        stepInProgressID,
		SessionID:       sc.reviewSessionID,
		WorkflowID:      "wf1",
		MoveID:          moveID,
		WIPAdmitted:     false,
		QueuedForStepID: stepInProgressID,
	}
	sc.svc.handleTaskMoved(sc.ctx, queuedMoveEvent)
	sc.svc.handleTaskMoved(sc.ctx, queuedMoveEvent)
	if entry, loadErr := entryStore.Load(sc.ctx, moveID); loadErr != nil || entry == nil || entry.Options != *options {
		t.Fatalf("private entry after duplicate queued deliveries = %+v err=%v, want retained options", entry, loadErr)
	}
	if got := len(capturedPromptsForExecution(sc.agentMgr, "ae-review")); got != 0 {
		t.Fatalf("prompts before WIP promotion = %d, want 0", got)
	}

	// Promotion is normally performed by the WIP reconciler. This simulates
	// the durable promotion write and then delivers its event twice. Both
	// deliveries must reuse the same private move row, while only one claims
	// destination entry.
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.QueuedAt = nil
	if task.Metadata == nil {
		task.Metadata = make(map[string]interface{})
	}
	task.Metadata[models.MetaKeyQueuePromotionPending] = true
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatalf("persist simulated promotion: %v", err)
	}
	entryCompleted := make(chan struct{})
	var entryCompleteOnce sync.Once
	var entryCallsMu sync.Mutex
	entryCalls := 0
	sc.svc.onTaskQueuePromotionEntryComplete = func() {
		entryCallsMu.Lock()
		entryCalls++
		entryCallsMu.Unlock()
		entryCompleteOnce.Do(func() { close(entryCompleted) })
	}
	sc.svc.handleTaskQueuePromoted(sc.ctx, watcher.TaskEventData{TaskID: "task-1"})
	sc.svc.handleTaskQueuePromoted(sc.ctx, watcher.TaskEventData{TaskID: "task-1"})
	select {
	case <-entryCompleted:
	case <-time.After(2 * time.Second):
		t.Fatal("destination entry did not complete after queue promotion")
	}
	if entry, loadErr := entryStore.Load(sc.ctx, moveID); loadErr != nil || entry != nil {
		t.Fatalf("private entry after promotion = %+v err=%v, want consumed", entry, loadErr)
	}
	entryCallsMu.Lock()
	gotEntryCalls := entryCalls
	entryCallsMu.Unlock()
	if gotEntryCalls != 1 {
		t.Fatalf("destination entry calls = %d, want 1", gotEntryCalls)
	}
	if got := len(sc.agentMgr.capturedPrompts); got != 1 {
		t.Fatalf("prompts after duplicate promotion = %d, want 1", got)
	}
	if got := len(messages.userMessages); got != 1 {
		t.Fatalf("user messages after duplicate promotion = %d, want 1", got)
	}
	latest, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("reload promoted task: %v", err)
	}
	if _, pending := latest.Metadata[models.MetaKeyWorkflowMovePending]; pending {
		t.Fatalf("workflow move marker remained after destination entry: %#v", latest.Metadata)
	}
}

func TestStartupReconcilesPendingMoveAfterRestart(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	entryStore := newPendingMoveEntryStore()
	sc.svc.SetMoveEntryStore(entryStore)
	move := &messagequeue.PendingMove{
		MoveID:          "restart-pending-move",
		TaskID:          "task-1",
		WorkflowID:      "wf1",
		WorkflowStepID:  stepInProgressID,
		FromWorkflowID:  "wf1",
		FromStepID:      stepInReviewID,
		SenderSessionID: sc.reviewSessionID,
		EntryOptions: &workflowmove.EntryOptions{
			ResetContext:   true,
			Instructions:   "Preserve this restart handoff.",
			AgentProfileID: profileImpl,
		},
	}
	sc.svc.messageQueue.SetPendingMove(sc.ctx, sc.reviewSessionID, move)
	seedExecutorRunning(t, sc.repo, sc.reviewSessionID, "task-1", "ae-review")

	sc.svc.reconcileExecutorSessionsOnStartup(sc.ctx)
	sc.svc.reconcilePendingMovesOnStartup(sc.ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := sc.repo.GetTask(sc.ctx, "task-1")
		if err != nil {
			t.Fatalf("reload task after startup recovery: %v", err)
		}
		if task.WorkflowStepID == stepInProgressID {
			return
		}
		time.Sleep(time.Millisecond)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("reload task after startup recovery timeout: %v", err)
	}
	t.Fatalf("workflow step after startup recovery = %q, want %q", task.WorkflowStepID, stepInProgressID)
}

func TestWorkflowMoveDuplicateTaskMovedDeliveryAppliesOnce(t *testing.T) {
	sc, entryStore, messages, moveID := prepareDuplicateWorkflowMoveScenario(t)
	event := watcher.TaskMovedEventData{
		TaskID:          "task-1",
		FromStepID:      stepInReviewID,
		ToStepID:        stepInProgressID,
		SessionID:       sc.reviewSessionID,
		WorkflowID:      "wf1",
		MoveID:          moveID,
		WIPAdmitted:     true,
		TaskDescription: "duplicate move delivery",
	}
	sc.svc.handleTaskMoved(sc.ctx, event)
	sc.svc.handleTaskMoved(sc.ctx, event)

	assertWorkflowMoveConsumedOnce(t, sc, entryStore, messages, moveID)
}

func TestWorkflowMoveDuplicateRecoveryDeliveryAppliesOnce(t *testing.T) {
	sc, entryStore, messages, moveID := prepareDuplicateWorkflowMoveScenario(t)
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load recovery task: %v", err)
	}
	sc.svc.recoverWorkflowMoveEntry(sc.ctx, task)
	sc.svc.recoverWorkflowMoveEntry(sc.ctx, task)

	assertWorkflowMoveConsumedOnce(t, sc, entryStore, messages, moveID)
}

func TestPendingMoveDuplicateAgentReadyAppliesOnce(t *testing.T) {
	sc, _ := buildRetryablePendingMoveScenario(t)
	sc.stepGetter.steps[stepInProgressID].AgentProfileID = profileReview

	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	baseStore, err := workflowmove.NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	lifecycleStore, ok := baseStore.(workflowmove.LifecycleStore)
	if !ok {
		t.Fatal("SQLite move entry store does not implement LifecycleStore")
	}
	entryStore := &recordingWorkflowMoveStore{LifecycleStore: lifecycleStore}
	sc.svc.SetMoveEntryStore(entryStore)
	sc.agentMgr.isAgentRunning = true
	sc.agentMgr.isAgentReadyFn = func(context.Context, string) bool { return true }
	seedExecutorRunning(t, sc.repo, sc.reviewSessionID, "task-1", "ae-review")
	messages := &mockMessageCreator{}
	sc.svc.messageCreator = messages

	const moveID = "duplicate-ready-move"
	move := &messagequeue.PendingMove{
		MoveID:         moveID,
		TaskID:         "task-1",
		FromWorkflowID: "wf1",
		FromStepID:     stepInReviewID,
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
		EntryOptions: &workflowmove.EntryOptions{
			ResetContext:   true,
			Instructions:   "Preserve this ready-signal handoff.",
			AgentProfileID: profileReview,
		},
	}
	admitted, err := sc.svc.messageQueue.AdmitPendingMove(sc.ctx, sc.reviewSessionID, move)
	if err != nil || !admitted {
		t.Fatalf("AdmitPendingMove = (%v, %v), want (true, nil)", admitted, err)
	}
	ready := watcher.AgentEventData{
		TaskID:           "task-1",
		SessionID:        sc.reviewSessionID,
		AgentExecutionID: "ae-review",
		AgentProfileID:   profileReview,
	}
	sc.svc.handleAgentReady(sc.ctx, ready)
	sc.svc.handleAgentReady(sc.ctx, ready)

	assertWorkflowMoveConsumedOnce(t, sc, entryStore, messages, moveID)
}

func TestPendingMovePersistentHandoffCommitsTaskEntryAndClaimAtomically(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	sc.stepGetter.steps[stepInProgressID].AgentProfileID = profileReview
	sc.stepGetter.steps[stepInProgressID].WIPLimit = 1
	if _, err := sc.repo.DB().Exec(`ALTER TABLE workflow_steps ADD COLUMN wip_limit INTEGER NOT NULL DEFAULT 0`); err != nil {
		t.Fatalf("add authoritative WIP column: %v", err)
	}
	for position, stepID := range []string{stepInProgressID, stepInReviewID, stepReviewedID} {
		if _, err := sc.repo.DB().Exec(`INSERT OR IGNORE INTO workflow_steps (id, workflow_id, name, position) VALUES (?, 'wf1', ?, ?)`, stepID, stepID, position); err != nil {
			t.Fatalf("seed authoritative workflow step: %v", err)
		}
	}
	if _, err := sc.repo.DB().Exec(`UPDATE workflow_steps SET wip_limit = 1 WHERE id = ?`, stepInProgressID); err != nil {
		t.Fatalf("persist authoritative WIP limit: %v", err)
	}
	if err := sc.repo.CreateTask(sc.ctx, &models.Task{
		ID: "persistent-progress-occupant", WorkspaceID: "ws1", WorkflowID: "wf1",
		WorkflowStepID: stepInProgressID, State: v1.TaskStateTODO, WIPAdmitted: true,
	}); err != nil {
		t.Fatalf("create WIP occupant: %v", err)
	}
	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	sc.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	entryStore, err := workflowmove.NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	sc.svc.SetMoveEntryStore(entryStore)
	sc.agentMgr.resolveProfileInfo = &executor.AgentProfileInfo{
		ProfileID: profileReview, AgentID: "agent-review", Model: "model-qa",
	}
	move := &messagequeue.PendingMove{
		MoveID: "persistent-deferred-move", TaskID: "task-1",
		FromWorkflowID: "wf1", FromStepID: stepInReviewID,
		WorkflowID: "wf1", WorkflowStepID: stepInProgressID,
		EntryOptions: &workflowmove.EntryOptions{
			ResetContext: true, Instructions: "handoff", AgentProfileID: profileReview,
		},
	}
	admitted, err := sc.svc.messageQueue.AdmitPendingMove(sc.ctx, sc.reviewSessionID, move)
	if err != nil || !admitted {
		t.Fatalf("AdmitPendingMove = (%v, %v), want (true, nil)", admitted, err)
	}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)

	pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID)
	if err != nil || pending != nil {
		t.Fatalf("pending claim after handoff = (%#v, %v), want nil", pending, err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.WorkflowStepID != stepInProgressID || task.WIPAdmitted || task.QueuedForStepID != stepInProgressID {
		t.Fatalf("committed task placement = step=%q admitted=%v queued=%q", task.WorkflowStepID, task.WIPAdmitted, task.QueuedForStepID)
	}
	if got := queuedMoveEntryID(task); got != move.MoveID {
		t.Fatalf("committed move marker = %q, want %q", got, move.MoveID)
	}
	entry, err := entryStore.Load(sc.ctx, move.MoveID)
	if err != nil || entry == nil || entry.Options != *move.EntryOptions {
		t.Fatalf("committed private entry = (%#v, %v)", entry, err)
	}
}

func TestPendingMovePersistentHandoffDoesNotMarkSessionWaitingBeforeCommit(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	sc.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		MoveID: "move-not-admitted", TaskID: "task-1",
		FromWorkflowID: "wf1", FromStepID: stepInReviewID,
		WorkflowID: "wf1", WorkflowStepID: stepInProgressID,
	})
	stored, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("GetTaskSession after rejected commit: %v", err)
	}
	if stored.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING until pending move commits", stored.State)
	}
}

func TestPendingMovePersistentHandoffDropsExactPermanentlyInvalidTarget(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	sc.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	move := &messagequeue.PendingMove{
		MoveID: "move-invalid-target", TaskID: "task-1", FromWorkflowID: "wf1", FromStepID: stepInReviewID,
		WorkflowID: "wf1", WorkflowStepID: "missing-step",
	}
	if admitted, err := sc.svc.messageQueue.AdmitPendingMove(sc.ctx, sc.reviewSessionID, move); err != nil || !admitted {
		t.Fatalf("AdmitPendingMove = (%v, %v), want (true, nil)", admitted, err)
	}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)
	if pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID); err != nil || pending != nil {
		t.Fatalf("pending invalid move = (%#v, %v), want nil", pending, err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil || task.WorkflowStepID != stepInReviewID {
		t.Fatalf("task after invalid move = (%#v, %v), want source step", task, err)
	}
}

func TestPendingMovePersistentHandoffRetainsTargetLookupFailure(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	sc.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	sc.svc.workflowStepGetter = &failingPendingMoveStepGetter{
		WorkflowStepGetter: sc.svc.workflowStepGetter,
		targetID:           stepInProgressID,
		err:                errors.New("temporary target lookup failure"),
	}
	move := &messagequeue.PendingMove{
		MoveID: "move-target-retry", TaskID: "task-1", FromWorkflowID: "wf1", FromStepID: stepInReviewID,
		WorkflowID: "wf1", WorkflowStepID: stepInProgressID,
	}
	if admitted, err := sc.svc.messageQueue.AdmitPendingMove(sc.ctx, sc.reviewSessionID, move); err != nil || !admitted {
		t.Fatalf("AdmitPendingMove = (%v, %v), want (true, nil)", admitted, err)
	}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)
	if pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID); err != nil || pending == nil || pending.MoveID != move.MoveID {
		t.Fatalf("pending retryable move = (%#v, %v), want retained exact row", pending, err)
	}
	stored, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil || stored.State != models.TaskSessionStateRunning {
		t.Fatalf("session after lookup failure = (%#v, %v), want RUNNING", stored, err)
	}
}

func TestPendingMoveRetryableLookupReconcilesWithoutAnotherAgentReady(t *testing.T) {
	sc, queueRepo := buildRetryablePendingMoveScenario(t)
	done := make(chan struct{})
	sc.svc.pendingMoveReconciliationWait = func(context.Context) bool { return true }
	sc.svc.onPendingMoveReconciliationComplete = func() { close(done) }
	getter := &retryingPendingMoveStepGetter{
		WorkflowStepGetter: sc.svc.workflowStepGetter,
		targetID:           stepInProgressID,
		failures:           1,
		lookupErr:          errors.New("temporary target lookup failure"),
	}
	sc.svc.workflowStepGetter = getter
	move := admitRetryablePendingMove(t, sc)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pending move reconciliation did not complete")
	}
	if getter.callCount() < 2 {
		t.Fatalf("target lookup calls = %d, want scheduled reconciliation", getter.callCount())
	}
	if pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID); err != nil || pending != nil {
		t.Fatalf("pending move after successful reconciliation = (%#v, %v), want nil", pending, err)
	}
}

func TestPendingMoveRetryableProfileValidationReconcilesWithoutAnotherAgentReady(t *testing.T) {
	sc, queueRepo := buildRetryablePendingMoveScenario(t)
	done := make(chan struct{})
	sc.agentMgr.resolveProfileErr = errors.New("temporary profile lookup failure")
	sc.svc.pendingMoveReconciliationWait = func(context.Context) bool {
		sc.agentMgr.resolveProfileErr = nil
		sc.agentMgr.resolveProfileInfo = &executor.AgentProfileInfo{
			ProfileID: profileReview,
			AgentID:   "agent-review",
			Model:     "model-qa",
		}
		return true
	}
	sc.svc.onPendingMoveReconciliationComplete = func() { close(done) }
	move := &messagequeue.PendingMove{
		MoveID: "move-profile-reconcile", TaskID: "task-1", FromWorkflowID: "wf1", FromStepID: stepInReviewID,
		WorkflowID: "wf1", WorkflowStepID: stepInProgressID,
		EntryOptions: &workflowmove.EntryOptions{AgentProfileID: profileReview},
	}
	if admitted, err := sc.svc.messageQueue.AdmitPendingMove(sc.ctx, sc.reviewSessionID, move); err != nil || !admitted {
		t.Fatalf("AdmitPendingMove = (%v, %v), want admitted", admitted, err)
	}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pending move profile reconciliation did not complete")
	}
	if pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID); err != nil || pending != nil {
		t.Fatalf("pending move after successful profile reconciliation = (%#v, %v), want nil", pending, err)
	}
}

func TestPendingMoveRetryableCommitErrorReconcilesWithoutAnotherAgentReady(t *testing.T) {
	sc, queueRepo := buildRetryablePendingMoveScenario(t)
	done := make(chan struct{})
	sc.svc.pendingMoveReconciliationWait = func(context.Context) bool { return true }
	sc.svc.onPendingMoveReconciliationComplete = func() { close(done) }
	delegate := sc.svc.repo.(pendingWorkflowMoveCommitter)
	committer := &retryingPendingMoveCommitter{
		sessionExecutorStore: sc.svc.repo,
		delegate:             delegate,
		err:                  errors.New("temporary commit failure"),
		failures:             1,
	}
	sc.svc.repo = committer
	move := admitRetryablePendingMove(t, sc)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pending move commit reconciliation was not scheduled")
	}
	if got := committer.callCount(); got != 2 {
		t.Fatalf("commit attempts = %d, want initial failure plus one successful retry", got)
	}
	if pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID); err != nil || pending != nil {
		t.Fatalf("pending move after commit retry = (%#v, %v), want nil", pending, err)
	}
}

func TestPendingMoveReconciliationRetriesTransientPendingRowLoadError(t *testing.T) {
	sc, queueRepo := buildRetryablePendingMoveScenario(t)
	done := make(chan struct{})
	sc.svc.pendingMoveReconciliationWait = func(context.Context) bool { return true }
	sc.svc.onPendingMoveReconciliationComplete = func() { close(done) }
	getter := &retryingPendingMoveStepGetter{
		WorkflowStepGetter: sc.svc.workflowStepGetter,
		targetID:           stepInProgressID,
		failures:           1,
		lookupErr:          errors.New("temporary target lookup failure"),
	}
	sc.svc.workflowStepGetter = getter
	queueWithFailure := &retryingPendingMoveQueueRepository{
		Repository: queueRepo,
		err:        errors.New("temporary pending row load failure"),
		failures:   1,
	}
	sc.svc.messageQueue = messagequeue.NewService(queueWithFailure, messagequeue.DefaultMaxPerSession, testLogger())
	move := admitRetryablePendingMove(t, sc)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pending move reconciliation did not complete after transient row load failure")
	}
	if got := queueWithFailure.callCount(); got < 2 {
		t.Fatalf("pending row load attempts = %d, want retry after transient failure", got)
	}
	if pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID); err != nil || pending != nil {
		t.Fatalf("pending move after row-load retry = (%#v, %v), want nil", pending, err)
	}
}

// Reviewer-requested contract coverage: persistent row-load failures consume
// the same bounded budget and settle the source session without deleting the
// row whose identity could not be authoritatively read.
func TestPendingMoveReconciliationLoadErrorsExhaustAndSettleSession(t *testing.T) {
	sc, queueRepo := buildRetryablePendingMoveScenario(t)
	done := make(chan struct{})
	sc.svc.pendingMoveReconciliationWait = func(context.Context) bool { return true }
	sc.svc.onPendingMoveReconciliationComplete = func() { close(done) }
	getter := &retryingPendingMoveStepGetter{
		WorkflowStepGetter: sc.svc.workflowStepGetter,
		targetID:           stepInProgressID,
		failures:           1,
		lookupErr:          errors.New("temporary target lookup failure"),
	}
	sc.svc.workflowStepGetter = getter
	queueWithFailure := &retryingPendingMoveQueueRepository{
		Repository: queueRepo,
		err:        errors.New("persistent pending row load failure"),
		failures:   100,
	}
	sc.svc.messageQueue = messagequeue.NewService(queueWithFailure, messagequeue.DefaultMaxPerSession, testLogger())
	move := admitRetryablePendingMove(t, sc)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pending move reconciliation did not exhaust row-load retries")
	}
	if got := queueWithFailure.callCount(); got != pendingMoveReconciliationAttempts {
		t.Fatalf("pending row load attempts = %d, want bounded %d", got, pendingMoveReconciliationAttempts)
	}
	if pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID); err != nil || pending == nil {
		t.Fatalf("pending move after row-load exhaustion = (%#v, %v), want retained", pending, err)
	}
	stored, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil || stored.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session after row-load exhaustion = (%#v, %v), want WAITING_FOR_INPUT", stored, err)
	}
}

func TestPendingMoveReconciliationDoesNotMoveUnderSuccessorTurn(t *testing.T) {
	sc, queueRepo := buildRetryablePendingMoveScenario(t)
	done := make(chan struct{})
	turns := &repoTurnService{repo: sc.repo}
	sc.svc.turnService = turns
	sc.agentMgr.isPassthrough = true
	getter := &retryingPendingMoveStepGetter{
		WorkflowStepGetter: sc.svc.workflowStepGetter,
		targetID:           stepInProgressID,
		failures:           1,
		lookupErr:          errors.New("temporary target lookup failure"),
	}
	sc.svc.workflowStepGetter = getter
	var startErr error
	var startOnce sync.Once
	sc.svc.pendingMoveReconciliationWait = func(context.Context) bool {
		startOnce.Do(func() {
			_, startErr = turns.StartTurn(sc.ctx, sc.reviewSessionID)
		})
		return true
	}
	sc.svc.onPendingMoveReconciliationComplete = func() { close(done) }
	move := admitRetryablePendingMove(t, sc)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pending move reconciliation did not observe successor turn")
	}
	if startErr != nil {
		t.Fatalf("start successor turn: %v", startErr)
	}
	if pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID); err != nil || pending == nil {
		t.Fatalf("pending move under successor turn = (%#v, %v), want retained", pending, err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil || task.WorkflowStepID != stepInReviewID {
		t.Fatalf("task moved under successor turn = (%#v, %v), want source step", task, err)
	}
	stored, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil || stored.State != models.TaskSessionStateRunning {
		t.Fatalf("successor session was settled = (%#v, %v), want RUNNING", stored, err)
	}
}

func TestPendingMoveRetryableLookupExhaustionSettlesSession(t *testing.T) {
	sc, queueRepo := buildRetryablePendingMoveScenario(t)
	done := make(chan struct{})
	sc.svc.pendingMoveReconciliationWait = func(context.Context) bool { return true }
	sc.svc.onPendingMoveReconciliationComplete = func() { close(done) }
	getter := &retryingPendingMoveStepGetter{
		WorkflowStepGetter: sc.svc.workflowStepGetter,
		targetID:           stepInProgressID,
		failures:           100,
		lookupErr:          errors.New("persistent target lookup failure"),
	}
	sc.svc.workflowStepGetter = getter
	move := admitRetryablePendingMove(t, sc)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pending move reconciliation did not exhaust")
	}
	if got := getter.callCount(); got != 4 {
		t.Fatalf("target lookup calls after exhaustion = %d, want 4 bounded attempts", got)
	}
	if pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID); err != nil || pending == nil {
		t.Fatalf("pending move after exhausted reconciliation = (%#v, %v), want retained", pending, err)
	}
	stored, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil || stored.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session after exhausted reconciliation = (%#v, %v), want WAITING_FOR_INPUT", stored, err)
	}
}

func buildRetryablePendingMoveScenario(t *testing.T) (*pendingMoveScenario, messagequeue.Repository) {
	t.Helper()
	sc := buildPendingMoveScenario(t)
	if _, err := sc.repo.DB().Exec(`ALTER TABLE workflow_steps ADD COLUMN wip_limit INTEGER NOT NULL DEFAULT 0`); err != nil {
		t.Fatal(err)
	}
	for position, stepID := range []string{stepInProgressID, stepInReviewID, stepReviewedID} {
		if _, err := sc.repo.DB().Exec(`INSERT OR IGNORE INTO workflow_steps (id, workflow_id, name, position) VALUES (?, 'wf1', ?, ?)`, stepID, stepID, position); err != nil {
			t.Fatal(err)
		}
	}
	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	entryStore, err := workflowmove.NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.SetMoveEntryStore(entryStore)
	return sc, queueRepo
}

func admitRetryablePendingMove(t *testing.T, sc *pendingMoveScenario) *messagequeue.PendingMove {
	t.Helper()
	move := &messagequeue.PendingMove{
		MoveID: "move-target-reconcile", TaskID: "task-1", FromWorkflowID: "wf1", FromStepID: stepInReviewID,
		WorkflowID: "wf1", WorkflowStepID: stepInProgressID,
	}
	if admitted, err := sc.svc.messageQueue.AdmitPendingMove(sc.ctx, sc.reviewSessionID, move); err != nil || !admitted {
		t.Fatalf("AdmitPendingMove = (%v, %v), want admitted", admitted, err)
	}
	return move
}

func TestPendingMovePersistentHandoffRevalidatesOptionsBeforeCommit(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	sc.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	sc.agentMgr.resolveProfileErr = errors.New("profile was removed after admission")
	move := &messagequeue.PendingMove{
		MoveID: "move-revalidate-options", TaskID: "task-1", FromWorkflowID: "wf1", FromStepID: stepInReviewID,
		WorkflowID: "wf1", WorkflowStepID: stepInProgressID,
		EntryOptions: &workflowmove.EntryOptions{AgentProfileID: "profile-removed"},
	}
	if admitted, err := sc.svc.messageQueue.AdmitPendingMove(sc.ctx, sc.reviewSessionID, move); err != nil || !admitted {
		t.Fatalf("AdmitPendingMove = (%v, %v), want (true, nil)", admitted, err)
	}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)
	if pending, err := queueRepo.GetPendingMove(sc.ctx, sc.reviewSessionID); err != nil || pending == nil || pending.MoveID != move.MoveID {
		t.Fatalf("pending move after failed revalidation = (%#v, %v), want retained", pending, err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil || task.WorkflowStepID != stepInReviewID {
		t.Fatalf("task after failed revalidation = (%#v, %v), want source step", task, err)
	}
	stored, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil || stored.State != models.TaskSessionStateRunning {
		t.Fatalf("session after failed revalidation = (%#v, %v), want RUNNING", stored, err)
	}
}

func TestWorkflowMoveEntryLockIsReleasedFromRegistry(t *testing.T) {
	svc := &Service{}
	release := svc.lockWorkflowMoveEntry("move-lock-lifecycle")
	release()
	retained := 0
	svc.workflowMoveEntryLocks.Range(func(_, _ interface{}) bool {
		retained++
		return true
	})
	if retained != 0 {
		t.Fatalf("retained workflow move locks = %d, want 0", retained)
	}
}

func TestWorkflowMoveQueueOriginUsesDurableLifecycleDispatch(t *testing.T) {
	if !isLifecycleAutomationOrigin("workflow_move") {
		t.Fatal("workflow move origin is not recognized as durable lifecycle dispatch")
	}
}

func TestWorkflowMoveQueueAcknowledgementFinalizesEntryAndMarker(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	baseStore, err := workflowmove.NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	sc.svc.SetMoveEntryStore(baseStore)
	const moveID = "move-ack-finalizes"
	if err := baseStore.Save(sc.ctx, &workflowmove.Entry{ID: moveID, TaskID: "task-1", Options: workflowmove.EntryOptions{Instructions: "handoff"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := sc.repo.SetTaskMetadataKey(sc.ctx, "task-1", models.MetaKeyWorkflowMovePending, map[string]interface{}{
		"from_step_id": stepInReviewID, "move_id": moveID,
	}); err != nil {
		t.Fatalf("SetTaskMetadataKey: %v", err)
	}
	if !sc.svc.markWorkflowMoveDispatchReady(sc.ctx, moveID, sc.reviewSessionID) ||
		!sc.svc.claimWorkflowMoveDispatch(sc.ctx, moveID, sc.reviewSessionID) {
		t.Fatal("failed to prepare and claim workflow move prompt boundary")
	}
	metadata := map[string]interface{}{
		"origin":                            workflowMoveLifecycleOrigin,
		messagequeue.MetadataDeferredMoveID: moveID,
	}
	if _, err := sc.svc.messageQueue.CancelAll(sc.ctx, sc.reviewSessionID); err != nil {
		t.Fatalf("CancelAll: %v", err)
	}
	queued, _, accepted, err := sc.svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		sc.ctx, sc.reviewSessionID, "task-1", "handoff", "", messagequeue.QueuedByServer,
		false, nil, metadata, "workflow-move:"+moveID, true,
	)
	if err != nil || !accepted {
		t.Fatalf("QueueLifecycleMessageWithCoalesceKey = (%#v, %v, %v)", queued, accepted, err)
	}
	reserved, ok := sc.svc.messageQueue.ReserveQueued(sc.ctx, sc.reviewSessionID)
	if !ok {
		t.Fatal("ReserveQueued did not return workflow move prompt")
	}
	sc.svc.acknowledgeLifecycleQueueEntry(sc.ctx, sc.reviewSessionID, reserved)
	if entry, err := baseStore.Load(sc.ctx, moveID); err != nil || entry != nil {
		t.Fatalf("entry after acknowledgement = (%#v, %v), want nil", entry, err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if _, pending := task.Metadata[models.MetaKeyWorkflowMovePending]; pending {
		t.Fatalf("workflow move marker survived acknowledgement: %#v", task.Metadata)
	}
}

func TestWorkflowMoveQueuedPreDispatchClaimStaysReplayable(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	store, err := workflowmove.NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.SetMoveEntryStore(store)
	const moveID = "move-queued-pre-dispatch"
	if err := store.Save(sc.ctx, &workflowmove.Entry{ID: moveID, TaskID: "task-1", Options: workflowmove.EntryOptions{Instructions: "handoff"}}); err != nil {
		t.Fatal(err)
	}
	if !sc.svc.markWorkflowMoveDispatchReady(sc.ctx, moveID, sc.reviewSessionID) {
		t.Fatal("markWorkflowMoveDispatchReady failed")
	}
	queued := &messagequeue.QueuedMessage{
		SessionID: sc.reviewSessionID, TaskID: "task-1", Content: "handoff",
		Metadata: map[string]interface{}{
			"origin":                            workflowMoveLifecycleOrigin,
			messagequeue.MetadataDeferredMoveID: moveID,
		},
	}
	afterClaim := sc.svc.queuedLifecycleAfterClaim(sc.ctx, queued, nil, true)
	if err := afterClaim(); err != nil {
		t.Fatal(err)
	}
	entry, err := store.Load(sc.ctx, moveID)
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil || entry.Phase != workflowmove.EntryPhaseDispatchReady {
		t.Fatalf("phase before runtime acceptance = %#v, want dispatch_ready", entry)
	}
}

func TestWorkflowMovePassthroughFinalizesOnlyAfterPTYAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name      string
		writeErr  error
		wantPhase workflowmove.EntryPhase
		wantEntry bool
	}{
		{name: "accepted", wantEntry: false},
		{name: "rejected", writeErr: errors.New("pty rejected prompt"), wantPhase: workflowmove.EntryPhaseDispatchClaimed, wantEntry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := buildPendingMoveScenario(t)
			sc.agentMgr.isPassthrough = true
			sc.agentMgr.passthroughStdinErr = tc.writeErr
			db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
			store, err := workflowmove.NewSQLiteEntryStore(db, db)
			if err != nil {
				t.Fatal(err)
			}
			sc.svc.SetMoveEntryStore(store)
			moveID := "move-passthrough-" + tc.name
			options := &workflowmove.EntryOptions{Instructions: "handoff over PTY"}
			if err := store.Save(sc.ctx, &workflowmove.Entry{ID: moveID, TaskID: "task-1", Options: *options}); err != nil {
				t.Fatal(err)
			}
			if err := sc.repo.SetTaskMetadataKey(sc.ctx, "task-1", models.MetaKeyWorkflowMovePending, map[string]interface{}{
				"move_id": moveID, "from_step_id": stepInReviewID,
			}); err != nil {
				t.Fatal(err)
			}
			if !sc.svc.markWorkflowMoveDispatchReady(sc.ctx, moveID, sc.reviewSessionID) {
				t.Fatal("markWorkflowMoveDispatchReady failed")
			}
			session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
			if err != nil {
				t.Fatal(err)
			}
			sc.svc.autoStartPassthroughOnEnter(
				withWorkflowMoveEntryID(sc.ctx, moveID), "task-1", session,
				sc.stepGetter.steps[stepInReviewID], "task", moveID, options,
			)
			entry, err := store.Load(sc.ctx, moveID)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.wantEntry {
				if entry != nil {
					t.Fatalf("entry after PTY acceptance = %#v, want finalized", entry)
				}
				return
			}
			if entry == nil || entry.Phase != tc.wantPhase {
				t.Fatalf("entry after definite PTY rejection = %#v, want %s", entry, tc.wantPhase)
			}
		})
	}
}

func TestWorkflowMoveDispatchClaimAndFinalizationAreIdempotent(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	baseStore, err := workflowmove.NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	sc.svc.SetMoveEntryStore(baseStore)
	const moveID = "move-dispatch-once"
	if err := baseStore.Save(sc.ctx, &workflowmove.Entry{
		ID: moveID, TaskID: "task-1", Options: workflowmove.EntryOptions{ResetContext: true, Instructions: "handoff", AgentProfileID: profileReview},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := sc.repo.SetTaskMetadataKey(sc.ctx, "task-1", models.MetaKeyWorkflowMovePending, map[string]interface{}{
		"from_step_id": stepInReviewID, "move_id": moveID,
	}); err != nil {
		t.Fatalf("SetTaskMetadataKey: %v", err)
	}
	start := make(chan struct{})
	results := make(chan bool, 2)
	for range 2 {
		go func() {
			<-start
			results <- sc.svc.claimWorkflowMoveDispatch(sc.ctx, moveID, sc.reviewSessionID)
		}()
	}
	close(start)
	winners := 0
	for range 2 {
		if <-results {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("dispatch claim winners = %d, want 1", winners)
	}
	sc.svc.consumeMoveEntry(sc.ctx, moveID, sc.reviewSessionID)
	if entry, err := baseStore.Load(sc.ctx, moveID); err != nil || entry != nil {
		t.Fatalf("entry after finalization = (%#v, %v), want nil", entry, err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if _, pending := task.Metadata[models.MetaKeyWorkflowMovePending]; pending {
		t.Fatalf("workflow move marker survived finalization: %#v", task.Metadata)
	}
}

func TestWorkflowMoveSessionlessAutoStartWaitsForRuntimePromptAcceptance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskWithoutSession(t, repo, "task-sessionless-move", stepInProgressID)
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-sessionless-move"] = &v1.Task{
		ID:          "task-sessionless-move",
		Title:       "Sessionless workflow move",
		Description: "Start only after the runtime accepts this prompt.",
		State:       v1.TaskStateInProgress,
	}

	processStarted := make(chan struct{})
	allowProcessStart := make(chan struct{})
	firstPromptEntered := make(chan struct{})
	retryPromptEntered := make(chan struct{})
	allowPromptAcceptance := make(chan struct{})
	promptRejected := errors.New("runtime rejected prompt before acceptance")
	var promptMu sync.Mutex
	promptAttempts := 0
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "exec-sessionless-move", nil
		},
		launchAgentFunc: func(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "running-sessionless-move", SessionID: req.SessionID, TaskID: req.TaskID,
				AgentExecutionID: "exec-sessionless-move", Status: "starting",
			}); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-sessionless-move", Status: v1.AgentStatusStarting}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error {
			close(processStarted)
			<-allowProcessStart
			return nil
		},
		promptAgentFunc: func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
			promptMu.Lock()
			promptAttempts++
			attempt := promptAttempts
			promptMu.Unlock()
			if attempt == 1 {
				close(firstPromptEntered)
				return nil, promptRejected
			}
			close(retryPromptEntered)
			<-allowPromptAcceptance
			return &executor.PromptResult{StopReason: "dispatched"}, nil
		},
	}
	stepGetter := newMockStepGetter()
	step := &wfmodels.WorkflowStep{
		ID: stepInProgressID, WorkflowID: "wf1", Name: "In Progress", Position: 1,
		AgentProfileID: "profile-sessionless-move",
		Events:         wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}
	stepGetter.steps[step.ID] = step
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	db := sqlx.NewDb(repo.DB(), "sqlite3")
	entryStore, err := workflowmove.NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	svc.SetMoveEntryStore(entryStore)

	const moveID = "move-sessionless-runtime-boundary"
	if err := entryStore.Save(ctx, &workflowmove.Entry{
		ID: moveID, TaskID: "task-sessionless-move",
		Options: workflowmove.EntryOptions{Instructions: "Preserve this private handoff."},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !svc.markWorkflowMoveDispatchReady(ctx, moveID, "") {
		t.Fatal("markWorkflowMoveDispatchReady failed")
	}
	if err := repo.SetTaskMetadataKey(ctx, "task-sessionless-move", models.MetaKeyWorkflowMovePending, map[string]interface{}{
		"from_step_id": stepInReviewID, "move_id": moveID,
	}); err != nil {
		t.Fatalf("SetTaskMetadataKey: %v", err)
	}
	task, err := repo.GetTask(ctx, "task-sessionless-move")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		moveCtx := withWorkflowMoveEntryID(ctx, moveID)
		svc.startTaskForLoadedStep(
			moveCtx, task, step, "workflow_move_test", false, moveID,
			&workflowmove.EntryOptions{Instructions: "Preserve this private handoff."},
			"", "profile-sessionless-move", "", "", false,
		)
	}()

	select {
	case <-processStarted:
	case <-time.After(time.Second):
		t.Fatal("runtime process did not start")
	}
	entry, err := entryStore.Load(ctx, moveID)
	if err != nil {
		t.Fatalf("Load before runtime readiness: %v", err)
	}
	if entry == nil || entry.Phase != workflowmove.EntryPhaseDispatchReady {
		t.Fatalf("entry before runtime readiness = %#v, want dispatch_ready", entry)
	}
	if entry.TargetSessionID == "" {
		t.Fatal("dispatch_ready entry did not durably bind the prepared target session")
	}

	close(allowProcessStart)
	select {
	case <-firstPromptEntered:
	case <-time.After(time.Second):
		t.Fatal("runtime prompt was not dispatched after process start")
	}
	deadline := time.Now().Add(time.Second)
	for {
		entry, err = entryStore.Load(ctx, moveID)
		if err != nil {
			t.Fatalf("Load after definite prompt rejection: %v", err)
		}
		if entry == nil {
			t.Fatal("definite prompt rejection finalized the workflow move entry")
		}
		session, sessionErr := repo.GetTaskSession(ctx, entry.TargetSessionID)
		if sessionErr == nil && session.State == models.TaskSessionStateWaitingForInput {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session did not roll back after definite prompt rejection: entry=%#v session=%#v err=%v", entry, session, sessionErr)
		}
		time.Sleep(time.Millisecond)
	}
	if entry == nil || entry.Phase != workflowmove.EntryPhaseDispatchReady {
		t.Fatalf("entry after definite prompt rejection = %#v, want dispatch_ready", entry)
	}

	recoveredTask, err := repo.GetTask(ctx, "task-sessionless-move")
	if err != nil {
		t.Fatalf("GetTask for recovery: %v", err)
	}
	recoveryDone := make(chan struct{})
	go func() {
		defer close(recoveryDone)
		svc.recoverWorkflowMoveEntry(ctx, recoveredTask)
	}()
	select {
	case <-retryPromptEntered:
	case <-time.After(time.Second):
		t.Fatal("target-bound recovery did not retry the rejected prompt")
	}
	entry, err = entryStore.Load(ctx, moveID)
	if err != nil {
		t.Fatalf("Load before retry acceptance: %v", err)
	}
	if entry == nil || entry.Phase != workflowmove.EntryPhaseDispatchReady {
		t.Fatalf("entry before retry acceptance = %#v, want dispatch_ready", entry)
	}
	close(allowPromptAcceptance)
	select {
	case <-recoveryDone:
	case <-time.After(time.Second):
		t.Fatal("sessionless recovery did not return after prompt acceptance")
	}
	deadline = time.Now().Add(time.Second)
	for {
		entry, err = entryStore.Load(ctx, moveID)
		if err != nil {
			t.Fatalf("Load after prompt acceptance: %v", err)
		}
		if entry == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("entry after prompt acceptance = %#v, want finalized", entry)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestWorkflowMoveEntryRecoveryRetainsPrivateOptionsUntilQueueAcknowledgement(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	entryStore := newPendingMoveEntryStore()
	sc.svc.SetMoveEntryStore(entryStore)

	// Recovery must be able to resume an already-committed move without the
	// original task.moved notification. Keep the target session idle and use a
	// target step with no auto-start action so the test isolates the durable
	// entry hand-off and queue delivery.
	sc.stepGetter.steps[stepInProgressID] = &wfmodels.WorkflowStep{
		ID: stepInProgressID, WorkflowID: "wf1", Name: "In Progress", Position: 1,
		AgentProfileID: profileReview,
	}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := sc.repo.UpdateTaskSession(sc.ctx, session); err != nil {
		t.Fatalf("persist idle review session: %v", err)
	}

	const moveID = "restart-recovered-move"
	options := &workflowmove.EntryOptions{Instructions: "Continue from the durable move."}
	if err := entryStore.Save(sc.ctx, &workflowmove.Entry{ID: moveID, TaskID: "task-1", Options: *options}); err != nil {
		t.Fatalf("save private move entry: %v", err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = stepInProgressID
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.Metadata = map[string]interface{}{
		models.MetaKeyWorkflowMovePending: map[string]interface{}{
			"from_step_id": stepInReviewID,
			"move_id":      moveID,
		},
	}
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatalf("persist committed move marker: %v", err)
	}
	if err := sc.svc.messageQueue.SetAutoRun(sc.ctx, sc.reviewSessionID, false); err != nil {
		t.Fatalf("pause queue auto-run: %v", err)
	}

	// This is the startup reconciliation action for the durable marker.
	sc.svc.recoverWorkflowMoveEntry(sc.ctx, task)

	if entry, loadErr := entryStore.Load(sc.ctx, moveID); loadErr != nil || entry == nil {
		t.Fatalf("private entry after recovery = %+v err=%v, want retained until prompt acknowledgement", entry, loadErr)
	}
	recovered, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("reload recovered task: %v", err)
	}
	if _, pending := recovered.Metadata[models.MetaKeyWorkflowMovePending]; !pending {
		t.Fatalf("workflow move marker cleared before prompt acknowledgement: %#v", recovered.Metadata)
	}
	status := sc.svc.messageQueue.GetStatus(sc.ctx, sc.reviewSessionID)
	if status.Count == 0 || !strings.Contains(status.Entries[len(status.Entries)-1].Content, options.Instructions) {
		t.Fatalf("recovered queue = %+v, want one-time instructions", status.Entries)
	}
}

func TestWorkflowMoveRecoveryRevivesFailedBoundTargetSession(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	store := newPendingMoveEntryStore()
	sc.svc.SetMoveEntryStore(store)
	sc.stepGetter.steps[stepInProgressID] = &wfmodels.WorkflowStep{
		ID: stepInProgressID, WorkflowID: "wf1", Name: "In Progress", Position: 1,
		AgentProfileID: profileReview,
	}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	session.State = models.TaskSessionStateFailed
	session.ErrorMessage = "previous runtime failed"
	if err := sc.repo.UpdateTaskSession(sc.ctx, session); err != nil {
		t.Fatal(err)
	}
	const moveID = "move-revive-failed-target"
	if err := store.Save(sc.ctx, &workflowmove.Entry{
		ID: moveID, TaskID: "task-1", TargetSessionID: session.ID,
		Options: workflowmove.EntryOptions{Instructions: "retry destination"},
	}); err != nil {
		t.Fatal(err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	task.WorkflowStepID = stepInProgressID
	task.QueuedForStepID = ""
	task.Metadata = map[string]interface{}{models.MetaKeyWorkflowMovePending: map[string]interface{}{
		"from_step_id": stepInReviewID, "move_id": moveID,
	}}
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatal(err)
	}
	sc.svc.recoverWorkflowMoveEntry(sc.ctx, task)
	reloaded, err := sc.repo.GetTaskSession(sc.ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.State == models.TaskSessionStateFailed {
		t.Fatalf("target session state = %s, want revived for retry", reloaded.State)
	}
}

func TestWorkflowMoveRecoveryDoesNotReviveCancelledBoundTargetSession(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	store := newPendingMoveEntryStore()
	sc.svc.SetMoveEntryStore(store)
	sc.stepGetter.steps[stepInProgressID] = &wfmodels.WorkflowStep{ID: stepInProgressID, WorkflowID: "wf1", Name: "In Progress", Position: 1}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	session.State = models.TaskSessionStateCancelled
	if err := sc.repo.UpdateTaskSession(sc.ctx, session); err != nil {
		t.Fatal(err)
	}
	const moveID = "move-cancelled-target"
	if err := store.Save(sc.ctx, &workflowmove.Entry{
		ID: moveID, TaskID: "task-1", TargetSessionID: session.ID,
		Options: workflowmove.EntryOptions{Instructions: "must not replay"},
	}); err != nil {
		t.Fatal(err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	task.WorkflowStepID = stepInProgressID
	task.QueuedForStepID = ""
	task.Metadata = map[string]interface{}{models.MetaKeyWorkflowMovePending: map[string]interface{}{
		"from_step_id": stepInReviewID, "move_id": moveID,
	}}
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatal(err)
	}
	sc.svc.recoverWorkflowMoveEntry(sc.ctx, task)
	reloaded, err := sc.repo.GetTaskSession(sc.ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.State != models.TaskSessionStateCancelled {
		t.Fatalf("cancelled target state = %s, want CANCELLED", reloaded.State)
	}
	if entry, err := store.Load(sc.ctx, moveID); err != nil || entry == nil {
		t.Fatalf("cancelled target entry = (%#v, %v), want retained fail-closed", entry, err)
	}
}

// Reviewer-requested contract coverage: the destination must not acquire
// ownership when durable queue transfer fails.
func TestReuseSessionForStepTransferFailurePreservesSourceOwnership(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	transferErr := errors.New("queue transfer unavailable")
	sc.svc.messageQueue = messagequeue.NewService(
		failingSessionTransferRepository{Repository: messagequeue.NewMemoryRepository(), err: transferErr},
		messagequeue.DefaultMaxPerSession,
		testLogger(),
	)
	source, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	target, err := sc.repo.GetTaskSession(sc.ctx, sc.implSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sc.svc.reuseSessionForStep(sc.ctx, "task-1", source, target); !errors.Is(err, transferErr) {
		t.Fatalf("reuseSessionForStep error = %v, want transfer failure", err)
	}
	reloadedSource, err := sc.repo.GetTaskSession(sc.ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	reloadedTarget, err := sc.repo.GetTaskSession(sc.ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloadedSource.IsPrimary || reloadedSource.State != models.TaskSessionStateRunning {
		t.Fatalf("source ownership changed after transfer failure: %#v", reloadedSource)
	}
	if reloadedTarget.IsPrimary || reloadedTarget.State != models.TaskSessionStateCompleted {
		t.Fatalf("destination ownership changed after transfer failure: %#v", reloadedTarget)
	}
}

func TestApplyPendingMovePermanentCommitMismatchDeletesExactRow(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	sc.svc.workflowStore = &workflowStore{}
	move := &messagequeue.PendingMove{
		MoveID: "move-permanent-source-mismatch", TaskID: "task-1",
		FromWorkflowID: "wf1", FromStepID: stepInReviewID,
		WorkflowID: "wf1", WorkflowStepID: stepInProgressID,
	}
	if admitted, err := queueRepo.InsertPendingMoveIfAbsent(sc.ctx, sc.reviewSessionID, move); err != nil || !admitted {
		t.Fatalf("InsertPendingMoveIfAbsent = (%v, %v)", admitted, err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	task.WorkflowStepID = stepReviewedID
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatal(err)
	}
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatal(err)
	}
	sc.svc.applyPendingMove(sc.ctx, task.ID, session.ID, session, move)
	stored, err := queueRepo.GetPendingMove(sc.ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Fatalf("permanently invalid pending row survived: %#v", stored)
	}
}

// --- Pending-move scenario builder & assertions ---

const (
	stepInProgressID = "step-in-progress"
	stepInReviewID   = "step-in-review"
	stepReviewedID   = "step-reviewed"

	profileImpl   = "profile-impl"
	profileReview = "profile-review"
)

// pendingMoveScenario is the seeded fixture used by deferred-move tests.
// It owns the repo + service + mock agent manager so a single value carries
// every reference an assertion needs without long parameter lists.
type pendingMoveScenario struct {
	ctx              context.Context
	svc              *Service
	repo             *sqliterepo.Repository
	agentMgr         *mockAgentManager
	stepGetter       *mockStepGetter
	implSessionID    string
	reviewSessionID  string
	implRelaunchExec string
}

// buildPendingMoveScenario sets up the full repro scenario:
//   - 3 workflow steps: In Progress (auto_start, on_turn_complete → Review),
//     In Review (auto_start, on_turn_complete → Reviewed), Reviewed (terminal).
//   - Task currently at "In Review", with two sessions: an Impl session that
//     was completed earlier (revivable — has executors_running), and a Review
//     session that's currently RUNNING and primary.
//   - PendingMove + legacy hand-off prompt seeded as if the QA agent just called
//     move_task_kandev mid-turn.
//   - Mock LaunchAgent that fires the boot signal asynchronously so the
//     resume path can complete in tests without a real agent process.
func buildPendingMoveScenario(t *testing.T) *pendingMoveScenario {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	repo := setupTestRepo(t)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	stepGetter := newPendingMoveStepGetter()

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkflowID: "wf1", WorkflowStepID: stepInReviewID,
		Title: "Test", Description: "Implement a python buggy fibonnacci",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	implSessionID := seedImplSession(t, repo, now)
	reviewSessionID := seedReviewSession(t, repo, now)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-1"] = &v1.Task{
		ID: "task-1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test", Description: "Implement a python buggy fibonnacci",
		State: v1.TaskStateInProgress,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})

	svc := &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: stepGetter,
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       messagequeue.NewServiceMemory(log),
		executor:           exec,
		scheduler:          sched,
	}
	svc.SetWorkflowStepGetter(stepGetter)

	const implRelaunchExec = "ae-impl-relaunch"
	wireBootReadySimulator(svc, agentMgr, implRelaunchExec)

	const handoffPrompt = "You were moved to this step with the following message: " +
		"The file fibonacci.py has two bugs — fix them."
	if _, err := svc.messageQueue.QueueMessage(
		ctx, reviewSessionID, "task-1", handoffPrompt, "", messagequeue.QueuedByMoveTask, false, nil,
	); err != nil {
		t.Fatalf("queue hand-off prompt: %v", err)
	}
	svc.messageQueue.SetPendingMove(ctx, reviewSessionID, &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
	})

	return &pendingMoveScenario{
		ctx:              ctx,
		svc:              svc,
		repo:             repo,
		agentMgr:         agentMgr,
		stepGetter:       stepGetter,
		implSessionID:    implSessionID,
		reviewSessionID:  reviewSessionID,
		implRelaunchExec: implRelaunchExec,
	}
}

func prepareDuplicateWorkflowMoveScenario(t *testing.T) (*pendingMoveScenario, *recordingWorkflowMoveStore, *mockMessageCreator, string) {
	t.Helper()
	sc := buildPendingMoveScenario(t)
	sc.stepGetter.steps[stepInProgressID].AgentProfileID = profileReview

	db := sqlx.NewDb(sc.repo.DB(), "sqlite3")
	baseStore, err := workflowmove.NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	lifecycleStore, ok := baseStore.(workflowmove.LifecycleStore)
	if !ok {
		t.Fatal("SQLite move entry store does not implement LifecycleStore")
	}
	entryStore := &recordingWorkflowMoveStore{LifecycleStore: lifecycleStore}
	sc.svc.SetMoveEntryStore(entryStore)
	sc.agentMgr.isAgentRunning = true
	sc.agentMgr.isAgentReadyFn = func(context.Context, string) bool { return true }
	seedExecutorRunning(t, sc.repo, sc.reviewSessionID, "task-1", "ae-review")
	messages := &mockMessageCreator{}
	sc.svc.messageCreator = messages

	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load move session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := sc.repo.UpdateTaskSession(sc.ctx, session); err != nil {
		t.Fatalf("persist move session: %v", err)
	}

	const moveID = "duplicate-workflow-move"
	options := workflowmove.EntryOptions{
		ResetContext:   true,
		Instructions:   "Preserve this duplicate-safe handoff.",
		AgentProfileID: profileReview,
	}
	if err := entryStore.Save(sc.ctx, &workflowmove.Entry{ID: moveID, TaskID: "task-1", Options: options}); err != nil {
		t.Fatalf("save private move entry: %v", err)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load move task: %v", err)
	}
	task.WorkflowStepID = stepInProgressID
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.Metadata = map[string]interface{}{
		models.MetaKeyWorkflowMovePending: map[string]interface{}{
			"from_step_id": stepInReviewID,
			"move_id":      moveID,
		},
	}
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatalf("persist committed move: %v", err)
	}
	return sc, entryStore, messages, moveID
}

func assertWorkflowMoveConsumedOnce(t *testing.T, sc *pendingMoveScenario, entryStore *recordingWorkflowMoveStore, messages *mockMessageCreator, moveID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		entry, err := entryStore.Load(sc.ctx, moveID)
		if err != nil {
			t.Fatalf("load replay entry: %v", err)
		}
		if entry == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("replay entry remained at phase %q: %#v", entry.Phase, entry)
		}
		time.Sleep(time.Millisecond)
	}
	if got := len(capturedPromptsForExecution(sc.agentMgr, "ae-review")); got != 1 {
		t.Fatalf("prompt deliveries = %d, want 1", got)
	}
	if got := len(messages.userMessages); got != 1 {
		t.Fatalf("user messages = %d, want 1", got)
	}
	claims := entryStore.successfulClaims()
	wantClaims := []workflowmove.EntryPhase{
		workflowmove.EntryPhaseExitApplied,
		workflowmove.EntryPhaseProfileApplied,
		workflowmove.EntryPhaseResetApplied,
		workflowmove.EntryPhaseConfigApplied,
		workflowmove.EntryPhaseActionsApplied,
		workflowmove.EntryPhaseDispatchReady,
		workflowmove.EntryPhaseDispatchClaimed,
		workflowmove.EntryPhaseDispatchAccepted,
	}
	if len(claims) != len(wantClaims) {
		t.Fatalf("successful move phases = %#v, want %#v", claims, wantClaims)
	}
	for i, claim := range claims {
		if claim.next != wantClaims[i] {
			t.Fatalf("successful move phase %d = %q, want %q", i, claim.next, wantClaims[i])
		}
	}
	if got := entryStore.finalizeCount(); got != 1 {
		t.Fatalf("workflow move finalizations = %d, want 1", got)
	}
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("reload finalized move task: %v", err)
	}
	if _, pending := task.Metadata[models.MetaKeyWorkflowMovePending]; pending {
		t.Fatalf("workflow move marker remained after finalization: %#v", task.Metadata)
	}
}

// newPendingMoveStepGetter builds the 3-step workflow used by the scenario.
// Both auto_start steps have UNCONDITIONAL on_turn_complete rules — that's
// the workflow shape that exposed the original ping-pong bug, so we keep it.
func newPendingMoveStepGetter() *mockStepGetter {
	sg := newMockStepGetter()
	sg.steps[stepInProgressID] = &wfmodels.WorkflowStep{
		ID: stepInProgressID, WorkflowID: "wf1", Name: "In Progress", Position: 1,
		AgentProfileID: profileImpl,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": stepInReviewID}},
			},
		},
	}
	sg.steps[stepInReviewID] = &wfmodels.WorkflowStep{
		ID: stepInReviewID, WorkflowID: "wf1", Name: "In Review", Position: 2,
		AgentProfileID: profileReview,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": stepReviewedID}},
			},
		},
	}
	sg.steps[stepReviewedID] = &wfmodels.WorkflowStep{
		ID: stepReviewedID, WorkflowID: "wf1", Name: "Reviewed", Position: 3,
	}
	return sg
}

// seedImplSession seeds the previously-completed Impl session with an
// executors_running record so reuseSessionForStep revives it as
// WAITING_FOR_INPUT (matching the real-world "previously launched" path).
func seedImplSession(t *testing.T, repo *sqliterepo.Repository, now time.Time) string {
	t.Helper()
	const id = "session-impl"
	completedAt := now.Add(-1 * time.Minute)
	sess := &models.TaskSession{
		ID:                id,
		TaskID:            "task-1",
		AgentProfileID:    profileImpl,
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		AgentExecutionID:  "ae-impl-original",
		State:             models.TaskSessionStateCompleted,
		CompletedAt:       &completedAt,
		StartedAt:         now.Add(-2 * time.Minute),
		UpdatedAt:         completedAt,
	}
	if err := repo.CreateTaskSession(context.Background(), sess); err != nil {
		t.Fatalf("create impl session: %v", err)
	}
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: id, SessionID: id, TaskID: "task-1",
		ResumeToken: "resume-token-impl", AgentExecutionID: "ae-impl-original",
		CreatedAt: now.Add(-2 * time.Minute), UpdatedAt: completedAt,
	}); err != nil {
		t.Fatalf("upsert executors_running for impl: %v", err)
	}
	return id
}

// seedReviewSession seeds the currently-active Review session as primary,
// RUNNING — the QA agent that's about to fire its move_task_kandev call.
func seedReviewSession(t *testing.T, repo *sqliterepo.Repository, now time.Time) string {
	t.Helper()
	const id = "session-review"
	sess := &models.TaskSession{
		ID:                id,
		TaskID:            "task-1",
		AgentProfileID:    profileReview,
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		AgentExecutionID:  "ae-review",
		State:             models.TaskSessionStateRunning,
		IsPrimary:         true,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	if err := repo.CreateTaskSession(context.Background(), sess); err != nil {
		t.Fatalf("create review session: %v", err)
	}
	return id
}

// wireBootReadySimulator stubs LaunchAgent to fire handleAgentBootReady ~50ms
// after returning. Real agentctl bootstrap publishes events.AgentBootReady from
// outside the LaunchAgent call; mirroring that timing here lets the resume
// path complete in unit tests without spawning a real subprocess.
func wireBootReadySimulator(svc *Service, agentMgr *mockAgentManager, newExecID string) {
	promptReady := make(chan struct{})
	agentMgr.isAgentReadyFn = func(_ context.Context, _ string) bool {
		select {
		case <-promptReady:
			return true
		default:
			return false
		}
	}
	agentMgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		// Simulate the lifecycle manager's persistExecutorRunning: in production
		// the row is upserted in lockstep with executionStore.Add; here we mirror
		// that timing so the orchestrator's GetExecutionIDForSession lookup
		// resolves to the new exec id immediately.
		if svc != nil && svc.repo != nil {
			_ = svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
				ID:               req.SessionID,
				SessionID:        req.SessionID,
				TaskID:           req.TaskID,
				AgentExecutionID: newExecID,
				ContainerID:      "container-relaunch",
				Status:           "starting",
			})
		}
		go func() {
			time.Sleep(50 * time.Millisecond)
			svc.handleAgentBootReady(context.Background(), watcher.AgentEventData{
				TaskID:           req.TaskID,
				SessionID:        req.SessionID,
				AgentExecutionID: newExecID,
				AgentProfileID:   req.AgentProfileID,
			})
			close(promptReady)
		}()
		return &executor.LaunchAgentResponse{
			AgentExecutionID: newExecID,
			ContainerID:      "container-relaunch",
			Status:           v1.AgentStatusReady,
		}, nil
	}
}

// startStepHistorySampler polls task.WorkflowStepID and appends every change
// to a slice. Returned channel closes when sampling ends; the *[]string is
// safe to read from the caller after that.
func (sc *pendingMoveScenario) startStepHistorySampler(t *testing.T, duration time.Duration) (<-chan struct{}, *[]string) {
	t.Helper()
	done := make(chan struct{})
	history := []string{stepInReviewID}
	go func() {
		defer close(done)
		seen := stepInReviewID
		deadline := time.Now().Add(duration)
		for time.Now().Before(deadline) {
			task, err := sc.repo.GetTask(sc.ctx, "task-1")
			if err == nil && task.WorkflowStepID != seen {
				seen = task.WorkflowStepID
				history = append(history, seen)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	return done, &history
}

// assertOneTransitionToInProgress checks every postcondition of the scenario:
// task moved to In Progress, exactly one transition, sessions in the right
// state, and the hand-off prompt landed on the impl session (delivered or
// queued — either is acceptable for this regression).
func (sc *pendingMoveScenario) assertOneTransitionToInProgress(t *testing.T, stepHistory []string) {
	t.Helper()

	finalTask, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load final task: %v", err)
	}

	dedup := dedupConsecutive(stepHistory)
	t.Logf("workflow_step_id transition history: %v", stepNamesFromIDs(dedup, sc.stepGetter))

	if finalTask.WorkflowStepID != stepInProgressID {
		t.Errorf("final workflow_step_id = %q, want %q (In Progress)", finalTask.WorkflowStepID, stepInProgressID)
	}

	expected := []string{stepInReviewID, stepInProgressID}
	if !sliceEqual(dedup, expected) {
		t.Errorf("transition history = %v, want %v\n  (this means the deferred-move triggered spurious additional transitions — the bug)",
			stepNamesFromIDs(dedup, sc.stepGetter), stepNamesFromIDs(expected, sc.stepGetter))
	}

	rev, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	if rev.State != models.TaskSessionStateCompleted {
		t.Errorf("review session state = %q, want COMPLETED (parked by the profile switch)", rev.State)
	}
	if rev.IsPrimary {
		t.Error("review session must no longer be primary (the impl session takes over)")
	}

	impl, err := sc.repo.GetTaskSession(sc.ctx, sc.implSessionID)
	if err != nil {
		t.Fatalf("load impl session: %v", err)
	}
	if !impl.IsPrimary {
		t.Error("impl session must be primary after the deferred move applies")
	}
	if impl.State == models.TaskSessionStateCompleted {
		t.Errorf("impl session state = %q, expected non-terminal (revived for a new turn)", impl.State)
	}

	sc.assertHandoffDeliveredOrQueued(t)
}

// assertHandoffDeliveredOrQueued checks the hand-off prompt landed on the impl
// session — either delivered to its agent (PromptAgent capture) or sitting in
// the queue waiting for delivery. Both are acceptable; the failure mode the
// regression catches is "lost" (neither delivered nor queued) or "delivered
// to the wrong session".
func (sc *pendingMoveScenario) assertHandoffDeliveredOrQueued(t *testing.T) {
	t.Helper()
	implPrompts := capturedPromptsForExecution(sc.agentMgr, sc.implRelaunchExec)
	implQueued := sc.svc.messageQueue.GetStatus(sc.ctx, sc.implSessionID)

	if len(implPrompts) == 0 && implQueued.Count == 0 {
		t.Error("hand-off prompt was neither delivered to the impl session nor queued for it")
		return
	}
	for _, p := range implPrompts {
		if strings.Contains(p, "fibonacci.py has two bugs") {
			return
		}
	}
	for _, entry := range implQueued.Entries {
		if strings.Contains(entry.Content, "fibonacci.py has two bugs") {
			return
		}
	}
	t.Errorf("hand-off prompt was neither delivered nor queued with the expected content")
}

// --- Helpers ---

// capturedPromptsForExecution returns only the prompts that were sent to the
// given agent execution ID. The earlier version ignored its selector and
// returned every recorded prompt — which would let the test pass even if the
// hand-off had been delivered to the wrong session.
func capturedPromptsForExecution(agentMgr *mockAgentManager, executionID string) []string {
	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	out := make([]string, 0, len(agentMgr.capturedPromptCalls))
	for _, c := range agentMgr.capturedPromptCalls {
		if c.ExecutionID == executionID {
			out = append(out, c.Prompt)
		}
	}
	return out
}

func dedupConsecutive(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := []string{in[0]}
	for i := 1; i < len(in); i++ {
		if in[i] != in[i-1] {
			out = append(out, in[i])
		}
	}
	return out
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stepNamesFromIDs(ids []string, sg *mockStepGetter) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if step, ok := sg.steps[id]; ok {
			out = append(out, step.Name)
		} else {
			out = append(out, id)
		}
	}
	return out
}

// TestHandleAgentBootReady_DoesNotTriggerOnTurnComplete locks in the post-fix
// invariant: a boot-ready signal (agent's ACP session has just initialized,
// no turn has run yet) must NEVER step the workflow. The lifecycle layer now
// publishes events.AgentBootReady — distinct from events.AgentReady — and the
// orchestrator routes it to handleAgentBootReady which only flips the session
// to WAITING_FOR_INPUT.
//
// Before this split, both signals shared events.AgentReady and the
// orchestrator tried to disambiguate them with the resumeInProgressSessions
// flag. That flag had a race: when the boot ready arrived BEFORE
// persistResumeState wrote state=STARTING, handleAgentReady's state guard
// returned without consuming the flag, leaking it to the next event and
// firing on_turn_complete against the wrong session.
//
// This test fires the boot signal directly into handleAgentBootReady to
// confirm: (a) no on_turn_complete evaluation runs, (b) the session ends up
// WAITING_FOR_INPUT regardless of what state it was in.
func TestHandleAgentBootReady_DoesNotTriggerOnTurnComplete(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	repo := setupTestRepo(t)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	// One step with an unconditional on_turn_complete. If a boot-ready
	// somehow reaches the turn-end path, this rule fires and the task moves —
	// the user-visible symptom of the original bug.
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-current"] = &wfmodels.WorkflowStep{
		ID: "step-current", WorkflowID: "wf1", Name: "Current", Position: 1,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step-next"}},
			},
		},
	}
	stepGetter.steps["step-next"] = &wfmodels.WorkflowStep{
		ID: "step-next", WorkflowID: "wf1", Name: "Next", Position: 2,
	}

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkflowID: "wf1", WorkflowStepID: "step-current",
		Title: "T", Description: "D", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Two scenarios the boot signal must handle correctly:
	//   - state=STARTING (the textbook case: persistResumeState wrote it)
	//   - state=WAITING_FOR_INPUT (the racy case: boot signal beat
	//     persistResumeState, or reviveReusedSession left it WAITING)
	cases := []struct {
		name     string
		startSt  models.TaskSessionState
		expectSt models.TaskSessionState
	}{
		{"STARTING", models.TaskSessionStateStarting, models.TaskSessionStateWaitingForInput},
		{"WAITING_FOR_INPUT (race-with-persistResumeState)", models.TaskSessionStateWaitingForInput, models.TaskSessionStateWaitingForInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := "s1-" + tc.name
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: sessionID, TaskID: "task-1", AgentProfileID: "profile-impl",
				AgentExecutionID: "ae-current",
				State:            tc.startSt,
				IsPrimary:        true,
				StartedAt:        now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("create session: %v", err)
			}

			taskRepo := newMockTaskRepo()
			taskRepo.tasks["task-1"] = &v1.Task{
				ID: "task-1", WorkflowID: "wf1", State: v1.TaskStateInProgress,
			}

			agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
			log := testLogger()
			exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
			svc := &Service{
				logger:             log,
				repo:               repo,
				workflowStepGetter: stepGetter,
				taskRepo:           taskRepo,
				agentManager:       agentMgr,
				messageQueue:       messagequeue.NewServiceMemory(log),
				executor:           exec,
			}
			svc.SetWorkflowStepGetter(stepGetter)

			// Reset task to step-current in case a prior subtest moved it.
			tk, _ := repo.GetTask(ctx, "task-1")
			tk.WorkflowStepID = "step-current"
			_ = repo.UpdateTask(ctx, tk)

			// Fire the new boot-only event. The handler must NOT run on_turn_complete.
			svc.handleAgentBootReady(ctx, watcher.AgentEventData{
				TaskID: "task-1", SessionID: sessionID,
				AgentExecutionID: "ae-current",
				AgentProfileID:   "profile-impl",
			})

			finalTask, err := repo.GetTask(ctx, "task-1")
			if err != nil {
				t.Fatalf("load task: %v", err)
			}
			if finalTask.WorkflowStepID != "step-current" {
				t.Errorf("workflow_step_id = %q, want %q (boot signal must not move the workflow)",
					finalTask.WorkflowStepID, "step-current")
			}

			finalSess, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				t.Fatalf("load session: %v", err)
			}
			if finalSess.State != tc.expectSt {
				t.Errorf("session.State = %q, want %q", finalSess.State, tc.expectSt)
			}
			resolvedAt, ok := finalSess.Metadata[models.SessionMetaKeyRecoveryResolvedAt].(string)
			if !ok || resolvedAt == "" {
				t.Fatalf("recovery resolution timestamp = %#v, want RFC3339 string", finalSess.Metadata[models.SessionMetaKeyRecoveryResolvedAt])
			}
			if _, err := time.Parse(time.RFC3339Nano, resolvedAt); err != nil {
				t.Fatalf("recovery resolution timestamp %q is invalid: %v", resolvedAt, err)
			}
		})
	}
}

// TestHandleAgentBootReady_DrainsOrphanedQueuedMessage reproduces the
// production stuck-queue symptom: a workflow auto-start prompt is queued
// against a session, the agent dies before the turn completes (so no
// agent.ready fires to drain it), and the user resumes the session. The
// session ends up WAITING_FOR_INPUT with the message still on the queue —
// "1 queued" displayed in the UI forever — because handleAgentBootReady
// only flipped state but never drained.
//
// After the fix, handleAgentBootReady takes the queued message and dispatches
// it (via executeQueuedMessage in a goroutine). The test wires a full
// executor + seeded executors_running row so the goroutine's PromptTask
// call lands on a working code path instead of nil-derefing s.executor
// under -race.
func TestHandleAgentBootReady_DrainsOrphanedQueuedMessage(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	cases := []struct {
		name    string
		startSt models.TaskSessionState
	}{
		{"STARTING -> WAITING_FOR_INPUT (boot completes resume)", models.TaskSessionStateStarting},
		{"already WAITING_FOR_INPUT (boot raced persistResumeState)", models.TaskSessionStateWaitingForInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatalf("create workspace: %v", err)
			}
			if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatalf("create workflow: %v", err)
			}
			if err := repo.CreateTask(ctx, &models.Task{
				ID: "task-1", WorkflowID: "wf1", WorkflowStepID: "step-merge",
				Title: "T", Description: "D", State: v1.TaskStateInProgress,
				CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("create task: %v", err)
			}

			sessionID := "s1"
			const executionID = "exec-1"
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: sessionID, TaskID: "task-1", AgentProfileID: "profile-impl",
				AgentExecutionID: executionID,
				State:            tc.startSt,
				IsPrimary:        true,
				StartedAt:        now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("create session: %v", err)
			}
			// Seed executors_running so PromptTask -> ensureSessionRunning ->
			// executor.GetExecutionBySession finds the agent and skips resume.
			seedExecutorRunning(t, repo, sessionID, "task-1", executionID)

			taskRepo := newMockTaskRepo()
			taskRepo.tasks["task-1"] = &v1.Task{
				ID: "task-1", WorkflowID: "wf1", State: v1.TaskStateInProgress,
			}

			log := testLogger()
			agentMgr := &mockAgentManager{
				repoForExecutionLookup: repo,
				isAgentRunning:         true, // satisfy GetExecutionBySession's IsAgentRunningForSession check
			}
			svc := &Service{
				logger:       log,
				repo:         repo,
				taskRepo:     taskRepo,
				agentManager: agentMgr,
				messageQueue: messagequeue.NewServiceMemory(log),
				// Wire a real executor so the executeQueuedMessage goroutine
				// spawned by drainQueuedMessageForPromptableSession can safely call
				// PromptTask -> executor.GetExecutionBySession without nil-derefing.
				executor: executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{}),
			}

			// Seed an orphaned workflow auto-start prompt — what the production
			// bug looked like in the DB at task 9378f7cf.
			if _, err := svc.messageQueue.QueueMessage(
				ctx, sessionID, "task-1", "ROLE: Merge operator. ...", "",
				messagequeue.QueuedByWorkflow, false, nil,
			); err != nil {
				t.Fatalf("queue orphaned prompt: %v", err)
			}
			if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 1 {
				t.Fatalf("precondition: queue count = %d, want 1", got)
			}

			svc.handleAgentBootReady(ctx, watcher.AgentEventData{
				TaskID: "task-1", SessionID: sessionID,
			})

			if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 0 {
				t.Errorf("queue count after boot ready = %d, want 0 (orphaned message must be drained)", got)
			}

			// The handler synchronously flips the session to WAITING_FOR_INPUT
			// (line 173 of event_handlers_agent.go) and then spawns
			// executeQueuedMessage in a goroutine; that goroutine calls
			// PromptTask which immediately moves state to RUNNING. We can race
			// with that goroutine on slow CI runners, so accept either
			// WAITING_FOR_INPUT (goroutine hasn't transitioned yet) or RUNNING
			// (goroutine got ahead of us). Either proves the boot-ready flip
			// landed; the orphaned-message regression we guard against would
			// leave state stuck on STARTING with the queue still full.
			finalSess, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				t.Fatalf("load session: %v", err)
			}
			if finalSess.State != models.TaskSessionStateWaitingForInput &&
				finalSess.State != models.TaskSessionStateRunning {
				t.Errorf("session.State = %q, want WAITING_FOR_INPUT or RUNNING (post-flip, possibly post-goroutine)", finalSess.State)
			}
		})
	}
}

// TestHandleAgentBootReady_DoesNotDrainForTerminalSession guards against
// reviving a queued message on a session that was cancelled or completed —
// the user explicitly stopped this session, the queued prompt should NOT be
// dispatched, and the early-return for terminal states must continue to
// short-circuit before the drain.
func TestHandleAgentBootReady_DoesNotDrainForTerminalSession(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	repo := setupTestRepo(t)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkflowID: "wf1", WorkflowStepID: "step-merge",
		Title: "T", Description: "D", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	sessionID := "s1"
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: "task-1", AgentProfileID: "profile-impl",
		State:     models.TaskSessionStateCancelled,
		IsPrimary: true,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	log := testLogger()
	svc := &Service{
		logger:       log,
		repo:         repo,
		taskRepo:     newMockTaskRepo(),
		agentManager: &mockAgentManager{repoForExecutionLookup: repo},
		messageQueue: messagequeue.NewServiceMemory(log),
	}

	if _, err := svc.messageQueue.QueueMessage(
		ctx, sessionID, "task-1", "stuck prompt", "",
		messagequeue.QueuedByWorkflow, false, nil,
	); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}

	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID: "task-1", SessionID: sessionID,
	})

	if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 1 {
		t.Errorf("queue count after boot ready on terminal session = %d, want 1 (must not drain)", got)
	}
}

func TestHandleAgentBootReady_WaitsForNonCancellationGuardHolder(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateStarting, ""); err != nil {
		t.Fatalf("set session starting: %v", err)
	}

	log := testLogger()
	svc := &Service{
		logger:       log,
		repo:         repo,
		taskRepo:     newMockTaskRepo(),
		agentManager: &mockAgentManager{repoForExecutionLookup: repo},
		messageQueue: messagequeue.NewServiceMemory(log),
	}

	lock, release := svc.acquireCancelInFlightGuard("s1")
	lock.Lock()
	handlerDone := make(chan struct{})
	go func() {
		svc.handleAgentBootReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})
		close(handlerDone)
	}()

	select {
	case <-handlerDone:
		lock.Unlock()
		release()
		t.Fatal("boot-ready was dropped while a non-cancellation stream handler held the guard")
	case <-time.After(100 * time.Millisecond):
	}

	lock.Unlock()
	release()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("boot-ready did not resume after the guard was released")
	}

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %q, want %q", session.State, models.TaskSessionStateWaitingForInput)
	}
}

func TestHandleAgentBootReady_DoesNotDrainWhileCancelInFlight(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	log := testLogger()
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptDone:             make(chan struct{}),
	}
	svc := &Service{
		logger:       log,
		repo:         repo,
		taskRepo:     newMockTaskRepo(),
		agentManager: agentMgr,
		messageQueue: messagequeue.NewServiceMemory(log),
		executor:     executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{}),
	}

	if _, err := svc.messageQueue.QueueMessage(
		ctx, "s1", "t1", "queued after cancel", "",
		messagequeue.QueuedByUser, false, nil,
	); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}
	endCancel := svc.beginCancelInFlight("s1")
	defer endCancel()
	lock, release := svc.acquireCancelInFlightGuard("s1")
	defer release()
	lock.Lock()
	defer lock.Unlock()

	svc.handleAgentBootReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

	status := svc.messageQueue.GetStatus(ctx, "s1")
	if status.Count != 1 {
		t.Fatalf("queue count after boot ready during cancel = %d, want 1", status.Count)
	}
	if len(agentMgr.capturedPrompts) != 0 {
		t.Fatalf("expected no queued prompt dispatch during cancel, got %d prompts", len(agentMgr.capturedPrompts))
	}
}
