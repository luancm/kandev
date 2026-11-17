package lifecycle

import (
	"context"
	"errors"
	"math"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

var ErrErrandPermissionOwnerStale = errors.New("errand permission owner is stale")

// ErrandPermissionTarget identifies the current independent execution and transport.
type ErrandPermissionTarget struct {
	Owner             ExecutionOwner
	ExecutionID       string
	IncarnationID     string
	HarnessGeneration uint64
	StreamID          string
}

// ErrandPermissionSnapshot binds a live list to one execution and connection.
// It is an in-process capability; its contents are not permission authority.
type ErrandPermissionSnapshot struct {
	Permissions []streams.PendingAgentPermission
	target      ErrandPermissionTarget
	execution   *AgentExecution
	client      *agentctl.Client
	stream      *agentctl.PermissionStream
	permissions []streams.PendingAgentPermission
}

func (m *Manager) SnapshotErrandPermissions(ctx context.Context, target ErrandPermissionTarget) (*ErrandPermissionSnapshot, error) {
	if !validErrandPermissionTarget(target) {
		return nil, errandPermissionNotSent(ErrErrandPermissionOwnerStale)
	}
	execution, ok := m.executionStore.Get(target.ExecutionID)
	if !ok {
		return nil, errandPermissionNotSent(ErrExecutionNotFound)
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := m.checkErrandPermissionTarget(requestCtx, execution, client, target); err != nil {
		return nil, errandPermissionNotSent(err)
	}
	stream, err := client.CapturePermissionStream()
	if err != nil {
		return nil, err
	}
	permissions, err := stream.ListPendingPermissions(requestCtx)
	if err != nil {
		return nil, err
	}
	if err = m.checkErrandPermissionTarget(requestCtx, execution, client, target); err != nil {
		return nil, errandPermissionNotSent(err)
	}
	return &ErrandPermissionSnapshot{Permissions: copyErrandPermissions(permissions), permissions: copyErrandPermissions(permissions), target: target, execution: execution, client: client, stream: stream}, nil
}

func (m *Manager) ResolveErrandPermission(ctx context.Context, snapshot *ErrandPermissionSnapshot, source models.ErrandPermissionSource, optionID string) (*streams.PermissionResolveResponse, error) {
	if !snapshot.hasPermission(source.RequestID, source.PendingID, optionID, false) {
		return nil, errandPermissionNotSent(ErrErrandPermissionOwnerStale)
	}
	var response *streams.PermissionResolveResponse
	err := m.withErrandPermissionSnapshot(ctx, snapshot, source, func(ctx context.Context) error {
		var err error
		response, err = snapshot.stream.ResolvePermission(ctx, source.RequestID, source.PendingID, optionID)
		return err
	})
	return response, err
}

func (m *Manager) CancelErrandPermission(ctx context.Context, snapshot *ErrandPermissionSnapshot, source models.ErrandPermissionSource) (*streams.PermissionCancelResponse, error) {
	if !snapshot.hasPermission(source.RequestID, source.PendingID, "", true) {
		return nil, errandPermissionNotSent(ErrErrandPermissionOwnerStale)
	}
	var response *streams.PermissionCancelResponse
	err := m.withErrandPermissionSnapshot(ctx, snapshot, source, func(ctx context.Context) error {
		var err error
		response, err = snapshot.stream.CancelPermission(ctx, source.RequestID, source.PendingID)
		return err
	})
	return response, err
}

func (m *Manager) withErrandPermissionSnapshot(ctx context.Context, snapshot *ErrandPermissionSnapshot, source models.ErrandPermissionSource, call func(context.Context) error) error {
	if snapshot == nil || snapshot.execution == nil || snapshot.stream == nil {
		return errandPermissionNotSent(ErrErrandPermissionOwnerStale)
	}
	client, release := snapshot.execution.AcquireAgentCtlClient()
	defer release()
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if client != snapshot.client {
		return errandPermissionNotSent(ErrErrandPermissionOwnerStale)
	}
	if err := m.checkErrandPermissionTarget(requestCtx, snapshot.execution, client, snapshot.target); err != nil {
		return errandPermissionNotSent(err)
	}
	if err := m.verifyErrandPermissionSource(requestCtx, snapshot.target, source); err != nil {
		return errandPermissionNotSent(err)
	}
	current, exists := m.executionStore.Get(snapshot.target.ExecutionID)
	if !exists || current != snapshot.execution || !snapshot.execution.matchesErrandPermissionTarget(snapshot.target) || !client.RuntimeCurrent() {
		return errandPermissionNotSent(ErrErrandPermissionOwnerStale)
	}
	return call(requestCtx)
}

func (m *Manager) verifyErrandPermissionSource(ctx context.Context, target ErrandPermissionTarget, source models.ErrandPermissionSource) error {
	if m.streamManager == nil {
		return ErrErrandPermissionOwnerStale
	}
	reader, ok := m.streamManager.deliveryRepository().(interface {
		LookupErrandPermissionSource(context.Context, models.ErrandPermissionOwner, string, string) (*models.ErrandPermissionSource, error)
	})
	if !ok {
		return ErrErrandPermissionOwnerStale
	}
	owner := models.ErrandPermissionOwner{ErrandID: target.Owner.ErrandID, TaskID: target.Owner.TaskID, Generation: target.Owner.Generation, ExecutionID: target.ExecutionID}
	if source.Owner != owner {
		return ErrErrandPermissionOwnerStale
	}
	found, err := reader.LookupErrandPermissionSource(ctx, owner, source.RequestID, source.PendingID)
	if err != nil {
		return err
	}
	if found == nil || *found != source {
		return ErrErrandPermissionOwnerStale
	}
	return nil
}

func (m *Manager) checkErrandPermissionTarget(ctx context.Context, execution *AgentExecution, client *agentctl.Client, target ErrandPermissionTarget) error {
	current, exists := m.executionStore.Get(target.ExecutionID)
	if !exists || current != execution || client == nil || !client.RuntimeCurrent() || !m.runtimeExecutionCurrent(execution) || !execution.matchesErrandPermissionTarget(target) {
		return ErrErrandPermissionOwnerStale
	}
	if m.streamManager == nil {
		return ErrErrandPermissionOwnerStale
	}
	reader, ok := m.streamManager.deliveryRepository().(interface {
		ErrandHarnessBindingStale(context.Context, string, *models.ErrandHarnessBinding) (bool, error)
	})
	if !ok {
		return ErrErrandPermissionOwnerStale
	}
	binding := &models.ErrandHarnessBinding{ErrandID: target.Owner.ErrandID, Generation: target.Owner.Generation, ExecutionID: target.ExecutionID, IncarnationID: target.IncarnationID, HarnessGeneration: int64(target.HarnessGeneration), StreamID: target.StreamID}
	stale, err := reader.ErrandHarnessBindingStale(ctx, target.Owner.TaskID, binding)
	if err != nil {
		return err
	}
	if stale || !execution.matchesErrandPermissionTarget(target) {
		return ErrErrandPermissionOwnerStale
	}
	return nil
}

func validErrandPermissionTarget(target ErrandPermissionTarget) bool {
	return target.Owner.Kind == ExecutionOwnerErrand && target.Owner.ErrandID != "" && target.Owner.Generation > 0 && target.Owner.TaskID != "" && target.Owner.WorkspaceID != "" && target.Owner.SessionID == "" && target.ExecutionID != "" && target.IncarnationID != "" && target.HarnessGeneration > 0 && target.HarnessGeneration <= math.MaxInt64 && target.StreamID != ""
}

func (e *AgentExecution) matchesErrandPermissionTarget(target ErrandPermissionTarget) bool {
	if e == nil || e.ID != target.ExecutionID || e.Owner != target.Owner || e.TaskID != "" || e.WorkspaceID != target.Owner.WorkspaceID || e.SessionID != "" || e.DeliveryMode != DurableDeliveryV1 || e.DeliveryIncarnationID != target.IncarnationID || e.DeliveryHarnessGeneration != target.HarnessGeneration || e.DeliveryStreamID != target.StreamID {
		return false
	}
	return true
}

func (s *ErrandPermissionSnapshot) hasPermission(requestID, pendingID, optionID string, cancel bool) bool {
	if s == nil || requestID == "" || pendingID == "" || (!cancel && optionID == "") {
		return false
	}
	for _, permission := range s.permissions {
		if permission.RequestID != requestID || permission.PendingID != pendingID {
			continue
		}
		if cancel {
			return true
		}
		for _, option := range permission.Options {
			if option.OptionID == optionID {
				return true
			}
		}
	}
	return false
}

func copyErrandPermissions(permissions []streams.PendingAgentPermission) []streams.PendingAgentPermission {
	copyOf := append([]streams.PendingAgentPermission(nil), permissions...)
	for i := range copyOf {
		copyOf[i].Options = append([]streams.PermissionChoice(nil), copyOf[i].Options...)
	}
	return copyOf
}

func errandPermissionNotSent(err error) error { return &agentctl.RequestNotSentError{Err: err} }
