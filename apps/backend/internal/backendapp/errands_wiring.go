package backendapp

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
)

type errandAgentProfileLister interface {
	ListAgents(ctx context.Context) ([]*agentsettingsmodels.Agent, error)
	ListAgentProfiles(ctx context.Context, agentID string) ([]*agentsettingsmodels.AgentProfile, error)
}

type errandTaskReader interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
}

type errandExecutorValidator interface {
	ValidateAgentProfileForTaskExecutor(ctx context.Context, taskID, profileID string) error
}

// errandProfileCatalog lists the enabled agent profiles a task's workspace can
// use and validates one against the task's effective executor.
type errandProfileCatalog struct {
	profiles  errandAgentProfileLister
	tasks     errandTaskReader
	validator errandExecutorValidator
}

func (c errandProfileCatalog) ListTaskProfiles(ctx context.Context, taskID string) ([]errands.ProfileInfo, error) {
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("load task for errand profiles: %w", err)
	}
	agents, err := c.profiles.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list agents for errand profiles: %w", err)
	}
	var infos []errands.ProfileInfo
	for _, agent := range agents {
		profiles, err := c.profiles.ListAgentProfiles(ctx, agent.ID)
		if err != nil {
			return nil, fmt.Errorf("list profiles of agent %s: %w", agent.ID, err)
		}
		for _, profile := range profiles {
			if profile.DeletedAt != nil || !profile.Enabled ||
				(profile.WorkspaceID != "" && profile.WorkspaceID != task.WorkspaceID) {
				continue
			}
			infos = append(infos, errands.ProfileInfo{
				ID: profile.ID, Name: profile.Name, Agent: profile.AgentDisplayName, Model: profile.Model,
				Passthrough: profile.CLIPassthrough,
			})
		}
	}
	return infos, nil
}

func (c errandProfileCatalog) ValidateForTaskExecutor(ctx context.Context, taskID, profileID string) error {
	return c.validator.ValidateAgentProfileForTaskExecutor(ctx, taskID, profileID)
}

// wireErrands builds the errands service on the orchestrator, registers its
// lifecycle hooks, and exposes it to the task WebSocket handlers. It returns
// the service for the MCP handlers, or nil when errands are not wired. Queued
// errands and cancellation cascades run regardless of the release toggle, so
// existing errands stay governed when it is off.
func wireErrands(p routeParams, taskHandlers *taskhandlers.TaskHandlers) *errands.Service {
	if p.orchestratorSvc == nil || p.taskRepo == nil {
		return nil
	}
	adapter := orchestrator.NewErrandAdapter(p.orchestratorSvc)
	service, err := errands.New(errands.Dependencies{
		Store:    p.taskRepo,
		Launcher: adapter,
		Stopper:  adapter,
		MCP:      adapter,
		Profiles: errands.NewProfileResolver(errandProfileCatalog{
			profiles: p.agentSettingsRepo, tasks: p.taskRepo, validator: p.taskSvc,
		}),
		Events: errands.NewBusPublisher(p.eventBus, func(ctx context.Context, taskID string) (string, error) {
			task, err := p.taskRepo.GetTask(ctx, taskID)
			if err != nil {
				return "", err
			}
			return task.WorkspaceID, nil
		}, p.log),
		Logger: p.log,
	})
	if err != nil {
		p.log.Error("errands service unavailable", zap.Error(err))
		return nil
	}
	service.SetDeliverer(orchestrator.NewErrandParentDeliverer(p.orchestratorSvc))
	transcript := errandTranscript{repo: p.taskRepo}
	service.SetTurnReader(transcript)
	service.SetDeliveryLog(transcript)
	// features.errands is restart-required, so the boot value is the live value.
	service.SetEnabled(func() bool { return p.features.Errands })
	p.orchestratorSvc.SetErrandController(service)
	p.orchestratorSvc.SetErrandsEnabled(func() bool { return p.features.Errands })
	p.orchestratorSvc.SetParentAwaitEnder(service)
	p.taskSvc.SetTaskErrandCanceller(p.orchestratorSvc)
	service.StartWorkers(p.ctx)
	p.addCleanup(func() error {
		service.StopWorkers()
		return nil
	})
	taskHandlers.SetErrands(service, p.features.Errands)
	return service
}
