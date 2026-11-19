package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-ERRANDS-005.1 AC-TASKS-ERRANDS-012.3
func TestErrandLaunchFailureRoleReadErrorLeavesSharedTaskState(t *testing.T) {
	for _, tc := range []struct {
		name string

		productionCallbacks    bool
		failRoleReadAfterState bool
		wantTaskState          v1.TaskState
	}{
		{
			name: "repository fallback fails closed",

			failRoleReadAfterState: true,
			wantTaskState:          v1.TaskStateReview,
		},
		{
			name: "session and task callbacks fail closed",

			productionCallbacks:    true,
			failRoleReadAfterState: true,
			wantTaskState:          v1.TaskStateReview,
		},
		{
			name:                "ordinary session still fails task",
			productionCallbacks: true,
			wantTaskState:       v1.TaskStateFailed,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.tasks["task-1"] = &models.Task{ID: "task-1", State: v1.TaskStateReview}
			repo.sessions["session-1"] = &models.TaskSession{
				ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateStarting,
			}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			if tc.productionCallbacks {
				setLaunchFailureRuntimeCallbacks(exec, repo)
			}

			roleReadErr := errors.New("session role read failed")
			roleReadArmed := false
			roleReadFailed := false
			repo.getTaskSessionFunc = func(_ context.Context, sessionID string) (*models.TaskSession, error) {
				if roleReadArmed {
					roleReadArmed = false
					roleReadFailed = true
					return nil, roleReadErr
				}
				repo.mu.Lock()
				defer repo.mu.Unlock()
				return cloneMockTaskSession(repo.sessions[sessionID]), nil
			}
			if tc.failRoleReadAfterState {
				exec.SetOnLaunchFailed(func(context.Context, string, string, string, error) {
					roleReadArmed = true
				})
			}

			launchErr := errors.New("provider diagnostic token=must-not-leak")
			safeErr := exec.handleLaunchFailure(
				context.Background(), "task-1", "session-1", "", "", launchErr,
			)
			if safeErr == nil || !errors.Is(safeErr, launchErr) {
				t.Fatalf("handleLaunchFailure error = %v, want sanitized error preserving launch cause", safeErr)
			}
			if strings.Contains(safeErr.Error(), "must-not-leak") {
				t.Fatalf("handleLaunchFailure exposed provider diagnostic: %v", safeErr)
			}
			if tc.failRoleReadAfterState && !roleReadFailed {
				t.Fatal("session role read did not fail after the session transition")
			}
			if got := repo.sessions["session-1"].State; got != models.TaskSessionStateFailed {
				t.Fatalf("session state = %s, want FAILED", got)
			}
			if got := repo.tasks["task-1"].State; got != tc.wantTaskState {
				t.Fatalf("task state = %s, want %s", got, tc.wantTaskState)
			}
		})
	}
}

// @covers AC-TASKS-ERRANDS-005.1 AC-TASKS-ERRANDS-012.3
func TestRuntimeTaskStateFallbackRoleReadFailureIsClosed(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sessionState models.TaskSessionState
		write        func(*Executor, context.Context) error
	}{
		{
			name:         "FAILED",
			sessionState: models.TaskSessionStateFailed,
			write: func(exec *Executor, ctx context.Context) error {
				return exec.writeTaskFailedForRuntime(ctx, "task-1", "errand-1")
			},
		},
		{
			name:         "IN_PROGRESS",
			sessionState: models.TaskSessionStateRunning,
			write: func(exec *Executor, ctx context.Context) error {
				return exec.writeTaskInProgressForRuntime(ctx, "task-1", "errand-1")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.tasks["task-1"] = &models.Task{ID: "task-1", State: v1.TaskStateReview}
			repo.sessions["errand-1"] = &models.TaskSession{
				ID: "errand-1", TaskID: "task-1", State: tc.sessionState,
			}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			roleReadErr := errors.New("session role read failed")
			readCalls := 0
			repo.getTaskSessionFunc = func(_ context.Context, sessionID string) (*models.TaskSession, error) {
				readCalls++
				if readCalls == 1 {
					return nil, roleReadErr
				}
				repo.mu.Lock()
				defer repo.mu.Unlock()
				return cloneMockTaskSession(repo.sessions[sessionID]), nil
			}

			err := tc.write(exec, context.Background())
			if !errors.Is(err, roleReadErr) {
				t.Fatalf("runtime task-state write error = %v, want session role read error", err)
			}
			if got := readCalls; got != 1 {
				t.Fatalf("session reads = %d, want one required role/activity read", got)
			}
			if got := repo.tasks["task-1"].State; got != v1.TaskStateReview {
				t.Fatalf("task state = %s, want unchanged REVIEW", got)
			}
		})
	}
}

func TestRuntimeTaskStateFallbackRejectsMismatchedSessions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		sessionState models.TaskSessionState
		write        func(*Executor, context.Context) error
	}{
		{
			name:         "FAILED",
			sessionState: models.TaskSessionStateFailed,
			write: func(exec *Executor, ctx context.Context) error {
				return exec.writeTaskFailedForRuntime(ctx, "task-1", "session-1")
			},
		},
		{
			name:         "IN_PROGRESS",
			sessionState: models.TaskSessionStateRunning,
			write: func(exec *Executor, ctx context.Context) error {
				return exec.writeTaskInProgressForRuntime(ctx, "task-1", "session-1")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.tasks["task-1"] = &models.Task{ID: "task-1", State: v1.TaskStateReview}
			repo.sessions["session-1"] = &models.TaskSession{
				ID: "session-1", TaskID: "another-task", State: tc.sessionState,
			}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)

			if err := tc.write(exec, context.Background()); err != nil {
				t.Fatalf("runtime task-state write: %v", err)
			}
			if got := repo.tasks["task-1"].State; got != v1.TaskStateReview {
				t.Fatalf("task state = %s, want unchanged REVIEW", got)
			}
			if got := len(repo.updateTaskStateIfNotArchivedCalls); got != 0 {
				t.Fatalf("task state writes = %d, want none for a mismatched session", got)
			}
		})
	}
}

func setLaunchFailureRuntimeCallbacks(exec *Executor, repo *mockRepository) {
	exec.SetOnSessionStateTransition(func(
		ctx context.Context,
		taskID, sessionID string,
		expected *models.TaskSessionState,
		state models.TaskSessionState,
		errorMessage string,
		onChanged func(),
	) (bool, models.TaskSessionState, error) {
		session, err := repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return false, "", err
		}
		if expected != nil && session.State != *expected {
			return false, session.State, nil
		}
		if err := repo.UpdateTaskSessionState(ctx, sessionID, state, errorMessage); err != nil {
			return false, session.State, err
		}
		if onChanged != nil {
			onChanged()
		}
		return true, state, nil
	})
	exec.SetOnTaskStateChange(func(ctx context.Context, taskID string, state v1.TaskState) error {
		_, _, err := repo.UpdateTaskStateIfNotArchived(ctx, taskID, state)
		return err
	})
	exec.SetOnTaskRuntimeStateReconcile(func(
		ctx context.Context, taskID, sessionID string, state v1.TaskState,
	) error {
		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			return err
		}
		if task == nil || task.ArchivedAt != nil {
			return nil
		}
		session, err := repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return err
		}
		if session == nil || session.TaskID != taskID {
			return nil
		}
		if state == v1.TaskStateFailed && session.State != models.TaskSessionStateFailed {
			return nil
		}
		if state == v1.TaskStateInProgress && !isRuntimeWorkingSessionState(session.State) {
			return nil
		}
		return exec.updateTaskState(ctx, taskID, state)
	})
}

// @covers AC-TASKS-ERRANDS-005.1 AC-TASKS-ERRANDS-012.3
func TestRuntimeTaskStateFallbackRequiresCurrentSession(t *testing.T) {
	for _, target := range []v1.TaskState{v1.TaskStateFailed, v1.TaskStateInProgress} {
		for _, evidence := range []string{"empty ID", "missing row", "stale state", "ordinary eligible"} {
			t.Run(string(target)+"/"+evidence, func(t *testing.T) {
				repo := newMockRepository()
				repo.tasks["task-1"] = &models.Task{ID: "task-1", State: v1.TaskStateReview}
				sessionState := models.TaskSessionStateFailed
				if target == v1.TaskStateInProgress {
					sessionState = models.TaskSessionStateRunning
				}
				repo.sessions["session-1"] = &models.TaskSession{ID: "session-1", TaskID: "task-1", State: sessionState}
				sessionID := "session-1"
				switch evidence {
				case "empty ID":
					sessionID = ""
				case "missing row":
					delete(repo.sessions, sessionID)
				case "stale state":
					repo.sessions[sessionID].State = models.TaskSessionStateWaitingForInput
				}
				// Exercise the repository's defensive nil result as well as a missing identifier.
				repo.getTaskSessionFunc = func(_ context.Context, id string) (*models.TaskSession, error) {
					repo.mu.Lock()
					defer repo.mu.Unlock()
					return cloneMockTaskSession(repo.sessions[id]), nil
				}
				exec := newTestExecutor(t, &mockAgentManager{}, repo)
				write := exec.writeTaskFailedForRuntime
				if target == v1.TaskStateInProgress {
					write = exec.writeTaskInProgressForRuntime
				}
				if err := write(context.Background(), "task-1", sessionID); err != nil {
					t.Fatalf("runtime task-state write: %v", err)
				}
				wantState, wantWrites := v1.TaskStateReview, 0
				if evidence == "ordinary eligible" {
					wantState, wantWrites = target, 1
				}
				if got := repo.tasks["task-1"].State; got != wantState {
					t.Fatalf("task state = %s, want %s", got, wantState)
				}
				if got := len(repo.updateTaskStateIfNotArchivedCalls); got != wantWrites {
					t.Fatalf("task writes = %d, want %d", got, wantWrites)
				}
			})
		}
	}
}
