package lifecycle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetainedErrandRecoversMissingAcknowledgementFromExactJournalAdmission(t *testing.T) {
	ctx := t.Context()
	repo, _ := runOwnerTestRepository(t)
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1, WorkspaceID: "workspace"}
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: owner.TaskID, Title: "task"}))
	require.NoError(t, repo.CreateErrand(ctx, &models.Errand{
		ID: owner.ErrandID, TaskID: owner.TaskID, ParentSessionID: "parent",
		Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone,
	}))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, owner.ErrandID, 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, owner.ErrandID, owner.Generation, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.CreateErrandTurn(ctx, owner.ErrandID, &models.Turn{ID: "turn", TaskID: owner.TaskID}))
	stream := NewStreamManager(newSessionTestLogger(), StreamCallbacks{}, nil, nil)
	t.Cleanup(stream.Wait)
	stream.setAgentDeliveryRepository(repo)
	sm := NewSessionManager(newSessionTestLogger(), nil)
	sm.SetDependencies(nil, stream, nil, nil)
	execution := &AgentExecution{
		ID: "execution", Owner: owner, ACPSessionID: "native", DeliveryMode: DurableDeliveryV1,
		DeliveryIncarnationID: "incarnation", DeliveryStreamID: "stream", DeliveryHarnessGeneration: 1, promptTurnID: "turn",
	}
	require.NoError(t, sm.bindErrandDelivery(ctx, execution))
	stored, err := sm.persistErrandDeliveryBeforeDispatch(ctx, execution, "hello", nil, "original", 1)
	require.NoError(t, err)
	peer := &journal.Submission{
		ID: stored.ID, SessionID: stored.SessionID, IncarnationID: stored.IncarnationID, HarnessGeneration: 1,
		StreamID: execution.DeliveryStreamID, Hash: stored.PayloadHash, State: journal.SubmissionAccepted,
	}
	status := &agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Durable: true, Version: journal.CurrentProtocolVersion},
		SessionID:         peer.SessionID, StreamID: peer.StreamID, IncarnationID: peer.IncarnationID, HarnessGeneration: 1,
	}
	client := retainedErrandAdmissionPeer(t, peer)
	manager := &Manager{streamManager: stream, sessionManager: sm}
	instance := &ExecutorInstance{Client: client, DeliveryStatus: status}

	wrongBinding := &AgentExecution{ID: execution.ID, Owner: owner, ACPSessionID: "foreign-native"}
	_, err = manager.restoreErrandDelivery(ctx, wrongBinding, instance)
	assert.ErrorIs(t, err, ErrDurableAdoptionIdentityMismatch)
	unchanged, err := repo.GetAgentDeliverySubmissionMetadata(ctx, stored.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionDispatching, unchanged.State)

	recovered := &AgentExecution{ID: execution.ID, Owner: owner, ACPSessionID: execution.ACPSessionID}
	_, err = manager.restoreErrandDelivery(ctx, recovered, instance)
	require.NoError(t, err)
	require.Equal(t, stored.ID, recovered.deliverySubmissionIDSnapshot())
	accepted, err := repo.GetAgentDeliverySubmissionMetadata(ctx, stored.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionAccepted, accepted.State)
	require.Equal(t, stored.PayloadHash, accepted.PayloadHash)
	row, err := repo.GetErrand(ctx, owner.ErrandID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleQueued, row.Lifecycle, "public launch bookkeeping owns the transition")
}

func retainedErrandAdmissionPeer(t *testing.T, peer *journal.Submission) *agentctl.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/api/v1/agent/submissions/"+peer.ID {
			t.Errorf("unexpected recovery request %s %s", req.Method, req.URL.Path)
			http.NotFound(w, req)
			return
		}
		_ = json.NewEncoder(w).Encode(peer)
	}))
	t.Cleanup(server.Close)
	client := createTestClient(t, server.URL)
	t.Cleanup(client.Close)
	return client
}

func TestRecoveredErrandSubmissionRequiresExactAdmittedTurn(t *testing.T) {
	const session = "errand:owned"
	execution := &AgentExecution{ID: "execution", DeliveryDescriptor: &agentctl.DeliveryStatus{
		SessionID: session, IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream",
		Stream: &journal.Stream{StreamID: "stream", HighWater: 8, Acknowledged: 4},
	}}
	descriptor := &models.ErrandExecutionDescriptor{ActiveSubmissionID: "prompt:original", ActiveTurnID: "turn", PromptGeneration: 3}
	stored := &models.AgentDeliverySubmission{
		ID: descriptor.ActiveSubmissionID, SessionID: session, State: models.DeliverySubmissionAccepted,
		IncarnationID: "incarnation", DispatchAttemptID: execution.ID, HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: "hash",
	}
	accepted := journal.Submission{
		ID: stored.ID, SessionID: session, IncarnationID: stored.IncarnationID, HarnessGeneration: 2,
		StreamID: "stream", Hash: stored.PayloadHash, State: journal.SubmissionAccepted,
	}
	completed := accepted
	completed.State = journal.SubmissionCompleted
	completed.TerminalEventRetained = true
	completed.TerminalSequence = 8
	acknowledged := completed
	acknowledged.TerminalSequence = 4
	outsideSnapshot := completed
	outsideSnapshot.TerminalSequence = 9
	foreign := accepted
	foreign.Hash = "different"
	unconfirmed := accepted
	unconfirmed.State = journal.SubmissionDispatching
	awaitingReceipt := *stored
	awaitingReceipt.State = models.DeliverySubmissionDispatching

	for _, tc := range []struct {
		name   string
		peer   journal.Submission
		stored *models.AgentDeliverySubmission
		want   bool
	}{
		{"accepted", accepted, stored, true},
		{"acknowledged live dispatch", unconfirmed, stored, true},
		{"retained terminal", completed, stored, true},
		{"terminal already acknowledged", acknowledged, stored, false},
		{"terminal outside captured prefix", outsideSnapshot, stored, false},
		{"hash mismatch", foreign, stored, false},
		{"unconfirmed dispatch", unconfirmed, &awaitingReceipt, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesRecoveredErrandSubmission(execution, descriptor, tc.stored, &tc.peer); got != tc.want {
				t.Errorf("matches recovered submission = %t, want %t", got, tc.want)
			}
		})
	}
}
