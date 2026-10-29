package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrandLaunch describes the errand an IntentStart launch serves. The errands
// service fills it from its own durable row, never from caller-supplied text.
type ErrandLaunch struct {
	ErrandID        string
	ParentSessionID string
	Title           string
	// SubmissionID is the errand's reserved first-prompt submission identity.
	SubmissionID         string
	MCPExcludedServerIDs []string
}

// errandStartOrigin classifies errand launches as automatic: the parent's agent
// chose to start the work, so the session ceiling queues the errand instead of
// letting it override the ceiling like a user action.
func errandStartOrigin(errand *ErrandLaunch) launchOrigin {
	if errand == nil {
		return ""
	}
	return launchOriginAutomatic
}

// LaunchErrandSession starts one errand session on the task's shared workspace
// with the explicit profile and the reserved identity in req. A ceiling refusal
// returns ErrSessionCeilingRefused and creates nothing.
func (s *Service) LaunchErrandSession(ctx context.Context, req errands.LaunchRequest) (*LaunchSessionResponse, error) {
	if req.ErrandID == "" || req.TaskID == "" || req.ParentSessionID == "" || req.AgentProfileID == "" ||
		req.TurnID == "" || req.SubmissionID == "" || strings.TrimSpace(req.Objective) == "" {
		return nil, errors.New("errand launch requires an errand, task, parent, profile, objective, and reserved identity")
	}
	ctx = models.WithReservedLaunchIdentity(ctx, models.ReservedLaunchIdentity{
		TaskID: req.TaskID, SessionID: req.ErrandID, TurnID: req.TurnID, SubmissionID: req.SubmissionID,
	})
	mcpServerIDs, err := s.enabledErrandMCPServers(ctx, req)
	if err != nil {
		return nil, err
	}
	return s.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:          req.TaskID,
		Intent:          IntentStart,
		AgentProfileID:  req.AgentProfileID,
		ProfileExplicit: true,
		Prompt:          req.Objective,
		MCPServerIDs:    mcpServerIDs,
		Errand: &ErrandLaunch{
			ErrandID: req.ErrandID, ParentSessionID: req.ParentSessionID,
			Title: req.Title, SubmissionID: req.SubmissionID,
			MCPExcludedServerIDs: req.MCPExcludedServerIDs,
		},
	})
}

// enabledErrandMCPServers drops snapshot definitions that were disabled or
// removed after the parent selected them. It returns nil when none remain, so
// the session keeps its default scope.
func (s *Service) enabledErrandMCPServers(ctx context.Context, req errands.LaunchRequest) ([]string, error) {
	if len(req.MCPServerIDs) == 0 || s.mcpSelectionReader == nil {
		return nil, nil
	}
	task, err := s.repo.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, fmt.Errorf("load task for errand MCP selections: %w", err)
	}
	ids := slices.DeleteFunc(slices.Clone(req.MCPServerIDs), func(id string) bool {
		return slices.Contains(req.MCPExcludedServerIDs, id)
	})
	if len(ids) == 0 {
		return nil, nil
	}
	enabled, err := s.mcpSelectionReader.EnabledDefinitionIDs(ctx, task.WorkspaceID, ids)
	if err != nil {
		return nil, fmt.Errorf("resolve errand MCP selections: %w", err)
	}
	if len(enabled) == 0 {
		return nil, nil
	}
	return enabled, nil
}

// deferredLaunchClaimForStart claims the task's start-later intent for an
// ordinary start. An errand start owns no such intent, so it holds an empty
// claim that releases and consumes nothing.
func (s *Service) deferredLaunchClaimForStart(ctx context.Context, taskID string, opts startTaskOptions) *deferredLaunchClaim {
	if opts.Errand != nil {
		return &deferredLaunchClaim{svc: s, taskID: taskID}
	}
	return s.claimDeferredLaunchForStart(ctx, taskID)
}

// withErrandSessionContext makes the executor stamp the errand role and parent
// on the session row it inserts.
func withErrandSessionContext(ctx context.Context, errand *ErrandLaunch) context.Context {
	if errand == nil {
		return ctx
	}
	return executor.WithErrandSession(ctx, errand.ParentSessionID)
}

// configureErrandInitialLaunch binds the errand's reserved submission identity
// to the first prompt.
func (s *Service) configureErrandInitialLaunch(
	ctx context.Context, errand *ErrandLaunch, session *models.TaskSession, launchOptions *executor.LaunchOptions,
) {
	if errand == nil || session == nil {
		return
	}
	s.configureInitialDeliveryAdmission(session, startCreatedSessionOptions{deliverySubmissionID: errand.SubmissionID}, launchOptions)
}

// errandOriginContent renders the trusted first-turn block of an errand from
// config/prompts/errand.md: identity, objective, and the result contract.
func errandOriginContent(errand *ErrandLaunch) string {
	if errand == nil || sysprompt.StripTags(errand.ParentSessionID) == "" {
		return ""
	}
	return sysprompt.FormatErrandContext(errand.ParentSessionID, errand.Title)
}

// applyErrandOriginContext prepends the errand block to the first turn and
// returns the content the first-turn injector must whitelist. It reuses the
// trusted-content slot of spawner attribution, which an errand never has.
func applyErrandOriginContext(prompt, spawnContext string, errand *ErrandLaunch) (string, string) {
	content := errandOriginContent(errand)
	if content == "" {
		return prompt, spawnContext
	}
	return sysprompt.Wrap(content) + "\n\n" + prompt, content
}
