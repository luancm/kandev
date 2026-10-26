package errands

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
)

// TaskWorkspaceResolver returns the workspace that owns a task.
type TaskWorkspaceResolver func(ctx context.Context, taskID string) (string, error)

type busPublisher struct {
	bus         bus.EventBus
	workspaceOf TaskWorkspaceResolver
	logger      *logger.Logger
}

// NewBusPublisher publishes errand.updated on the event bus. The payload is
// the errand DTO plus the owning workspace, which routes it to that
// workspace's clients only. An errand whose workspace cannot be resolved is
// not published.
func NewBusPublisher(eventBus bus.EventBus, workspaceOf TaskWorkspaceResolver, log *logger.Logger) EventPublisher {
	return &busPublisher{bus: eventBus, workspaceOf: workspaceOf, logger: log}
}

func (p *busPublisher) PublishErrandUpdated(ctx context.Context, errand *models.Errand, status models.ErrandStatus) {
	if p.bus == nil {
		return
	}
	workspaceID, err := p.workspaceOf(ctx, errand.TaskID)
	if err != nil || workspaceID == "" {
		p.logger.Warn("errand.updated not published: workspace unresolved",
			zap.String("task_id", errand.TaskID), zap.String("errand_id", errand.ID), zap.Error(err))
		return
	}
	payload := dto.ErrandUpdatedEvent{ErrandDTO: dto.FromErrand(errand, status), WorkspaceID: workspaceID}
	if err := p.bus.Publish(ctx, events.ErrandUpdated, bus.NewEvent(events.ErrandUpdated, "errands", payload)); err != nil {
		p.logger.Warn("errand.updated publish failed",
			zap.String("task_id", errand.TaskID), zap.String("errand_id", errand.ID), zap.Error(err))
	}
}
