package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type errandPermissionRuntimeFixture struct {
	runtimeapi.Runtime
	snapshot func(context.Context, runtimeapi.ErrandPermissionTarget) (*runtimeapi.ErrandPermissionSnapshot, error)
	resolve  func(context.Context, *runtimeapi.ErrandPermissionSnapshot, models.ErrandPermissionSource, string) (*streams.PermissionResolveResponse, error)
}

func (f errandPermissionRuntimeFixture) SnapshotErrandPermissions(ctx context.Context, target runtimeapi.ErrandPermissionTarget) (*runtimeapi.ErrandPermissionSnapshot, error) {
	return f.snapshot(ctx, target)
}

func (f errandPermissionRuntimeFixture) ResolveErrandPermission(ctx context.Context, snapshot *runtimeapi.ErrandPermissionSnapshot, source models.ErrandPermissionSource, optionID string) (*streams.PermissionResolveResponse, error) {
	return f.resolve(ctx, snapshot, source, optionID)
}

func TestErrandAdapterResolvesExactOwnedPermission(t *testing.T) {
	for _, tc := range []struct {
		name       string
		humanInput bool
	}{
		{name: "provider action"},
		{name: "native question", humanInput: true},
	} {
		t.Run(tc.name, func(t *testing.T) { exerciseOwnedErrandPermission(t, tc.humanInput) })
	}
}

func exerciseOwnedErrandPermission(t *testing.T, humanInput bool) {
	t.Helper()
	f := newErrandLaunchFixture(t)
	launch := f.request()
	seedOwnedLaunchReceipt(t, f, launch, models.DeliverySubmissionAccepted, "execution", false)
	request := seedErrandPermissionRequest(t, f, launch, humanInput)
	pending, err := f.repo.LatestErrandPendingPermissions(t.Context(), []string{launch.ErrandID})
	require.NoError(t, err)
	require.NotNil(t, pending[launch.ErrandID])
	messageID := pending[launch.ErrandID].ID
	choice := streams.PermissionChoice{OptionID: request.OptionID, Name: "Allow", Kind: streams.PermissionOptionKindAllowOnce}
	snapshot := &runtimeapi.ErrandPermissionSnapshot{Permissions: []streams.PendingAgentPermission{{
		RequestID: request.RequestID, PendingID: request.PendingID, HumanInput: humanInput, Options: []streams.PermissionChoice{choice},
	}}}
	resolutions := 0
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	adapter := NewErrandAdapter(f.svc)
	adapter.SetOwnedLaunchRuntime(errandPermissionRuntimeFixture{
		snapshot: func(_ context.Context, target runtimeapi.ErrandPermissionTarget) (*runtimeapi.ErrandPermissionSnapshot, error) {
			require.Equal(t, runtimeapi.ExecutionOwnerErrand, target.Owner.Kind)
			require.Equal(t, launch.TaskID, target.Owner.TaskID)
			require.Equal(t, launch.ErrandID, target.Owner.ErrandID)
			require.Empty(t, target.Owner.SessionID)
			require.Equal(t, "execution", target.ExecutionID)
			require.Equal(t, "incarnation", target.IncarnationID)
			require.Equal(t, "stream", target.StreamID)
			require.EqualValues(t, 1, target.HarnessGeneration)
			return snapshot, nil
		},
		resolve: func(ctx context.Context, observed *runtimeapi.ErrandPermissionSnapshot, source models.ErrandPermissionSource, optionID string) (*streams.PermissionResolveResponse, error) {
			resolutions++
			require.Same(t, snapshot, observed)
			require.Equal(t, request.OptionID, optionID)
			require.Equal(t, launch.SubmissionID, source.SubmissionID)
			require.Equal(t, launch.TurnID, source.TurnID)
			require.Equal(t, request.PendingID, source.PendingID)
			claimed, err := f.repo.GetErrandMessage(ctx, launch.ErrandID, messageID)
			require.NoError(t, err)
			require.Contains(t, claimed.Metadata, "permission_resolution")
			cancel()
			return &streams.PermissionResolveResponse{
				RequestID: request.RequestID, PendingID: request.PendingID, OptionID: optionID,
				OptionKind: choice.Kind, Status: "resolved",
			}, nil
		},
	}, nil)
	err = adapter.RespondToErrandPermission(ctx, request)
	if humanInput {
		require.ErrorIs(t, err, ErrPermissionHumanInputRequired)
		require.Zero(t, resolutions)
		return
	}
	require.NoError(t, err)
	updated, err := f.repo.GetErrandMessage(t.Context(), launch.ErrandID, messageID)
	require.NoError(t, err)
	require.Equal(t, string(models.PermissionStatusApproved), updated.Metadata["status"])
	require.Empty(t, updated.TaskSessionID)
	require.ErrorIs(t, adapter.RespondToErrandPermission(t.Context(), request), ErrPermissionAlreadyResolved)
	require.Equal(t, 1, resolutions)
}

func seedErrandPermissionRequest(t *testing.T, f *errandLaunchFixture, launch errands.LaunchRequest, humanInput bool) errands.PermissionResponseRequest {
	t.Helper()
	request := errands.PermissionResponseRequest{
		TaskID: launch.TaskID, ErrandID: launch.ErrandID, RequestID: "request", PendingID: "pending", OptionID: "allow",
	}
	agent := streams.AgentEvent{
		Type: streams.EventTypePermissionRequest, RequestID: request.RequestID, PendingID: request.PendingID,
		PermissionTitle: "Allow action", PermissionHumanInput: humanInput,
		PermissionOptions: []streams.PermissionOption{{OptionID: request.OptionID, Name: "Allow", Kind: streams.PermissionOptionKindAllowOnce}},
		TurnID:            launch.TurnID, PromptGeneration: 1,
	}
	payload, err := json.Marshal(agent)
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: models.ErrandExecutionIdentity(launch.ErrandID), TaskID: launch.TaskID, ExecutionID: "execution",
		IncarnationID: "incarnation", HarnessGeneration: 1, OwnerGeneration: 1, StreamID: "stream",
		ExpectedStreamID: "stream", SubmissionID: launch.SubmissionID, Sequence: 1, EventType: agent.Type, Payload: payload,
	}
	received, err := f.repo.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
	require.NoError(t, err)
	require.True(t, received)
	projected, err := f.repo.ProjectAgentDeliveryEvent(t.Context(), event, nil)
	require.NoError(t, err)
	require.True(t, projected)
	return request
}
