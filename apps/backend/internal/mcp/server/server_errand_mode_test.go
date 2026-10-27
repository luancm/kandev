package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errandCatalogWant is the exact errand catalog of AC-TASKS-ERRANDS-009.2.
var errandCatalogWant = []string{
	"list_workspaces_kandev",
	"list_workflows_kandev",
	"list_workflow_steps_kandev",
	"list_tasks_kandev",
	"list_related_tasks_kandev",
	"list_task_sessions_kandev",
	"get_task_conversation_kandev",
	"get_task_plan_kandev",
}

var errandParentToolsWant = []string{
	"start_errand_kandev",
	"await_errands_kandev",
	"cancel_errand_kandev",
}

func TestModeForProfile_MapsTheErrandSurface(t *testing.T) {
	require.Equal(t, ModeErrand, modeForProfile(mcpprofile.New(mcpprofile.SurfaceErrand, nil, nil)))
}

func TestErrandCatalog_IsExactlyTheEightReadOnlyTools(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{}
	s := NewWithProfile(backend, "errand-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceErrand, nil, nil))

	assert.ElementsMatch(t, errandCatalogWant, getRegisteredToolNames(s))
}

func TestErrandCatalog_NeverRegistersPluginTools(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{}
	s := NewWithProfile(backend, "errand-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceErrand, nil, nil))
	definition := plugintools.Definition{
		PluginID: "notifications", LocalName: "notify", ExposedName: plugintools.ExposedName("notifications", "notify"),
		Description: "Notify a user", InputSchema: []byte(`{"type":"object"}`),
		Surfaces: []string{plugintools.SurfaceKanban},
	}
	require.NoError(t, s.SetPluginTools(plugintools.Snapshot{
		Generation: "g", Revision: 1, Tools: []plugintools.Definition{definition},
	}))

	assert.ElementsMatch(t, errandCatalogWant, getRegisteredToolNames(s))
}

// An errand inherits workspace MCP servers and provider flags from its parent.
// Neither may widen the fixed Kandev catalog or add plugin tools.
func TestErrandCatalog_StaysFixedWithProvidersAndCapabilities(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{}
	profile := mcpprofile.New(mcpprofile.SurfaceErrand,
		[]mcpprofile.Capability{mcpprofile.CapabilityUserQuestion, mcpprofile.CapabilityTaskTitle, mcpprofile.CapabilityErrands},
		[]string{"github", "gitlab"})
	s := NewWithProfile(backend, "errand-session", "task-1", 0, log, "", false, profile)

	assert.ElementsMatch(t, errandCatalogWant, getRegisteredToolNames(s))
}

func TestParentTools_RegisteredOnlyWithErrandsCapability(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{}

	without := NewWithProfile(backend, "parent-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil))
	for _, name := range errandParentToolsWant {
		assert.NotContains(t, getRegisteredToolNames(without), name)
	}

	with := NewWithProfile(backend, "parent-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityErrands}, nil))
	for _, name := range errandParentToolsWant {
		assert.Contains(t, getRegisteredToolNames(with), name)
	}
}

func TestParentTools_NeverReachErrandSessions(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{}
	errandWithCapability := NewWithProfile(backend, "errand-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceErrand, []mcpprofile.Capability{mcpprofile.CapabilityErrands}, nil))

	for _, name := range errandParentToolsWant {
		assert.NotContains(t, getRegisteredToolNames(errandWithCapability), name)
	}
	assert.ElementsMatch(t, errandCatalogWant, getRegisteredToolNames(errandWithCapability))
}

func TestSetMode_ErrandUsesTheFixedCatalogAndRestoresTask(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{}

	s := New(backend, "test-session", "test-task", 0, log, "", false, ModeTask)
	require.Contains(t, getRegisteredToolNames(s), "ask_user_question_kandev")

	s.SetMode(ModeErrand)
	cold := NewWithProfile(backend, "test-session", "test-task", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceErrand, nil, nil))
	assert.ElementsMatch(t, getRegisteredToolNames(cold), getRegisteredToolNames(s))
	assert.ElementsMatch(t, errandCatalogWant, getRegisteredToolNames(s))

	s.SetMode(ModeTask)
	assert.Contains(t, getRegisteredToolNames(s), "create_task_kandev")
	assert.Contains(t, getRegisteredToolNames(s), "ask_user_question_kandev")
}

func TestStartErrand_TakesCallerIdentityFromTheSession(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{response: map[string]interface{}{"errand_id": "e-1"}}
	s := NewWithProfile(backend, "parent-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityErrands}, nil))

	result := callTool(t, s, "start_errand_kandev", map[string]interface{}{
		"title":         "Check logs",
		"prompt":        "Find the failing test",
		"agent_profile": "fast",
	})

	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPStartErrand, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "parent-session", payload["sender_session_id"])
	assert.Equal(t, "task-1", payload["sender_task_id"])
	assert.Equal(t, "task-1", payload["task_id"])
	assert.Equal(t, "Find the failing test", payload["prompt"])
	assert.Equal(t, "fast", payload["agent_profile"])
}

func TestStartErrand_RequiresTitleAndPrompt(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{}
	s := NewWithProfile(backend, "parent-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityErrands}, nil))

	result := callTool(t, s, "start_errand_kandev", map[string]interface{}{"title": "only a title"})
	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction, "an invalid start must not reach the backend")
}

func TestCancelErrand_TakesCallerIdentityFromTheSession(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{response: map[string]interface{}{"status": "cancelled"}}
	s := NewWithProfile(backend, "parent-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityErrands}, nil))

	result := callTool(t, s, "cancel_errand_kandev", map[string]interface{}{
		"errand_id": "e-1",
	})

	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCancelErrand, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "parent-session", payload["sender_session_id"])
	assert.Equal(t, "e-1", payload["errand_id"])
}

func TestAwaitErrands_RejectsOutOfRangeArguments(t *testing.T) {
	log := newTestLogger(t)
	cases := map[string]map[string]interface{}{
		"no ids":        {"errand_ids": []string{}, "mode": "all"},
		"six ids":       {"errand_ids": []string{"a", "b", "c", "d", "e", "f"}, "mode": "all"},
		"bad mode":      {"errand_ids": []string{"a"}, "mode": "some"},
		"negative wait": {"errand_ids": []string{"a"}, "mode": "any", "max_wait_seconds": -1},
		"long wait":     {"errand_ids": []string{"a"}, "mode": "any", "max_wait_seconds": 601},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			backend := &testBackend{}
			s := NewWithProfile(backend, "parent-session", "task-1", 0, log, "", false,
				mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityErrands}, nil))
			result := callTool(t, s, "await_errands_kandev", args)
			assert.True(t, result.IsError)
			assert.Empty(t, backend.lastAction, "an invalid await must not reach the backend")
		})
	}
}

// An await abandoned by the agent's MCP client releases the claim it holds, so
// the results it was about to deliver stay deliverable.
func TestAwaitErrands_ReleasesTheTokenWhenTheCallIsAbandoned(t *testing.T) {
	log := newTestLogger(t)
	backend := &abandonedAwaitBackend{started: make(chan struct{})}
	s := NewWithProfile(backend, "parent-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityErrands}, nil))

	handler := s.mcpServer.ListTools()["await_errands_kandev"].Handler
	request := mcplib.CallToolRequest{}
	request.Params.Name = "await_errands_kandev"
	request.Params.Arguments = map[string]interface{}{"errand_ids": []interface{}{"e-1"}, "mode": "any"}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = handler(ctx, request)
	}()
	<-backend.started
	cancel()
	<-done

	require.Equal(t, []string{ws.ActionMCPAwaitErrands, ws.ActionMCPAwaitErrandsRelease}, backend.actions)
	assert.NotEmpty(t, backend.token)
	assert.Equal(t, backend.token, backend.releasedToken)
}

func TestAwaitErrands_SendsCallerIdentityAndDefaultsWaitToTenMinutes(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{response: map[string]interface{}{"errands": []interface{}{}}}
	s := NewWithProfile(backend, "parent-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityErrands}, nil))

	result := callTool(t, s, "await_errands_kandev", map[string]interface{}{
		"errand_ids": []string{"e-1", "e-2"},
		"mode":       "all",
	})

	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPAwaitErrands, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "parent-session", payload["sender_session_id"])
	assert.EqualValues(t, 600, payload["max_wait_seconds"])
	assert.NotEmpty(t, payload["await_token"])
}

// The await keeps the MCP stream alive with progress notifications while the
// backend waits, as ask_user_question does.
func TestAwaitErrands_StreamsProgressWhileWaiting(t *testing.T) {
	prev := errandAwaitKeepAliveInterval
	errandAwaitKeepAliveInterval = 5 * time.Millisecond
	t.Cleanup(func() { errandAwaitKeepAliveInterval = prev })

	release := make(chan struct{})
	backend := &releasableAwaitBackend{release: release}
	log := newTestLogger(t)
	s := NewWithProfile(backend, "parent-session", "task-1", 0, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{mcpprofile.CapabilityErrands}, nil))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	s.RegisterRoutes(router)
	ts := httptest.NewServer(router)
	defer ts.Close()

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`
	initResp := postJSONRPC(t, ts.URL+"/mcp", initReq, "")
	require.Equal(t, http.StatusOK, initResp.statusCode, "init: %s", initResp.body)

	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"await_errands_kandev","arguments":{"errand_ids":["e-1"],"mode":"any"}}}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(callBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", initResp.sessionID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	released := false
	progressSeen := 0
	finalSeen := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		switch {
		case strings.Contains(payload, "notifications/progress"):
			progressSeen++
			if !released {
				close(release)
				released = true
			}
		case strings.Contains(payload, `"id":2`):
			finalSeen = true
		}
		if finalSeen {
			break
		}
	}
	require.NoError(t, scanner.Err())
	if !released {
		close(release)
	}
	assert.GreaterOrEqual(t, progressSeen, 1, "expected at least one await progress notification")
	assert.True(t, finalSeen, "expected the await result to be delivered")
}

// abandonedAwaitBackend blocks the await until its context ends and records
// every action so the test can check the release that follows.
type abandonedAwaitBackend struct {
	started       chan struct{}
	actions       []string
	token         string
	releasedToken string
}

func (b *abandonedAwaitBackend) RequestPayload(ctx context.Context, action string, payload, _ interface{}) error {
	b.actions = append(b.actions, action)
	fields, _ := payload.(map[string]interface{})
	switch action {
	case ws.ActionMCPAwaitErrands:
		b.token, _ = fields["await_token"].(string)
		close(b.started)
		<-ctx.Done()
		return ctx.Err()
	case ws.ActionMCPAwaitErrandsRelease:
		b.releasedToken, _ = fields["await_token"].(string)
	}
	return nil
}

// releasableAwaitBackend holds the await until the test releases it.
type releasableAwaitBackend struct {
	release <-chan struct{}
}

func (b *releasableAwaitBackend) RequestPayload(ctx context.Context, action string, _, result interface{}) error {
	if action == ws.ActionMCPAwaitErrands {
		select {
		case <-b.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if result != nil {
		data, _ := json.Marshal(map[string]interface{}{"errands": []interface{}{}})
		return json.Unmarshal(data, result)
	}
	return nil
}
