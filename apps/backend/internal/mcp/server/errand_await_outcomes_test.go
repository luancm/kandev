package mcp

import (
	"context"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/journal"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	_ "github.com/mattn/go-sqlite3"
	"path/filepath"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

type lateAwaitBackend struct {
	cancel  context.CancelFunc
	actions []string
}

func (b *lateAwaitBackend) RequestPayload(_ context.Context, action string, _ any, result any) error {
	b.actions = append(b.actions, action)
	if action == ws.ActionMCPAwaitErrands {
		b.cancel()
		if out, ok := result.(*map[string]interface{}); ok {
			*out = map[string]interface{}{"errands": []any{map[string]any{"result": "must never escape"}}}
		}
	}
	return nil
}

// @covers AC-TASKS-ERRANDS-004.8
func TestAwaitErrorAndCancellationNeverReturnSuccessAfterAbort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &lateAwaitBackend{cancel: cancel}
	s := New(backend, "parent", "task", 0, newTestLogger(t), "", false, ModeTask)
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{"errand_ids": []any{"child"}, "mode": "all"}
	result, err := s.awaitErrandsHandler()(ctx, req)
	require.NoError(t, err)
	require.True(t, result.IsError, "cancel selected error; a late backend success must not escape")
	require.Contains(t, backend.actions, ws.ActionMCPAwaitErrandsRelease)
}

type journalAwaitFixture struct {
	journal    *journal.Journal
	owner      models.ErrandAwaitIdentity
	captureErr error
}

func (s *journalAwaitFixture) CaptureAwaitInvocation(_ context.Context, token string) (models.ErrandAwaitIdentity, error) {
	id := s.owner
	id.Token = token
	return id, s.captureErr
}
func (s *journalAwaitFixture) RecordAwaitAbort(ctx context.Context, id models.ErrandAwaitIdentity) error {
	return s.journal.RecordAwaitAbort(ctx, id)
}
func (s *journalAwaitFixture) PendingAwaitAborts(ctx context.Context) ([]models.ErrandAwaitIdentity, error) {
	return s.journal.PendingAwaitAborts(ctx)
}
func (s *journalAwaitFixture) ConfirmAwaitAbort(ctx context.Context, id models.ErrandAwaitIdentity) error {
	return s.journal.ConfirmAwaitAbort(ctx, id)
}

// @covers AC-TASKS-ERRANDS-004.8
func TestAwaitAbortReplaySurvivesBackendDisconnect(t *testing.T) {
	for _, claimed := range []bool{false, true} {
		t.Run(fmt.Sprint(claimed), func(t *testing.T) { exerciseAbortBridge(t, claimed, false, false) })
	}
}
func TestAwaitAbortJournalFailureRetriesWhileAlive(t *testing.T) {
	exerciseAbortBridge(t, true, true, false)
}

func TestAwaitJournalLossDoesNotInferAbort(t *testing.T) { exerciseAbortBridge(t, true, true, true) }

func exerciseAbortBridge(t *testing.T, claimed, journalOffline, processLost bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	conn, err := sqlx.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "tasks.db")+"?_foreign_keys=on&_busy_timeout=5000")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	conn.SetMaxOpenConns(1)
	repo, err := sqlite.NewWithDB(conn, conn, nil)
	require.NoError(t, err)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws", Name: "bridge"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "ws", Title: "bridge"}))
	row := &models.Errand{ID: "child", TaskID: "task", ParentSessionID: "parent", Title: "child", Objective: "observe", AgentProfileID: "profile", FirstTurnID: "turn", SubmissionID: "initial", Lifecycle: models.ErrandLifecycleCancelled, CancelSource: models.ErrandCancelSourceUser, ResultKind: models.ErrandResultOutput, ResultState: models.ErrandResultPending, ResultText: "one exact result"}
	require.NoError(t, repo.CreateErrand(ctx, row))
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { _ = j.Close() })
	store := &journalAwaitFixture{journal: j, owner: models.ErrandAwaitIdentity{TaskID: "task", ParentSessionID: "parent", ParentIncarnationID: "launched", HarnessGeneration: 2, SubmissionID: "original"}}
	client := NewChannelBackendClient(newTestLogger(t))
	t.Cleanup(client.Close)
	s := New(client, "parent", "task", 0, newTestLogger(t), "", false, ModeTask)
	s.SetAwaitAbortStore(store)
	req := mcplib.CallToolRequest{}
	req.Params.Arguments = map[string]any{"errand_ids": []any{"child"}, "mode": "all"}
	done := make(chan *mcplib.CallToolResult, 1)
	go func() { result, _ := s.awaitErrandsHandler()(ctx, req); done <- result }()
	original := nextAwaitBridgeRequest(t, ctx, client)
	require.Equal(t, ws.ActionMCPAwaitErrands, original.Action)
	id, ok := mcporigin.AwaitInvocationFromMessage(original)
	require.True(t, ok)
	require.Equal(t, "original", id.SubmissionID)
	require.NoError(t, repo.BeginErrandAwait(ctx, id))
	inventory := models.ErrandInventory([]*models.Errand{row})
	if claimed {
		snapshots, err := repo.ClaimResults(ctx, "task", "parent", models.ErrandDeliveryAwait, id.Token, inventory, time.Now(), inventory)
		require.NoError(t, err)
		require.NoError(t, repo.OfferErrandAwait(ctx, id, []models.ErrandAwaitDeliveryItem{{ErrandID: row.ID, ResultRevision: snapshots[0].ResultRevision}}))
	}
	if journalOffline {
		require.NoError(t, j.Close())
	}
	client.BindRequestToStream(original.ID, "disconnected")
	client.FailStreamRequests("disconnected", errors.New("backend disconnected"))
	abort := nextAwaitBridgeRequest(t, ctx, client)
	require.Equal(t, ws.ActionMCPAwaitErrandsRelease, abort.Action)
	pending, err := j.PendingAwaitAborts(ctx)
	if journalOffline {
		require.Error(t, err)
	} else {
		require.NoError(t, err)
		require.Equal(t, []models.ErrandAwaitIdentity{id}, pending, "commit abort intent before sending")
	}
	client.FailRequest(abort.ID, errors.New("still offline"))
	var outcome *mcplib.CallToolResult
	select {
	case outcome = <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.True(t, outcome.IsError)
	late, _ := ws.NewResponse(original.ID, original.Action, map[string]any{"errands": []any{map[string]any{"result": "late success"}}})
	client.HandleResponse(late)
	require.NoError(t, j.Close())
	j, err = journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	store.journal = j
	if processLost {
		fresh := New(client, "parent", "task", 0, newTestLogger(t), "", false, ModeTask)
		fresh.SetAwaitAbortStore(store)
		fresh.replayAwaitAbortsOnce(ctx)
		current, err := repo.GetErrand(ctx, row.ID)
		require.NoError(t, err)
		require.Equal(t, models.ErrandResultDelivered, current.ResultState, "missing journal intent cannot infer release after loss")
		select {
		case unexpected := <-client.GetRequestChannel():
			t.Fatalf("unexpected recovery dispatch: %s", unexpected.Action)
		default:
		}
		return
	}
	replayDone := make(chan struct{})
	go func() { defer close(replayDone); s.replayAwaitAbortsOnce(ctx) }()
	replay := nextAwaitBridgeRequest(t, ctx, client)
	require.Equal(t, ws.ActionMCPAwaitErrandsRelease, replay.Action)
	replayID, ok := mcporigin.AwaitInvocationFromMessage(replay)
	require.True(t, ok)
	require.Equal(t, id, replayID)
	require.True(t, mcporigin.MessageCarriesTrustedInternalCall(replay))
	_, err = repo.AbortErrandAwait(ctx, replayID)
	require.NoError(t, err)
	response, _ := ws.NewResponse(replay.ID, replay.Action, map[string]any{"released": true})
	client.HandleResponse(response)
	select {
	case <-replayDone:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	pending, err = j.PendingAwaitAborts(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
	current, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, current.ResultState)
	if !claimed {
		_, err = repo.ClaimResults(ctx, "task", "parent", models.ErrandDeliveryAwait, id.Token, inventory, time.Now(), inventory)
		require.Error(t, err, "tombstone fences delayed worker")
	}
	require.True(t, outcome.IsError, "late success never flips selected error")
	s.replayAwaitAbortsOnce(ctx)
	select {
	case msg := <-client.GetRequestChannel():
		t.Fatalf("unexpected resend: %s", msg.Action)
	default:
	}
}
func nextAwaitBridgeRequest(t *testing.T, ctx context.Context, c *ChannelBackendClient) *ws.Message {
	t.Helper()
	select {
	case request := <-c.GetRequestChannel():
		return request
	case <-ctx.Done():
		t.Fatal(ctx.Err())
		return nil
	}
}
