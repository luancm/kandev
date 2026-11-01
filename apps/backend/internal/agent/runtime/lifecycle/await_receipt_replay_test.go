package lifecycle

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	nativeacp "github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/codexappserver"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// nativeAwaitReplayFrame crosses the public native adapter with a pinned-schema
// item/completed fixture. No prompt or await request is issued.
func nativeAwaitReplayFrame(t *testing.T, delivery models.ErrandAwaitDelivery) streams.AgentEvent {
	t.Helper()
	inputR, inputW := io.Pipe()
	outputR, outputW := io.Pipe()
	t.Cleanup(func() { _ = inputR.Close(); _ = inputW.Close(); _ = outputR.Close(); _ = outputW.Close() })
	a := codexappserver.NewAdapter(&shared.Config{}, newTestLogger())
	require.NoError(t, a.Connect(inputW, outputR))
	t.Cleanup(func() { _ = a.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(inputR)
		for scanner.Scan() {
			var request struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				return
			}
			if request.Method != "thread/start" {
				t.Errorf("unexpected native request %s; replay must not prompt", request.Method)
				return
			}
			_ = json.NewEncoder(outputW).Encode(map[string]any{"id": request.ID, "result": map[string]any{"thread": map[string]any{"id": "native-parent"}}})
		}
	}()
	t.Cleanup(func() { _ = a.Close(); _ = inputR.Close(); <-done })
	_, err := a.NewSession(t.Context(), nil)
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{"duration_ms": 7, "end_reason": "all_ready", "errands": []any{map[string]any{"errand_id": "child", "status": "done", "result": strings.Repeat("actual ", 2000)}}, "await_delivery": delivery})
	require.NoError(t, err)
	native := map[string]any{"content": []any{map[string]any{"type": "text", "text": string(body)}}}
	frame := map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "native-parent", "turnId": "native-turn", "item": map[string]any{"type": "mcpToolCall", "id": "native-call", "server": "kandev", "tool": "await_errands_kandev", "arguments": map[string]any{}, "status": "completed", "error": nil, "result": native}}}
	require.NoError(t, json.NewEncoder(outputW).Encode(frame))
	for {
		select {
		case event := <-a.Updates():
			if event.Type == streams.EventTypeToolUpdate {
				require.NotNil(t, event.ErrandAwaitReceipt)
				return event
			}
		case <-time.After(5 * time.Second):
			t.Fatal("native result was not observed")
		}
	}
}

func openAwaitReplayRepository(t *testing.T, path string, initialized bool) *tasksqlite.Repository {
	t.Helper()
	raw, err := db.OpenSQLite(path)
	require.NoError(t, err)
	connection := sqlx.NewDb(raw, "sqlite3")
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	if initialized {
		return tasksqlite.NewWithInitializedDB(connection, connection, nil)
	}
	repository, err := tasksqlite.NewWithDB(connection, connection, nil)
	require.NoError(t, err)
	return repository
}

func seedAwaitReplay(t *testing.T, repository *tasksqlite.Repository) models.ErrandAwaitDelivery {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, repository.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Replay"}))
	require.NoError(t, repository.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "workspace", WorkflowStepID: "step", Title: "Replay"}))
	require.NoError(t, repository.CreateTaskSession(ctx, &models.TaskSession{ID: "session-1", TaskID: "task", QueueIncarnationID: "owner", State: models.TaskSessionStateRunning}))
	require.NoError(t, repository.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running", SessionID: "session-1", TaskID: "task", AgentExecutionID: "execution", Status: models.ExecutorRunningStatusRunning}))
	require.NoError(t, repository.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: "session-1", IncarnationID: "owner", Generation: 1, NativeSessionID: "native-parent", CreationReason: "initial"}))
	row := &models.Errand{ID: "child", TaskID: "task", ParentSessionID: "session-1", Title: "Child", Objective: "Read", AgentProfileID: "profile", FirstTurnID: "child-turn", SubmissionID: "child-submission", Lifecycle: models.ErrandLifecycleQueued, ResultText: strings.Repeat("actual ", 2000), ResultState: models.ErrandResultPending}
	require.NoError(t, repository.CreateErrand(ctx, row))
	id := models.ErrandAwaitIdentity{TaskID: "task", ParentSessionID: "session-1", ParentIncarnationID: "owner", HarnessGeneration: 1, Token: "attempt"}
	require.NoError(t, repository.BeginErrandAwait(ctx, id))
	rows, err := repository.ListErrandsByParent(ctx, "session-1")
	require.NoError(t, err)
	inventory := models.ErrandInventory(rows)
	_, err = repository.ClaimResults(ctx, "task", "session-1", models.ErrandDeliveryAwait, "attempt", inventory, time.Now().UTC(), inventory)
	require.NoError(t, err)
	delivery := models.ErrandAwaitDelivery{Version: 1, Token: "attempt", Items: []models.ErrandAwaitDeliveryItem{{ErrandID: row.ID, ResultRevision: row.ResultRevision}}}
	require.NoError(t, repository.OfferErrandAwait(ctx, id, delivery.Items))
	return delivery
}

func awaitReplayState(t *testing.T, r *tasksqlite.Repository) string {
	t.Helper()
	var state string
	require.NoError(t, r.DB().QueryRow("SELECT state FROM errand_await_attempt WHERE token='attempt'").Scan(&state))
	return state
}

// @covers AC-TASKS-ERRANDS-004.8
func TestAwaitNativeReceiptReconcilesAfterBackendRestart(t *testing.T) {
	exerciseAwaitNativeReceiptRestart(t, nativeAwaitReplayFrame)
}

// @covers AC-TASKS-ERRANDS-004.8
func TestAwaitCodexACPNativeReceiptReconcilesAfterBackendRestart(t *testing.T) {
	exerciseAwaitNativeReceiptRestart(t, acpAwaitReplayFrame)
}

func exerciseAwaitNativeReceiptRestart(t *testing.T, frame func(*testing.T, models.ErrandAwaitDelivery) streams.AgentEvent) {
	t.Helper()
	for _, crashAfterInbox := range []bool{false, true} {
		t.Run(fmt.Sprintf("inbox_committed_%t", crashAfterInbox), func(t *testing.T) {
			// Retained evidence is admitted even when new Errands calls are disabled.
			t.Setenv("KANDEV_FEATURES_ERRANDS", "false")
			path := filepath.Join(t.TempDir(), "backend.db")
			repository := openAwaitReplayRepository(t, path, false)
			delivery := seedAwaitReplay(t, repository)
			native := frame(t, delivery)
			journalPath := filepath.Join(t.TempDir(), "native.bbolt")
			store, err := journal.Open(journal.Config{Path: journalPath})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			payload, err := json.Marshal(native)
			require.NoError(t, err)
			committed, err := store.Append(t.Context(), journal.Event{SessionID: "session-1", IncarnationID: "owner", HarnessGeneration: 1, StreamID: "stream", Type: native.Type, Payload: payload})
			require.NoError(t, err)
			// Backend is unavailable at the journal commit boundary.
			require.Equal(t, "offered", awaitReplayState(t, repository))
			require.NoError(t, store.Close())
			store, err = journal.Open(journal.Config{Path: journalPath})
			require.NoError(t, err)
			execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session-1", DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1}
			if crashAfterInbox {
				native.DeliveryStreamID, native.DeliveryIncarnationID, native.DeliveryHarnessGeneration, native.DeliverySequence = "stream", "owner", 1, committed.Sequence
				_, err = repository.ReceiveAgentDeliveryEvent(t.Context(), durableAgentEvent(execution, native), 1)
				require.NoError(t, err)
				require.Equal(t, "received", awaitReplayState(t, repository))
				cursor, err := repository.GetAgentDeliveryCursor(t.Context(), "stream")
				require.NoError(t, err)
				require.Zero(t, cursor.ProjectedSequence)
			}
			require.NoError(t, repository.DB().Close())
			repository = openAwaitReplayRepository(t, path, true)
			descriptor, err := store.RecoveryDescriptor(t.Context(), "session-1", "owner", 1, "stream")
			require.NoError(t, err)
			require.Zero(t, descriptor.Stream.Acknowledged, "crash happened before ACK")
			execution.DeliveryDescriptor = &descriptor
			acked := make(chan error, 1)
			server := awaitReplayServer(t, store, repository, acked)
			endpoint, err := url.Parse(server.URL)
			require.NoError(t, err)
			port, err := strconv.Atoi(endpoint.Port())
			require.NoError(t, err)
			client := agentctl.NewClient(endpoint.Hostname(), port, newTestLogger(), agentctl.WithAuthToken("test-owner"))
			execution.agentctl = client
			publications := 0
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) {
				require.Equal(t, "received", awaitReplayState(t, repository), "publication must follow committed receipt")
				if event.ErrandAwaitReceipt != nil {
					publications++
				}
			}}, nil, nil)
			t.Cleanup(sm.Wait)
			sm.setAgentDeliveryRepository(repository)
			require.NoError(t, sm.ReplayRecoveredDelivery(t.Context(), execution))
			select {
			case err := <-acked:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("committed replay was not ACKed")
			}
			require.Equal(t, "received", awaitReplayState(t, repository))
			require.Equal(t, 1, publications)
			cursor, err := repository.GetAgentDeliveryCursor(t.Context(), "stream")
			require.NoError(t, err)
			require.EqualValues(t, 1, cursor.ReceivedSequence)
			require.EqualValues(t, 1, cursor.ProjectedSequence)
			replayed, _, err := store.Replay(t.Context(), "stream", 1, 10)
			require.NoError(t, err)
			require.Empty(t, replayed)
			var proofSequence int64
			require.NoError(t, repository.DB().QueryRow("SELECT receipt_sequence FROM errand_await_attempt WHERE token='attempt'").Scan(&proofSequence))
			require.EqualValues(t, 1, proofSequence, "compact proof survives journal ACK/pruning")
		})
	}
}

func awaitReplayServer(t *testing.T, store *journal.Journal, r *tasksqlite.Repository, acked chan<- error) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-owner" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case request.URL.Path == "/api/v1/agent/delivery/stream" && request.Method == http.MethodGet:
			after, _ := strconv.ParseUint(request.URL.Query().Get("after"), 10, 64)
			events, stream, err := store.Replay(request.Context(), "stream", after, 10)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"events": events, "stream": stream})
		case request.URL.Path == "/api/v1/agent/delivery/stream/ack":
			if awaitReplayState(t, r) != "received" {
				acked <- fmt.Errorf("ACK before receipt")
				w.WriteHeader(500)
				return
			}
			err := store.Acknowledge(request.Context(), "stream", 1)
			acked <- err
			w.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet:
			descriptor, err := store.RecoveryDescriptor(request.Context(), "session-1", "owner", 1, "stream")
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			_ = json.NewEncoder(w).Encode(descriptor)
		default:
			t.Errorf("unexpected request %s %s; receipt replay cannot prompt or enqueue", request.Method, request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// @covers AC-TASKS-ERRANDS-004.13
func TestAwaitNativeReceiptLossAndToolCardRemainUnresolved(t *testing.T) {
	for _, scenario := range []string{"before-journal", "append-failure", "tool-card"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("KANDEV_FEATURES_ERRANDS", "false")
			repository := openAwaitReplayRepository(t, filepath.Join(t.TempDir(), "backend.db"), false)
			delivery := seedAwaitReplay(t, repository)
			native := nativeAwaitReplayFrame(t, delivery)
			path := filepath.Join(t.TempDir(), "native.bbolt")
			store, err := journal.Open(journal.Config{Path: path})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			if scenario == "append-failure" {
				require.NoError(t, store.Close())
			}
			if scenario != "before-journal" {
				if scenario == "tool-card" {
					native.ErrandAwaitReceipt = nil
				}
				payload, err := json.Marshal(native)
				require.NoError(t, err)
				_, err = store.Append(t.Context(), journal.Event{SessionID: "session-1", IncarnationID: "owner", HarnessGeneration: 1, StreamID: "stream", Type: native.Type, Payload: payload})
				if scenario == "append-failure" {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
			require.NoError(t, store.Close())
			store, err = journal.Open(journal.Config{Path: path})
			require.NoError(t, err)
			descriptor, err := store.RecoveryDescriptor(t.Context(), "session-1", "owner", 1, "stream")
			require.NoError(t, err)
			acked := make(chan error, 1)
			server := awaitReplayServer(t, store, repository, acked)
			endpoint, err := url.Parse(server.URL)
			require.NoError(t, err)
			port, err := strconv.Atoi(endpoint.Port())
			require.NoError(t, err)
			client := agentctl.NewClient(endpoint.Hostname(), port, newTestLogger(), agentctl.WithAuthToken("test-owner"))
			execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session-1", DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1, DeliveryDescriptor: &descriptor, agentctl: client}
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			t.Cleanup(sm.Wait)
			sm.setAgentDeliveryRepository(repository)
			require.NoError(t, sm.ReplayRecoveredDeliveryWithoutLifecycleCallbacks(t.Context(), execution))
			require.Equal(t, "uncertain", awaitReplayState(t, repository), "recovered missing observation must not become receipt or authorize release")
			row, err := repository.GetErrand(t.Context(), "child")
			require.NoError(t, err)
			require.Equal(t, models.ErrandResultDelivered, row.ResultState)
			require.Equal(t, "attempt", row.DeliveryToken)
		})
	}
}

func acpAwaitReplayFrame(t *testing.T, delivery models.ErrandAwaitDelivery) streams.AgentEvent {
	t.Helper()
	inputR, inputW := io.Pipe()
	outputR, outputW := io.Pipe()
	t.Cleanup(func() { _ = inputR.Close(); _ = inputW.Close(); _ = outputR.Close(); _ = outputW.Close() })
	a := nativeacp.NewAdapter(&shared.Config{AgentID: "codex-acp", WorkDir: t.TempDir()}, newTestLogger())
	require.NoError(t, a.Connect(inputW, outputR))
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(inputR)
		for scanner.Scan() {
			var request struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				return
			}
			var result any
			switch request.Method {
			case "initialize":
				result = map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "agentInfo": map[string]any{"name": "codex-acp", "version": "fixture"}}
			case "session/new":
				result = map[string]any{"sessionId": "native-parent"}
			default:
				t.Errorf("unexpected ACP request %s; receipt replay cannot prompt", request.Method)
				return
			}
			_ = json.NewEncoder(outputW).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
		}
	}()
	t.Cleanup(func() { _ = inputR.Close(); _ = outputW.Close(); _ = a.Close(); <-done })
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, a.Initialize(ctx))
	_, err := a.NewSession(ctx, nil)
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{"duration_ms": 7, "end_reason": "all_ready", "errands": []any{map[string]any{"errand_id": "child", "status": "done", "result": strings.Repeat("actual ", 2000)}}, "await_delivery": delivery})
	require.NoError(t, err)
	input := map[string]any{"server": "kandev", "tool": "await_errands_kandev", "arguments": map[string]any{}}
	for _, update := range []map[string]any{
		{"sessionUpdate": "tool_call", "toolCallId": "native-call", "kind": "execute", "status": "in_progress", "title": "mcp.kandev.await_errands_kandev", "rawInput": input, "_meta": map[string]any{"is_mcp_tool_call": true}},
		{"sessionUpdate": "tool_call_update", "toolCallId": "native-call", "status": "completed", "rawOutput": map[string]any{"error": nil, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": string(body)}}}}},
	} {
		require.NoError(t, json.NewEncoder(outputW).Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "native-parent", "update": update}}))
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case event := <-a.Updates():
			if event.Type == streams.EventTypeToolUpdate {
				require.NotNil(t, event.ErrandAwaitReceipt)
				require.Equal(t, streams.ErrandAwaitToolName, event.ToolName)
				return event
			}
		case <-deadline.C:
			t.Fatal("ACP native result not observed")
		}
	}
}
