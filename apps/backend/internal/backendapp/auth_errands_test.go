package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/errands"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type authErrandTestLauncher struct{}

func (authErrandTestLauncher) LaunchErrand(context.Context, errands.LaunchRequest) error { return nil }

type authErrandTestStopper struct{}

func (authErrandTestStopper) StopErrandSession(context.Context, string, string) error { return nil }

type authErrandTestProfiles struct{}

func (authErrandTestProfiles) ResolveProfile(context.Context, string, string, string) (errands.ResolvedProfile, error) {
	return errands.ResolvedProfile{}, nil
}

func (authErrandTestProfiles) CompatibleProfiles(context.Context, string) ([]errands.ProfileInfo, error) {
	return nil, nil
}

type authErrandTestMCP struct{}

func (authErrandTestMCP) SessionSnapshot(context.Context, string, string) (errands.MCPSnapshot, error) {
	return errands.MCPSnapshot{}, nil
}

type authErrandTestEvents struct{}

func (authErrandTestEvents) PublishErrandUpdated(context.Context, *models.Errand, models.ErrandStatus) {
}

// @covers AC-TASKS-ERRANDS-008.3 AC-TASKS-ERRANDS-005.1 AC-TASKS-ERRANDS-012.3
func TestGatewayAuthorizesQueuedErrandIDsThroughTheirOwningTask(t *testing.T) {
	ctx := context.Background()
	taskSvc, taskRepo, officeRepo := newRunSubscriptionCheckHarness(t)
	cfg := &config.Config{}
	cfg.Features.Auth = true
	authSvc := newEnabledAuthService(t, cfg)
	userA, err := authSvc.AdminCreateUser(ctx, "errand-a@example.test", "memberpass123", "Errand A", "member")
	require.NoError(t, err)
	userB, err := authSvc.AdminCreateUser(ctx, "errand-b@example.test", "memberpass123", "Errand B", "member")
	require.NoError(t, err)
	_, tokenA, err := authSvc.MintToken(ctx, userA.ID, "errand scope test", 1)
	require.NoError(t, err)
	_, tokenB, err := authSvc.MintToken(ctx, userB.ID, "errand scope test", 1)
	require.NoError(t, err)

	workspaceA := seedOwnedWorkspace(t, taskSvc, userA.ID)
	workspaceB := seedOwnedWorkspace(t, taskSvc, userB.ID)
	for _, task := range []*models.Task{
		{ID: "task-a", WorkspaceID: workspaceA, Title: "A task", State: v1.TaskStateCreated, Priority: "medium"},
		{ID: "task-b", WorkspaceID: workspaceB, Title: "B task", State: v1.TaskStateCreated, Priority: "medium"},
		{ID: "task-b-sibling", WorkspaceID: workspaceB, Title: "B sibling", State: v1.TaskStateCreated, Priority: "medium"},
	} {
		require.NoError(t, taskRepo.CreateTask(ctx, task))
	}
	require.NoError(t, taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "parent-b", TaskID: "task-b", State: models.TaskSessionStateWaitingForInput,
		StartedAt: time.Now().UTC(),
	}))
	for _, id := range []string{"queued-owner", "queued-foreign", "queued-mismatch"} {
		require.NoError(t, taskRepo.CreateErrand(ctx, &models.Errand{
			ID: id, TaskID: "task-b", ParentSessionID: "parent-b", Title: id, Objective: "Audit the change",
			AgentProfileID: "profile-reviewer", FirstTurnID: "first-" + id, SubmissionID: "submit-" + id,
			Lifecycle: models.ErrandLifecycleQueued, AwaitingResult: true, ResultState: models.ErrandResultNone,
		}))
		_, sessionErr := taskRepo.GetTaskSession(ctx, id)
		require.ErrorIs(t, sessionErr, models.ErrTaskSessionNotFound, "queued identity has no session row")
	}
	require.NoError(t, taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "retained-errand", TaskID: "task-b", Role: models.SessionRoleErrand,
		ParentSessionID: "parent-b", State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(),
	}))
	require.NoError(t, taskRepo.CreateErrand(ctx, &models.Errand{
		ID: "retained-errand", TaskID: "task-b", ParentSessionID: "parent-b", Title: "Retained",
		Objective: "Retained errand", AgentProfileID: "profile-reviewer", FirstTurnID: "retained-first",
		SubmissionID: "retained-submit", Lifecycle: models.ErrandLifecycleLaunched, ResultState: models.ErrandResultNone,
	}))

	errService, err := errands.New(errands.Dependencies{
		Store: taskRepo, Launcher: authErrandTestLauncher{}, Stopper: authErrandTestStopper{},
		Profiles: authErrandTestProfiles{}, MCP: authErrandTestMCP{}, Events: authErrandTestEvents{},
	})
	require.NoError(t, err)
	errService.SetEnabled(func() bool { return true })

	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	gateway := gateways.NewGateway(log)
	gateway.SetAuthPolicy(gatewayAuthPolicy(authSvc, taskSvc, taskRepo, officeRepo))
	gateway.Dispatcher.RegisterFunc(ws.ActionErrandCancel, func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var request struct {
			SessionID string `json:"session_id"`
		}
		if err := msg.ParsePayload(&request); err != nil {
			return nil, err
		}
		row, err := errService.Lookup(ctx, request.SessionID)
		if err != nil {
			return nil, err
		}
		cancelled, err := errService.Cancel(ctx, errands.CancelRequest{
			ErrandID: row.ID, TaskID: row.TaskID, Source: models.ErrandCancelSourceUser,
		})
		if err != nil {
			return nil, err
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]string{"status": string(cancelled.Status)})
	})
	gateway.Dispatcher.RegisterFunc(ws.ActionSessionDelete, func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var request struct {
			SessionID string `json:"session_id"`
		}
		if err := msg.ParsePayload(&request); err != nil {
			return nil, err
		}
		session, err := taskRepo.GetTaskSession(ctx, request.SessionID)
		if err != nil {
			return nil, err
		}
		if err := taskRepo.DeleteTaskSession(ctx, session); err != nil {
			return nil, err
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"deleted": true})
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if authorization := c.GetHeader("Authorization"); strings.HasPrefix(authorization, "Bearer ") {
			if identity, ok := authSvc.ResolveBearer(c.Request.Context(), strings.TrimPrefix(authorization, "Bearer ")); ok {
				authn.SetOnGin(c, identity)
			}
		}
		c.Next()
	})
	gateway.SetupRoutes(router)
	hubCtx, stopHub := context.WithCancel(ctx)
	hubDone := make(chan struct{})
	go func() {
		gateway.Hub.Run(hubCtx)
		close(hubDone)
	}()
	server := httptest.NewServer(router)
	t.Cleanup(func() {
		server.Close()
		stopHub()
		<-hubDone
	})

	ownerConn := dialErrandScopeSocket(t, server.URL, tokenB)
	foreignConn := dialErrandScopeSocket(t, server.URL, tokenA)
	foreign := dispatchErrandScopeRequest(t, foreignConn, "foreign", ws.ActionErrandCancel,
		map[string]string{"session_id": "queued-foreign"})
	require.Equal(t, ws.MessageTypeError, foreign.Type)
	assertErrorCode(t, foreign, ws.ErrorCodeNotFound)
	assertQueuedErrandUnchanged(t, taskRepo, "queued-foreign")

	// Both tasks belong to the same authenticated user, so this denies only if
	// the gateway binds the reserved ID to its recorded task before dispatch.
	mismatch := dispatchErrandScopeRequest(t, ownerConn, "mismatch", ws.ActionErrandCancel,
		map[string]string{"session_id": "queued-mismatch", "task_id": "task-b-sibling"})
	require.Equal(t, ws.MessageTypeError, mismatch.Type)
	assertErrorCode(t, mismatch, ws.ErrorCodeNotFound)
	assertQueuedErrandUnchanged(t, taskRepo, "queued-mismatch")

	owned := dispatchErrandScopeRequest(t, ownerConn, "owned", ws.ActionErrandCancel,
		map[string]string{"session_id": "queued-owner"})
	require.Equal(t, ws.MessageTypeResponse, owned.Type)
	ownedErrand, err := taskRepo.GetErrand(ctx, "queued-owner")
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleCancelled, ownedErrand.Lifecycle)
	require.Equal(t, models.ErrandCancelSourceUser, ownedErrand.CancelSource)
	require.Equal(t, models.ErrandResultPending, ownedErrand.ResultState)

	// A retained errand still follows ordinary session authorization while the
	// feature is off; the errand-specific cancellation route is not required.
	errService.SetEnabled(func() bool { return false })
	retained, err := taskRepo.GetTaskSession(ctx, "retained-errand")
	require.NoError(t, err)
	require.Equal(t, models.SessionRoleErrand, retained.Role)
	deleted := dispatchErrandScopeRequest(t, ownerConn, "retained", ws.ActionSessionDelete,
		map[string]string{"session_id": "retained-errand"})
	require.Equal(t, ws.MessageTypeResponse, deleted.Type)
	_, sessionErr := taskRepo.GetTaskSession(ctx, "retained-errand")
	require.ErrorIs(t, sessionErr, models.ErrTaskSessionNotFound)
}

func assertQueuedErrandUnchanged(t *testing.T, repo *sqlite.Repository, id string) {
	t.Helper()
	row, err := repo.GetErrand(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, models.ErrandLifecycleQueued, row.Lifecycle)
	require.Equal(t, models.ErrandCancelSource(""), row.CancelSource)
	require.Equal(t, models.ErrandResultState("none"), row.ResultState)
	require.Equal(t, int64(1), row.Version)
}

func dialErrandScopeSocket(t *testing.T, baseURL, token string) *gorillaws.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws"
	header := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, response, err := gorillaws.DefaultDialer.Dial(url, header)
	if response != nil && response.Body != nil {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Errorf("close WebSocket handshake response: %v", closeErr)
		}
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func dispatchErrandScopeRequest(t *testing.T, conn *gorillaws.Conn, id, action string, payload map[string]string) ws.Message {
	t.Helper()
	request, err := ws.NewRequest(id, action, payload)
	require.NoError(t, err)
	require.NoError(t, conn.WriteJSON(request))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	var response ws.Message
	require.NoError(t, conn.ReadJSON(&response))
	require.Equal(t, id, response.ID)
	return response
}

func assertErrorCode(t *testing.T, response ws.Message, code string) {
	t.Helper()
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, code, payload.Code)
}
