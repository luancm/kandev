package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

type sharedKubernetesControlStub struct {
	createRequest *agentctl.CreateInstanceRequest
}

func (c *sharedKubernetesControlStub) CreateInstance(
	_ context.Context,
	request *agentctl.CreateInstanceRequest,
) (*agentctl.CreateInstanceResponse, error) {
	c.createRequest = request
	return &agentctl.CreateInstanceResponse{ID: request.ID, Port: 41001}, nil
}

func (*sharedKubernetesControlStub) GetInstance(
	context.Context,
	string,
) (*agentctl.InstanceInfo, error) {
	return nil, agentctl.ErrInstanceNotFound
}

func TestGetOrCreateSharedKubernetesInstanceRestoresDurableJournalPath(t *testing.T) {
	control := &sharedKubernetesControlStub{}
	req := &ExecutorCreateRequest{
		InstanceID:                "new-execution",
		SessionID:                 "session-1",
		TaskID:                    "task-1",
		TaskEnvironmentID:         "environment-1",
		WorkspacePath:             kubernetesWorkspacePath,
		DurableJournalOwnerID:     "session-1",
		DeliveryIncarnationID:     "incarnation-1",
		DeliveryHarnessGeneration: 2,
		DeliveryStreamID:          "incarnation-1:g2",
		Metadata: map[string]interface{}{
			MetadataKeyKubernetesRuntimeWorkspaceMode: string(kubeexecutor.WorkspaceModeManagedPVC),
		},
	}

	response, created, err := getOrCreateSharedKubernetesInstance(
		context.Background(), control, req, "remote-instance",
	)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, &agentctl.CreateInstanceResponse{ID: "remote-instance", Port: 41001}, response)
	require.NotNil(t, control.createRequest)
	require.Equal(t, "incarnation-1:g2", control.createRequest.DeliveryStreamID)
	require.Equal(t,
		"/workspace/.kandev/agentctl-journals/session-1/delivery.bbolt",
		control.createRequest.DurableJournalPath,
	)
}

type ambiguousForeignSharedControl struct{ gets, deletes int }

func (c *ambiguousForeignSharedControl) GetInstance(context.Context, string) (*agentctl.InstanceInfo, error) {
	c.gets++
	if c.gets == 1 {
		return nil, agentctl.ErrInstanceNotFound
	}
	return &agentctl.InstanceInfo{ID: "remote", Port: 41001, WorkspacePath: kubernetesWorkspacePath, ErrandMCPOwner: errandMCPOwner(ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "foreign", Generation: 1})}, nil
}
func (*ambiguousForeignSharedControl) CreateInstance(context.Context, *agentctl.CreateInstanceRequest) (*agentctl.CreateInstanceResponse, error) {
	return nil, context.DeadlineExceeded
}
func (c *ambiguousForeignSharedControl) DeleteInstance(context.Context, string) error {
	c.deletes++
	return nil
}
func TestSharedKubernetesAmbiguousForeignOwnerDoesNotAuthorizeDeletion(t *testing.T) {
	control := &ambiguousForeignSharedControl{}
	req := &ExecutorCreateRequest{WorkspacePath: kubernetesWorkspacePath, ErrandMCPOwner: errandMCPOwner(ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1})}
	_, created, err := getOrCreateSharedKubernetesInstance(context.Background(), control, req, "remote")
	require.Error(t, err)
	if created {
		require.NoError(t, control.DeleteInstance(context.Background(), "remote"))
	}
	require.Zero(t, control.deletes)
}

type ambiguousMatchingSharedControl struct{ gets int }

func (c *ambiguousMatchingSharedControl) GetInstance(context.Context, string) (*agentctl.InstanceInfo, error) {
	c.gets++
	if c.gets == 1 {
		return nil, agentctl.ErrInstanceNotFound
	}
	return &agentctl.InstanceInfo{ID: "remote", Port: 41001, WorkspacePath: kubernetesWorkspacePath, ErrandMCPOwner: errandMCPOwner(ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1})}, nil
}
func (*ambiguousMatchingSharedControl) CreateInstance(context.Context, *agentctl.CreateInstanceRequest) (*agentctl.CreateInstanceResponse, error) {
	return nil, context.DeadlineExceeded
}
func TestSharedKubernetesAmbiguousMatchingOwnerIsAdoptedWithoutCleanupAuthority(t *testing.T) {
	control := &ambiguousMatchingSharedControl{}
	req := &ExecutorCreateRequest{WorkspacePath: kubernetesWorkspacePath, ErrandMCPOwner: errandMCPOwner(ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1})}
	response, created, err := getOrCreateSharedKubernetesInstance(context.Background(), control, req, "remote")
	require.NoError(t, err)
	require.NotNil(t, response)
	require.False(t, created)
}

type unsupportedErrandControl struct{ gets, creates int }

func (c *unsupportedErrandControl) RequireCapability(context.Context, string) error {
	return errors.New("old helper")
}
func (c *unsupportedErrandControl) GetInstance(context.Context, string) (*agentctl.InstanceInfo, error) {
	c.gets++
	return &agentctl.InstanceInfo{ID: "remote", Port: 41001, WorkspacePath: kubernetesWorkspacePath, ErrandMCPOwner: errandMCPOwner(ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1})}, nil
}
func (c *unsupportedErrandControl) CreateInstance(context.Context, *agentctl.CreateInstanceRequest) (*agentctl.CreateInstanceResponse, error) {
	c.creates++
	return nil, nil
}
func TestSharedKubernetesRejectsOldErrandHelperBeforeReadOrCreate(t *testing.T) {
	control := &unsupportedErrandControl{}
	req := &ExecutorCreateRequest{ErrandMCPOwner: errandMCPOwner(ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1})}
	_, created, err := getOrCreateSharedKubernetesInstance(context.Background(), control, req, "remote")
	require.Error(t, err)
	require.False(t, created)
	require.Zero(t, control.gets)
	require.Zero(t, control.creates)
}

func (*ambiguousForeignSharedControl) RequireCapability(context.Context, string) error  { return nil }
func (*ambiguousMatchingSharedControl) RequireCapability(context.Context, string) error { return nil }
