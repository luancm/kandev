package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type atomicManualMoveAdmitter interface {
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

type pendingWorkflowMoveCommitter interface {
	CommitPendingWorkflowMove(
		context.Context,
		string,
		string,
		string,
		string,
		string,
		string,
		string,
		int,
		*v1.TaskState,
	) (*models.Task, *workflowmove.EntryOptions, bool, bool, error)
}

// seedCASWorkflowStep inserts a bare workflow step row for the compare-and-
// swap admission tests below, which only need CreateTask's WorkflowStepID
// foreign key to resolve, not a full workflow.
func seedCASWorkflowStep(t *testing.T, repo *Repository, workflowID, stepID string, position int) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`),
		stepID, workflowID, stepID, position, now, now); err != nil {
		t.Fatalf("seed workflow step %s: %v", stepID, err)
	}
}

func ensureWorkflowStepAdmissionColumns(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.db.Exec(`ALTER TABLE workflow_steps ADD COLUMN wip_limit INTEGER NOT NULL DEFAULT 0`); err != nil {
		t.Fatalf("add workflow step admission columns: %v", err)
	}
}

// TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_AppliesOnlyWhenExpectedStepMatches
// is the AC-46/48 functional (non-concurrent) coverage for the new CAS
// repository method: applied=true and the admission/queue semantics of the
// unconditional UpdateTaskWithWorkflowStepAdmission are preserved when the
// precondition holds, and applied=false with the task row left untouched
// when it does not.
func TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_AppliesOnlyWhenExpectedStepMatches(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-cas")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-cas", WorkspaceID: "workspace-cas", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-cas", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-cas", "step-target", 1)

	task := &models.Task{
		ID: "task-cas-1", WorkspaceID: "workspace-cas", WorkflowID: "workflow-cas",
		WorkflowStepID: "step-source", Title: "CAS candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Mismatched expectedStepID: the task has already left "step-source" from
	// this call's point of view (it's still there, but we assert a different
	// precondition) — applied must be false, no error, and the row untouched.
	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, "not-the-current-step", "step-target", 0)
	if err != nil {
		t.Fatalf("UpdateTaskWithWorkflowStepAdmissionIfAtStep (mismatch): %v", err)
	}
	if applied {
		t.Fatalf("expected applied=false when expectedStepID does not match the persisted step")
	}
	reloaded, err := repo.GetTask(ctx, "task-cas-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reloaded.WorkflowStepID != "step-source" {
		t.Fatalf("expected task to remain at step-source after a lost CAS, got %q", reloaded.WorkflowStepID)
	}

	// Matching expectedStepID: applied=true and the move lands, same as the
	// unconditional variant.
	applied, err = repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, "step-source", "step-target", 0)
	if err != nil {
		t.Fatalf("UpdateTaskWithWorkflowStepAdmissionIfAtStep (match): %v", err)
	}
	if !applied {
		t.Fatalf("expected applied=true when expectedStepID matches the persisted step")
	}
	reloaded, err = repo.GetTask(ctx, "task-cas-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reloaded.WorkflowStepID != "step-target" || !reloaded.WIPAdmitted {
		t.Fatalf("expected task admitted at step-target, got %+v", reloaded)
	}
}

func TestUpdateTaskWithWorkflowStepAdmissionAndStateIfAtStepCommitsEntryAndTaskTogether(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ensureWorkflowStepAdmissionColumns(t, repo)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-manual-move-cas")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-manual-move-cas", WorkspaceID: "workspace-manual-move-cas", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-manual-move-cas", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-manual-move-cas", "step-target", 1)
	task := &models.Task{
		ID: "task-manual-move-cas", WorkspaceID: "workspace-manual-move-cas", WorkflowID: "workflow-manual-move-cas",
		WorkflowStepID: "step-source", Title: "Move candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	entryStore, err := workflowmove.NewSQLiteEntryStore(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	admitter, ok := interface{}(repo).(atomicManualMoveAdmitter)
	if !ok {
		t.Fatal("repository does not implement atomic direct move admission")
	}

	desired, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	desired.Position = 4
	desired.Metadata[models.MetaKeyWorkflowMovePending] = map[string]interface{}{
		"from_step_id": "step-source",
		"move_id":      "move-atomic",
	}
	entry := &workflowmove.Entry{
		ID: "move-atomic", TaskID: task.ID,
		Options: workflowmove.EntryOptions{ResetContext: true, Instructions: "handoff", AgentProfileID: "profile-qa"},
	}
	state := v1.TaskStateInProgress
	admitted, applied, err := admitter.UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
		ctx, desired, "step-source", "step-target", 0, &state, true, entry,
	)
	if err != nil || !admitted || !applied {
		t.Fatalf("atomic admission = (%v, %v, %v), want (true, true, nil)", admitted, applied, err)
	}

	storedTask, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after move: %v", err)
	}
	if storedTask.WorkflowStepID != "step-target" || storedTask.Position != 4 {
		t.Fatalf("stored task destination = (%q, %d), want (step-target, 4)", storedTask.WorkflowStepID, storedTask.Position)
	}
	storedEntry, err := entryStore.Load(ctx, entry.ID)
	if err != nil {
		t.Fatalf("Load entry: %v", err)
	}
	if storedEntry == nil || storedEntry.Options != entry.Options {
		t.Fatalf("stored entry = %#v, want %#v", storedEntry, entry)
	}

	loser, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask loser: %v", err)
	}
	loser.Metadata[models.MetaKeyWorkflowMovePending] = map[string]interface{}{"move_id": "move-loser"}
	_, applied, err = admitter.UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
		ctx, loser, "step-source", "step-target", 0, &state, true,
		&workflowmove.Entry{ID: "move-loser", TaskID: task.ID, Options: workflowmove.EntryOptions{Instructions: "must not persist"}},
	)
	if err != nil || applied {
		t.Fatalf("lost admission = (%v, %v), want (false, nil)", applied, err)
	}
	losingEntry, err := entryStore.Load(ctx, "move-loser")
	if err != nil {
		t.Fatalf("Load losing entry: %v", err)
	}
	if losingEntry != nil {
		t.Fatalf("losing entry persisted: %#v", losingEntry)
	}
}

func TestUpdateTaskWithWorkflowStepAdmissionAndStateIfAtStepUsesDirectRebaseWithoutEntry(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ensureWorkflowStepAdmissionColumns(t, repo)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-direct-rebase")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-direct-rebase", WorkspaceID: "workspace-direct-rebase", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-direct-rebase", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-direct-rebase", "step-target", 1)
	task := &models.Task{
		ID: "task-direct-rebase", WorkspaceID: "workspace-direct-rebase", WorkflowID: "workflow-direct-rebase",
		WorkflowStepID: "step-source", Title: "Direct candidate", Position: 1, WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	desired, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	desired.Position = 9
	desired.Metadata[models.MetaKeyManualMoveLifecyclePending] = map[string]interface{}{"from_step_id": "step-source"}
	admitter := interface{}(repo).(atomicManualMoveAdmitter)
	if _, applied, err := admitter.UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
		ctx, desired, "step-source", "step-target", 0, nil, false, nil,
	); err != nil || !applied {
		t.Fatalf("direct admission = (%v, %v), want (true, nil)", applied, err)
	}
	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after direct admission: %v", err)
	}
	if stored.Position != 9 {
		t.Fatalf("position = %d, want 9", stored.Position)
	}
	if _, ok := stored.Metadata[models.MetaKeyManualMoveLifecyclePending]; !ok {
		t.Fatalf("manual move lifecycle marker was dropped: %#v", stored.Metadata)
	}
}

func TestDirectManualMoveUsesAuthoritativeTargetWIPLimit(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ensureWorkflowStepAdmissionColumns(t, repo)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-direct-limit")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-direct-limit", WorkspaceID: "workspace-direct-limit", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-direct-limit", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-direct-limit", "step-target", 1)
	if _, err := repo.db.Exec(`UPDATE workflow_steps SET wip_limit = 1 WHERE id = 'step-target'`); err != nil {
		t.Fatal(err)
	}
	for _, task := range []*models.Task{
		{ID: "task-direct-occupant", WorkspaceID: "workspace-direct-limit", WorkflowID: "workflow-direct-limit", WorkflowStepID: "step-target", Title: "Occupant", WIPAdmitted: true},
		{ID: "task-direct-candidate", WorkspaceID: "workspace-direct-limit", WorkflowID: "workflow-direct-limit", WorkflowStepID: "step-source", Title: "Candidate", WIPAdmitted: true},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	desired, err := repo.GetTask(ctx, "task-direct-candidate")
	if err != nil {
		t.Fatal(err)
	}
	admitter := interface{}(repo).(atomicManualMoveAdmitter)
	admitted, applied, err := admitter.UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
		ctx, desired, "step-source", "step-target", 0, nil, true, nil,
	)
	if err != nil || !applied {
		t.Fatalf("direct admission = (%v, %v), want applied", applied, err)
	}
	if admitted {
		t.Fatal("direct move admitted using stale unlimited caller metadata")
	}
	stored, err := repo.GetTask(ctx, desired.ID)
	if err != nil || stored.QueuedForStepID != "step-target" {
		t.Fatalf("direct move placement = (%#v, %v), want queued", stored, err)
	}
}

func TestDirectManualMoveRejectsTargetMovedToAnotherWorkflow(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ensureWorkflowStepAdmissionColumns(t, repo)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-direct-target-race")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-direct-target-race", WorkspaceID: "workspace-direct-target-race", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-direct-target-race", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-direct-target-race", "step-target", 1)
	task := &models.Task{ID: "task-direct-target-race", WorkspaceID: "workspace-direct-target-race", WorkflowID: "workflow-direct-target-race", WorkflowStepID: "step-source", Title: "Candidate", WIPAdmitted: true}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	desired, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`UPDATE workflow_steps SET workflow_id = 'workflow-replaced' WHERE id = 'step-target'`); err != nil {
		t.Fatal(err)
	}
	admitter := interface{}(repo).(atomicManualMoveAdmitter)
	_, applied, err := admitter.UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
		ctx, desired, "step-source", "step-target", 0, nil, true, nil,
	)
	if err == nil || applied {
		t.Fatalf("target workflow race = (%v, %v), want rejected", applied, err)
	}
	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil || stored.WorkflowStepID != "step-source" {
		t.Fatalf("task after target workflow race = (%#v, %v), want source", stored, err)
	}
}

func TestUpdateTaskWithWorkflowStepAdmissionAndStateIfAtStepRejectsPendingDeferredMove(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-direct-conflict")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-direct-conflict", WorkspaceID: "workspace-direct-conflict", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-direct-conflict", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-direct-conflict", "step-target", 1)
	task := &models.Task{
		ID: "task-direct-conflict", WorkspaceID: "workspace-direct-conflict", WorkflowID: "workflow-direct-conflict",
		WorkflowStepID: "step-source", Title: "Direct candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	if err := queueRepo.SetPendingMove(ctx, "session-deferred", &messagequeue.PendingMove{
		MoveID: "move-deferred", TaskID: task.ID, FromWorkflowID: task.WorkflowID, FromStepID: task.WorkflowStepID,
		WorkflowID: task.WorkflowID, WorkflowStepID: "step-target",
	}); err != nil {
		t.Fatalf("SetPendingMove: %v", err)
	}
	desired, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	desired.Position = 7
	admitter := interface{}(repo).(atomicManualMoveAdmitter)
	if _, applied, err := admitter.UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
		ctx, desired, "step-source", "step-target", 0, nil, true, nil,
	); err != nil || applied {
		t.Fatalf("direct admission with deferred claim = (%v, %v), want (false, nil)", applied, err)
	}
	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after conflict: %v", err)
	}
	if stored.WorkflowStepID != "step-source" || stored.Position == 7 {
		t.Fatalf("conflicting direct move changed task: %#v", stored)
	}
}

func TestUpdateTaskWithWorkflowStepAdmissionAndStateIfAtStepRejectsEntryMarkerMismatch(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-entry-mismatch")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-entry-mismatch", WorkspaceID: "workspace-entry-mismatch", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-entry-mismatch", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-entry-mismatch", "step-target", 1)
	task := &models.Task{
		ID: "task-entry-mismatch", WorkspaceID: "workspace-entry-mismatch", WorkflowID: "workflow-entry-mismatch",
		WorkflowStepID: "step-source", Title: "Move candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	entryStore, err := workflowmove.NewSQLiteEntryStore(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	desired, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	desired.Metadata[models.MetaKeyWorkflowMovePending] = map[string]interface{}{
		"from_step_id": "step-source", "move_id": "different-move",
	}
	entry := &workflowmove.Entry{ID: "move-entry", TaskID: task.ID, Options: workflowmove.EntryOptions{Instructions: "handoff"}}
	admitter := interface{}(repo).(atomicManualMoveAdmitter)
	if _, applied, err := admitter.UpdateTaskWithWorkflowStepAdmissionAndStateIfAtStep(
		ctx, desired, "step-source", "step-target", 0, nil, true, entry,
	); err == nil || applied {
		t.Fatalf("mismatched admission = (%v, %v), want (false, error)", applied, err)
	}
	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after mismatch: %v", err)
	}
	if stored.WorkflowStepID != "step-source" {
		t.Fatalf("mismatched admission moved task to %q", stored.WorkflowStepID)
	}
	storedEntry, err := entryStore.Load(ctx, entry.ID)
	if err != nil {
		t.Fatalf("Load entry: %v", err)
	}
	if storedEntry != nil {
		t.Fatalf("mismatched entry persisted: %#v", storedEntry)
	}
}

func TestCommitPendingWorkflowMoveAtomicallyTransitionsTaskAndConsumesExactClaim(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ensureWorkflowStepAdmissionColumns(t, repo)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-deferred-commit")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-deferred-commit", WorkspaceID: "workspace-deferred-commit", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-deferred-commit", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-deferred-commit", "step-target", 1)
	task := &models.Task{
		ID: "task-deferred-commit", WorkspaceID: "workspace-deferred-commit", WorkflowID: "workflow-deferred-commit",
		WorkflowStepID: "step-source", Title: "Deferred candidate", State: v1.TaskStateInProgress, WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	entryStore, err := workflowmove.NewSQLiteEntryStore(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	wantOptions := workflowmove.EntryOptions{
		ResetContext: true, Instructions: "handoff", AgentProfileID: "profile-qa",
	}
	move := &messagequeue.PendingMove{
		MoveID: "move-deferred-commit", TaskID: task.ID,
		FromWorkflowID: task.WorkflowID, FromStepID: task.WorkflowStepID,
		WorkflowID: task.WorkflowID, WorkflowStepID: "step-target", Position: 6,
		EntryOptions: &wantOptions,
	}
	admitted, err := queueRepo.InsertPendingMoveIfAbsent(ctx, "session-deferred-commit", move)
	if err != nil || !admitted {
		t.Fatalf("InsertPendingMoveIfAbsent = (%v, %v), want (true, nil)", admitted, err)
	}
	committer, ok := interface{}(repo).(pendingWorkflowMoveCommitter)
	if !ok {
		t.Fatal("repository does not implement atomic deferred move commit")
	}
	completed := v1.TaskStateCompleted
	storedTask, options, admitted, applied, err := committer.CommitPendingWorkflowMove(
		ctx, "session-deferred-commit", move.MoveID, move.TaskID,
		move.FromWorkflowID, move.FromStepID, move.WorkflowID, move.WorkflowStepID,
		0, &completed,
	)
	if err != nil || !admitted || !applied {
		t.Fatalf("CommitPendingWorkflowMove = (%v, %v, %v), want (true, true, nil)", admitted, applied, err)
	}
	if storedTask == nil || storedTask.WorkflowStepID != move.WorkflowStepID || storedTask.Position != move.Position || storedTask.State != completed {
		t.Fatalf("committed task = %#v", storedTask)
	}
	if options == nil || *options != wantOptions {
		t.Fatalf("committed options = %#v, want %#v", options, wantOptions)
	}
	marker, ok := storedTask.Metadata[models.MetaKeyWorkflowMovePending].(map[string]interface{})
	if !ok || marker["move_id"] != move.MoveID || marker["from_step_id"] != move.FromStepID {
		t.Fatalf("workflow move marker = %#v", storedTask.Metadata[models.MetaKeyWorkflowMovePending])
	}
	if pending, err := queueRepo.GetPendingMove(ctx, "session-deferred-commit"); err != nil || pending != nil {
		t.Fatalf("pending move after commit = (%#v, %v), want nil", pending, err)
	}
	entry, err := entryStore.Load(ctx, move.MoveID)
	if err != nil || entry == nil || entry.Options != wantOptions {
		t.Fatalf("workflow move entry = (%#v, %v)", entry, err)
	}
	if _, options, admitted, applied, err := committer.CommitPendingWorkflowMove(
		ctx, "session-deferred-commit", move.MoveID, move.TaskID,
		move.FromWorkflowID, move.FromStepID, move.WorkflowID, move.WorkflowStepID,
		0, &completed,
	); err != nil || applied || admitted || options != nil {
		t.Fatalf("replayed commit: options=%#v admitted=%v applied=%v err=%v; want no options, admission, application, or error", options, admitted, applied, err)
	}
}

func TestCommitPendingWorkflowMovePersistsLifecycleMarkerWithoutEntryOptions(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ensureWorkflowStepAdmissionColumns(t, repo)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-deferred-plain")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-deferred-plain", WorkspaceID: "workspace-deferred-plain", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-deferred-plain", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-deferred-plain", "step-target", 1)
	task := &models.Task{
		ID: "task-deferred-plain", WorkspaceID: "workspace-deferred-plain", WorkflowID: "workflow-deferred-plain",
		WorkflowStepID: "step-source", Title: "Deferred candidate", State: v1.TaskStateInProgress, WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	move := &messagequeue.PendingMove{
		MoveID: "move-deferred-plain", TaskID: task.ID,
		FromWorkflowID: task.WorkflowID, FromStepID: task.WorkflowStepID,
		WorkflowID: task.WorkflowID, WorkflowStepID: "step-target",
	}
	if admitted, err := queueRepo.InsertPendingMoveIfAbsent(ctx, "session-deferred-plain", move); err != nil || !admitted {
		t.Fatalf("InsertPendingMoveIfAbsent = (%v, %v), want (true, nil)", admitted, err)
	}
	stored, options, admitted, applied, err := repo.CommitPendingWorkflowMove(
		ctx, "session-deferred-plain", move.MoveID, move.TaskID,
		move.FromWorkflowID, move.FromStepID, move.WorkflowID, move.WorkflowStepID, 0, nil,
	)
	if err != nil || !admitted || !applied || options != nil {
		t.Fatalf("CommitPendingWorkflowMove: options=%#v admitted=%v applied=%v err=%v; want no options, successful admission and application", options, admitted, applied, err)
	}
	descriptor, ok := stored.Metadata[models.MetaKeyManualMoveLifecyclePending].(map[string]interface{})
	if !ok || descriptor["from_step_id"] != move.FromStepID || descriptor["move_id"] != move.MoveID {
		t.Fatalf("manual lifecycle marker = %#v, want exact deferred move descriptor", stored.Metadata[models.MetaKeyManualMoveLifecyclePending])
	}
}

func TestCommitPendingWorkflowMoveUsesAuthoritativeTargetWIPLimit(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-deferred-limit")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-deferred-limit", WorkspaceID: "workspace-deferred-limit", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-deferred-limit", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-deferred-limit", "step-target", 1)
	ensureWorkflowStepAdmissionColumns(t, repo)
	if _, err := repo.db.Exec(`UPDATE workflow_steps SET wip_limit = 1 WHERE id = 'step-target'`); err != nil {
		t.Fatal(err)
	}
	for _, task := range []*models.Task{
		{ID: "task-occupant", WorkspaceID: "workspace-deferred-limit", WorkflowID: "workflow-deferred-limit", WorkflowStepID: "step-target", Title: "Occupant", WIPAdmitted: true},
		{ID: "task-candidate", WorkspaceID: "workspace-deferred-limit", WorkflowID: "workflow-deferred-limit", WorkflowStepID: "step-source", Title: "Candidate", WIPAdmitted: true},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatal(err)
	}
	move := &messagequeue.PendingMove{
		MoveID: "move-authoritative-limit", TaskID: "task-candidate",
		FromWorkflowID: "workflow-deferred-limit", FromStepID: "step-source",
		WorkflowID: "workflow-deferred-limit", WorkflowStepID: "step-target",
	}
	if admitted, err := queueRepo.InsertPendingMoveIfAbsent(ctx, "session-candidate", move); err != nil || !admitted {
		t.Fatalf("InsertPendingMoveIfAbsent = (%v, %v)", admitted, err)
	}
	stored, _, admitted, applied, err := repo.CommitPendingWorkflowMove(
		ctx, "session-candidate", move.MoveID, move.TaskID,
		move.FromWorkflowID, move.FromStepID, move.WorkflowID, move.WorkflowStepID,
		0, nil, // stale caller metadata says unlimited
	)
	if err != nil || !applied {
		t.Fatalf("CommitPendingWorkflowMove = (%v, %v), want applied", applied, err)
	}
	if admitted || stored == nil || stored.QueuedForStepID != "step-target" {
		t.Fatalf("authoritative admission = (%v, %#v), want queued at target", admitted, stored)
	}
}

func TestCommitPendingWorkflowMoveClassifiesPermanentSourceMismatch(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ensureWorkflowStepAdmissionColumns(t, repo)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-deferred-mismatch")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-deferred-mismatch", WorkspaceID: "workspace-deferred-mismatch", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-deferred-mismatch", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-deferred-mismatch", "step-other", 1)
	seedCASWorkflowStep(t, repo, "workflow-deferred-mismatch", "step-target", 2)
	task := &models.Task{ID: "task-deferred-mismatch", WorkspaceID: "workspace-deferred-mismatch", WorkflowID: "workflow-deferred-mismatch", WorkflowStepID: "step-source", Title: "Moved elsewhere", WIPAdmitted: true}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatal(err)
	}
	move := &messagequeue.PendingMove{MoveID: "move-stale-source", TaskID: task.ID, FromWorkflowID: task.WorkflowID, FromStepID: "step-source", WorkflowID: task.WorkflowID, WorkflowStepID: "step-target"}
	if admitted, err := queueRepo.InsertPendingMoveIfAbsent(ctx, "session-stale", move); err != nil || !admitted {
		t.Fatalf("InsertPendingMoveIfAbsent = (%v, %v)", admitted, err)
	}
	task.WorkflowStepID = "step-other"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	_, _, _, applied, err := repo.CommitPendingWorkflowMove(
		ctx, "session-stale", move.MoveID, move.TaskID, move.FromWorkflowID, move.FromStepID,
		move.WorkflowID, move.WorkflowStepID, 0, nil,
	)
	if applied || !errors.Is(err, workflowmove.ErrPermanentPendingMoveMismatch) {
		t.Fatalf("source mismatch = (%v, %v), want permanent mismatch", applied, err)
	}
}

func TestCommitPendingWorkflowMoveRevalidatesTargetInsideTransaction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *Repository)
	}{
		{name: "deleted", mutate: func(t *testing.T, repo *Repository) {
			if _, err := repo.db.Exec(`DELETE FROM workflow_steps WHERE id = 'step-target'`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "moved to another workflow", mutate: func(t *testing.T, repo *Repository) {
			if _, err := repo.db.Exec(`UPDATE workflow_steps SET workflow_id = 'workflow-other' WHERE id = 'step-target'`); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			ensureWorkflowStepAdmissionColumns(t, repo)
			ctx := context.Background()
			seedWorkspace(t, repo, "workspace-target-revalidate")
			if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-target-revalidate", WorkspaceID: "workspace-target-revalidate", Name: "Workflow"}); err != nil {
				t.Fatal(err)
			}
			seedCASWorkflowStep(t, repo, "workflow-target-revalidate", "step-source", 0)
			seedCASWorkflowStep(t, repo, "workflow-target-revalidate", "step-target", 1)
			task := &models.Task{ID: "task-target-revalidate", WorkspaceID: "workspace-target-revalidate", WorkflowID: "workflow-target-revalidate", WorkflowStepID: "step-source", Title: "Candidate", WIPAdmitted: true}
			if err := repo.CreateTask(ctx, task); err != nil {
				t.Fatal(err)
			}
			queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
			if err != nil {
				t.Fatal(err)
			}
			move := &messagequeue.PendingMove{MoveID: "move-target-revalidate", TaskID: task.ID, FromWorkflowID: task.WorkflowID, FromStepID: "step-source", WorkflowID: task.WorkflowID, WorkflowStepID: "step-target"}
			if admitted, err := queueRepo.InsertPendingMoveIfAbsent(ctx, "session-target-revalidate", move); err != nil || !admitted {
				t.Fatalf("InsertPendingMoveIfAbsent = (%v, %v)", admitted, err)
			}
			tc.mutate(t, repo)
			_, _, _, applied, err := repo.CommitPendingWorkflowMove(
				ctx, "session-target-revalidate", move.MoveID, move.TaskID, move.FromWorkflowID,
				move.FromStepID, move.WorkflowID, move.WorkflowStepID, 0, nil,
			)
			if applied || !errors.Is(err, workflowmove.ErrPermanentPendingMoveMismatch) {
				t.Fatalf("target revalidation = (%v, %v), want permanent mismatch", applied, err)
			}
			stored, err := repo.GetTask(ctx, task.ID)
			if err != nil || stored.WorkflowStepID != "step-source" {
				t.Fatalf("task changed after invalid target: (%#v, %v)", stored, err)
			}
			pending, err := queueRepo.GetPendingMove(ctx, "session-target-revalidate")
			if err != nil || pending == nil {
				t.Fatalf("exact row was not retained for orchestrator cleanup: (%#v, %v)", pending, err)
			}
		})
	}
}

func TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_PreservesConcurrentTaskEdits(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-cas-preserve")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-cas-preserve", WorkspaceID: "workspace-cas-preserve", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-cas-preserve", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-cas-preserve", "step-target", 1)

	task := &models.Task{
		ID: "task-cas-preserve", WorkspaceID: "workspace-cas-preserve", WorkflowID: "workflow-cas-preserve",
		WorkflowStepID: "step-source", Title: "original", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	stale, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	current, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask current: %v", err)
	}
	current.Title = "edited concurrently"
	if err := repo.UpdateTask(ctx, current); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, stale, "step-source", "step-target", 0)
	if err != nil {
		t.Fatalf("UpdateTaskWithWorkflowStepAdmissionIfAtStep: %v", err)
	}
	if !applied {
		t.Fatal("expected the step compare-and-swap to apply")
	}
	reloaded, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after CAS: %v", err)
	}
	if reloaded.Title != "edited concurrently" {
		t.Fatalf("title = %q, want concurrent edit preserved", reloaded.Title)
	}
}

// TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_QueuesWhenStepIsFull mirrors
// the unconditional admission method's "limited full target queues instead
// of rejecting" behavior (it never rejects for WIP capacity, only for the
// CAS precondition), proving the CAS variant did not regress that contract.
func TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_QueuesWhenStepIsFull(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-cas-full")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-cas-full", WorkspaceID: "workspace-cas-full", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-cas-full", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-cas-full", "step-target", 1)

	occupant := &models.Task{
		ID: "task-cas-occupant", WorkspaceID: "workspace-cas-full", WorkflowID: "workflow-cas-full",
		WorkflowStepID: "step-target", Title: "Occupant", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, occupant); err != nil {
		t.Fatalf("CreateTask(occupant): %v", err)
	}
	candidate := &models.Task{
		ID: "task-cas-candidate", WorkspaceID: "workspace-cas-full", WorkflowID: "workflow-cas-full",
		WorkflowStepID: "step-source", Title: "Candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, candidate); err != nil {
		t.Fatalf("CreateTask(candidate): %v", err)
	}

	applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, candidate, "step-source", "step-target", 1)
	if err != nil {
		t.Fatalf("UpdateTaskWithWorkflowStepAdmissionIfAtStep: %v", err)
	}
	if !applied {
		t.Fatalf("expected applied=true: the CAS precondition held even though the step is at capacity")
	}
	reloaded, err := repo.GetTask(ctx, "task-cas-candidate")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reloaded.WorkflowStepID != "step-target" {
		t.Fatalf("expected the task to move into step-target (queued, not rejected), got %q", reloaded.WorkflowStepID)
	}
	if reloaded.WIPAdmitted {
		t.Fatalf("expected WIPAdmitted=false for a queued task at a full step")
	}
	if reloaded.QueuedForStepID != "step-target" {
		t.Fatalf("expected QueuedForStepID=step-target, got %q", reloaded.QueuedForStepID)
	}
}

// TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_TaskNotFound preserves the
// ErrTaskNotFound sentinel the unconditional variant guarantees, for a task
// deleted concurrently with the CAS attempt.
func TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_TaskNotFound(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	missing := &models.Task{ID: "task-cas-missing", WorkspaceID: "does-not-exist"}
	_, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, missing, "step-source", "step-target", 0)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}

// TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_ConcurrentRace proves two
// concurrent CAS attempts against the same expected-step never both apply on
// SQLite. Unlike the PostgreSQL row-lock test
// (TestPostgresUpdateTaskWithWorkflowStepAdmission_ConcurrentLastSlot), this
// does not exercise a dialect-specific row lock (SQLite's admission path
// takes no explicit lock — see updateTaskWithWorkflowStepAdmission) but
// SQLite's single-writer semantics still serialize the two write
// transactions, so at most one CAS precondition can observe "step-source"
// and win.
func TestUpdateTaskWithWorkflowStepAdmissionIfAtStep_ConcurrentRace(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-cas-race")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-cas-race", WorkspaceID: "workspace-cas-race", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-cas-race", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-cas-race", "step-target", 1)

	task := &models.Task{
		ID: "task-cas-race", WorkspaceID: "workspace-cas-race", WorkflowID: "workflow-cas-race",
		WorkflowStepID: "step-source", Title: "Race candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	const attempts = 8
	start := make(chan struct{})
	results := make(chan struct {
		applied bool
		err     error
	}, attempts)
	done := make(chan struct{})
	for i := 0; i < attempts; i++ {
		go func() {
			<-start
			// Each goroutine loads its own copy so concurrent in-memory
			// mutation of a shared *models.Task is not itself the source of
			// any observed serialization.
			own, err := repo.GetTask(ctx, "task-cas-race")
			if err != nil {
				results <- struct {
					applied bool
					err     error
				}{false, err}
				return
			}
			applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, own, "step-source", "step-target", 0)
			results <- struct {
				applied bool
				err     error
			}{applied, err}
		}()
	}
	go func() {
		close(start)
		close(done)
	}()
	<-done

	appliedCount := 0
	for i := 0; i < attempts; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("concurrent CAS attempt: %v", r.err)
		}
		if r.applied {
			appliedCount++
		}
	}
	if appliedCount != 1 {
		t.Fatalf("expected exactly one concurrent CAS attempt to apply, got %d", appliedCount)
	}
	reloaded, err := repo.GetTask(ctx, "task-cas-race")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if reloaded.WorkflowStepID != "step-target" {
		t.Fatalf("expected the task to have moved to step-target exactly once, got %q", reloaded.WorkflowStepID)
	}
}
