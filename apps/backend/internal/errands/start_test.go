package errands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ERRANDS-001.1 AC-TASKS-ERRANDS-001.2 AC-TASKS-ERRANDS-001.6
func TestStartLaunchesWithReservedIdentityAndTheParentMCPSnapshot(t *testing.T) {
	h := newHarness(t)
	result := h.mustStart("audit")

	if result.Status != models.ErrandStatusRunning || result.ProfileName != "Reviewer" {
		t.Fatalf("result = %s/%s, want running/Reviewer", result.Status, result.ProfileName)
	}
	calls := h.launcher.calls()
	if len(calls) != 1 {
		t.Fatalf("launch calls = %d, want 1", len(calls))
	}
	call, identity := calls[0], h.launcher.identity[0]
	errand := result.Errand
	if call.ErrandID != errand.ID || call.TaskID != testTask || call.ParentSessionID != testParent ||
		call.AgentProfileID != "profile-1" || call.Objective != "Do audit" || call.Title != "audit" {
		t.Fatalf("launch request = %+v", call)
	}
	if identity.SessionID != errand.ID || identity.TurnID != errand.FirstTurnID ||
		identity.SubmissionID != errand.SubmissionID || identity.TaskID != testTask {
		t.Fatalf("reserved identity = %+v, want the errand's reserved identity", identity)
	}
	if strings.Join(call.MCPServerIDs, ",") != "mcp-1,mcp-2" || strings.Join(errand.MCPServerIDs, ",") != "mcp-1,mcp-2" {
		t.Fatalf("MCP snapshot = %v/%v, want the parent's selections", call.MCPServerIDs, errand.MCPServerIDs)
	}
	if h.profiles.defaultP != "parent-profile" || h.profiles.selector != "" {
		t.Fatalf("profile resolution got selector %q default %q, want the parent's profile as the default",
			h.profiles.selector, h.profiles.defaultP)
	}
	stored, err := h.store.GetErrand(context.Background(), errand.ID)
	if err != nil || stored.Lifecycle != models.ErrandLifecycleLaunched || !stored.AwaitingResult {
		t.Fatalf("stored errand = %+v (%v), want launched and awaiting its first result", stored, err)
	}
}

func TestStartPublishesTheQueuedRowBeforeCallingTheLauncher(t *testing.T) {
	h := newHarness(t)
	var order []string
	h.publisher.log = &order
	h.launcher.onCall = func(LaunchRequest) { order = append(order, "launch") }
	h.mustStart("audit")

	if got := strings.Join(order, ","); got != "publish:queued,launch,publish:running" {
		t.Fatalf("event order = %s, want queued row published before the launcher runs", got)
	}
}

// @covers AC-TASKS-ERRANDS-001.4
func TestStartRefusesIneligibleParentsAndCreatesNothing(t *testing.T) {
	cases := map[string]*models.TaskSession{
		"errand parent":      {ID: "errand-parent", TaskID: testTask, Role: models.SessionRoleErrand},
		"passthrough parent": {ID: "pty-parent", TaskID: testTask, IsPassthrough: true},
		"other task parent":  {ID: "other-parent", TaskID: "task-2"},
	}
	for name, parent := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.store.addSession(parent)
			_, err := h.service.Start(context.Background(), StartRequest{
				TaskID: testTask, ParentSessionID: parent.ID, Title: "x", Objective: "y",
			})
			assertReason(t, err, ReasonNotSupported)
			assertNothingCreated(t, h)
		})
	}
	t.Run("missing parent", func(t *testing.T) {
		h := newHarness(t)
		_, err := h.service.Start(context.Background(), StartRequest{
			TaskID: testTask, ParentSessionID: "ghost", Title: "x", Objective: "y",
		})
		assertReason(t, err, ReasonNotSupported)
		assertNothingCreated(t, h)
	})
}

func TestStartRejectsBlankArgumentsAndProfileFailures(t *testing.T) {
	h := newHarness(t)
	_, err := h.service.Start(context.Background(), StartRequest{TaskID: testTask, ParentSessionID: testParent, Title: " ", Objective: "y"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("blank title error = %v, want ErrInvalidRequest", err)
	}

	h.profiles.err = newError(ReasonProfileAmbiguous, "ambiguous", map[string]interface{}{"available": []string{"a"}})
	_, err = h.start("audit")
	assertReason(t, err, ReasonProfileAmbiguous)
	assertNothingCreated(t, h)
}

// @covers AC-TASKS-ERRANDS-008.1
func TestStartRefusesTheSixthLiveErrandWithLimitDetails(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < MaxLiveErrands; i++ {
		h.mustStart(fmt.Sprintf("errand %d", i))
	}
	_, err := h.start("one too many")
	assertReason(t, err, ReasonLimitReached)
	var typed *Error
	if !errors.As(err, &typed) || typed.Details["limit"] != MaxLiveErrands || typed.Details["live"] != MaxLiveErrands {
		t.Fatalf("details = %v, want limit and live", typed.Details)
	}
	rows, _ := h.store.ListErrandsByTask(context.Background(), testTask)
	if len(rows) != MaxLiveErrands || len(h.launcher.calls()) != MaxLiveErrands {
		t.Fatalf("rows = %d launches = %d, want exactly %d", len(rows), len(h.launcher.calls()), MaxLiveErrands)
	}
}

func TestStartEnforcesTheLimitUnderConcurrentStarts(t *testing.T) {
	h := newHarness(t)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.start(fmt.Sprintf("errand %d", i))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	started, refused := 0, 0
	for err := range results {
		if err == nil {
			started++
			continue
		}
		if reason, _ := ReasonOf(err); reason != ReasonLimitReached {
			t.Fatalf("unexpected error: %v", err)
		}
		refused++
	}
	if started != MaxLiveErrands || refused != 12-MaxLiveErrands {
		t.Fatalf("started %d refused %d, want %d and %d", started, refused, MaxLiveErrands, 12-MaxLiveErrands)
	}
}

// @covers AC-TASKS-ERRANDS-008.2
func TestStartQueuesWhenTheCeilingRefusesAndWritesNoSession(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(int, LaunchRequest) error { return ErrCeilingRefused }
	result := h.mustStart("audit")

	if result.Status != models.ErrandStatusQueued || result.Errand.Lifecycle != models.ErrandLifecycleQueued {
		t.Fatalf("result = %s/%s, want a queued errand", result.Status, result.Errand.Lifecycle)
	}
	if _, err := h.store.GetTaskSession(context.Background(), result.Errand.ID); err == nil {
		t.Fatal("a queued errand created a session row")
	}
}

// @covers AC-TASKS-ERRANDS-001.3
func TestStartRemovesTheErrandWhenLaunchFailsBeforeAnySession(t *testing.T) {
	h := newHarness(t)
	workspaceErr := errors.New("workspace preparing")
	h.launcher.behave = func(int, LaunchRequest) error { return workspaceErr }
	_, err := h.start("audit")

	if !errors.Is(err, workspaceErr) {
		t.Fatalf("error = %v, want the launcher's error preserved for classification", err)
	}
	rows, _ := h.store.ListErrandsByTask(context.Background(), testTask)
	if len(rows) != 0 {
		t.Fatalf("errand rows after a synchronous failure = %d, want none", len(rows))
	}
	statuses := h.publisher.statuses()
	if len(statuses) != 2 || statuses[0] != models.ErrandStatusQueued || statuses[1] != models.ErrandStatusFailed {
		t.Fatalf("published = %v, want queued then failed so clients drop the placeholder", statuses)
	}
}

func TestStartKeepsAFailedErrandWhenTheLaunchLeftASession(t *testing.T) {
	h := newHarness(t)
	h.launcher.behave = func(_ int, request LaunchRequest) error {
		h.store.addSession(&models.TaskSession{
			ID: request.ErrandID, TaskID: request.TaskID, Role: models.SessionRoleErrand,
			ParentSessionID: request.ParentSessionID, State: models.TaskSessionStateFailed,
		})
		return errors.New("agent failed to start")
	}
	result := h.mustStart("audit")

	if result.Status != models.ErrandStatusFailed || result.Errand.LastOutcome != models.ErrandOutcomeFailed {
		t.Fatalf("result = %s/%s, want a failed errand", result.Status, result.Errand.LastOutcome)
	}
	if result.Errand.ResultKind != models.ErrandResultFailure || result.Errand.ResultState != models.ErrandResultPending ||
		!strings.Contains(result.Errand.ResultText, "agent failed to start") {
		t.Fatalf("result = %+v, want a pending failure result", result.Errand)
	}
	if got := h.deliverer.delivered(); len(got) != 1 || got[0] != testParent {
		t.Fatalf("deliveries = %v, want one for the parent", got)
	}
}

func assertReason(t *testing.T, err error, want Reason) {
	t.Helper()
	got, ok := ReasonOf(err)
	if !ok || got != want {
		t.Fatalf("error = %v (reason %q), want reason %q", err, got, want)
	}
}

func assertNothingCreated(t *testing.T, h *harness) {
	t.Helper()
	rows, _ := h.store.ListErrandsByTask(context.Background(), testTask)
	if len(rows) != 0 || len(h.launcher.calls()) != 0 || len(h.publisher.statuses()) != 0 {
		t.Fatalf("rows=%d launches=%d events=%d, want none", len(rows), len(h.launcher.calls()), len(h.publisher.statuses()))
	}
}
