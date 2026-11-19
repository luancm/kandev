//go:build linux

package lifecycle

import (
	"context"
	"net/http"
	"os/exec"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	controlapi "github.com/kandev/kandev/internal/agentctl/server/api"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestOwnedErrandInterruptionReplaysJournalAfterOriginalWriterExits(t *testing.T) {
	ctx := t.Context()
	writer := exec.CommandContext(ctx, "sleep", "60")
	writer.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, writer.Start())
	t.Cleanup(func() {
		if writer.ProcessState == nil {
			_ = writer.Process.Kill()
			_ = writer.Wait()
		}
	})
	identity, err := processidentity.Capture(writer.Process.Pid)
	require.NoError(t, err)

	const taskID, errandID, turnID = "task", "owned-interruption", "interrupted-turn"
	binding := models.ErrandHarnessBinding{
		ErrandID: errandID, Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation",
		HarnessGeneration: 1, StreamID: "owned-stream", NativeSessionID: "native",
	}
	payload := []byte(`{"text":"perform the work"}`)
	submission := &models.AgentDeliverySubmission{
		ID: "prompt:owned", SessionID: models.ErrandExecutionIdentity(errandID), IncarnationID: binding.IncarnationID,
		HarnessGeneration: 1, DispatchAttemptID: binding.ExecutionID, State: models.DeliverySubmissionAccepted,
		Payload: payload, PayloadHash: journal.SubmissionHash(payload),
	}
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Owned interruption"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{
		ID: errandID, TaskID: taskID, ParentSessionID: "parent", Lifecycle: models.ErrandLifecycleLaunched,
		AwaitingResult: true, FirstTurnID: turnID, SubmissionID: submission.ID, ResultState: models.ErrandResultNone,
	}))
	_, err = repo.ReserveErrandExecutionGeneration(ctx, errandID, 0)
	require.NoError(t, err)
	_, err = repo.BindErrandExecution(ctx, errandID, binding.Generation, binding.ExecutionID)
	require.NoError(t, err)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &binding, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, errandID, &models.Turn{ID: turnID, TaskID: taskID}))
	created, err := repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{
		ErrandID: errandID, Generation: binding.Generation, ExecutionID: binding.ExecutionID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: binding.HarnessGeneration, StreamID: binding.StreamID,
		NativeSessionID: binding.NativeSessionID, PromptGeneration: 1, TurnID: turnID, Submission: submission,
		OriginalRuntime: identity,
	})
	require.NoError(t, err)
	require.True(t, created)

	root := t.TempDir()
	seedRetainedOwnedErrandOutput(t, root, binding, submission)
	var reads atomic.Int32
	instances, router := errandInterruptionControl(t, taskID, binding, submission.SessionID)
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/delivery/retained" {
			reads.Add(1)
		}
		router.ServeHTTP(w, r)
	})
	manager := newTestManager(t)
	manager.dataDir = root
	manager.SetRuntimeOwner(runtimeOwnerForTestClient(t, client))
	manager.SetAgentDeliveryRepository(repo)
	manager.recoveryComplete.Store(true)
	require.NoError(t, manager.ReconcileOwnedErrandInterruptions(ctx))
	require.Zero(t, reads.Load())
	_, live := instances.GetInstance(binding.ExecutionID)
	require.True(t, live)

	require.NoError(t, writer.Process.Kill())
	require.Error(t, writer.Wait())
	require.NoError(t, manager.ReconcileOwnedErrandInterruptions(ctx))
	require.EqualValues(t, 1, reads.Load())
	_, live = instances.GetInstance(binding.ExecutionID)
	require.False(t, live)
	target, outcome, err := repo.ReadOwnedErrandLatestTerminal(ctx, errandID)
	require.NoError(t, err)
	require.Equal(t, turnID, target.TurnID)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, outcome)
	messages, _, err := repo.ListErrandMessages(ctx, errandID, "", 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "output retained before interruption", messages[0].Content)
	require.NoError(t, manager.ReconcileOwnedErrandInterruptions(ctx))
	require.EqualValues(t, 1, reads.Load())
	for range 2 {
		require.NoError(t, manager.retireOwnedErrandInterruption(ctx, target))
	}
	location, err := journal.ResolveLocation(root, journal.ErrandStorageOwnerID(binding.ErrandID))
	require.NoError(t, err)
	retained, err := journal.Open(journal.Config{Path: location.Path, ExistingOnly: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = retained.Close() })
	prior, err := retained.GetSubmission(ctx, submission.ID)
	require.NoError(t, err)
	require.True(t, prior.Retired)
	require.Equal(t, journal.SubmissionInterruptedUnknown, prior.State)
	unresolved, err := retained.HasUnresolvedSubmissions(ctx)
	require.NoError(t, err)
	require.False(t, unresolved, "the old submission cannot block an explicit successor prompt")
}

func errandInterruptionControl(t *testing.T, taskID string, binding models.ErrandHarnessBinding, sessionID string) (*instance.Manager, http.Handler) {
	t.Helper()
	cfg := &config.Config{AuthToken: "secret", Ports: config.PortConfig{Base: 39000, Max: 39500}, Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP}}
	log := logger.Default()
	instances := instance.NewManager(cfg, log)
	t.Cleanup(func() { require.NoError(t, instances.Shutdown(context.Background())) })
	instances.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		return http.NotFoundHandler()
	})
	_, err := instances.CreateInstance(t.Context(), &instance.CreateRequest{
		ID: binding.ExecutionID, SessionID: sessionID, WorkspacePath: t.TempDir(),
		ErrandMCPOwner: &agentctltypes.ErrandMCPOwner{TaskID: taskID, ErrandID: binding.ErrandID, Generation: binding.Generation},
	})
	require.NoError(t, err)
	return instances, controlapi.NewControlServer(cfg, instances, log).Router()
}

func seedRetainedOwnedErrandOutput(t *testing.T, root string, binding models.ErrandHarnessBinding, submission *models.AgentDeliverySubmission) {
	t.Helper()
	ctx := t.Context()
	location := journal.CheckStorage(root, journal.ErrandStorageOwnerID(binding.ErrandID))
	retained, err := journal.Open(journal.Config{Path: location.Path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = retained.Close() })
	_, err = retained.PutSubmission(ctx, journal.Submission{
		ID: submission.ID, Hash: submission.PayloadHash, Payload: submission.Payload, SessionID: submission.SessionID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: 1, StreamID: binding.StreamID, State: journal.SubmissionAccepted,
	})
	require.NoError(t, err)
	_, err = retained.Append(ctx, journal.Event{
		SessionID: submission.SessionID, IncarnationID: binding.IncarnationID, HarnessGeneration: 1,
		StreamID: binding.StreamID, SubmissionID: submission.ID, Type: "message_chunk",
		Payload: []byte(`{"type":"message_chunk","session_id":"native","text":"output retained before interruption","prompt_generation":1}`),
	})
	require.NoError(t, err)
	require.NoError(t, retained.Close())
}
