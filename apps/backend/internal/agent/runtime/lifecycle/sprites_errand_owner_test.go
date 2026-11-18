package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	sprites "github.com/superfly/sprites-go"
)

func TestSpriteInstanceLookupDistinguishesConfirmedAbsence(t *testing.T) {
	target := httptest.NewServer(http.NotFoundHandler())
	defer target.Close()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sprites/test/proxy" {
			proxyAgentctlWebSocket(w, r, upgrader, target.Listener.Addr().String(), spritesAgentctlPort, 41001)
			return
		}
		if r.URL.Path == "/v1/sprites/test" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"test","id":"test","status":"running"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer api.Close()
	client := sprites.New("token", sprites.WithBaseURL(api.URL), sprites.WithDisableControl())
	sprite, err := client.GetSprite(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	exec := newTestSpritesExecutor(nil)
	exec.agentctlPort = spritesAgentctlPort
	_, err = exec.getExistingInstanceInfo(context.Background(), sprite, "missing")
	if !errors.Is(err, agentctl.ErrInstanceNotFound) {
		t.Fatalf("lookup error = %v, want confirmed absence", err)
	}
}

func TestSpriteErrandReconnectRequiresExactOwnerEvidence(t *testing.T) {
	var mu sync.Mutex
	status := http.StatusOK
	payload := `{"id":"old","port":41001,"errand_mcp_owner":{"task_id":"task","errand_id":"child","generation":1}}`
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}))
	defer target.Close()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sprites/test/proxy" {
			proxyAgentctlWebSocket(w, r, upgrader, target.Listener.Addr().String(), spritesAgentctlPort, 41001)
			return
		}
		if r.URL.Path == "/v1/sprites/test" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"test","id":"test","status":"running"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer api.Close()
	sprite, err := sprites.New("token", sprites.WithBaseURL(api.URL), sprites.WithDisableControl()).GetSprite(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	exec := newTestSpritesExecutor(nil)
	exec.agentctlPort = spritesAgentctlPort
	req := &ExecutorCreateRequest{ErrandMCPOwner: errandMCPOwner(ExecutionOwner{Kind: ExecutionOwnerErrand, TaskID: "task", ErrandID: "child", Generation: 1})}
	port, err := exec.spriteReconnectPort(context.Background(), sprite, req, "old")
	if err != nil || port != 41001 {
		t.Fatalf("matching owner port=%d err=%v", port, err)
	}
	mu.Lock()
	payload = `{"id":"old","port":41001,"errand_mcp_owner":{"task_id":"task","errand_id":"foreign","generation":1}}`
	mu.Unlock()
	if _, err = exec.spriteReconnectPort(context.Background(), sprite, req, "old"); err == nil {
		t.Fatal("foreign owner accepted")
	}
	mu.Lock()
	status = http.StatusInternalServerError
	mu.Unlock()
	if _, err = exec.spriteReconnectPort(context.Background(), sprite, req, "old"); err == nil || errors.Is(err, agentctl.ErrInstanceNotFound) {
		t.Fatalf("uncertain lookup error = %v", err)
	}
	mu.Lock()
	status = http.StatusNotFound
	mu.Unlock()
	if _, err = exec.spriteReconnectPort(context.Background(), sprite, req, "old"); !errors.Is(err, agentctl.ErrInstanceNotFound) {
		t.Fatalf("absence error = %v", err)
	}
}
