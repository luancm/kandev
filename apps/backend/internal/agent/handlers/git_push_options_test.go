package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/agentctl"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestWsPushForwardsPushOptions proves the ws payload's two optional inputs
// reach the agentctl request body together, rather than one being dropped.
func TestWsPushForwardsPushOptions(t *testing.T) {
	var body map[string]any
	h, server := gitHandlerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"success":true,"operation":"push","output":"done","pushed_remote":"backup","pushed_branch":"feature/work"}`))
	})
	defer server.Close()

	msg, err := ws.NewRequest("id", "action", GitPushRequest{
		SessionID:      "s",
		Repo:           "repo",
		Remote:         "backup",
		ExpectedBranch: "feature/work",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := h.wsPush(context.Background(), msg)
	if err != nil {
		t.Fatalf("wsPush: %v", err)
	}
	if body["remote"] != "backup" {
		t.Errorf("body[remote] = %#v, want backup", body["remote"])
	}
	if body["expected_branch"] != "feature/work" {
		t.Errorf("body[expected_branch] = %#v, want feature/work", body["expected_branch"])
	}
	// The destination fields must survive the round trip back to the caller.
	payload := string(response.Payload)
	if !strings.Contains(payload, `"pushed_remote":"backup"`) {
		t.Errorf("response payload lost pushed_remote: %s", payload)
	}
	if !strings.Contains(payload, `"pushed_branch":"feature/work"`) {
		t.Errorf("response payload lost pushed_branch: %s", payload)
	}
}

func TestWsPushCallbacksCarryRepositoryScopeAndTimestamp(t *testing.T) {
	for _, tt := range []struct {
		name    string
		success bool
		body    string
	}{
		{
			name:    "success",
			success: true,
			body:    `{"success":true,"operation":"push","pushed_remote":"origin","pushed_branch":"main"}`,
		},
		{
			name:    "failure",
			success: false,
			body:    `{"success":false,"operation":"push","error":"rejected"}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, server := gitHandlerServer(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			})
			defer server.Close()

			callback := make(chan *client.GitOperationResult, 1)
			if tt.success {
				h.SetOnGitOperationSucceededWithResult(func(_ context.Context, _, _, _ string, result *client.GitOperationResult, _ string) {
					callback <- result
				})
			} else {
				h.SetOnGitOperationFailedWithResult(func(_ context.Context, _, _, _ string, result *client.GitOperationResult) {
					callback <- result
				})
			}

			before := time.Now().UTC()
			msg, err := ws.NewRequest("id", "action", GitPushRequest{SessionID: "s", Repo: "services"})
			if err != nil {
				t.Fatal(err)
			}
			response, err := h.wsPush(context.Background(), msg)
			if err != nil {
				t.Fatalf("wsPush: %v", err)
			}
			after := time.Now().UTC()

			var got *client.GitOperationResult
			select {
			case got = <-callback:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for git callback")
			}
			if got.Repository != "services" {
				t.Errorf("callback repository = %q, want services", got.Repository)
			}
			if got.OccurredAt.Before(before) || got.OccurredAt.After(after) {
				t.Errorf("callback occurrence time = %s, want between %s and %s", got.OccurredAt, before, after)
			}
			if strings.Contains(string(response.Payload), `"repository":`) {
				t.Errorf("callback metadata leaked into response payload: %s", response.Payload)
			}
		})
	}
}

func TestWsPushFreshStatusCarriesRepositoryScope(t *testing.T) {
	h, server := gitHandlerServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/git/push":
			_, _ = w.Write([]byte(`{"success":true,"operation":"push"}`))
		case "/api/v1/git/status":
			_, _ = w.Write([]byte(`{"success":true,"repository_name":"service-repo","branch":"main","remote_branch":"origin/main","head_commit":"head","remote_head_commit":"head","remote_ahead":0,"remote_behind":0}`))
		default:
			http.NotFound(w, r)
		}
	})
	defer server.Close()

	statusCh := make(chan *client.GitStatusResult, 1)
	h.SetOnGitOperationSucceededWithStatus(func(_ context.Context, _, _, _ string, status *client.GitStatusResult) {
		statusCh <- status
	})
	msg, err := ws.NewRequest("id", "action", GitPushRequest{SessionID: "s", Repo: "services"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.wsPush(context.Background(), msg); err != nil {
		t.Fatalf("wsPush: %v", err)
	}
	select {
	case status := <-statusCh:
		if status.Repository != "services" {
			t.Errorf("status repository = %q, want services", status.Repository)
		}
		if status.RepositoryName != "service-repo" {
			t.Errorf("agentctl repository name = %q, want service-repo", status.RepositoryName)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fresh status callback")
	}
}
