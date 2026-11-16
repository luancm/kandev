package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func attachAwaitRefresh(t *testing.T, sm *StreamManager, callback func(context.Context, string)) {
	t.Helper()
	manager := &Manager{streamManager: sm}
	setter, ok := any(manager).(interface {
		SetErrandAwaitRefresh(func(context.Context, string))
	})
	require.True(t, ok, "lifecycle must refresh committed await evidence independently of ordinary callbacks")
	setter.SetErrandAwaitRefresh(callback)
}

func awaitProjectionExecution() *AgentExecution {
	return &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session-1", DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1}
}

// @covers AC-TASKS-ERRANDS-004.13
func TestAwaitOrderedTerminalAndLateReceiptRefreshWithoutCallbacks(t *testing.T) {
	r := openAwaitReplayRepository(t, filepath.Join(t.TempDir(), "backend.db"), false)
	delivery := seedAwaitReplay(t, r)
	_, err := r.DB().Exec("UPDATE errand_await_attempt SET turn_id='origin-turn' WHERE token='attempt'")
	require.NoError(t, err)
	execution := awaitProjectionExecution()
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) {
		t.Error("suppressed lifecycle callback ran")
	}}, nil, nil)
	t.Cleanup(sm.Wait)
	sm.setAgentDeliveryRepository(r)
	var observed []models.ErrandDeliveryState
	attachAwaitRefresh(t, sm, func(ctx context.Context, parent string) {
		require.Equal(t, execution.SessionID, parent)
		row, err := r.GetErrand(ctx, "child")
		require.NoError(t, err)
		observed = append(observed, row.DeliveryState)
	})
	terminal := streams.AgentEvent{Type: streams.EventTypeComplete, TurnID: "origin-turn", DeliveryStreamID: "stream", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1, DeliverySequence: 1}
	prepared, err := sm.prepareAgentEvent(t.Context(), execution, nil, r, terminal)
	require.NoError(t, err)
	require.Equal(t, "offered", awaitReplayState(t, r))
	prepared.skipCallback, prepared.skipAcknowledgement = true, true
	require.NoError(t, sm.processPreparedAgentEvent(t.Context(), execution, nil, r, prepared, 0))
	require.Equal(t, "uncertain", awaitReplayState(t, r))
	require.Equal(t, []models.ErrandDeliveryState{models.ErrandDeliveryUncertain}, observed)
	native := nativeAwaitReplayFrame(t, delivery)
	native.DeliveryStreamID, native.DeliveryIncarnationID, native.DeliveryHarnessGeneration, native.DeliverySequence = "stream", "owner", 1, 2
	prepared, err = sm.prepareAgentEvent(t.Context(), execution, nil, r, native)
	require.NoError(t, err)
	prepared.skipCallback, prepared.skipAcknowledgement = true, true
	require.NoError(t, sm.processPreparedAgentEvent(t.Context(), execution, nil, r, prepared, 0))
	require.Equal(t, "received", awaitReplayState(t, r))
	require.Equal(t, []models.ErrandDeliveryState{models.ErrandDeliveryUncertain, models.ErrandDeliveryReceived}, observed)
	// The received-prefix path skips already-applied callbacks too.
	stored := durableAgentEvent(execution, native)
	require.NoError(t, sm.projectReceivedDeliveryEvent(t.Context(), execution, r, stored))
	require.Equal(t, models.ErrandDeliveryReceived, observed[len(observed)-1])
}

func TestAwaitRecoveredClassificationFollowsCommittedNativeEvidence(t *testing.T) {
	for _, positive := range []bool{false, true} {
		t.Run(strconv.FormatBool(positive), func(t *testing.T) {
			t.Setenv("KANDEV_FEATURES_ERRANDS", "false")
			r := openAwaitReplayRepository(t, filepath.Join(t.TempDir(), "backend.db"), false)
			delivery := seedAwaitReplay(t, r)
			native := nativeAwaitReplayFrame(t, delivery)
			if !positive {
				native.ErrandAwaitReceipt = nil
			}
			store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "journal.bbolt")})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			body, err := json.Marshal(native)
			require.NoError(t, err)
			_, err = store.Append(t.Context(), journal.Event{SessionID: "session-1", IncarnationID: "owner", HarnessGeneration: 1, StreamID: "stream", Type: native.Type, Payload: body})
			require.NoError(t, err)
			descriptor, err := store.RecoveryDescriptor(t.Context(), "session-1", "owner", 1, "stream")
			require.NoError(t, err)
			server := awaitReplayServer(t, store, r, make(chan error, 1))
			endpoint, err := url.Parse(server.URL)
			require.NoError(t, err)
			port, err := strconv.Atoi(endpoint.Port())
			require.NoError(t, err)
			execution := awaitProjectionExecution()
			execution.DeliveryDescriptor = &descriptor
			execution.agentctl = agentctl.NewClient(endpoint.Hostname(), port, newTestLogger(), agentctl.WithAuthToken("test-owner"))
			t.Cleanup(execution.agentctl.Close)
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			t.Cleanup(sm.Wait)
			sm.setAgentDeliveryRepository(r)
			var observed []models.ErrandDeliveryState
			attachAwaitRefresh(t, sm, func(ctx context.Context, _ string) {
				row, err := r.GetErrand(ctx, "child")
				require.NoError(t, err)
				observed = append(observed, row.DeliveryState)
			})
			require.NoError(t, sm.ReplayRecoveredDeliveryWithoutLifecycleCallbacks(t.Context(), execution))
			want := models.ErrandDeliveryUncertain
			if positive {
				want = models.ErrandDeliveryReceived
			}
			require.NotEmpty(t, observed)
			require.Equal(t, want, observed[len(observed)-1])
			if positive {
				require.NotContains(t, observed, models.ErrandDeliveryUncertain, "recovered native evidence must reduce before any warning")
			}
			require.Equal(t, string(want), awaitReplayState(t, r))
		})
	}
}

func TestAwaitRecoveredEmptyStreamClassifiesOnlyCapturedOwner(t *testing.T) {
	r := openAwaitReplayRepository(t, filepath.Join(t.TempDir(), "backend.db"), false)
	seedAwaitReplay(t, r)
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	t.Cleanup(sm.Wait)
	sm.setAgentDeliveryRepository(r)
	execution := awaitProjectionExecution()
	execution.DeliveryDescriptor = &journal.RecoveryDescriptor{Stream: &journal.Stream{StreamID: "stream", SessionID: "session-1", IncarnationID: "owner", HarnessGeneration: 1}}
	// A different recovered owner cannot classify the retained invocation.
	execution.DeliveryIncarnationID = "foreign"
	require.NoError(t, sm.ReplayRecoveredDeliveryWithoutLifecycleCallbacks(t.Context(), execution))
	require.Equal(t, "offered", awaitReplayState(t, r))
	execution.DeliveryIncarnationID = "owner"
	stale := errors.New("recovery lease replaced")
	err := sm.replayRecoveredDelivery(t.Context(), execution, true, func() error { return stale })
	require.ErrorIs(t, err, stale)
	require.Equal(t, "offered", awaitReplayState(t, r), "a replaced recovery lease cannot classify")
	require.NoError(t, sm.ReplayRecoveredDeliveryWithoutLifecycleCallbacks(t.Context(), execution))
	require.Equal(t, "uncertain", awaitReplayState(t, r))
}
