package orchestrator

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

func TestProcessOnEnterWorkflowMoveResetFailureLeavesEntryReplayable(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-move-reset-failure", "session-move-reset-failure", "step-source")
	session, err := repo.GetTaskSession(ctx, "session-move-reset-failure")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if err := repo.SetSessionMetadataKey(ctx, session.ID, "context_window", map[string]interface{}{
		"size": int64(200000), "used": int64(190000),
	}); err != nil {
		t.Fatalf("seed context window metadata: %v", err)
	}
	session, err = repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}

	stepGetter := newMockStepGetter()
	targetStep := &wfmodels.WorkflowStep{
		ID: "step-target", WorkflowID: "wf1", Name: "Target",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterResetAgentContext},
			{Type: wfmodels.OnEnterSetSessionMode, Config: map[string]interface{}{"mode": "plan"}},
			{Type: wfmodels.OnEnterAutoStartAgent},
		}},
	}
	stepGetter.steps[targetStep.ID] = targetStep
	agentManager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), agentManager)
	svc.repo = failSetSessionMetadataRepo{repoStore: repo}

	db := sqlx.NewDb(repo.DB(), "sqlite3")
	entryStore, err := workflowmove.NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("create move entry store: %v", err)
	}
	svc.SetMoveEntryStore(entryStore)
	const moveID = "move-reset-failure"
	if err := entryStore.Save(ctx, &workflowmove.Entry{
		ID: moveID, TaskID: "task-move-reset-failure",
		Options: workflowmove.EntryOptions{ResetContext: true, Instructions: "retry this move"},
	}); err != nil {
		t.Fatalf("save move entry: %v", err)
	}
	lifecycle := entryStore.(workflowmove.LifecycleStore)
	advanced, err := lifecycle.ClaimPhase(ctx, moveID, workflowmove.EntryPhaseTransitionCommitted, workflowmove.EntryPhaseProfileApplied, session.ID)
	if err != nil || !advanced {
		t.Fatalf("bind move entry to target session = (%v, %v)", advanced, err)
	}

	svc.processOnEnter(withWorkflowMoveEntryID(ctx, moveID), "task-move-reset-failure", session, targetStep, "task description")

	entry, err := lifecycle.Load(ctx, moveID)
	if err != nil {
		t.Fatalf("reload move entry: %v", err)
	}
	if entry == nil || entry.Phase != workflowmove.EntryPhaseProfileApplied {
		t.Fatalf("move entry after reset cleanup failure = %#v, want profile_applied", entry)
	}
	if len(agentManager.restartProcessCalls) != 0 {
		t.Fatalf("lazy reset cleanup failure must not reset a provider, got %d calls", len(agentManager.restartProcessCalls))
	}
	if len(agentManager.setSessionModeCalls) != 0 {
		t.Fatalf("reset cleanup failure must block later on_enter config, got %d mode calls", len(agentManager.setSessionModeCalls))
	}
	if len(agentManager.capturedPromptCalls) != 0 {
		t.Fatalf("reset cleanup failure must block prompt dispatch, got %d prompts", len(agentManager.capturedPromptCalls))
	}
}
