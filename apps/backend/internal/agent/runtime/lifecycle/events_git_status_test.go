package lifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/gitremote"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// Verifies the fix for "Changes panel shows no header / no existing changes".
// agentctl tags every per-repo GitStatusUpdate with RepositoryName; that field
// must survive the lifecycle PublishGitStatus translation so the orchestrator
// (and thus the frontend) sees it.
func TestPublishGitStatus_PropagatesRepositoryName(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	eventBus := bus.NewMemoryEventBus(log)
	pub := NewEventPublisher(eventBus, log)

	received := make(chan *bus.Event, 1)
	subj := events.BuildGitEventSubject("sess-multi")
	sub, err := eventBus.Subscribe(subj, func(_ context.Context, ev *bus.Event) error {
		received <- ev
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	exec := &AgentExecution{
		ID:        "exec-1",
		TaskID:    "task-1",
		SessionID: "sess-multi",
	}
	pub.PublishGitStatus(exec, &agentctl.GitStatusUpdate{
		Timestamp:        time.Now(),
		RepositoryName:   "frontend",
		IsSubmodule:      true,
		Branch:           "feature/x",
		HeadCommit:       "local-head",
		BaseCommit:       "base-head",
		RemoteBranch:     "contributor/feature/x",
		RemoteHeadCommit: "upstream-head",
		RemoteAhead:      2,
		RemoteBehind:     1,
		Modified:         []string{"src/app.tsx"},
		Files:            map[string]agentctl.FileInfo{"src/app.tsx": {Path: "src/app.tsx"}},
	})

	select {
	case ev := <-received:
		payload, ok := ev.Data.(*GitEventPayload)
		if !ok || payload == nil {
			t.Fatalf("expected *GitEventPayload, got %T", ev.Data)
		}
		if payload.Status == nil {
			t.Fatal("expected non-nil Status on payload")
		}
		if payload.Status.RepositoryName != "frontend" {
			t.Errorf("repository_name was dropped: got %q", payload.Status.RepositoryName)
		}
		if !payload.Status.IsSubmodule {
			t.Error("is_submodule was dropped")
		}
		if payload.Status.HeadCommit != "local-head" || payload.Status.BaseCommit != "base-head" {
			t.Errorf("commit comparison SHAs were dropped: head=%q base=%q", payload.Status.HeadCommit, payload.Status.BaseCommit)
		}
		if payload.Status.RemoteHeadCommit != "upstream-head" || payload.Status.RemoteAhead != 2 || payload.Status.RemoteBehind != 1 {
			t.Errorf("upstream evidence was dropped: head=%q ahead=%d behind=%d", payload.Status.RemoteHeadCommit, payload.Status.RemoteAhead, payload.Status.RemoteBehind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for git status event")
	}
}

func TestPublishGitStatus_PropagatesHeadRemote(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	eventBus := bus.NewMemoryEventBus(log)
	pub := NewEventPublisher(eventBus, log)

	received := make(chan *bus.Event, 1)
	subj := events.BuildGitEventSubject("sess-head")
	sub, err := eventBus.Subscribe(subj, func(_ context.Context, ev *bus.Event) error {
		received <- ev
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	exec := &AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "sess-head"}
	pub.PublishGitStatus(exec, &agentctl.GitStatusUpdate{
		Timestamp:             time.Now(),
		Branch:                "feature/x",
		RemoteRolesGeneration: "generation-1",
		ActionHead:            &gitremote.RemoteRefObservation{State: gitremote.ObservationUnknown},
		TrackingUpstream:      &gitremote.RemoteRefObservation{State: gitremote.ObservationAbsent},
		HeadRemote: &agentctl.GitHeadRemote{
			Provider: "github",
			Host:     "github.com",
			Owner:    "fork",
			Repo:     "project",
			Branch:   "feature/x",
		},
	})

	select {
	case ev := <-received:
		payload, ok := ev.Data.(*GitEventPayload)
		if !ok || payload == nil || payload.Status == nil {
			t.Fatalf("expected status payload, got %T", ev.Data)
		}
		body, err := json.Marshal(payload.Status)
		if err != nil {
			t.Fatalf("marshal status: %v", err)
		}
		var status map[string]any
		if err := json.Unmarshal(body, &status); err != nil {
			t.Fatalf("unmarshal status: %v", err)
		}
		head, ok := status["head_remote"].(map[string]any)
		if !ok || head["owner"] != "fork" || head["repo"] != "project" {
			t.Fatalf("head_remote = %v, want propagated fork/project identity", status["head_remote"])
		}
		if status["remote_roles_generation"] != "generation-1" {
			t.Fatalf("remote roles generation = %v, want generation-1", status["remote_roles_generation"])
		}
		if action, ok := status["action_head"].(map[string]any); !ok || action["observation_state"] != string(gitremote.ObservationUnknown) {
			t.Fatalf("action_head = %v, want unknown observation", status["action_head"])
		}
		if tracking, ok := status["tracking_upstream"].(map[string]any); !ok || tracking["observation_state"] != string(gitremote.ObservationAbsent) {
			t.Fatalf("tracking_upstream = %v, want absent observation", status["tracking_upstream"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for git status event")
	}
}

func TestPublishGitStatus_PropagatesComparisonEvidence(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	eventBus := bus.NewMemoryEventBus(log)
	pub := NewEventPublisher(eventBus, log)
	received := make(chan *bus.Event, 1)
	sub, err := eventBus.Subscribe(events.BuildGitEventSubject("sess-comparison"), func(_ context.Context, ev *bus.Event) error {
		received <- ev
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	pub.PublishGitStatus(&AgentExecution{ID: "exec-1", TaskID: "task-1", SessionID: "sess-comparison"}, &agentctl.GitStatusUpdate{
		Timestamp: time.Now(),
		Comparison: &agentctl.GitComparisonStatus{
			ContextGeneration: "generation-1",
			State:             gitremote.ResolutionUnresolved,
			Reason:            "comparison ref is not available locally",
			BaseCommit:        "stored-base",
		},
	})

	select {
	case ev := <-received:
		payload, ok := ev.Data.(*GitEventPayload)
		if !ok || payload == nil || payload.Status == nil || payload.Status.Comparison == nil {
			t.Fatalf("comparison evidence dropped: %T", ev.Data)
		}
		if payload.Status.Comparison.State != gitremote.ResolutionUnresolved || payload.Status.Comparison.BaseCommit != "stored-base" {
			t.Fatalf("comparison evidence = %+v", payload.Status.Comparison)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for comparison status event")
	}
}
