package statussummary

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// errandsSource is an ErrandsLoader stand-in. The test changes the
// authoritative errand summary and observes which events re-read it.
type errandsSource struct {
	mu      sync.Mutex
	current ErrandsSummary
	reads   int
	err     error
}

func (s *errandsSource) set(summary ErrandsSummary) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = summary
}

func (s *errandsSource) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *errandsSource) load(context.Context, string) (ErrandsSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	return s.current, s.err
}

func (s *errandsSource) takeReads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	reads := s.reads
	s.reads = 0
	return reads
}

func TestTaskStatusSummaryErrandsRoundTripsAndValidates(t *testing.T) {
	summary := TaskStatusSummary{Errands: &ErrandsSummary{Live: 2, Waiting: true}}
	payload, err := summary.SemanticJSON()
	if err != nil {
		t.Fatalf("semantic JSON: %v", err)
	}
	var decoded TaskStatusSummary
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode semantic JSON: %v", err)
	}
	if decoded.Errands == nil || *decoded.Errands != *summary.Errands {
		t.Fatalf("errands changed after round trip: %+v", decoded.Errands)
	}
	if !summary.SemanticEqual(decoded) {
		t.Fatal("errands summary should be semantic across a round trip")
	}
	if summary.SemanticEqual(TaskStatusSummary{Errands: &ErrandsSummary{Live: 2}}) {
		t.Fatal("a change in the waiting flag must change semantic equality")
	}

	negative := TaskStatusSummary{Errands: &ErrandsSummary{Live: -1}}
	if err := negative.Validate(); err == nil {
		t.Fatal("summary accepted a negative live errand count")
	}
}

func TestTaskStatusSummaryOmitsErrandsWhenNone(t *testing.T) {
	payload, err := (TaskStatusSummary{Revision: 3}).SemanticJSON()
	if err != nil {
		t.Fatalf("semantic JSON: %v", err)
	}
	if strings.Contains(string(payload), "errands") {
		t.Fatalf("summary without errands carries the field: %s", payload)
	}
}

func TestProjectorDerivesErrandsFromLoaderAndOmitsZero(t *testing.T) {
	projector, store, eventBus, _, _ := newProjectorTest(t)
	source := &errandsSource{}
	projector.loadErrands = source.load
	const taskID = "task-errands-derive"

	source.set(ErrandsSummary{Live: 2, Waiting: true})
	publishProjectorEvent(t, eventBus, events.ErrandUpdated, events.ErrandUpdated, map[string]interface{}{
		"task_id": taskID, "workspace_id": "workspace-1",
	})
	if got := store.summary(taskID); got == nil || got.Errands == nil || *got.Errands != (ErrandsSummary{Live: 2, Waiting: true}) {
		t.Fatalf("errands not projected: %+v", got)
	}

	source.set(ErrandsSummary{})
	publishProjectorEvent(t, eventBus, events.ErrandUpdated, events.ErrandUpdated, map[string]interface{}{
		"task_id": taskID, "workspace_id": "workspace-1",
	})
	if got := store.summary(taskID); got == nil || got.Errands != nil {
		t.Fatalf("zero errand projection must be omitted: %+v", got)
	}
}

// errandRefreshEvents are the sources that can change the errand projection.
var errandRefreshEvents = []string{events.ErrandUpdated, events.TaskSessionStateChanged, events.SessionRemoved}

func TestProjectorRefreshesErrandsOnlyOnErrandSources(t *testing.T) {
	const taskID = "task-errands-sources"
	for _, eventType := range errandRefreshEvents {
		projector, _, _, _, _ := newProjectorTest(t)
		source := &errandsSource{current: ErrandsSummary{Live: 1}}
		projector.loadErrands = source.load
		handleTaskEvent(t, projector, eventType, taskID, nil)
		if reads := source.takeReads(); reads == 0 {
			t.Fatalf("%s did not re-read the errand projection", eventType)
		}
	}

	projector, _, _, _, _ := newProjectorTest(t)
	source := &errandsSource{current: ErrandsSummary{Live: 1}}
	projector.loadErrands = source.load
	handleTaskEvent(t, projector, events.TaskUpdated, taskID, nil)
	source.takeReads()
	handleTaskEvent(t, projector, events.MessageAdded, taskID, map[string]interface{}{
		"author_type": "agent", "type": "message",
	})
	if reads := source.takeReads(); reads != 0 {
		t.Fatalf("unrelated message re-read the errand projection %d times", reads)
	}
}

func TestProjectorRestoresErrandsAfterRestartAndKeepsStoredBaselineWithoutLoader(t *testing.T) {
	const taskID = "task-errands-restart"
	projector, store, _, _, _ := newProjectorTest(t)
	store.rows[taskID] = &StoredTaskStatusSummary{
		TaskID: taskID, WorkspaceID: "workspace-1",
		Summary: TaskStatusSummary{Revision: 4, Errands: &ErrandsSummary{Live: 3}},
	}
	handleTaskEvent(t, projector, events.TaskUpdated, taskID, nil)
	if got := store.summary(taskID); got == nil || got.Errands == nil || got.Errands.Live != 3 {
		t.Fatalf("stored errand baseline lost without a loader: %+v", got)
	}

	source := &errandsSource{current: ErrandsSummary{Live: 1, Waiting: true}}
	restarted := NewProjector(ProjectorConfig{
		Store:            store,
		ResolveWorkspace: func(context.Context, string) (string, error) { return "workspace-1", nil },
		LoadErrands:      source.load,
	})
	handleTaskEvent(t, restarted, events.ErrandUpdated, taskID, nil)
	if got := store.summary(taskID); got == nil || got.Errands == nil || *got.Errands != (ErrandsSummary{Live: 1, Waiting: true}) {
		t.Fatalf("restarted projector did not re-read the authoritative errands: %+v", got)
	}
}

func TestProjectorErrandLoadFailureIsNotSilent(t *testing.T) {
	projector, _, _, _, _ := newProjectorTest(t)
	source := &errandsSource{}
	source.fail(context.DeadlineExceeded)
	projector.loadErrands = source.load
	event := bus.NewEvent(events.ErrandUpdated, "test", map[string]interface{}{"task_id": "task-errands-fail", "workspace_id": "workspace-1"})
	if err := projector.handleEvent(context.Background(), event); err == nil {
		t.Fatal("errand load failure was swallowed")
	}
}
