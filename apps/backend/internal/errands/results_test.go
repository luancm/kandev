package errands

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

type terminalTranscript struct{ repo *sqlite.Repository }

func (r terminalTranscript) ReadTurn(ctx context.Context, sessionID, turnID string) (TurnView, error) {
	turn, err := r.repo.GetTurn(ctx, turnID)
	if err != nil {
		return TurnView{}, err
	}
	if turn.TaskSessionID != sessionID {
		return TurnView{}, fmt.Errorf("turn %s belongs to another session", turnID)
	}
	messages, err := r.repo.ListMessagesByTurnID(ctx, turnID)
	view := TurnView{Completed: turn.CompletedAt != nil}
	for _, message := range messages {
		if message.AuthorType == models.MessageAuthorAgent && (message.Type == "" || message.Type == models.MessageTypeMessage) {
			view.LastAgentText = message.Content
		}
		if message.AuthorType == models.MessageAuthorUser && view.EarliestUserSender == "" {
			view.EarliestUserSender, _ = message.Metadata["sender_session_id"].(string)
		}
	}
	return view, err
}

func (r terminalTranscript) LatestTurn(ctx context.Context, sessionID string) (string, TurnView, error) {
	turns, err := r.repo.ListTurnsBySession(ctx, sessionID)
	if err != nil || len(turns) == 0 {
		return "", TurnView{}, err
	}
	id := turns[len(turns)-1].ID
	view, err := r.ReadTurn(ctx, sessionID, id)
	return id, view, err
}

func newTerminalFixture(t *testing.T) (*harness, *sqlite.Repository, *models.Errand, *models.AgentDeliveryEffect) {
	t.Helper()
	conn := serviceClaimDB(t, false)
	repo, err := sqlite.NewWithDB(conn, conn, nil)
	require.NoError(t, err)
	require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "terminal-ws", Name: "Terminal settlement"}))
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: testTask, WorkspaceID: "terminal-ws", Title: "Exact terminal"}))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: testParent, TaskID: testTask,
		AgentProfileID: "parent-profile", State: models.TaskSessionStateWaitingForInput}))
	child := &models.TaskSession{ID: "terminal-child", TaskID: testTask, ParentSessionID: testParent,
		Role: models.SessionRoleErrand, State: models.TaskSessionStateRunning}
	require.NoError(t, repo.CreateTaskSession(t.Context(), child))
	child, err = repo.GetTaskSession(t.Context(), child.ID)
	require.NoError(t, err)
	require.NoError(t, repo.CreateHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{
		SessionID: child.ID, IncarnationID: child.QueueIncarnationID, Generation: 1,
		NativeSessionID: "terminal-native-session", CreationReason: "initial",
	}))
	row := &models.Errand{ID: child.ID, TaskID: testTask, ParentSessionID: testParent, Title: "Exact terminal", Objective: "Capture A",
		AgentProfileID: "profile-1", FirstTurnID: "terminal-A", SubmissionID: "submit-terminal-A",
		Lifecycle: models.ErrandLifecycleLaunched, AwaitingResult: true, ResultState: models.ErrandResultNone}
	require.NoError(t, repo.CreateErrand(t.Context(), row))
	effect := reserveTerminalTurn(t, repo, row, row.FirstTurnID, row.SubmissionID, "parent")
	h := newHarness(t)
	h.service.store = repo
	h.stopper.store = repo
	h.stopper.target = &ErrandTeardownTarget{TaskID: row.TaskID, SessionID: row.ID,
		SessionIncarnation: effect.IncarnationID, TurnID: effect.TurnID, SubmissionID: effect.SubmissionID}
	h.service.SetTurnReader(terminalTranscript{repo})
	return h, repo, row, effect
}

func reserveTerminalTurn(t *testing.T, repo *sqlite.Repository, row *models.Errand, turnID, submissionID, origin string) *models.AgentDeliveryEffect {
	t.Helper()
	require.NoError(t, repo.CreateTurn(t.Context(), &models.Turn{ID: turnID, TaskID: row.TaskID, TaskSessionID: row.ID}))
	session, err := repo.GetTaskSession(t.Context(), row.ID)
	require.NoError(t, err)
	_, err = repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: row.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "dispatch:" + submissionID,
		PayloadHash: "hash:" + submissionID, Payload: []byte("prompt:" + submissionID), State: models.DeliverySubmissionDispatching,
	})
	require.NoError(t, err)
	effect := &models.AgentDeliveryEffect{TaskID: row.TaskID, EffectKey: "terminal:" + turnID, EffectType: db.ErrandTurnSettlementEffect,
		State: models.DeliveryEffectPending, SessionID: row.ID, IncarnationID: session.QueueIncarnationID,
		StreamID: "terminal-execution", HarnessGeneration: 1, TurnID: turnID, SubmissionID: submissionID, Outcome: origin}
	require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), row.TaskID, effect))
	return effect
}

func completeTerminalTurn(t *testing.T, repo *sqlite.Repository, effect *models.AgentDeliveryEffect, text, cause string) {
	t.Helper()
	require.NoError(t, repo.CreateMessage(t.Context(), &models.Message{ID: "output:" + effect.TurnID, TaskID: effect.TaskID,
		TaskSessionID: effect.SessionID, TurnID: effect.TurnID, AuthorType: models.MessageAuthorAgent, Content: text}))
	require.NoError(t, repo.CompleteTurn(t.Context(), effect.TurnID))
	if cause != "" {
		effect.Outcome += ":" + cause
		require.NoError(t, repo.ReserveErrandTurnSettlement(t.Context(), effect.TaskID, effect))
	}
}

func settledTerminalFixture(t *testing.T, text string) (*harness, *sqlite.Repository, *models.Errand) {
	t.Helper()
	h, repo, row, effect := newTerminalFixture(t)
	completeTerminalTurn(t, repo, effect, text, "done")
	require.NoError(t, repo.UpdateTaskSessionState(t.Context(), row.ID, models.TaskSessionStateWaitingForInput, ""))
	require.NoError(t, h.service.ReconcileTerminalDecisions(t.Context(), row.ID))
	return h, repo, row
}

type blockedTurnReader struct {
	reader      TurnReader
	readStarted chan struct{}
	readRelease chan struct{}
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

func (r *blockedTurnReader) ReadTurn(ctx context.Context, sessionID, turnID string) (TurnView, error) {
	r.readStarted <- struct{}{}
	<-r.readRelease
	return r.reader.ReadTurn(ctx, sessionID, turnID)
}

func (r *blockedTurnReader) LatestTurn(ctx context.Context, sessionID string) (string, TurnView, error) {
	return r.reader.LatestTurn(ctx, sessionID)
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
			h, repo, errand, effect := newTerminalFixture(t)
			completeTerminalTurn(t, repo, effect, "late answer", "")
			observer := &cancellationObservedStore{Store: repo, cancelled: make(chan struct{}, 1)}
			h.service.store = observer
			reader := &blockedTurnReader{
				readStarted: make(chan struct{}, 1),
				readRelease: make(chan struct{}),
				reader:      terminalTranscript{repo},
			}
			h.service.SetTurnReader(reader)

			settled := make(chan error, 1)
			go func() {
				settled <- h.service.OnErrandTurnSettled(t.Context(), TurnSettlement{
					SessionID: errand.ID, TurnID: errand.FirstTurnID, SubmissionID: errand.SubmissionID, Outcome: tc.outcome,
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

// @covers AC-TASKS-ERRANDS-003.1, AC-TASKS-ERRANDS-003.5
func TestCompletedSettlementBeforeTurnCancellationKeepsTheResult(t *testing.T) {
	h, repo, errand, effect := newTerminalFixture(t)
	completeTerminalTurn(t, repo, effect, "completed answer", "")
	if err := h.service.OnErrandTurnSettled(t.Context(), TurnSettlement{
		SessionID: errand.ID, TurnID: errand.FirstTurnID, SubmissionID: errand.SubmissionID, Outcome: models.ErrandOutcomeDone,
	}); err != nil {
		t.Fatalf("settle before cancellation: %v", err)
	}
	settled, err := repo.GetErrand(t.Context(), errand.ID)
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
	after, err := repo.GetErrand(t.Context(), errand.ID)
	if err != nil {
		t.Fatalf("read cancelled row: %v", err)
	}
	if after.Lifecycle != models.ErrandLifecycleLaunched || after.ResultState != settled.ResultState ||
		after.ResultKind != settled.ResultKind || after.ResultText != settled.ResultText ||
		after.ResultTurnID != settled.ResultTurnID || after.LastOutcome != settled.LastOutcome {
		t.Fatalf("cancellation changed the result that settled first: %+v", after)
	}
}

func TestTerminalSettlementPreservesFailureAndEmptyOutput(t *testing.T) {
	for _, test := range []struct {
		name, output, summary, want string
		outcome                     models.ErrandOutcome
		kind                        models.ErrandResultKind
	}{
		{name: "empty successful output", outcome: models.ErrandOutcomeDone, kind: models.ErrandResultOutput},
		{name: "sanitized failure summary", output: "partial output", summary: "safe failure", want: "safe failure", outcome: models.ErrandOutcomeFailed, kind: models.ErrandResultFailure},
		{name: "canonical failure output", output: "canonical failure", want: "canonical failure", outcome: models.ErrandOutcomeFailed, kind: models.ErrandResultFailure},
		{name: "failure without output", want: failureNotice, outcome: models.ErrandOutcomeFailed, kind: models.ErrandResultFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, repo, row, effect := newTerminalFixture(t)
			completeTerminalTurn(t, repo, effect, test.output, "")
			require.NoError(t, h.service.OnErrandTurnSettled(t.Context(), TurnSettlement{
				SessionID: row.ID, TurnID: effect.TurnID, SubmissionID: effect.SubmissionID,
				Outcome: test.outcome, FailureSummary: test.summary,
			}))
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
