package dto

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeErrandWaitingProvider is the live provider with errand support: it
// reports the errand state of one parent session.
type fakeErrandWaitingProvider struct {
	fakeForegroundActivityProvider
	hasErrands  bool
	outstanding bool
}

func (f *fakeErrandWaitingProvider) ParentErrandState(string) (bool, bool) {
	return f.hasErrands, f.outstanding
}

// Parents without an errand row carry no waiting_on_errands key at all, so
// existing sessions keep their payloads byte-for-byte.
// @covers AC-TASKS-ERRANDS-006.3
func TestWaitingOnErrandsIsOmittedWithoutAnErrandRow(t *testing.T) {
	session := &models.TaskSession{ID: "parent-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput}
	providers := map[string]ForegroundActivityProvider{
		"no errand support": &fakeForegroundActivityProvider{value: v1.ForegroundActivityGenerating},
		"no errand rows":    &fakeErrandWaitingProvider{hasErrands: false, outstanding: false},
	}
	for name, provider := range providers {
		full := FromTaskSession(session)
		EnrichForegroundActivity(&full, provider)
		if full.WaitingOnErrands != nil {
			t.Fatalf("%s: full WaitingOnErrands = %v, want nil", name, *full.WaitingOnErrands)
		}
		if _, ok := encodeToMap(t, full)["waiting_on_errands"]; ok {
			t.Fatalf("%s: full payload carries waiting_on_errands", name)
		}

		summary := FromTaskSessionSummary(session)
		EnrichForegroundActivitySummary(&summary, provider)
		if _, ok := encodeToMap(t, summary)["waiting_on_errands"]; ok {
			t.Fatalf("%s: summary payload carries waiting_on_errands", name)
		}
	}
}

// Once the parent owns an errand row, the field is always explicit, true only
// while the idle parent is outstanding, and false for every other state.
// @covers AC-TASKS-ERRANDS-006.3
func TestWaitingOnErrandsProjectsOnceTheParentOwnsAnErrandRow(t *testing.T) {
	tests := []struct {
		name        string
		state       models.TaskSessionState
		outstanding bool
		want        bool
	}{
		{name: "idle with outstanding errands", state: models.TaskSessionStateWaitingForInput, outstanding: true, want: true},
		{name: "idle with nothing outstanding", state: models.TaskSessionStateWaitingForInput, outstanding: false, want: false},
		{name: "running with outstanding errands", state: models.TaskSessionStateRunning, outstanding: true, want: false},
		{name: "running with nothing outstanding", state: models.TaskSessionStateRunning, outstanding: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &models.TaskSession{ID: "parent-1", TaskID: "task-1", State: tt.state}
			provider := &fakeErrandWaitingProvider{hasErrands: true, outstanding: tt.outstanding}

			full := FromTaskSession(session)
			EnrichForegroundActivity(&full, provider)
			summary := FromTaskSessionSummary(session)
			EnrichForegroundActivitySummary(&summary, provider)

			for name, payload := range map[string]map[string]interface{}{
				"full":    encodeToMap(t, full),
				"summary": encodeToMap(t, summary),
			} {
				if payload["waiting_on_errands"] != tt.want {
					t.Fatalf("%s waiting_on_errands = %v, want explicit %v", name, payload["waiting_on_errands"], tt.want)
				}
			}
		})
	}
}

// WaitingOnErrands is the single derivation behind the DTO and the session
// events, so the idle-only rule is pinned here.
// @covers AC-TASKS-ERRANDS-006.3
func TestWaitingOnErrandsDerivesFromIdleStateAndOutstanding(t *testing.T) {
	if !WaitingOnErrands(models.TaskSessionStateWaitingForInput, true) {
		t.Fatal("idle parent with outstanding errands must wait on them")
	}
	for _, state := range []models.TaskSessionState{models.TaskSessionStateRunning, models.TaskSessionStateCompleted} {
		if WaitingOnErrands(state, true) {
			t.Fatalf("state %s reported waiting on errands", state)
		}
	}
}

// The session-event value follows the same derivation and stays nil for a
// parent without errand rows, so the event payload only gains the key then.
// @covers AC-TASKS-ERRANDS-006.3
func TestErrandWaitingPayloadMatchesTheDTOProjection(t *testing.T) {
	if got := ErrandWaitingPayload("parent-1", models.TaskSessionStateWaitingForInput, nil); got != nil {
		t.Fatalf("nil provider payload = %v, want nil", *got)
	}
	none := &fakeErrandWaitingProvider{hasErrands: false}
	if got := ErrandWaitingPayload("parent-1", models.TaskSessionStateWaitingForInput, none); got != nil {
		t.Fatalf("payload without errand rows = %v, want nil", *got)
	}
	waiting := &fakeErrandWaitingProvider{hasErrands: true, outstanding: true}
	got := ErrandWaitingPayload("parent-1", models.TaskSessionStateWaitingForInput, waiting)
	if got == nil || !*got {
		t.Fatalf("payload = %v, want explicit true", got)
	}
}
