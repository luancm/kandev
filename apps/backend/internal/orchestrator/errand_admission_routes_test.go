package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type firstErrandAdmissionManager struct {
	*errandLaunchManager
	durable bool
}

func (m *firstErrandAdmissionManager) DurableDeliveryCapabilityForExecution(context.Context, string) (agentruntime.DurableDeliveryCapability, bool) {
	return agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true}, m.durable
}

func TestErrandAdmissionNewErrandLaunchUsesReservedIdentity(t *testing.T) {
	for _, durable := range []bool{false, true} {
		name := map[bool]string{false: messagequeue.DeliveryProtocolLegacy, true: messagequeue.DeliveryProtocolV1}[durable]
		t.Run(name, func(t *testing.T) {
			f := newErrandLaunchFixture(t)
			service, _ := newErrandServiceOnRealStack(t, f)
			manager := &firstErrandAdmissionManager{errandLaunchManager: f.manager, durable: durable}
			f.svc.agentManager = manager
			f.svc.executor = executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{})
			started := make(chan struct{}, 1)
			manager.startAgentProcessFunc = func(context.Context, string) error { started <- struct{}{}; return nil }
			result := startErrand(t, service, f, "first prompt")
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("first prompt worker did not start")
			}
			manager.mu.Lock()
			admit, accepted := manager.initialPromptAdmissionCallback, manager.initialPromptDispatchCallback
			manager.mu.Unlock()
			require.NotNil(t, admit)
			require.NotNil(t, accepted)
			require.NoError(t, admit(t.Context(), "prepared first prompt", nil))
			pending, err := f.repo.ListUnresolvedErrandParentAdmissions(t.Context(), result.Errand.ID)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			require.Equal(t, result.Errand.FirstTurnID, pending[0].TurnID)
			require.Equal(t, normalizeDeliverySubmissionID(result.Errand.SubmissionID), pending[0].SubmissionID)
			accepted()
			row, err := f.repo.GetErrand(t.Context(), result.Errand.ID)
			require.NoError(t, err)
			require.EqualValues(t, 1, row.ParentAdmissionGeneration)
		})
	}
}

func awaitAdmissionRow(t *testing.T, f *admissionIntegrationFixture) {
	t.Helper()
	require.Eventually(t, func() bool {
		row, err := f.repo.GetErrand(t.Context(), f.row.ID)
		return err == nil && row.ParentAdmissionGeneration == 1 && row.ResultState == models.ErrandResultSuperseded
	}, 5*time.Second, 10*time.Millisecond, "provider acceptance resolves the exact parent reservation")
}

func TestErrandAdmissionFIFOAndSendNowRetainSourceProvenance(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		for _, route := range []string{"fifo", "send_now"} {
			t.Run(protocol+"/"+route, func(t *testing.T) {
				f := newAdmissionIntegrationFixture(t, protocol)
				db := sqlx.NewDb(f.repo.DB(), "sqlite3")
				queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
				require.NoError(t, err)
				queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
				f.svc.messageQueue = queue
				metadata := map[string]interface{}{errands.ParentPromptSenderMetadataKey: f.parent}
				if route == "send_now" {
					// A user source first must not hide the retained parent source.
					_, err := queue.QueueMessage(t.Context(), f.session.ID, f.taskID, "user context", "", messagequeue.QueuedByUser, false, nil)
					require.NoError(t, err)
				}
				entry, err := queue.QueueMessageWithMetadata(t.Context(), f.session.ID, f.taskID, "parent instruction", "", messagequeue.QueuedByAgent, false, nil, metadata)
				require.NoError(t, err)
				before, err := f.repo.GetErrand(t.Context(), f.row.ID)
				require.NoError(t, err)
				require.Zero(t, before.ParentAdmissionGeneration)
				require.Equal(t, models.ErrandResultPending, before.ResultState)
				if route == "fifo" {
					require.NoError(t, queue.SetAutoRun(t.Context(), f.session.ID, true))
					require.True(t, f.svc.drainQueuedBeforeWorkflowTransition(t.Context(), f.taskID, f.session.ID, f.session))
				} else {
					status := queue.GetStatus(t.Context(), f.session.ID)
					identity, err := queue.ResolveSessionIdentity(t.Context(), f.taskID, f.session.ID)
					require.NoError(t, err)
					claim, err := queue.ClaimSendNowForSession(t.Context(), identity, status.Entries)
					require.NoError(t, err)
					reservation := f.svc.markQueuedDispatchInFlightWithIdentityLocked(t.Context(), claim.Identity, claim.Dispatch.ID, nil)
					defer f.svc.clearQueuedDispatchInFlightIfCurrent(f.session.ID, reservation)
					ctx := f.svc.queuedRecoveryContext(t.Context(), reservation)
					require.NoError(t, f.svc.claimSendNowExecution(f.session.ID, claim.Dispatch.ID))
					_, dispatchErr := f.svc.promptSendNowClaim(ctx, claim)
					require.NoError(t, dispatchErr)
					f.svc.clearQueuedDispatchInFlightIfCurrent(f.session.ID, reservation)
				}
				awaitAdmissionRow(t, f)
				require.Eventually(t, func() bool {
					return f.svc.pendingQueuedDispatch(f.session.ID) == nil && f.svc.acceptedQueuedDispatchForSession(f.session.ID) == nil
				}, 5*time.Second, 10*time.Millisecond, "dispatch worker must release ownership before repository cleanup")
				require.NotEmpty(t, entry.ID)
			})
		}
	}
}

func TestErrandAdmissionInitialAndFallbackBindBothProtocols(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		for _, route := range []string{"initial", "fallback"} {
			t.Run(protocol+"/"+route, func(t *testing.T) {
				f := newAdmissionIntegrationFixture(t, protocol)
				manager := &retentionInitialPromptManager{durableDeliveryTestAgentManager: f.manager.durableDeliveryTestAgentManager}
				manager.isAgentRunning = false
				started := make(chan struct{}, 1)
				manager.startAgentProcessFunc = func(context.Context, string) error { started <- struct{}{}; return nil }
				f.svc.agentManager = manager
				f.svc.executor = executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{})
				ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{SenderSessionID: f.parent, SubmissionID: "peer:initial"})
				if route == "initial" {
					require.NoError(t, f.repo.UpdateTaskSessionState(ctx, f.session.ID, models.TaskSessionStateCreated, ""))
					_, err := f.svc.StartCreatedSession(ctx, f.taskID, f.session.ID, "errand-profile", "next", true, false, true, nil, nil)
					require.NoError(t, err)
				} else {
					// The queue worker has no sender on its ambient context. The
					// dispatch options must carry its retained source into fallback.
					ctx = t.Context()
					options := f.options(protocol, true)
					options.errandParentOrigin = errandPromptOrigin(ctx, f.session, options)
					_, err := f.svc.finishPromptDispatchFailure(ctx, f.taskID, f.session.ID, "next", false, true, nil,
						promptClaimRollback{previousSessionState: models.TaskSessionStateWaitingForInput}, options,
						executor.ErrExecutionNotFound, nil, false, nil)
					require.NoError(t, err)
				}
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("initial provider worker did not start")
				}
				manager.mu.Lock()
				admit, accepted := manager.initialPromptAdmissionCallback, manager.initialPromptDispatchCallback
				manager.mu.Unlock()
				require.NotNil(t, admit)
				require.NotNil(t, accepted)
				require.NoError(t, admit(ctx, "prepared next", nil))
				reservations, err := f.repo.ListUnresolvedErrandParentAdmissions(ctx, f.row.ID)
				require.NoError(t, err)
				require.Len(t, reservations, 1)
				expectedSubmission := "prompt:peer:initial"
				if route == "fallback" {
					expectedSubmission = "prompt:admission-submission"
				}
				require.Equal(t, expectedSubmission, reservations[0].SubmissionID)
				before, err := f.repo.GetErrand(ctx, f.row.ID)
				require.NoError(t, err)
				require.Zero(t, before.ParentAdmissionGeneration)
				accepted()
				awaitAdmissionRow(t, f)
			})
		}
	}
}
