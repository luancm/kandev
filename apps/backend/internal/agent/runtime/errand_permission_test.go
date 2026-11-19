package runtime_test

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/task/models"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

type errandPermissionBackend struct {
	*fakeBackend
	snapshot *lifecycle.ErrandPermissionSnapshot
}

func (b *errandPermissionBackend) SnapshotErrandPermissions(_ context.Context, _ lifecycle.ErrandPermissionTarget) (*lifecycle.ErrandPermissionSnapshot, error) {
	return b.snapshot, nil
}
func (b *errandPermissionBackend) ResolveErrandPermission(_ context.Context, snapshot *lifecycle.ErrandPermissionSnapshot, _ models.ErrandPermissionSource, _ string) (*streams.PermissionResolveResponse, error) {
	if snapshot != b.snapshot {
		return nil, errors.New("snapshot changed")
	}
	return &streams.PermissionResolveResponse{Status: "resolved"}, nil
}
func (b *errandPermissionBackend) CancelErrandPermission(_ context.Context, snapshot *lifecycle.ErrandPermissionSnapshot, _ models.ErrandPermissionSource) (*streams.PermissionCancelResponse, error) {
	if snapshot != b.snapshot {
		return nil, errors.New("snapshot changed")
	}
	return &streams.PermissionCancelResponse{Status: "cancelled"}, nil
}

type errandPermissionAPI interface {
	SnapshotErrandPermissions(context.Context, lifecycle.ErrandPermissionTarget) (*lifecycle.ErrandPermissionSnapshot, error)
	ResolveErrandPermission(context.Context, *lifecycle.ErrandPermissionSnapshot, models.ErrandPermissionSource, string) (*streams.PermissionResolveResponse, error)
	CancelErrandPermission(context.Context, *lifecycle.ErrandPermissionSnapshot, models.ErrandPermissionSource) (*streams.PermissionCancelResponse, error)
}

func TestRuntimeErrandPermissionDelegatesOpaqueSnapshot(t *testing.T) {
	backend := &errandPermissionBackend{fakeBackend: newFakeBackend(), snapshot: &lifecycle.ErrandPermissionSnapshot{}}
	api, ok := any(agentruntime.New(backend)).(errandPermissionAPI)
	require.True(t, ok)
	snapshot, err := api.SnapshotErrandPermissions(context.Background(), lifecycle.ErrandPermissionTarget{})
	require.NoError(t, err)
	require.Same(t, backend.snapshot, snapshot)
	_, err = api.ResolveErrandPermission(context.Background(), snapshot, models.ErrandPermissionSource{}, "allow")
	require.NoError(t, err)
	_, err = api.CancelErrandPermission(context.Background(), snapshot, models.ErrandPermissionSource{})
	require.NoError(t, err)
	unsupported, ok := any(agentruntime.New(newFakeBackend())).(errandPermissionAPI)
	require.True(t, ok)
	_, err = unsupported.SnapshotErrandPermissions(context.Background(), lifecycle.ErrandPermissionTarget{})
	require.ErrorIs(t, err, agentruntime.ErrUnsupported)
}
