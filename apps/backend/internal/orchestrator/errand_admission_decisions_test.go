package orchestrator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

func newOwnedErrandWithPendingResult(t *testing.T) (*errandLaunchFixture, *errands.Service, *models.Errand) {
	t.Helper()
	f := newErrandLaunchFixture(t)
	service, _ := newErrandServiceOnRealStack(t, f)
	now := time.Now().UTC()
	row := &models.Errand{ID: "owned-errand", TaskID: f.taskID, ParentSessionID: f.parent,
		Title: "inspect", Objective: "inspect dispatch", AgentProfileID: "errand-profile",
		FirstTurnID: "first-turn", SubmissionID: "first-submission", Lifecycle: models.ErrandLifecycleLaunched,
		LastOutcome: models.ErrandOutcomeDone, ResultState: models.ErrandResultPending,
		ResultText: "prior output", ResultKind: models.ErrandResultOutput, ResultTurnID: "prior-turn",
		CreatedAt: now, UpdatedAt: now}
	require.NoError(t, f.repo.CreateErrand(t.Context(), row))
	return f, service, row
}

func TestErrandAdmissionServiceReleaseUsesCapturedGeneration(t *testing.T) {
	f, service, row := newOwnedErrandWithPendingResult(t)
	_, err := f.repo.ClaimResults(t.Context(), f.taskID, f.parent, models.ErrandDeliveryTurnContext, "turn-release",
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, time.Now().UTC(), admissionClaimInventory(t, f.repo, f.parent))
	require.NoError(t, err)
	a := models.ErrandParentAdmission{TaskID: f.taskID, ParentSessionID: f.parent, ErrandID: row.ID,
		SenderSessionID: f.parent, TurnID: "admission-turn", SubmissionID: "prompt:admission-submission"}
	_, err = f.repo.ReserveErrandParentAdmission(t.Context(), a)
	require.NoError(t, err)
	require.NoError(t, f.repo.DecideErrandParentAdmission(t.Context(), a, models.ErrandAdmissionAccepted))
	require.NoError(t, service.ReleaseTurnClaim(t.Context(), "turn-release"))
	updated, err := f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, updated.ParentAdmissionGeneration)
	require.Equal(t, models.ErrandResultSuperseded, updated.ResultState, "release must use the generation from the immutable receipt")
}

func TestErrandAdmissionAbortBeforeDelayedAwaitClaim(t *testing.T) {
	f, service, row := newOwnedErrandWithPendingResult(t)
	parent, err := f.repo.GetTaskSession(t.Context(), f.parent)
	require.NoError(t, err)
	owner := models.ErrandAwaitIdentity{TaskID: f.taskID, ParentSessionID: f.parent,
		ParentIncarnationID: parent.QueueIncarnationID, HarnessGeneration: 1, Token: "early-abort"}
	require.NoError(t, service.AbortOwnedAwait(t.Context(), owner))
	_, err = f.repo.ClaimResults(t.Context(), f.taskID, f.parent, models.ErrandDeliveryAwait, "early-abort",
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, time.Now().UTC(), admissionClaimInventory(t, f.repo, f.parent))
	require.ErrorIs(t, err, models.ErrErrandVersionConflict)
	updated, err := f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, updated.ResultState)
}

func TestErrandAdmissionServiceAbortReleasesRealAwait(t *testing.T) {
	f, service, row := newOwnedErrandWithPendingResult(t)
	parent, err := f.repo.GetTaskSession(t.Context(), f.parent)
	require.NoError(t, err)
	owner := models.ErrandAwaitIdentity{TaskID: f.taskID, ParentSessionID: f.parent,
		ParentIncarnationID: parent.QueueIncarnationID, HarnessGeneration: 1, Token: "await-abort"}
	require.NoError(t, f.repo.BeginErrandAwait(t.Context(), owner))
	_, err = f.repo.ClaimResults(t.Context(), f.taskID, f.parent, models.ErrandDeliveryAwait, "await-abort",
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, time.Now().UTC(), admissionClaimInventory(t, f.repo, f.parent))
	require.NoError(t, err)
	require.NoError(t, service.AbortOwnedAwait(t.Context(), owner))
	updated, err := f.repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, updated.ResultState, "known await failure must durably abort before release")
	_, err = f.repo.ClaimResults(t.Context(), f.taskID, f.parent, models.ErrandDeliveryAwait, "await-abort",
		[]models.ErrandClaimCandidate{{ID: updated.ID, Version: updated.Version}}, time.Now().UTC(), admissionClaimInventory(t, f.repo, f.parent))
	require.ErrorIs(t, err, models.ErrErrandVersionConflict, "aborted token cannot reclaim")
}
