package errands

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

func settledTerminalFixture(t *testing.T, text string) (*harness, *sqlite.Repository, *models.Errand) {
	t.Helper()
	h, repo, row := newOwnedSettlementFixture(t, models.ErrandOutcomeDone, text)
	require.NoError(t, h.service.reconcileOwnedErrandResults(t.Context()))
	return h, repo, row
}

type blockedTurnReader struct {
	reader      TurnReader
	readStarted chan struct{}
	readRelease chan struct{}
}

type cancellationObservedStore struct {
	*sqlite.Repository
	cancelled chan struct{}
}

func (s *cancellationObservedStore) UpdateErrand(ctx context.Context, row *models.Errand) error {
	err := s.Repository.UpdateErrand(ctx, row)
	if err == nil && row.Lifecycle == models.ErrandLifecycleCancelled {
		select {
		case s.cancelled <- struct{}{}:
		default:
		}
	}
	return err
}

func (r *blockedTurnReader) ReadTurn(ctx context.Context, sessionID, turnID string) (TurnView, error) {
	r.readStarted <- struct{}{}
	<-r.readRelease
	return r.reader.ReadTurn(ctx, sessionID, turnID)
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
			h, repo, errand := newOwnedSettlementFixture(t, tc.outcome, "late answer")
			observer := &cancellationObservedStore{Repository: repo, cancelled: make(chan struct{}, 1)}
			h.service.store = observer
			reader := &blockedTurnReader{
				readStarted: make(chan struct{}, 1),
				readRelease: make(chan struct{}),
				reader:      ownedSettlementTranscript{repo: repo},
			}
			h.service.SetTurnReader(reader)

			settled := make(chan error, 1)
			go func() {
				settled <- h.service.reconcileOwnedErrandResult(t.Context(), repo, errand)
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
			cancelled, err := repo.GetErrand(t.Context(), errand.ID)
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
			after, err := repo.GetErrand(t.Context(), errand.ID)
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

func TestOwnedTerminalSettlementPreservesFailureAndEmptyOutput(t *testing.T) {
	for _, test := range []struct {
		name, output, want string
		outcome            models.ErrandOutcome
		kind               models.ErrandResultKind
	}{
		{name: "empty successful output", outcome: models.ErrandOutcomeDone, kind: models.ErrandResultOutput},
		{name: "canonical failure output", output: "canonical failure", want: "canonical failure", outcome: models.ErrandOutcomeFailed, kind: models.ErrandResultFailure},
		{name: "failure without output", want: failureNotice, outcome: models.ErrandOutcomeFailed, kind: models.ErrandResultFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, repo, row := newOwnedSettlementFixture(t, test.outcome, test.output)
			require.NoError(t, h.service.reconcileOwnedErrandResults(t.Context()))
			after, err := repo.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, test.outcome, after.LastOutcome)
			require.Equal(t, test.kind, after.ResultKind)
			require.Equal(t, test.want, after.ResultText)
			require.Equal(t, models.ErrandResultPending, after.ResultState)
			require.False(t, after.AwaitingResult)
		})
	}
}
