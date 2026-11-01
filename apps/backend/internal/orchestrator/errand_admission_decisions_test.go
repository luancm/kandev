package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandAdmissionServiceReleaseUsesCapturedGeneration(t *testing.T) {
	f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolLegacy)
	_, err := f.repo.ClaimResults(t.Context(), f.taskID, f.parent, models.ErrandDeliveryTurnContext, "turn-release",
		[]models.ErrandClaimCandidate{{ID: f.row.ID, Version: f.row.Version}}, time.Now().UTC(), admissionClaimInventory(t, f.repo, f.parent))
	require.NoError(t, err)
	a := fixtureAdmission(f)
	_, err = f.repo.ReserveErrandParentAdmission(t.Context(), a)
	require.NoError(t, err)
	require.NoError(t, f.repo.DecideErrandParentAdmission(t.Context(), a, models.ErrandAdmissionAccepted))
	require.NoError(t, f.errands.ReleaseTurnClaim(t.Context(), "turn-release"))
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, row.ParentAdmissionGeneration)
	require.Equal(t, models.ErrandResultSuperseded, row.ResultState, "release must use the generation from the immutable receipt")
}

func dispatchAdmissionFixture(t *testing.T, f *admissionIntegrationFixture, options promptTaskOptions) error {
	t.Helper()
	wireID := ""
	if options.deliveryProtocol == messagequeue.DeliveryProtocolV1 {
		wireID = normalizeDeliverySubmissionID(options.deliverySubmissionID)
	}
	_, _, err := f.svc.dispatchPreparedPrompt(t.Context(), f.taskID, f.session, f.session.AgentExecutionID,
		"next", nil, true, options, wireID, nil, nil, nil)
	return err
}

func fixtureAdmission(f *admissionIntegrationFixture) models.ErrandParentAdmission {
	return models.ErrandParentAdmission{TaskID: f.taskID, ParentSessionID: f.parent, ErrandID: f.row.ID,
		SenderSessionID: f.parent, TurnID: "admission-turn", SubmissionID: "prompt:admission-submission"}
}

// @covers AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-004.10
func TestErrandAdmissionDispatchRefusalAndUncertainty(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		for _, refused := range []bool{true, false} {
			name := protocol + map[bool]string{true: "/refused", false: "/unknown"}[refused]
			t.Run(name, func(t *testing.T) {
				f := newAdmissionIntegrationFixture(t, protocol)
				options := f.options(protocol, true)
				if refused {
					options.deliveryClaimUpdater = func(context.Context, string, string, string) error {
						return errors.New("durable claim rejected before dispatch")
					}
				} else {
					f.manager.promptErr = errors.New("transport response lost")
				}
				dispatchErr := dispatchAdmissionFixture(t, f, options)
				require.Error(t, dispatchErr)
				if !refused {
					require.ErrorIs(t, dispatchErr, agentruntime.ErrUncertainPromptDelivery)
					var uncertain interface{ ErrandParentPromptUncertain() bool }
					require.ErrorAs(t, dispatchErr, &uncertain, "caller must retain uncertain admission evidence")
					require.True(t, uncertain.ErrandParentPromptUncertain())
				}
				row, err := f.repo.GetErrand(t.Context(), f.row.ID)
				require.NoError(t, err)
				require.Zero(t, row.ParentAdmissionGeneration)
				require.Equal(t, models.ErrandResultPending, row.ResultState)
				replay, err := f.repo.ReserveErrandParentAdmission(t.Context(), fixtureAdmission(f))
				require.NoError(t, err)
				require.False(t, replay.Created)
				if refused {
					require.Equal(t, models.ErrandAdmissionRefused, replay.State)
					require.Empty(t, f.manager.capturedPrompts)
				} else {
					require.Equal(t, models.ErrandAdmissionUnknown, replay.State)
					require.Len(t, f.manager.capturedPrompts, 1)
				}
			})
		}
	}
}

func TestErrandAdmissionUncertainDirectPromptKeepsExactTurnOpen(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		t.Run(protocol, func(t *testing.T) {
			f := newAdmissionIntegrationFixture(t, protocol)
			f.manager.promptErr = errors.New("transport reply lost")
			ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{SenderSessionID: f.parent, SubmissionID: "peer:unknown"})
			_, err := f.svc.PromptTask(ctx, f.taskID, f.session.ID, "next", "", false, nil, true)
			require.ErrorIs(t, err, agentruntime.ErrUncertainPromptDelivery)
			turn, err := f.repo.GetTurn(t.Context(), "admission-turn")
			require.NoError(t, err)
			require.Nil(t, turn.CompletedAt, "uncertainty cannot fabricate terminal turn evidence")
		})
	}
}

func TestErrandAdmissionReservationReplayNeverDispatches(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		for _, state := range []models.ErrandAdmissionState{models.ErrandAdmissionReserved, models.ErrandAdmissionUnknown, models.ErrandAdmissionAccepted, models.ErrandAdmissionRefused} {
			t.Run(protocol+"/"+string(state), func(t *testing.T) {
				f := newAdmissionIntegrationFixture(t, protocol)
				a := fixtureAdmission(f)
				_, err := f.repo.ReserveErrandParentAdmission(t.Context(), a)
				require.NoError(t, err)
				if state != models.ErrandAdmissionReserved {
					require.NoError(t, f.repo.DecideErrandParentAdmission(t.Context(), a, state))
				}
				require.Error(t, dispatchAdmissionFixture(t, f, f.options(protocol, true)))
				require.Empty(t, f.manager.capturedPrompts, "replay is not admission permission")
			})
		}
	}
}

type admissionDecisionFailure struct{ *errands.Service }

func (f *admissionDecisionFailure) DecideParentAdmission(ctx context.Context, a models.ErrandParentAdmission, state models.ErrandAdmissionState) error {
	if state == models.ErrandAdmissionAccepted {
		return errors.New("admission decision storage unavailable")
	}
	return f.Service.DecideParentAdmission(ctx, a, state)
}

func TestErrandAdmissionRecoveryConsumesExactTurnEvidence(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		t.Run(protocol, func(t *testing.T) {
			f := newAdmissionIntegrationFixture(t, protocol)
			f.svc.SetErrandController(&admissionDecisionFailure{Service: f.errands})
			err := dispatchAdmissionFixture(t, f, f.options(protocol, true))
			var accepted *acceptedPromptDispatchError
			require.ErrorAs(t, err, &accepted, "a failed decision write cannot turn acceptance into refusal")
			require.Len(t, f.manager.capturedPrompts, 1)
			pending, err := f.repo.ListUnresolvedErrandParentAdmissions(t.Context(), f.row.ID)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			// Recovery reads the exact immutable turn, not whichever turn is latest.
			require.NoError(t, f.svc.reconcileErrandParentAdmissions(t.Context()))
			row, err := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			require.EqualValues(t, 1, row.ParentAdmissionGeneration)
			require.Equal(t, models.ErrandResultSuperseded, row.ResultState)
			require.Error(t, dispatchAdmissionFixture(t, f, f.options(protocol, true)))
			require.Len(t, f.manager.capturedPrompts, 1)
		})
	}
}

func TestErrandAdmissionRecoveryMissingEvidenceRemainsUnknown(t *testing.T) {
	f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolV1)
	a := fixtureAdmission(f)
	a.TurnID = "missing-turn"
	_, err := f.repo.ReserveErrandParentAdmission(t.Context(), a)
	require.NoError(t, err)
	// A backend prepared/dispatching submission does not prove native acceptance.
	_, err = f.svc.prepareAgentDeliverySubmission(t.Context(), f.session, f.session.AgentExecutionID, "next", nil,
		promptTaskOptions{deliveryProtocol: messagequeue.DeliveryProtocolV1, deliverySubmissionID: a.SubmissionID})
	require.NoError(t, err)
	require.NoError(t, f.svc.reconcileErrandParentAdmissions(t.Context()))
	replay, err := f.repo.ReserveErrandParentAdmission(t.Context(), a)
	require.NoError(t, err)
	require.Equal(t, models.ErrandAdmissionUnknown, replay.State)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Zero(t, row.ParentAdmissionGeneration)
	require.Equal(t, models.ErrandResultPending, row.ResultState)
}

func TestErrandAdmissionDirectPromptUsesAuthenticatedContext(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		t.Run(protocol, func(t *testing.T) {
			f := newAdmissionIntegrationFixture(t, protocol)
			ctx := errands.WithParentPromptOrigin(t.Context(), errands.ParentPromptOrigin{SenderSessionID: f.parent, SubmissionID: "peer:direct"})
			_, err := f.svc.PromptTask(ctx, f.taskID, f.session.ID, "next", "", false, nil, true)
			require.NoError(t, err)
			row, err := f.repo.GetErrand(t.Context(), f.row.ID)
			require.NoError(t, err)
			require.EqualValues(t, 1, row.ParentAdmissionGeneration)
			reservation, err := f.repo.ReserveErrandParentAdmission(ctx, models.ErrandParentAdmission{
				TaskID: f.taskID, ParentSessionID: f.parent, ErrandID: f.row.ID, SenderSessionID: f.parent,
				TurnID: "admission-turn", SubmissionID: "prompt:peer:direct",
			})
			require.NoError(t, err)
			require.False(t, reservation.Created)
			require.Equal(t, models.ErrandAdmissionAccepted, reservation.State)
		})
	}
}

func TestErrandAdmissionAbortBeforeDelayedAwaitClaim(t *testing.T) {
	f := newAdmissionIntegrationFixture(t, messagequeue.DeliveryProtocolLegacy)
	require.NoError(t, f.errands.AbortAwait(t.Context(), f.parent, "early-abort"))
	_, err := f.repo.ClaimResults(t.Context(), f.taskID, f.parent, models.ErrandDeliveryAwait, "early-abort",
		[]models.ErrandClaimCandidate{{ID: f.row.ID, Version: f.row.Version}}, time.Now().UTC(), admissionClaimInventory(t, f.repo, f.parent))
	require.ErrorIs(t, err, models.ErrErrandVersionConflict)
	row, err := f.repo.GetErrand(t.Context(), f.row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, row.ResultState)
}
