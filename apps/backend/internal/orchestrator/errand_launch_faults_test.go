package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type errandRuntimeInventoryRepo struct {
	*sqliterepo.Repository
	running *models.ExecutorRunning
	err     error
}

func (r *errandRuntimeInventoryRepo) GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error) {
	return r.running, r.err
}

func TestErrandRuntimeAbsenceEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		absent bool
	}{
		{"typed missing inventory", true},
		{"nil inventory", false},
		{"inventory read error", false},
		{"pending authenticated recovery", false},
		{"completed exact cleanup", true},
		{"incomplete cleanup", false},
		{"other execution cleanup", false},
		{"other session cleanup", false},
		{"stopped without pid", true},
		{"stopped with unconfirmed pid", false},
		{"dead process", true},
		{"unknown process", false},
		{"live process", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newErrandLaunchFixture(t)
			req := f.request()
			running := &models.ExecutorRunning{TaskID: req.TaskID, SessionID: req.ErrandID,
				AgentExecutionID: "persisted-execution", Status: models.ExecutorRunningStatusRunning, LocalPID: 123}
			store := &errandRuntimeInventoryRepo{Repository: f.repo, running: running}
			f.svc.repo = store
			liveness := models.ProcessLivenessUnknown
			f.manager.rowLivenessFn = func(*models.ExecutorRunning) models.ProcessLiveness { return liveness }
			switch tc.name {
			case "typed missing inventory":
				store.running, store.err = nil, models.ErrExecutorRunningNotFound
			case "nil inventory":
				store.running = nil
			case "inventory read error":
				store.running, store.err = nil, errors.New("inventory unavailable")
			case "pending authenticated recovery":
				f.svc.SetPendingRemoteRecoveryChecker(func(session, execution string) bool {
					return session == req.ErrandID && execution == running.AgentExecutionID
				})
				running.Status, running.LocalPID, liveness = models.ExecutorRunningStatusStopped, 0, models.ProcessLivenessDead
			case "stopped without pid":
				running.Status, running.LocalPID = models.ExecutorRunningStatusStopped, 0
			case "stopped with unconfirmed pid":
				running.Status = models.ExecutorRunningStatusStopped
			case "dead process":
				liveness = models.ProcessLivenessDead
			case "live process":
				liveness = models.ProcessLivenessAlive
			}
			switch tc.name {
			case "completed exact cleanup", "incomplete cleanup", "other execution cleanup", "other session cleanup", "pending authenticated recovery":
				sessionID, executionID := req.ErrandID, running.AgentExecutionID
				if tc.name == "other execution cleanup" {
					executionID = "old-execution"
				}
				if tc.name == "other session cleanup" {
					sessionID = f.parent
				}
				claim, owned := f.svc.claimExecutionTeardownWithToken(sessionID, executionID, executionTeardownIntentGraceful)
				require.True(t, owned)
				if tc.name != "incomplete cleanup" {
					f.svc.completeExecutionTeardownClaim(sessionID, executionID, claim)
				}
			}
			t.Run("teardown", func(t *testing.T) {
				owner := &errandTeardownOwner{svc: f.svc}
				_, absent, err := owner.executionForSession(t.Context(), req.ErrandID)
				want := tc.absent || tc.name == "nil inventory"
				require.Equal(t, want, absent)
				require.Equal(t, want, err == nil)
				if tc.name == "inventory read error" {
					require.ErrorIs(t, err, store.err)
				}
			})
		})
	}
}
