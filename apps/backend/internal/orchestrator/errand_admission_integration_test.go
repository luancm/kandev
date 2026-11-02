package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type admissionIntegrationFixture struct {
	*errandLaunchFixture
	errands *errands.Service
	session *models.TaskSession
	row     *models.Errand
	manager *retentionPromptAgentManager
}

func newAdmissionIntegrationFixture(t *testing.T, protocol string) *admissionIntegrationFixture {
	t.Helper()
	f := newErrandLaunchFixture(t)
	service, _ := newErrandServiceOnRealStack(t, f)
	now := time.Now().UTC()
	session := &models.TaskSession{ID: "admission-child", TaskID: f.taskID, Role: models.SessionRoleErrand,
		ParentSessionID: f.parent, State: models.TaskSessionStateWaitingForInput,
		AgentProfileID: "errand-profile", AgentExecutionID: "admission-execution", StartedAt: now, UpdatedAt: now}
	require.NoError(t, f.repo.CreateTaskSession(t.Context(), session))
	require.NoError(t, f.repo.CreateTurn(t.Context(), &models.Turn{ID: "admission-turn", TaskID: f.taskID,
		TaskSessionID: session.ID, StartedAt: now, CreatedAt: now, UpdatedAt: now}))
	seedExecutorRunning(t, f.repo, session.ID, f.taskID, session.AgentExecutionID)
	row := &models.Errand{ID: session.ID, TaskID: f.taskID, ParentSessionID: f.parent,
		Title: "inspect", Objective: "inspect dispatch", AgentProfileID: "errand-profile",
		FirstTurnID: "first-turn", SubmissionID: "first-submission", Lifecycle: models.ErrandLifecycleLaunched,
		LastOutcome: models.ErrandOutcomeDone, ResultState: models.ErrandResultPending,
		ResultText: "old output", ResultKind: models.ErrandResultOutput, ResultTurnID: "old-turn",
		CreatedAt: now, UpdatedAt: now}
	require.NoError(t, f.repo.CreateErrand(t.Context(), row))
	manager := &retentionPromptAgentManager{durableDeliveryTestAgentManager: &durableDeliveryTestAgentManager{
		mockAgentManager: &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: f.repo},
		advertised:       protocol == messagequeue.DeliveryProtocolV1,
		capability:       agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true},
	}}
	f.svc.agentManager = manager
	f.svc.executor = executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{})
	f.svc.messageCreator = &repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: f.repo}
	f.svc.SetTransientRetryMessageService(f.repo)
	return &admissionIntegrationFixture{errandLaunchFixture: f, errands: service, session: session, row: row, manager: manager}
}

func (f *admissionIntegrationFixture) options(protocol string, parent bool) promptTaskOptions {
	by := messagequeue.QueuedByUser
	if parent {
		by = messagequeue.QueuedByAgent
	}
	return promptTaskOptions{deliveryProtocol: protocol, deliverySubmissionID: "admission-submission",
		queuedMessage: &messagequeue.QueuedMessage{QueuedBy: by, Metadata: map[string]interface{}{
			"errand_parent_sender_session_id": f.parent,
		}},
	}
}

// @covers AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-004.10
func TestErrandAdmissionRealDispatchReservesBeforeProvider(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		for _, parent := range []bool{false, true} {
			t.Run(protocol+map[bool]string{false: "/user", true: "/parent"}[parent], func(t *testing.T) {
				f := newAdmissionIntegrationFixture(t, protocol)
				options := f.options(protocol, parent)
				f.manager.promptAgentFunc = func(ctx context.Context, _, _ string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
					before, err := f.repo.GetErrand(ctx, f.row.ID)
					require.NoError(t, err)
					require.Equal(t, models.ErrandResultPending, before.ResultState, "reservation cannot supersede before acceptance")
					competitor := models.ErrandParentAdmission{TaskID: f.taskID, ParentSessionID: f.parent,
						ErrandID: f.row.ID, SenderSessionID: f.parent, TurnID: "competing", SubmissionID: "competing"}
					_, reserveErr := f.repo.ReserveErrandParentAdmission(ctx, competitor)
					if parent {
						require.ErrorIs(t, reserveErr, models.ErrErrandVersionConflict, "provider I/O requires committed admission fence")
					} else {
						require.NoError(t, reserveErr)
						require.NoError(t, f.repo.DecideErrandParentAdmission(ctx, competitor, models.ErrandAdmissionRefused))
					}
					return &executor.PromptResult{}, nil
				}
				wireID := ""
				if protocol == messagequeue.DeliveryProtocolV1 {
					wireID = normalizeDeliverySubmissionID(options.deliverySubmissionID)
				}
				_, _, err := f.svc.dispatchPreparedPrompt(t.Context(), f.taskID, f.session, f.session.AgentExecutionID,
					"next", nil, true, options, wireID, nil, func() {}, nil)
				require.NoError(t, err)
				after, err := f.repo.GetErrand(t.Context(), f.row.ID)
				require.NoError(t, err)
				if parent {
					require.EqualValues(t, 1, after.ParentAdmissionGeneration)
					require.Equal(t, models.ErrandResultSuperseded, after.ResultState)
				} else {
					require.Zero(t, after.ParentAdmissionGeneration)
					require.Equal(t, models.ErrandResultPending, after.ResultState)
				}
			})
		}
	}
}

func TestErrandAdmissionServiceAbortReleasesRealAwait(t *testing.T) {
	f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolLegacy)
	_, err := f.repo.ClaimResults(t.Context(), f.taskID, f.parent, models.ErrandDeliveryAwait, "await-abort",
		[]models.ErrandClaimCandidate{{ID: f.row.ID, Version: f.row.Version}}, time.Now().UTC(), admissionClaimInventory(t, f.repo, f.parent))
	require.NoError(t, err)
	require.NoError(t, f.errands.ReleaseAwait(t.Context(), "await-abort"))
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, row.ResultState, "known await failure must durably abort before release")
	_, err = f.repo.ClaimResults(t.Context(), f.taskID, f.parent, models.ErrandDeliveryAwait, "await-abort",
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, time.Now().UTC(), admissionClaimInventory(t, f.repo, f.parent))
	require.True(t, errors.Is(err, models.ErrErrandVersionConflict), "aborted token cannot reclaim")
}
