package errands

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type blockedTurnReader struct {
	readStarted chan struct{}
	readRelease chan struct{}
	view        TurnView
	latestID    string
	latest      TurnView
}

type cancellationObservedStore struct {
	Store
	cancelled chan struct{}
}

func (s *cancellationObservedStore) UpdateErrand(ctx context.Context, row *models.Errand) error {
	err := s.Store.UpdateErrand(ctx, row)
	if err == nil && row.Lifecycle == models.ErrandLifecycleCancelled {
		select {
		case s.cancelled <- struct{}{}:
		default:
		}
	}
	return err
}

func (r *blockedTurnReader) ReadTurn(context.Context, string, string) (TurnView, error) {
	r.readStarted <- struct{}{}
	<-r.readRelease
	return r.view, nil
}

func (r *blockedTurnReader) LatestTurn(context.Context, string) (string, TurnView, error) {
	return r.latestID, r.latest, nil
}

func cancelForSettlementRace(ctx context.Context, h *harness, errand *models.Errand, kind string) error {
	switch kind {
	case "user":
		_, err := h.service.Cancel(ctx, cancelRequest(errand, models.ErrandCancelSourceUser))
		return err
	case "parent":
		_, err := h.service.Cancel(ctx, cancelRequest(errand, models.ErrandCancelSourceParent))
		return err
	case "parent_cascade":
		count, err := h.service.CancelForParent(ctx, testParent)
		if err == nil && count != 1 {
			return fmt.Errorf("CancelForParent count = %d, want 1", count)
		}
		return err
	case "task_cascade":
		count, err := h.service.CancelForTask(ctx, testTask)
		if err == nil && count != 1 {
			return fmt.Errorf("CancelForTask count = %d, want 1", count)
		}
		return err
	default:
		return nil
	}
}

// @covers AC-TASKS-ERRANDS-003.5, AC-TASKS-ERRANDS-007.1, AC-TASKS-ERRANDS-007.2
func TestLateSettlementPreservesUserCancellationNotice(t *testing.T) {
	cases := []struct {
		name    string
		outcome models.ErrandOutcome
		cancel  string
	}{
		{name: "late success after user cancellation", outcome: models.ErrandOutcomeDone, cancel: "user"},
		{name: "late failure after user cancellation", outcome: models.ErrandOutcomeFailed, cancel: "user"},
		{name: "late failure after parent cancellation", outcome: models.ErrandOutcomeFailed, cancel: "parent"},
		{name: "late success after parent cascade", outcome: models.ErrandOutcomeDone, cancel: "parent_cascade"},
		{name: "late failure after task cascade", outcome: models.ErrandOutcomeFailed, cancel: "task_cascade"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			errand := h.mustStart("audit").Errand
			observer := &cancellationObservedStore{Store: h.store, cancelled: make(chan struct{}, 1)}
			h.service.store = observer
			reader := &blockedTurnReader{
				readStarted: make(chan struct{}, 1),
				readRelease: make(chan struct{}),
				view: TurnView{
					LastAgentText: "late answer", EarliestUserSender: testParent,
				},
				latestID: errand.FirstTurnID,
			}
			h.service.SetTurnReader(reader)

			settled := make(chan error, 1)
			go func() {
				settled <- h.service.OnErrandTurnSettled(t.Context(), TurnSettlement{
					SessionID: errand.ID, TurnID: errand.FirstTurnID, Outcome: tc.outcome,
					FailureSummary: "late failure",
				})
			}()
			select {
			case <-reader.readStarted:
			case <-time.After(10 * time.Second):
				t.Fatal("settlement did not reach the transcript read barrier")
			}
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(reader.readRelease) }) })
			cancelledBy := make(chan error, 1)
			go func() { cancelledBy <- cancelForSettlementRace(t.Context(), h, errand, tc.cancel) }()
			select {
			case <-observer.cancelled:
			case <-time.After(10 * time.Second):
				t.Fatal("cancellation did not commit before settlement release")
			}
			cancelled, err := h.store.GetErrand(t.Context(), errand.ID)
			if err != nil {
				t.Fatalf("read cancelled row: %v", err)
			}
			if tc.cancel == "user" && (cancelled.ResultState != models.ErrandResultPending ||
				cancelled.ResultKind != models.ErrandResultCancelled || cancelled.ResultText != cancellationNotice) {
				t.Fatalf("user cancellation result = %q/%q/%q, want the pending cancellation notice", cancelled.ResultState, cancelled.ResultKind, cancelled.ResultText)
			}

			release.Do(func() { close(reader.readRelease) })
			select {
			case err := <-settled:
				if err != nil {
					t.Fatalf("late settlement: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("late settlement did not return")
			}
			if err := <-cancelledBy; err != nil {
				t.Fatalf("cancellation: %v", err)
			}
			after, err := h.store.GetErrand(t.Context(), errand.ID)
			if err != nil {
				t.Fatalf("read row after late settlement: %v", err)
			}
			cancelled = expectedAfterTeardownReceipt(cancelled, after)
			if !reflect.DeepEqual(after, cancelled) {
				t.Fatalf("late settlement changed cancelled row:\nbefore: %+v\nafter:  %+v", cancelled, after)
			}
		})
	}
}

// @covers AC-TASKS-ERRANDS-003.1, AC-TASKS-ERRANDS-003.5
func TestCompletedSettlementBeforeTurnCancellationKeepsTheResult(t *testing.T) {
	h := newHarness(t)
	errand := h.mustStart("audit").Errand
	view := TurnView{LastAgentText: "completed answer", EarliestUserSender: testParent, Completed: true}
	h.service.SetTurnReader(&cancellationTurns{
		views: map[string]TurnView{errand.FirstTurnID: view}, latestID: errand.FirstTurnID, latest: view,
	})
	if err := h.service.OnErrandTurnSettled(t.Context(), TurnSettlement{
		SessionID: errand.ID, TurnID: errand.FirstTurnID, Outcome: models.ErrandOutcomeDone,
	}); err != nil {
		t.Fatalf("settle before cancellation: %v", err)
	}
	settled, err := h.store.GetErrand(t.Context(), errand.ID)
	if err != nil {
		t.Fatalf("read settled row: %v", err)
	}
	if settled.ResultState != models.ErrandResultPending || settled.ResultText != "completed answer" ||
		settled.ResultKind != models.ErrandResultOutput || settled.LastOutcome != models.ErrandOutcomeDone {
		t.Fatalf("settled result = %+v, want the completed result", settled)
	}
	if err := h.service.OnErrandTurnCancelled(t.Context(), errand.ID, errand.FirstTurnID, errand.SubmissionID); err != nil {
		t.Fatalf("cancel callback after settlement: %v", err)
	}
	after, err := h.store.GetErrand(t.Context(), errand.ID)
	if err != nil {
		t.Fatalf("read cancelled row: %v", err)
	}
	if after.Lifecycle != models.ErrandLifecycleLaunched || after.ResultState != settled.ResultState ||
		after.ResultKind != settled.ResultKind || after.ResultText != settled.ResultText ||
		after.ResultTurnID != settled.ResultTurnID || after.LastOutcome != settled.LastOutcome {
		t.Fatalf("cancellation changed the result that settled first: %+v", after)
	}
}
