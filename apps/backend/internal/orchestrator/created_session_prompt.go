package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// taskPromptInputs carries what the task-mode first turn of a session needs.
type taskPromptInputs struct {
	prompt                   string
	taskID                   string
	sessionID                string
	session                  *models.TaskSession
	dbTask                   *models.Task
	configMode               bool
	titleOwner               bool
	includeCanvasGuidance    bool
	referenceContext         string
	promptReferenceContext   string
	pullRequestTargetContext string
}

// wrapTaskSessionPrompt composes the task-mode first turn of a session. An
// errand session gets its errand block as a trusted block, built the same way
// as first delivery; every other session gets the task context unchanged.
func (s *Service) wrapTaskSessionPrompt(ctx context.Context, in taskPromptInputs) (string, error) {
	errand, err := s.errandLaunchForSession(ctx, in.session)
	if err != nil {
		return "", err
	}
	prompt, errandContext := applyErrandOriginContext(in.prompt, "", errand)
	includeErrands, errandProfiles := s.parentErrandPromptOptions(ctx, in.taskID, in.session)
	return sysprompt.InjectKandevContextWithOptions(in.taskID, in.sessionID, prompt, sysprompt.KandevContextOptions{
		RequiresCompletionSignal:       errand == nil && s.WorkflowStepRequiresCompletionSignal(ctx, in.dbTask.WorkflowStepID),
		IncludeCoordinatorTaskControls: !in.configMode,
		IncludeTaskTitleTool:           !in.configMode && in.titleOwner,
		IncludeCanvasGuidance:          in.includeCanvasGuidance,
		Autopilot:                      in.dbTask.Autopilot,
		IncludeUserQuestionTool:        !in.dbTask.Autopilot && !in.session.IsPassthrough,
		IncludeParentQuestionTool:      in.dbTask.Autopilot && in.dbTask.ParentID != "",
		ErrandSession:                  errand != nil,
		IncludeErrandTools:             includeErrands && errand == nil,
		ErrandProfiles:                 errandProfiles,
	}, in.referenceContext, in.promptReferenceContext, in.pullRequestTargetContext, errandContext), nil
}
