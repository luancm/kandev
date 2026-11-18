package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/docker"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *DockerExecutor) stopErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	client, _, err := r.ensureClient()
	if err != nil {
		return err
	}
	return r.stopDockerErrandInstance(ctx, client, row, owner)
}

func (r *RemoteDockerExecutor) stopErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	session, err := r.connect(ctx, &ExecutorCreateRequest{Metadata: row.Metadata})
	if err != nil {
		return err
	}
	if session == nil {
		return ErrErrandRecoveryRequiresReconnect
	}
	defer func() { _ = session.close() }()
	if session.endpoints == nil || session.dockerClient == nil {
		return ErrErrandRecoveryRequiresReconnect
	}
	return r.reconnectDelegate(session).stopDockerErrandInstance(ctx, session.dockerClient, row, owner)
}

func (r *DockerExecutor) stopDockerErrandInstance(ctx context.Context, client *docker.Client, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	instanceID := getMetadataString(row.Metadata, MetadataKeyRemoteDockerAgentctlInstanceID)
	containerID := row.ContainerID
	if containerID == "" || instanceID == "" || row.TransientAuthToken == "" {
		return errors.New("errand stop requires recorded Docker container, instance and credentials")
	}
	if metadataID := getMetadataString(row.Metadata, MetadataKeyContainerID); metadataID != "" && metadataID != containerID {
		return errors.New("errand Docker container inventory disagrees")
	}
	info, err := client.GetContainerInfo(ctx, containerID)
	if err != nil {
		return err
	}
	if info.ID != containerID || info.State != containerStateRunning {
		return fmt.Errorf("%w: recorded Docker container is not reachable", ErrErrandRecoveryRequiresReconnect)
	}
	var ip string
	if r.endpoints == nil {
		ip, err = client.GetContainerIP(ctx, containerID)
		if err != nil {
			return err
		}
	}
	host, port, err := r.resolveEndpoint(ctx, client, containerID, AgentCtlPort, ip)
	if err != nil {
		return err
	}
	control := agentctl.NewControlClient(host, port, r.logger, agentctl.WithControlAuthToken(row.TransientAuthToken))
	defer control.Close()
	return control.StopErrandInstance(ctx, instanceID, owner)
}
