package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type recordingMoveEntryStore struct {
	entries []*workflowmove.Entry
}

func setSQLiteMoveEntryStore(t *testing.T, svc *Service, repo interface{ DB() *sql.DB }) workflowmove.EntryStore {
	t.Helper()
	db := sqlx.NewDb(repo.DB(), "sqlite3")
	store, err := workflowmove.NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	svc.SetMoveEntryStore(store)
	return store
}

func (s *recordingMoveEntryStore) Save(_ context.Context, entry *workflowmove.Entry) error {
	copy := *entry
	s.entries = append(s.entries, &copy)
	return nil
}

func (*recordingMoveEntryStore) Load(context.Context, string) (*workflowmove.Entry, error) {
	return nil, nil
}

func (*recordingMoveEntryStore) Delete(context.Context, string) error { return nil }

type atomicAdmissionMoveEntryStore struct {
	saveCalls int
}

func (s *atomicAdmissionMoveEntryStore) Save(context.Context, *workflowmove.Entry) error {
	s.saveCalls++
	return nil
}

func (*atomicAdmissionMoveEntryStore) Load(context.Context, string) (*workflowmove.Entry, error) {
	return nil, nil
}

func (*atomicAdmissionMoveEntryStore) Delete(context.Context, string) error { return nil }

type recordingAtomicMoveRepository struct {
	repository.TaskRepository
	delegate interface {
		UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
			context.Context,
			*models.Task,
			string,
			string,
			int,
			*v1.TaskState,
			bool,
			*workflowmove.Entry,
		) (bool, bool, error)
	}
	entries []*workflowmove.Entry
}

func (r *recordingAtomicMoveRepository) UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
	ctx context.Context,
	task *models.Task,
	expectedStepID string,
	targetStepID string,
	limit int,
	admittedState *v1.TaskState,
	queueExitPending bool,
	entry *workflowmove.Entry,
) (bool, bool, error) {
	if entry != nil {
		copy := *entry
		r.entries = append(r.entries, &copy)
	}
	return r.delegate.UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
		ctx, task, expectedStepID, targetStepID, limit, admittedState, queueExitPending, entry,
	)
}

type fakeWorkflowStepGetter struct {
	steps   map[string]*wfmodels.WorkflowStep
	nextErr error
	repo    *sqliterepo.Repository
}

func (f *fakeWorkflowStepGetter) GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if step, ok := f.steps[stepID]; ok {
		if f.repo != nil {
			if _, err := f.repo.DB().ExecContext(ctx, `
				INSERT INTO workflow_steps (id, workflow_id, name, position, wip_limit)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET workflow_id = excluded.workflow_id, wip_limit = excluded.wip_limit
			`, step.ID, step.WorkflowID, step.Name, step.Position, step.WIPLimit); err != nil {
				return nil, err
			}
		}
		return step, nil
	}
	return nil, errStepNotFoundForTest
}

func (f *fakeWorkflowStepGetter) GetNextStepByPosition(_ context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	if f.nextErr != nil {
		return nil, f.nextErr
	}
	var next *wfmodels.WorkflowStep
	for _, step := range f.steps {
		if step.WorkflowID != workflowID || step.Position <= currentPosition {
			continue
		}
		if next == nil || step.Position < next.Position {
			next = step
		}
	}
	return next, nil
}

func (f *fakeWorkflowStepGetter) ListStepsByWorkflow(_ context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error) {
	steps := make([]*wfmodels.WorkflowStep, 0, len(f.steps))
	for _, step := range f.steps {
		if step.WorkflowID == workflowID {
			steps = append(steps, step)
		}
	}
	return steps, nil
}

func setFakeWorkflowStepGetter(svc *Service, getter *fakeWorkflowStepGetter) {
	getter.repo, _ = svc.tasks.(*sqliterepo.Repository)
	svc.SetWorkflowStepGetter(getter)
}

type testStepNotFound struct{}

func (testStepNotFound) Error() string { return "step not found" }

var errStepNotFoundForTest = testStepNotFound{}

// TestService_SetWorkflowHidden_HealsStaleRecord verifies the helper used by
// the improve-kandev bootstrap to flip Hidden=true on workflows created
// before the flag was honored on insert.
func TestService_SetWorkflowHidden_HealsStaleRecord(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-stale", WorkspaceID: "ws-1", Name: "Improve Kandev", Hidden: false})

	if err := svc.SetWorkflowHidden(ctx, "wf-stale", true); err != nil {
		t.Fatalf("SetWorkflowHidden: %v", err)
	}

	visible, err := svc.ListWorkflows(ctx, "ws-1", false)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	for _, wf := range visible {
		if wf.ID == "wf-stale" {
			t.Fatalf("hidden workflow leaked into default listing: %+v", wf)
		}
	}

	all, err := svc.ListWorkflows(ctx, "ws-1", true)
	if err != nil {
		t.Fatalf("ListWorkflows(includeHidden): %v", err)
	}
	var found *models.Workflow
	for _, wf := range all {
		if wf.ID == "wf-stale" {
			found = wf
		}
	}
	if found == nil || !found.Hidden {
		t.Fatalf("expected wf-stale to be hidden after heal, got %+v", found)
	}
}

func TestService_UpdateTaskStateIfPrimarySessionStatePublishesLifecycleEvent(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	createRunningSession(t, ctx, repo, "session-1", "task-1", models.TaskSessionStateFailed)
	if err := svc.SetPrimarySession(ctx, "session-1"); err != nil {
		t.Fatalf("SetPrimarySession: %v", err)
	}
	eventBus.ClearEvents()

	updated, err := svc.UpdateTaskStateIfPrimarySessionState(
		ctx,
		"task-1",
		"session-1",
		models.TaskSessionStateFailed,
		v1.TaskStateFailed,
	)
	if err != nil {
		t.Fatalf("UpdateTaskStateIfSessionState: %v", err)
	}
	if !updated {
		t.Fatal("expected task state transition")
	}
	findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskStateChanged)
}

func TestService_MoveTaskRejectsInvalidWorkflowTargets(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)

	tests := []struct {
		name     string
		taskID   string
		targetWF string
		targetSt string
	}{
		{
			name:     "step belongs to another workflow",
			taskID:   "task-invalid-step",
			targetWF: "wf-source",
			targetSt: "step-target",
		},
		{
			name:     "workflow belongs to another workspace",
			taskID:   "task-other-workspace",
			targetWF: "wf-other-workspace",
			targetSt: "step-other-workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createMoveTask(t, ctx, repo, tt.taskID, "wf-source", "step-source", nil)

			_, err := svc.MoveTask(ctx, tt.taskID, tt.targetWF, tt.targetSt, 0)
			if err == nil {
				t.Fatalf("expected move to be rejected")
			}

			task, err := repo.GetTask(ctx, tt.taskID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if task.WorkflowID != "wf-source" || task.WorkflowStepID != "step-source" {
				t.Fatalf("task moved despite validation error: workflow=%s step=%s", task.WorkflowID, task.WorkflowStepID)
			}
		})
	}
}

func TestService_MoveTaskAllowsPendingReviewWhenSessionIdle(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-pending-review", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-pending-review", "task-pending-review", models.TaskSessionStateWaitingForInput, models.ReviewStatusPending)

	moved, err := svc.MoveTask(ctx, "task-pending-review", "wf-source", "step-review-target", 0)
	if err != nil {
		t.Fatalf("pending review on idle session should not block manual move: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-review-target" {
		t.Fatalf("expected step-review-target, got %s", moved.Task.WorkflowStepID)
	}
}

func TestService_MoveTaskToTerminalStepCompletesTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
	}
	createMoveTask(t, ctx, repo, "task-terminal", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	moved, err := svc.MoveTask(ctx, "task-terminal", "wf-source", "step-done", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	if moved.Task.State != v1.TaskStateCompleted {
		t.Fatalf("moved task state = %q, want COMPLETED", moved.Task.State)
	}

	task, err := repo.GetTask(ctx, "task-terminal")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("persisted task state = %q, want COMPLETED", task.State)
	}
	findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskStateChanged)
}

func TestService_MoveTaskToTerminalStepPreservesTerminalFailureStates(t *testing.T) {
	cases := []struct {
		name  string
		state v1.TaskState
	}{
		{name: "failed", state: v1.TaskStateFailed},
		{name: "cancelled", state: v1.TaskStateCancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedMoveWorkflows(t, ctx, repo)
			seedMoveSteps(svc)
			getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
			getter.steps["step-done"] = &wfmodels.WorkflowStep{
				ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
			}
			createMoveTask(t, ctx, repo, "task-terminal-"+tc.name, "wf-source", "step-source", nil)
			task, err := repo.GetTask(ctx, "task-terminal-"+tc.name)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			task.State = tc.state
			must(t, repo.UpdateTask(ctx, task))

			moved, err := svc.MoveTask(ctx, task.ID, "wf-source", "step-done", 0)
			if err != nil {
				t.Fatalf("MoveTask: %v", err)
			}
			if moved.Task.State != tc.state {
				t.Fatalf("moved task state = %q, want %q", moved.Task.State, tc.state)
			}

			task, err = repo.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if task.State != tc.state {
				t.Fatalf("persisted task state = %q, want %q", task.State, tc.state)
			}
		})
	}
}

func TestService_MoveTaskRecoveryCompletesFailedTaskAtTerminalStep(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
	}
	createMoveTask(t, ctx, repo, "task-recovery", "wf-source", "step-source", nil)
	task, err := repo.GetTask(ctx, "task-recovery")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.State = v1.TaskStateFailed
	must(t, repo.UpdateTask(ctx, task))

	moved, err := svc.MoveTaskWithOptions(ctx, task.ID, "wf-source", "step-done", 0, MoveTaskOptions{
		AllowFailedToCompletedRecovery: true,
	})
	if err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if moved.Task.State != v1.TaskStateCompleted {
		t.Fatalf("recovered task state = %q, want COMPLETED", moved.Task.State)
	}

	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after recovery: %v", err)
	}
	if stored.State != v1.TaskStateCompleted {
		t.Fatalf("persisted recovered task state = %q, want COMPLETED", stored.State)
	}
}

func TestService_MoveTaskRecoveryIsIdempotentAtTerminalStep(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
	}
	createMoveTask(t, ctx, repo, "task-recovery-idempotent", "wf-source", "step-done", nil)

	moved, err := svc.MoveTaskWithOptions(ctx, "task-recovery-idempotent", "wf-source", "step-done", 0, MoveTaskOptions{
		AllowFailedToCompletedRecovery: true,
	})
	if err != nil {
		t.Fatalf("MoveTaskWithOptions on target step: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-done" {
		t.Fatalf("idempotent recovery moved task to %q", moved.Task.WorkflowStepID)
	}
}

func TestService_MoveTaskFailsWhenTerminalStatusLookupFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.nextErr = errors.New("next step lookup failed")
	createMoveTask(t, ctx, repo, "task-terminal-lookup-error", "wf-source", "step-source", nil)

	_, err := svc.MoveTask(ctx, "task-terminal-lookup-error", "wf-source", "step-review-target", 0)
	if err == nil {
		t.Fatalf("expected move to fail when terminal status lookup fails")
	}

	task, err := repo.GetTask(ctx, "task-terminal-lookup-error")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.WorkflowStepID != "step-source" {
		t.Fatalf("task moved despite lookup error: %s", task.WorkflowStepID)
	}
}

func TestService_MoveTaskOutOfTerminalStepReopensTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
	}
	createMoveTask(t, ctx, repo, "task-reopened", "wf-source", "step-done", nil)
	task, err := repo.GetTask(ctx, "task-reopened")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.State = v1.TaskStateCompleted
	must(t, repo.UpdateTask(ctx, task))
	eventBus.ClearEvents()

	moved, err := svc.MoveTask(ctx, "task-reopened", "wf-source", "step-source", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	if moved.Task.State != v1.TaskStateTODO {
		t.Fatalf("moved task state = %q, want TODO", moved.Task.State)
	}

	task, err = repo.GetTask(ctx, "task-reopened")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.State != v1.TaskStateTODO {
		t.Fatalf("persisted task state = %q, want TODO", task.State)
	}
	findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskStateChanged)
}

func TestService_ApproveSessionToTerminalStepCompletesTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Approved", Position: 2,
	}
	createMoveTask(t, ctx, repo, "task-approved", "wf-source", "step-review-target", nil)
	createMoveSession(t, ctx, repo, "session-approved", "task-approved", models.TaskSessionStateWaitingForInput, models.ReviewStatusPending)
	eventBus.ClearEvents()

	result, err := svc.ApproveSession(ctx, "session-approved")
	if err != nil {
		t.Fatalf("ApproveSession: %v", err)
	}
	if result.Task == nil || result.Task.WorkflowStepID != "step-done" {
		t.Fatalf("approved task step = %+v, want step-done", result.Task)
	}
	if result.Task.State != v1.TaskStateCompleted {
		t.Fatalf("approved task state = %q, want COMPLETED", result.Task.State)
	}

	task, err := repo.GetTask(ctx, "task-approved")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("persisted task state = %q, want COMPLETED", task.State)
	}
	findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskStateChanged)
}

func TestService_MoveTaskRejectsRunningSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-running", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-running", "task-running", models.TaskSessionStateRunning, models.ReviewStatusNone)

	_, err := svc.MoveTask(ctx, "task-running", "wf-source", "step-review-target", 0)
	if err == nil {
		t.Fatalf("expected running session move to be rejected")
	}

	task, err := repo.GetTask(ctx, "task-running")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.WorkflowStepID != "step-source" {
		t.Fatalf("task moved despite running session: %s", task.WorkflowStepID)
	}
}

func TestService_MoveTaskWithOptionsAllowsRunningPrimarySession(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-running-primary", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-running-primary", "task-running-primary", models.TaskSessionStateRunning, models.ReviewStatusNone)
	eventBus.ClearEvents()

	moved, err := svc.MoveTaskWithOptions(ctx, "task-running-primary", "wf-source", "step-review-target", 0, MoveTaskOptions{
		AllowActivePrimarySession: true,
	})
	if err != nil {
		t.Fatalf("running primary session should be movable with explicit option: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-review-target" {
		t.Fatalf("expected step-review-target, got %s", moved.Task.WorkflowStepID)
	}

	event := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskMoved)
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map[string]interface{}", event.Data)
	}
	if got := data["session_id"]; got != "session-running-primary" {
		t.Fatalf("session_id = %v, want session-running-primary", got)
	}
	transitionID, ok := data["step_transition_id"].(int64)
	if !ok || transitionID == 0 {
		t.Fatalf("step_transition_id = %v (%T), want a positive ledger identifier", data["step_transition_id"], data["step_transition_id"])
	}
}

func TestService_MoveTaskWithEntryOptionsPersistsPrivateEntryAndPublishesMoveID(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-entry-options", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-entry-options", "task-entry-options", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)
	store := setSQLiteMoveEntryStore(t, svc, repo)
	eventBus.ClearEvents()

	moved, err := svc.MoveTaskWithOptions(ctx, "task-entry-options", "wf-source", "step-review-target", 0, MoveTaskOptions{
		EntryOptions: &workflowmove.EntryOptions{
			ResetContext:   true,
			Instructions:   "Create the PR ready for review.",
			AgentProfileID: "profile-qa",
		},
	})
	if err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if moved.MoveID == "" {
		t.Fatal("MoveID is empty")
	}
	entry, err := store.Load(ctx, moved.MoveID)
	if err != nil || entry == nil || entry.TaskID != "task-entry-options" {
		t.Fatalf("saved entry = (%+v, %v), want one entry for move %q", entry, err, moved.MoveID)
	}
	if entry.Options.Instructions != "Create the PR ready for review." {
		t.Fatalf("saved options = %+v", entry.Options)
	}

	event := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskMoved)
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map[string]interface{}", event.Data)
	}
	if got := data["move_id"]; got != moved.MoveID {
		t.Fatalf("event move_id = %v, want %q", got, moved.MoveID)
	}
	if _, exposed := data["entry_options"]; exposed {
		t.Fatal("task.moved event exposed private entry options")
	}
}

func TestService_MoveTaskUsesAtomicRepositoryForSharedMoveEntryStore(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-entry-atomic", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-entry-atomic", "task-entry-atomic", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)
	if _, err := repo.DB().ExecContext(ctx, `
		CREATE TABLE workflow_move_entries (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL DEFAULT '',
			options_json TEXT NOT NULL DEFAULT '{}'
		)
	`); err != nil {
		t.Fatalf("create workflow_move_entries: %v", err)
	}
	store := &atomicAdmissionMoveEntryStore{}
	svc.SetMoveEntryStore(store)
	atomicRepo := &recordingAtomicMoveRepository{TaskRepository: repo, delegate: repo}
	svc.tasks = atomicRepo

	moved, err := svc.MoveTaskWithOptions(ctx, "task-entry-atomic", "wf-source", "step-review-target", 0, MoveTaskOptions{
		EntryOptions: &workflowmove.EntryOptions{ResetContext: true, Instructions: "handoff"},
	})
	if err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if store.saveCalls != 0 {
		t.Fatalf("entry store Save calls = %d, want repository-owned transaction", store.saveCalls)
	}
	if len(atomicRepo.entries) != 1 || atomicRepo.entries[0].ID != moved.MoveID {
		t.Fatalf("atomic repository entries = %#v, want move %q", atomicRepo.entries, moved.MoveID)
	}
}

func TestService_MoveTaskRejectsIncompatibleAtomicEntryStoreBeforeMutation(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-entry-owner-mismatch", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-entry-owner-mismatch", "task-entry-owner-mismatch", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)
	raw, err := sql.Open("sqlite3", "file:workflow-move-owner-mismatch?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open separate entry database: %v", err)
	}
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = raw.Close() })
	otherDB := sqlx.NewDb(raw, "sqlite3")
	store, err := workflowmove.NewSQLiteEntryStore(otherDB, otherDB)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	svc.SetMoveEntryStore(store)

	_, err = svc.MoveTaskWithOptions(ctx, "task-entry-owner-mismatch", "wf-source", "step-review-target", 0, MoveTaskOptions{
		EntryOptions: &workflowmove.EntryOptions{Instructions: "handoff"},
	})
	if !errors.Is(err, workflowmove.ErrEntryStoreUnavailable) {
		t.Fatalf("MoveTaskWithOptions error = %v, want ErrEntryStoreUnavailable", err)
	}
	stored, err := repo.GetTask(ctx, "task-entry-owner-mismatch")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.WorkflowStepID != "step-source" {
		t.Fatalf("task moved to %q despite ownership mismatch", stored.WorkflowStepID)
	}
	var count int
	if err := otherDB.GetContext(ctx, &count, `SELECT COUNT(*) FROM workflow_move_entries`); err != nil {
		t.Fatalf("count separate entries: %v", err)
	}
	if count != 0 {
		t.Fatalf("separate entry store rows = %d, want 0", count)
	}
}

func TestService_MoveTaskWithEntryOptionsRejectsSessionlessNonAutoStartTarget(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-entry-without-target", "wf-source", "step-source", nil)
	svc.SetMoveEntryStore(&recordingMoveEntryStore{})

	_, err := svc.MoveTaskWithOptions(ctx, "task-entry-without-target", "wf-source", "step-review-target", 0, MoveTaskOptions{
		EntryOptions: &workflowmove.EntryOptions{Instructions: "Send this to a target session."},
	})
	if !errors.Is(err, workflowmove.ErrEntryTargetUnavailable) {
		t.Fatalf("error = %v, want ErrEntryTargetUnavailable", err)
	}
	task, getErr := repo.GetTask(ctx, "task-entry-without-target")
	if getErr != nil {
		t.Fatalf("GetTask: %v", getErr)
	}
	if task.WorkflowStepID != "step-source" {
		t.Fatalf("task moved despite unavailable entry target: %s", task.WorkflowStepID)
	}
}

func TestService_MoveTaskWithEntryOptionsPreflightFailureLeavesTaskUnchanged(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "profile unavailable", err: workflowmove.ErrProfileUnavailable},
		{name: "entry target unavailable", err: workflowmove.ErrEntryTargetUnavailable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedMoveWorkflows(t, ctx, repo)
			seedMoveSteps(svc)
			taskID := "task-entry-preflight-" + tc.name
			createMoveTask(t, ctx, repo, taskID, "wf-source", "step-source", nil)
			createMoveSession(t, ctx, repo, "session-"+taskID, taskID, models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)
			store := &recordingMoveEntryStore{}
			svc.SetMoveEntryStore(store)
			svc.SetMoveEntryPreflightValidator(func(context.Context, *models.Task, *wfmodels.WorkflowStep, *workflowmove.EntryOptions) error {
				return tc.err
			})

			_, err := svc.MoveTaskWithOptions(ctx, taskID, "wf-source", "step-review-target", 0, MoveTaskOptions{
				EntryOptions: &workflowmove.EntryOptions{Instructions: "must not commit"},
			})
			if !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want %v", err, tc.err)
			}
			task, getErr := repo.GetTask(ctx, taskID)
			if getErr != nil {
				t.Fatalf("GetTask: %v", getErr)
			}
			if task.WorkflowStepID != "step-source" || task.WorkflowID != "wf-source" {
				t.Fatalf("task moved despite preflight failure: workflow=%q step=%q", task.WorkflowID, task.WorkflowStepID)
			}
			if len(store.entries) != 0 {
				t.Fatalf("private entries = %+v, want none after preflight failure", store.entries)
			}
		})
	}
}

func TestService_MoveTaskQueuesFullWIPLimitedTarget(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	setFakeWorkflowStepGetter(svc, &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-full":   {ID: "step-full", WorkflowID: "wf-source", Name: "Full", Position: 1, WIPLimit: 1},
	}})
	createMoveTask(t, ctx, repo, "task-moving", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-occupant", "wf-source", "step-full", nil)

	moved, err := svc.MoveTask(ctx, "task-moving", "wf-source", "step-full", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	if moved.Task.WIPAdmitted {
		t.Fatal("overflow move consumed WIP capacity")
	}
	if moved.Task.QueuedForStepID != "step-full" {
		t.Fatalf("queued_for_step_id = %q, want step-full", moved.Task.QueuedForStepID)
	}
	if moved.Task.QueuedAt == nil {
		t.Fatal("overflow move did not record queued_at")
	}

	task, err := repo.GetTask(ctx, "task-moving")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.WorkflowStepID != "step-full" {
		t.Fatalf("workflow_step_id = %s, want step-full", task.WorkflowStepID)
	}
	event := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskMoved)
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map[string]interface{}", event.Data)
	}
	if admitted, _ := data["wip_admitted"].(bool); admitted {
		t.Fatal("task.moved event marked queued move as admitted")
	}
	if got := data["queued_for_step_id"]; got != "step-full" {
		t.Fatalf("task.moved queued_for_step_id = %v, want step-full", got)
	}
	if data["queued_at"] == nil {
		t.Fatal("task.moved event omitted queued_at")
	}
}

func TestService_MoveTaskWithEntryOptionsQueuesFullTarget(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	setFakeWorkflowStepGetter(svc, &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-full":   {ID: "step-full", WorkflowID: "wf-source", Name: "Full", Position: 1, WIPLimit: 1},
	}})
	createMoveTask(t, ctx, repo, "task-entry-queued", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-entry-occupant", "wf-source", "step-full", nil)
	createMoveSession(t, ctx, repo, "session-entry-queued", "task-entry-queued", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)
	store := setSQLiteMoveEntryStore(t, svc, repo)

	options := &workflowmove.EntryOptions{
		ResetContext:   true,
		Instructions:   "Run the focused workflow tests before handoff.",
		AgentProfileID: "profile-qa",
	}
	moved, err := svc.MoveTaskWithOptions(ctx, "task-entry-queued", "wf-source", "step-full", 0, MoveTaskOptions{
		EntryOptions: options,
	})
	if err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-full" || moved.Task.WIPAdmitted {
		t.Fatalf("moved task placement = step=%q admitted=%v, want step-full/queued", moved.Task.WorkflowStepID, moved.Task.WIPAdmitted)
	}
	if moved.Task.QueuedForStepID != "step-full" || moved.Task.QueuedAt == nil {
		t.Fatalf("moved task queue metadata = destination=%q queued_at=%v, want destination queue", moved.Task.QueuedForStepID, moved.Task.QueuedAt)
	}
	entry, err := store.Load(ctx, moved.MoveID)
	if err != nil || entry == nil || entry.ID != moved.MoveID {
		t.Fatalf("saved entry = (%+v, %v), want move %q", entry, err, moved.MoveID)
	}
	if entry.Options != *options {
		t.Fatalf("saved options = %+v, want %+v", entry.Options, *options)
	}
}

func TestService_ApproveSessionQueuesFullWIPLimitedTarget(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	sourceStep := &wfmodels.WorkflowStep{
		ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
			Type: wfmodels.OnTurnCompleteMoveToStep,
			Config: map[string]interface{}{
				"step_id": "step-full",
			},
		}}},
	}
	setFakeWorkflowStepGetter(svc, &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": sourceStep,
		"step-full":   {ID: "step-full", WorkflowID: "wf-source", Name: "Full", Position: 1, WIPLimit: 1},
	}})
	createMoveTask(t, ctx, repo, "task-approve", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-occupant", "wf-source", "step-full", nil)
	createMoveSession(t, ctx, repo, "session-approve", "task-approve", models.TaskSessionStateWaitingForInput, models.ReviewStatusPending)

	result, err := svc.ApproveSession(ctx, "session-approve")
	if err != nil {
		t.Fatalf("ApproveSession: %v", err)
	}
	if result.Task == nil {
		t.Fatal("approval result did not include task")
	}
	if result.Task.WIPAdmitted {
		t.Fatal("approval overflow move consumed WIP capacity")
	}
	if result.Task.QueuedForStepID != "step-full" {
		t.Fatalf("queued_for_step_id = %q, want step-full", result.Task.QueuedForStepID)
	}

	task, err := repo.GetTask(ctx, "task-approve")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.WorkflowStepID != "step-full" {
		t.Fatalf("workflow_step_id = %s, want step-full", task.WorkflowStepID)
	}

	session, err := repo.GetTaskSession(ctx, "session-approve")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.ReviewStatus != models.ReviewStatusApproved {
		t.Fatalf("review status = %q, want approved after queued approval", session.ReviewStatus)
	}
}

func TestService_MoveTaskAllowsSameStepReorderWhenStepAlreadyOverLimit(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	setFakeWorkflowStepGetter(svc, &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-full": {ID: "step-full", WorkflowID: "wf-source", Name: "Full", Position: 0, WIPLimit: 1},
	}})
	createMoveTask(t, ctx, repo, "task-moving", "wf-source", "step-full", nil)
	createMoveTask(t, ctx, repo, "task-occupant", "wf-source", "step-full", nil)

	moved, err := svc.MoveTask(ctx, "task-moving", "wf-source", "step-full", 5)
	if err != nil {
		t.Fatalf("same-step reorder should be exempt from WIP limit: %v", err)
	}
	if moved.Task.Position != 5 {
		t.Fatalf("position = %d, want 5", moved.Task.Position)
	}
}

func TestService_MoveTaskIgnoresArchivedAndEphemeralOccupantsForWIPLimit(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	setFakeWorkflowStepGetter(svc, &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source":  {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-limited": {ID: "step-limited", WorkflowID: "wf-source", Name: "Limited", Position: 1, WIPLimit: 1},
	}})
	now := time.Now().UTC()
	createMoveTask(t, ctx, repo, "task-moving", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-archived", "wf-source", "step-limited", &now)
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-ephemeral",
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-source",
		WorkflowStepID: "step-limited",
		Title:          "Ephemeral",
		State:          v1.TaskStateTODO,
		Priority:       "medium",
		IsEphemeral:    true,
	}); err != nil {
		t.Fatalf("CreateTask(ephemeral): %v", err)
	}

	moved, err := svc.MoveTask(ctx, "task-moving", "wf-source", "step-limited", 0)
	if err != nil {
		t.Fatalf("archived/ephemeral occupants should not consume WIP: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-limited" {
		t.Fatalf("step = %s, want step-limited", moved.Task.WorkflowStepID)
	}
}

func TestService_MoveTaskPullsNextFeederTaskOnVacate(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	setFakeWorkflowStepGetter(svc, &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-limited": {
			ID: "step-limited", WorkflowID: "wf-source", Name: "Limited", Position: 0,
			WIPLimit: 1, PullFromStepID: "step-feeder",
		},
		"step-feeder": {ID: "step-feeder", WorkflowID: "wf-source", Name: "Feeder", Position: 1},
		"step-target": {ID: "step-target", WorkflowID: "wf-target", Name: "Target", Position: 0},
	}})
	createMoveTask(t, ctx, repo, "task-vacating", "wf-source", "step-limited", nil)
	createMoveTask(t, ctx, repo, "task-low", "wf-source", "step-feeder", nil)
	createMoveTask(t, ctx, repo, "task-critical", "wf-source", "step-feeder", nil)
	setMoveTaskOrder(t, ctx, repo, "task-low", 0, "low")
	setMoveTaskOrder(t, ctx, repo, "task-critical", 0, "critical")
	eventBus.ClearEvents()

	_, err := svc.MoveTask(ctx, "task-vacating", "wf-target", "step-target", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	pulled, err := repo.GetTask(ctx, "task-critical")
	if err != nil {
		t.Fatalf("GetTask(task-critical): %v", err)
	}
	if pulled.WorkflowStepID != "step-limited" {
		t.Fatalf("critical feeder task step = %s, want step-limited", pulled.WorkflowStepID)
	}
	notPulled, err := repo.GetTask(ctx, "task-low")
	if err != nil {
		t.Fatalf("GetTask(task-low): %v", err)
	}
	if notPulled.WorkflowStepID != "step-feeder" {
		t.Fatalf("low feeder task step = %s, want step-feeder", notPulled.WorkflowStepID)
	}

	movedEvents := 0
	queuePromotedEvents := 0
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == events.TaskMoved {
			movedEvents++
		}
		if event.Type == events.TaskQueuePromoted {
			queuePromotedEvents++
		}
	}
	if movedEvents != 2 {
		t.Fatalf("task.moved events = %d, want 2", movedEvents)
	}
	if queuePromotedEvents != 0 {
		t.Fatalf("feeder promotion queue-promoted events = %d, want 0", queuePromotedEvents)
	}
}

func TestService_MoveTaskPullSkipsBlockedFeederCandidate(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	setFakeWorkflowStepGetter(svc, &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-limited": {
			ID: "step-limited", WorkflowID: "wf-source", Name: "Limited", Position: 0,
			WIPLimit: 1, PullFromStepID: "step-feeder",
		},
		"step-feeder": {ID: "step-feeder", WorkflowID: "wf-source", Name: "Feeder", Position: 1},
		"step-target": {ID: "step-target", WorkflowID: "wf-target", Name: "Target", Position: 0},
	}})
	createMoveTask(t, ctx, repo, "task-vacating", "wf-source", "step-limited", nil)
	createMoveTask(t, ctx, repo, "task-blocked", "wf-source", "step-feeder", nil)
	createMoveTask(t, ctx, repo, "task-eligible", "wf-source", "step-feeder", nil)
	setMoveTaskOrder(t, ctx, repo, "task-blocked", 0, "critical")
	setMoveTaskOrder(t, ctx, repo, "task-eligible", 1, "medium")
	createMoveSession(t, ctx, repo, "session-blocked", "task-blocked", models.TaskSessionStateRunning, models.ReviewStatusNone)

	_, err := svc.MoveTask(ctx, "task-vacating", "wf-target", "step-target", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	blocked, err := repo.GetTask(ctx, "task-blocked")
	if err != nil {
		t.Fatalf("GetTask(task-blocked): %v", err)
	}
	if blocked.WorkflowStepID != "step-feeder" {
		t.Fatalf("blocked task step = %s, want step-feeder", blocked.WorkflowStepID)
	}
	eligible, err := repo.GetTask(ctx, "task-eligible")
	if err != nil {
		t.Fatalf("GetTask(task-eligible): %v", err)
	}
	if eligible.WorkflowStepID != "step-limited" {
		t.Fatalf("eligible task step = %s, want step-limited", eligible.WorkflowStepID)
	}
}

func TestService_MoveTaskRejectsArchivedTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	now := time.Now().UTC()
	createMoveTask(t, ctx, repo, "task-archived", "wf-source", "step-source", &now)

	_, err := svc.MoveTask(ctx, "task-archived", "wf-source", "step-review-target", 0)
	if err == nil {
		t.Fatalf("expected archived task move to be rejected")
	}
}

func TestService_MoveTaskMovedEventIncludesSourceWorkflow(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-cross-workflow", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	_, err := svc.MoveTask(ctx, "task-cross-workflow", "wf-target", "step-target", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	updatedEvent := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskUpdated)
	updatedData, ok := updatedEvent.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("updated event data type = %T, want map[string]interface{}", updatedEvent.Data)
	}
	if got := updatedData["old_workflow_id"]; got != "wf-source" {
		t.Fatalf("old_workflow_id = %v, want wf-source", got)
	}

	event := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskMoved)
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map[string]interface{}", event.Data)
	}
	if got := data["from_workflow_id"]; got != "wf-source" {
		t.Fatalf("from_workflow_id = %v, want wf-source", got)
	}
	if got := data["to_workflow_id"]; got != "wf-target" {
		t.Fatalf("to_workflow_id = %v, want wf-target", got)
	}
}

func TestService_BulkMoveTasksUpdatedEventIncludesSourceWorkflow(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	createMoveTask(t, ctx, repo, "task-bulk-cross-workflow", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	_, err := svc.BulkMoveTasks(ctx, "wf-source", "", "wf-target", "step-target")
	if err != nil {
		t.Fatalf("BulkMoveTasks: %v", err)
	}

	updatedEvent := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskUpdated)
	updatedData, ok := updatedEvent.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("updated event data type = %T, want map[string]interface{}", updatedEvent.Data)
	}
	if got := updatedData["old_workflow_id"]; got != "wf-source" {
		t.Fatalf("old_workflow_id = %v, want wf-source", got)
	}
}

func TestService_BulkMoveTasksToTerminalStepCompletesTasks(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
	}
	createMoveTask(t, ctx, repo, "task-bulk-terminal", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	_, err := svc.BulkMoveTasks(ctx, "wf-source", "step-source", "wf-source", "step-done")
	if err != nil {
		t.Fatalf("BulkMoveTasks: %v", err)
	}

	task, err := repo.GetTask(ctx, "task-bulk-terminal")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("bulk-moved task state = %q, want COMPLETED", task.State)
	}
	findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskStateChanged)
}

func TestService_BulkMoveTasksToTerminalStepPreservesTerminalFailureStates(t *testing.T) {
	cases := []struct {
		name  string
		state v1.TaskState
	}{
		{name: "failed", state: v1.TaskStateFailed},
		{name: "cancelled", state: v1.TaskStateCancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedMoveWorkflows(t, ctx, repo)
			seedMoveSteps(svc)
			getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
			getter.steps["step-done"] = &wfmodels.WorkflowStep{
				ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
			}
			createMoveTask(t, ctx, repo, "task-bulk-terminal-"+tc.name, "wf-source", "step-source", nil)
			task, err := repo.GetTask(ctx, "task-bulk-terminal-"+tc.name)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			task.State = tc.state
			must(t, repo.UpdateTask(ctx, task))

			_, err = svc.BulkMoveTasks(ctx, "wf-source", "step-source", "wf-source", "step-done")
			if err != nil {
				t.Fatalf("BulkMoveTasks: %v", err)
			}

			task, err = repo.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if task.State != tc.state {
				t.Fatalf("bulk-moved task state = %q, want %q", task.State, tc.state)
			}
		})
	}
}

func TestService_BulkMoveSelectedTasksValidatesBatchBeforeMoving(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-batch-ok", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-batch-running", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-batch-running", "task-batch-running", models.TaskSessionStateRunning, models.ReviewStatusNone)

	_, err := svc.BulkMoveSelectedTasks(ctx, []string{"task-batch-ok", "task-batch-running"}, "wf-target", "step-target")
	if err == nil {
		t.Fatalf("expected selected batch move to be rejected")
	}

	for _, id := range []string{"task-batch-ok", "task-batch-running"} {
		task, err := repo.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", id, err)
		}
		if task.WorkflowID != "wf-source" || task.WorkflowStepID != "step-source" {
			t.Fatalf("%s moved despite rejected batch: workflow=%s step=%s", id, task.WorkflowID, task.WorkflowStepID)
		}
	}
}

func TestService_BulkMoveSelectedTasksQueuesOverCapacity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	setFakeWorkflowStepGetter(svc, &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-full":   {ID: "step-full", WorkflowID: "wf-source", Name: "Full", Position: 1, WIPLimit: 1},
	}})
	createMoveTask(t, ctx, repo, "task-batch-a", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-batch-b", "wf-source", "step-source", nil)

	result, err := svc.BulkMoveSelectedTasks(ctx, []string{"task-batch-a", "task-batch-b"}, "wf-source", "step-full")
	if err != nil {
		t.Fatalf("BulkMoveSelectedTasks: %v", err)
	}
	if result.MovedCount != 2 {
		t.Fatalf("moved_count = %d, want 2", result.MovedCount)
	}

	for index, id := range []string{"task-batch-a", "task-batch-b"} {
		task, err := repo.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", id, err)
		}
		if task.WorkflowStepID != "step-full" {
			t.Fatalf("%s workflow_step_id = %s, want step-full", id, task.WorkflowStepID)
		}
		if index == 0 && !task.WIPAdmitted {
			t.Fatal("first batch task was not admitted")
		}
		if index == 1 {
			if task.WIPAdmitted {
				t.Fatal("second batch task consumed WIP capacity")
			}
			if task.QueuedForStepID != "step-full" || task.QueuedAt == nil {
				t.Fatalf("second batch task queue metadata = (%q, %v), want destination queue", task.QueuedForStepID, task.QueuedAt)
			}
		}
	}
}

func TestService_BulkMoveSelectedTasksSkipsCurrentTargetAndAppendsInOrder(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-target-existing", "wf-target", "step-target", nil)
	createMoveTask(t, ctx, repo, "task-source-a", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-target-already", "wf-target", "step-target", nil)
	createMoveTask(t, ctx, repo, "task-source-b", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	result, err := svc.BulkMoveSelectedTasks(
		ctx,
		[]string{"task-source-a", "task-target-already", "task-source-b"},
		"wf-target",
		"step-target",
	)
	if err != nil {
		t.Fatalf("BulkMoveSelectedTasks: %v", err)
	}
	if result.MovedCount != 2 {
		t.Fatalf("MovedCount = %d, want 2", result.MovedCount)
	}

	sourceA, err := repo.GetTask(ctx, "task-source-a")
	if err != nil {
		t.Fatalf("GetTask(task-source-a): %v", err)
	}
	sourceB, err := repo.GetTask(ctx, "task-source-b")
	if err != nil {
		t.Fatalf("GetTask(task-source-b): %v", err)
	}
	if sourceA.Position != 2 || sourceB.Position != 3 {
		t.Fatalf("positions = (%d, %d), want (2, 3)", sourceA.Position, sourceB.Position)
	}

	movedEvents := 0
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == events.TaskMoved {
			movedEvents++
		}
	}
	if movedEvents != 2 {
		t.Fatalf("task.moved events = %d, want 2", movedEvents)
	}
}

func seedMoveWorkflows(t *testing.T, ctx context.Context, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
}) {
	t.Helper()
	must(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"}))
	must(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"}))
	must(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-source", WorkspaceID: "ws-1", Name: "Source"}))
	must(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-target", WorkspaceID: "ws-1", Name: "Target"}))
	must(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-other-workspace", WorkspaceID: "ws-2", Name: "Other"}))
}

func seedMoveSteps(svc *Service) {
	setFakeWorkflowStepGetter(svc, &fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source":          {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-review-target":   {ID: "step-review-target", WorkflowID: "wf-source", Name: "Review", Position: 1},
		"step-target":          {ID: "step-target", WorkflowID: "wf-target", Name: "Target", Position: 0},
		"step-other-workspace": {ID: "step-other-workspace", WorkflowID: "wf-other-workspace", Name: "Other", Position: 0},
	}})
}

func createMoveTask(t *testing.T, ctx context.Context, repo interface {
	CreateTask(context.Context, *models.Task) error
	ArchiveTask(context.Context, string) error
}, id, workflowID, stepID string, archivedAt *time.Time) {
	t.Helper()
	must(t, repo.CreateTask(ctx, &models.Task{
		ID:             id,
		WorkspaceID:    "ws-1",
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          id,
		State:          v1.TaskStateTODO,
		ArchivedAt:     archivedAt,
	}))
	if archivedAt != nil {
		must(t, repo.ArchiveTask(ctx, id))
	}
}

func setMoveTaskOrder(t *testing.T, ctx context.Context, repo interface {
	GetTask(context.Context, string) (*models.Task, error)
	UpdateTask(context.Context, *models.Task) error
}, id string, position int, priority string) {
	t.Helper()
	task, err := repo.GetTask(ctx, id)
	if err != nil {
		t.Fatalf("GetTask(%s): %v", id, err)
	}
	task.Position = position
	task.Priority = priority
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask(%s): %v", id, err)
	}
}

func createMoveSession(t *testing.T, ctx context.Context, repo interface {
	CreateTaskSession(context.Context, *models.TaskSession) error
}, id, taskID string, state models.TaskSessionState, reviewStatus models.ReviewStatus) {
	t.Helper()
	must(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:           id,
		TaskID:       taskID,
		State:        state,
		IsPrimary:    true,
		ReviewStatus: reviewStatus,
	}))
}

func findPublishedEvent(t *testing.T, published []*bus.Event, eventType string) *bus.Event {
	t.Helper()
	for _, event := range published {
		if event.Type == eventType {
			return event
		}
	}
	t.Fatalf("event %s not published; got %d events", eventType, len(published))
	return nil
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
