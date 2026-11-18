package lifecycle

import (
	"context"
	"errors"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *KubernetesExecutor) stopErrandInstance(ctx context.Context, row *models.ExecutorRunning, owner agentctltypes.ErrandMCPOwner) error {
	if r.environmentStore == nil || r.secretStore == nil {
		return errors.New("errand stop requires the Kubernetes environment and secret stores")
	}
	envID := getMetadataString(row.Metadata, MetadataKeyKubernetesResourceEnvironmentID)
	podUID := getMetadataString(row.Metadata, MetadataKeyKubernetesPodUID)
	remoteID := getMetadataString(row.Metadata, MetadataKeyKubernetesAgentctlInstanceID)
	workspaceOwnerTaskID := getMetadataString(row.Metadata, MetadataKeyKubernetesResourceTaskID)
	if envID == "" || podUID == "" || remoteID == "" || workspaceOwnerTaskID == "" ||
		row.SessionID != models.ErrandExecutionIdentity(owner.ErrandID) {
		return errors.New("errand stop requires the recorded Kubernetes environment, Pod UID and instance")
	}

	unlock := r.lockInstance(row.AgentExecutionID)
	defer unlock()
	ctx, cancel := kubernetesDurableContext(ctx)
	defer cancel()
	environment, err := r.environmentStore.GetKubernetesEnvironment(ctx, envID)
	if err != nil {
		return err
	}
	if environment.TaskID != workspaceOwnerTaskID || getMetadataString(environment.Metadata, MetadataKeyKubernetesPodUID) != podUID {
		return models.ErrWorkspaceReuseUnsafe
	}
	instance, err := r.taskKubernetesStopInstance(ctx, &ExecutorInstance{
		InstanceID: row.AgentExecutionID, TaskID: workspaceOwnerTaskID, SessionID: row.SessionID, Metadata: row.Metadata,
	}, environment)
	if err != nil {
		return err
	}
	if err := r.stopErrandInRecordedPod(ctx, instance, environment, owner); err != nil {
		return err
	}
	r.closeKubernetesSession(row.AgentExecutionID)
	return nil
}

func (r *KubernetesExecutor) stopErrandInRecordedPod(ctx context.Context, instance *ExecutorInstance, environment *models.KubernetesEnvironment, owner agentctltypes.ErrandMCPOwner) error {
	runtime, err := r.kubernetesCleanupRuntime(instance, nil)
	if err != nil {
		return err
	}
	recorded, identity, err := kubernetesCleanupInventory(instance)
	if err != nil {
		return err
	}
	pod, err := runtime.resources.GetPod(ctx, recorded.namespace, recorded.podName)
	if err != nil {
		return err
	}
	if err := verifyRecordedPod(pod, recorded.namespace, recorded.podName, recorded.podUID, identity); err != nil {
		return err
	}
	token, err := r.secretStore.Reveal(ctx, environment.ControlSecretID)
	if err != nil {
		return err
	}
	if token == "" {
		return errors.New("errand stop requires the Kubernetes control credential")
	}
	forward, control, err := r.connectHealthyKubernetesControl(ctx, runtime, pod)
	if err != nil {
		return err
	}
	defer func() { _ = forward.Close() }()
	defer control.Close()
	control.SetAuthToken(token)
	return control.StopErrandInstance(ctx, recorded.agentctlInstanceID, owner)
}
