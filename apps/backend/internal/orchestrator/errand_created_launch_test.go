package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	createdErrandSessionID = "created-errand"
	createdErrandObjective = "Inspect the handler behavior."
	createdErrandPrompt    = "Please preserve this exact request."
)

func seedCreatedErrand(t *testing.T, f *errandLaunchFixture, taskState v1.TaskState) *models.TaskSession {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	task.State = taskState
	require.NoError(t, f.repo.UpdateTask(ctx, task))
	f.taskRepo.tasks[f.taskID].State = taskState

	stepGetter := f.svc.workflowStepGetter.(*mockStepGetter)
	stepGetter.steps["step-1"].AgentProfileID = "step-profile"
	stepGetter.steps["step-1"].Prompt = "WORKFLOW-PREFIX-SENTINEL"
	stepGetter.steps["step-1"].WorkflowID = "wf1"

	require.NoError(t, f.repo.CreateErrand(ctx, &models.Errand{
		ID: createdErrandSessionID, TaskID: f.taskID, ParentSessionID: f.parent,
		Title: "Audit", Objective: createdErrandObjective, AgentProfileID: "errand-profile",
		Lifecycle: models.ErrandLifecycleLaunched, ResultState: models.ErrandResultNone,
	}))
	session := &models.TaskSession{
		ID: createdErrandSessionID, TaskID: f.taskID, Role: models.SessionRoleErrand,
		ParentSessionID: f.parent, State: models.TaskSessionStateCreated, AgentProfileID: "errand-profile",
		StartedAt: now, UpdatedAt: now,
	}
	require.NoError(t, f.repo.CreateTaskSession(ctx, session))
	require.NoError(t, f.repo.SetTaskMetadataKey(ctx, f.taskID, models.MetaKeyDeferredLaunch, map[string]interface{}{
		models.DeferredLaunchStartWhenUnblockedKey: true,
		models.DeferredLaunchUserIDKey:             "deferred-owner",
		models.DeferredLaunchRecordRecentUseKey:    true,
	}))
	require.NoError(t, f.repo.SetTaskMetadataKey(ctx, f.taskID, models.MetaKeyAgentTitlePending, true))
	return session
}

func startCreatedErrand(t *testing.T, f *errandLaunchFixture, sessionID string, taskState v1.TaskState, flag bool) *executor.LaunchAgentRequest {
	t.Helper()
	ctx := context.Background()
	seedCreatedErrand(t, f, taskState)
	f.svc.executor.SetErrandsEnabled(func() bool { return flag })
	attachments := []v1.MessageAttachment{{Type: "image", Data: "aW1hZ2U=", MimeType: "image/png", Name: "evidence.png"}}
	reference := v1.EntityReference{
		Version: v1.EntityReferenceVersion,
		Ref:     "kandev://workspace/ws1/task/related", Provider: "kandev", Kind: "task", ID: "related",
		Title: "Related task", URL: "/t/related", Scope: "ws1",
	}
	references := []v1.EntityReference{reference}
	preparedPrompt := AppendEntityReferenceContext(createdErrandPrompt, references)
	acceptedPromptContext := "ACCEPTED-SAVED-PROMPT-SNAPSHOT"
	_, err := f.svc.startCreatedSession(
		ctx, f.taskID, sessionID, "caller-profile", preparedPrompt,
		true, true, true, attachments, references, acceptedPromptContext,
		startCreatedSessionOptions{preserveDirectPrompt: true, promptReferencesPrepared: true},
	)
	require.NoError(t, err)
	require.Len(t, f.manager.launchedWith, 1)
	return f.manager.launchedWith[0]
}

func TestCreatedErrandLaunchStaysOutsideWorkflowOwnership(t *testing.T) {
	for _, flag := range []bool{false, true} {
		for _, taskState := range []v1.TaskState{v1.TaskStateTODO, v1.TaskStateReview} {
			t.Run(stringTaskState(taskState)+"/errands-flag-"+boolString(flag), func(t *testing.T) {
				ctx := context.Background()
				f := newErrandLaunchFixture(t)
				req := startCreatedErrand(t, f, createdErrandSessionID, taskState, flag)

				require.Equal(t, createdErrandSessionID, req.SessionID)
				require.Equal(t, "errand-profile", req.AgentProfileID)
				require.Equal(t, executor.McpModeErrand, req.McpMode)
				require.Equal(t, []v1.MessageAttachment{{Type: "image", Data: "aW1hZ2U=", MimeType: "image/png", Name: "evidence.png"}}, req.Attachments)
				require.Contains(t, req.TaskDescription, createdErrandPrompt)
				require.Equal(t, 1, strings.Count(req.TaskDescription, createdErrandPrompt))
				require.Contains(t, req.TaskDescription, sysprompt.DefaultPlanPrefix())
				require.Contains(t, req.TaskDescription, "ACCEPTED-SAVED-PROMPT-SNAPSHOT")
				require.Equal(t, 1, strings.Count(req.TaskDescription, "ACCEPTED-SAVED-PROMPT-SNAPSHOT"))
				require.Contains(t, req.TaskDescription, EntityReferenceContext([]v1.EntityReference{{
					Version: v1.EntityReferenceVersion,
					Ref:     "kandev://workspace/ws1/task/related", Provider: "kandev", Kind: "task", ID: "related",
					Title: "Related task", URL: "/t/related", Scope: "ws1",
				}}))
				require.Contains(t, req.TaskDescription, "errand of session parent-1")
				require.NotContains(t, req.TaskDescription, "WORKFLOW-PREFIX-SENTINEL")
				require.NotContains(t, strings.ToLower(req.TaskDescription), "step_complete_kandev")

				task, err := f.repo.GetTask(ctx, f.taskID)
				require.NoError(t, err)
				require.Equal(t, taskState, task.State)
				require.Equal(t, "step-1", task.WorkflowStepID)
				deferred, ok := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
				require.True(t, ok)
				require.Equal(t, true, deferred[models.DeferredLaunchStartWhenUnblockedKey])
				require.Equal(t, "deferred-owner", deferred[models.DeferredLaunchUserIDKey])
				require.Equal(t, true, deferred[models.DeferredLaunchRecordRecentUseKey])
				require.Equal(t, true, task.Metadata[models.MetaKeyAgentTitlePending])
				require.NotContains(t, task.Metadata, models.MetaKeyAgentTitleOwnerSessionID)
				f.taskRepo.mu.Lock()
				require.Empty(t, f.taskRepo.stateHistory[f.taskID])
				f.taskRepo.mu.Unlock()

				session, err := f.repo.GetTaskSession(ctx, createdErrandSessionID)
				require.NoError(t, err)
				require.Equal(t, "errand-profile", session.AgentProfileID)
				require.Equal(t, models.SessionRoleErrand, session.Role)
				require.Equal(t, f.parent, session.ParentSessionID)
				require.False(t, session.IsPrimary)
				parent, err := f.repo.GetTaskSession(ctx, f.parent)
				require.NoError(t, err)
				require.True(t, parent.IsPrimary)
				binding, err := f.repo.GetWorkflowSessionBinding(ctx, f.taskID, workflowSessionBindingTargetKey("step-1"))
				require.NoError(t, err)
				require.Nil(t, binding)
			})
		}
	}
}

func TestCreatedOrdinarySessionStillUsesWorkflowProfileAndPrompt(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	now := time.Now().UTC()
	session := &models.TaskSession{
		ID: "ordinary-created", TaskID: f.taskID, State: models.TaskSessionStateCreated,
		AgentProfileID: "ordinary-profile", StartedAt: now, UpdatedAt: now,
	}
	require.NoError(t, f.repo.CreateTaskSession(ctx, session))
	f.svc.workflowStepGetter.(*mockStepGetter).steps["step-1"].Prompt = "WORKFLOW-PREFIX-SENTINEL {{task_prompt}}"
	_, err := f.svc.startCreatedSession(
		ctx, f.taskID, session.ID, "caller-profile", "ordinary request",
		true, false, true, nil, nil, "", startCreatedSessionOptions{},
	)
	require.NoError(t, err)
	require.Len(t, f.manager.launchedWith, 1)
	require.Equal(t, "step-profile", f.manager.launchedWith[0].AgentProfileID)
	require.Contains(t, f.manager.launchedWith[0].TaskDescription, "WORKFLOW-PREFIX-SENTINEL")
	require.Contains(t, f.manager.launchedWith[0].TaskDescription, "ordinary request")
	f.taskRepo.mu.Lock()
	require.Equal(t, []v1.TaskState{v1.TaskStateScheduling}, f.taskRepo.stateHistory[f.taskID])
	f.taskRepo.mu.Unlock()
}

func TestCreatedErrandRequiresItsRetainedProfile(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	session := seedCreatedErrand(t, f, v1.TaskStateTODO)
	session.AgentProfileID = ""
	require.NoError(t, f.repo.UpdateTaskSession(ctx, session))

	_, err := f.svc.startCreatedSession(
		ctx, f.taskID, createdErrandSessionID, "caller-profile", createdErrandPrompt,
		true, false, true, nil, nil, "", startCreatedSessionOptions{},
	)
	require.ErrorContains(t, err, "agent_profile_id is required for errand session")
	require.Empty(t, f.manager.launchedWith)
	f.taskRepo.mu.Lock()
	require.Empty(t, f.taskRepo.stateHistory[f.taskID])
	f.taskRepo.mu.Unlock()
	task, err := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.Contains(t, task.Metadata, models.MetaKeyDeferredLaunch)
}

func TestCreatedErrandPreservesAcceptedEmptyPromptExpansion(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	seedCreatedErrand(t, f, v1.TaskStateTODO)
	promptService := newPromptServiceForLaunchFallbackTest(t)
	preparedPrompt, acceptedContext := promptService.AppendReferenceExpansionsWithContext(
		ctx, "Inspect @future-errand-rules without expanding after acceptance.", nil,
	)
	require.Empty(t, acceptedContext)
	_, err := promptService.CreatePrompt(ctx, "future-errand-rules", "Late errand rule definition.")
	require.NoError(t, err)
	f.svc.promptExpander = promptService

	_, err = f.svc.startCreatedSession(
		ctx, f.taskID, createdErrandSessionID, "caller-profile", preparedPrompt,
		true, false, true, nil, nil, acceptedContext,
		startCreatedSessionOptions{promptReferencesPrepared: true},
	)
	require.NoError(t, err)
	require.Len(t, f.manager.launchedWith, 1)
	require.Contains(t, f.manager.launchedWith[0].TaskDescription, "@future-errand-rules")
	require.NotContains(t, f.manager.launchedWith[0].TaskDescription, "Late errand rule definition.")
	require.Contains(t, f.manager.launchedWith[0].TaskDescription, "errand of session parent-1")
}

func TestCreatedErrandRoleReadErrorAbortsBeforeLaunch(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	seedCreatedErrand(t, f, v1.TaskStateTODO)
	readErr := errors.New("session role unavailable")
	f.svc.repo = &createdErrandSessionReadOverlay{sessionExecutorStore: f.repo, readErr: readErr}

	_, err := f.svc.startCreatedSession(
		ctx, f.taskID, createdErrandSessionID, "caller-profile", createdErrandPrompt,
		true, false, true, nil, nil, "", startCreatedSessionOptions{},
	)
	require.ErrorIs(t, err, readErr)
	require.Empty(t, f.manager.launchedWith)
	task, getErr := f.repo.GetTask(ctx, f.taskID)
	require.NoError(t, getErr)
	require.Equal(t, v1.TaskStateTODO, task.State)
	require.Contains(t, task.Metadata, models.MetaKeyDeferredLaunch)
	f.taskRepo.mu.Lock()
	require.Empty(t, f.taskRepo.stateHistory[f.taskID])
	f.taskRepo.mu.Unlock()
}

func TestCreatedErrandTerminalReloadDoesNotRedirectToOrdinarySibling(t *testing.T) {
	ctx := context.Background()
	f := newErrandLaunchFixture(t)
	seedCreatedErrand(t, f, v1.TaskStateTODO)
	now := time.Now().UTC()
	require.NoError(t, f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "ordinary-sibling", TaskID: f.taskID, State: models.TaskSessionStateWaitingForInput,
		AgentProfileID: "step-profile", StartedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
	}))
	f.svc.repo = &createdErrandSessionReadOverlay{
		sessionExecutorStore: f.repo,
		getSession: func(ctx context.Context, sessionID string, readNumber int) (*models.TaskSession, error) {
			session, err := f.repo.GetTaskSession(ctx, sessionID)
			if err == nil && sessionID == createdErrandSessionID && readNumber == 3 {
				session.State = models.TaskSessionStateCompleted
			}
			return session, err
		},
	}

	_, err := f.svc.startCreatedSession(
		ctx, f.taskID, createdErrandSessionID, "caller-profile", createdErrandPrompt,
		true, false, true, nil, nil, "", startCreatedSessionOptions{},
	)
	var superseded *executor.SessionStateSupersededError
	require.ErrorAs(t, err, &superseded)
	require.Empty(t, f.manager.launchedWith)
	f.taskRepo.mu.Lock()
	require.Empty(t, f.taskRepo.stateHistory[f.taskID])
	f.taskRepo.mu.Unlock()
}

func TestScheduleTaskForSessionErrandPreservesTaskEligibilityAndArchiveGuards(t *testing.T) {
	ctx := context.Background()
	t.Run("ineligible task", func(t *testing.T) {
		f := newErrandLaunchFixture(t)
		seedCreatedErrand(t, f, v1.TaskStateFailed)
		err := f.svc.scheduleTaskForSession(ctx, f.taskID, createdErrandSessionID)
		require.ErrorContains(t, err, "cannot schedule session")
		f.taskRepo.mu.Lock()
		require.Empty(t, f.taskRepo.stateHistory[f.taskID])
		f.taskRepo.mu.Unlock()
	})
	t.Run("archived task", func(t *testing.T) {
		f := newErrandLaunchFixture(t)
		seedCreatedErrand(t, f, v1.TaskStateTODO)
		require.NoError(t, f.repo.ArchiveTask(ctx, f.taskID))
		err := f.svc.scheduleTaskForSession(ctx, f.taskID, createdErrandSessionID)
		require.ErrorIs(t, err, executor.ErrTaskArchived)
		f.taskRepo.mu.Lock()
		require.Empty(t, f.taskRepo.stateHistory[f.taskID])
		f.taskRepo.mu.Unlock()
	})
}

type createdErrandSessionReadOverlay struct {
	sessionExecutorStore
	readErr    error
	reads      int
	getSession func(context.Context, string, int) (*models.TaskSession, error)
}

func (r *createdErrandSessionReadOverlay) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	r.reads++
	if r.readErr != nil {
		return nil, r.readErr
	}
	if r.getSession != nil {
		return r.getSession(ctx, sessionID, r.reads)
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
}

func stringTaskState(state v1.TaskState) string { return string(state) }

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
