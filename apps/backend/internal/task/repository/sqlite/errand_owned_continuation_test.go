package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

func TestOwnedErrandContinuationRequiresConfirmedClose(t *testing.T) {
	exerciseOwnedErrandContinuation(t, newRepoForSessionTests(t))
}

func TestPostgresOwnedErrandContinuationRequiresConfirmedClose(t *testing.T) {
	exerciseOwnedErrandContinuation(t, newPostgresErrandRepo(t))
}

func TestOwnedErrandContinuationRespectsTaskCleanupBarrier(t *testing.T) {
	exerciseOwnedErrandContinuationCleanupBarrier(t, newRepoForSessionTests)
}

func TestPostgresOwnedErrandContinuationRespectsTaskCleanupBarrier(t *testing.T) {
	exerciseOwnedErrandContinuationCleanupBarrier(t, newPostgresErrandRepo)
}

func exerciseOwnedErrandContinuationCleanupBarrier(t *testing.T, newRepo func(*testing.T) *Repository) {
	t.Helper()
	for _, stage := range []string{"new", "reserved", "accepted"} {
		t.Run(stage, func(t *testing.T) {
			ctx := t.Context()
			repo := newRepo(t)
			row := seedOwnedErrandCompletion(t, repo)
			_, err := repo.TransitionErrandTeardownIntent(ctx, row.ID, row.TeardownIntent.ID, row.Version, models.ErrandTeardownIntentCompleted)
			require.NoError(t, err)
			request := models.ErrandContinuation{
				TaskID: row.TaskID, ErrandID: row.ID, TurnID: "follow-up", SubmissionID: "prompt:errand:next-submission",
				AuthorID: "reader", Content: "Review the output",
			}
			if stage != "new" {
				prepared, err := repo.PrepareErrandContinuation(ctx, request)
				require.NoError(t, err)
				if stage == "accepted" {
					bindAcceptedErrandContinuation(t, repo, request, prepared)
				}
			}
			reserveBarrier(t, repo, row.TaskID, "delete:continuation-fence")
			before, err := repo.GetErrandExecutionDescriptor(ctx, row.ID)
			require.NoError(t, err)
			prepared, err := repo.PrepareErrandContinuation(ctx, request)
			if stage == "accepted" {
				require.NoError(t, err)
				require.True(t, prepared.Accepted)
			} else {
				require.ErrorIs(t, err, repoerrors.ErrTaskCleanupInProgress)
				require.Nil(t, prepared)
			}
			after, err := repo.GetErrandExecutionDescriptor(ctx, row.ID)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestOwnedErrandContinuationCountsReservedWorkAgainstCapacity(t *testing.T) {
	exerciseOwnedErrandContinuationCapacity(t, newRepoForSessionTests(t))
}

func TestPostgresOwnedErrandContinuationCountsReservedWorkAgainstCapacity(t *testing.T) {
	exerciseOwnedErrandContinuationCapacity(t, newPostgresErrandRepo(t))
}

func TestOwnedErrandParentContinuationReservesBeforeSupersedingResult(t *testing.T) {
	exerciseOwnedErrandParentContinuation(t, newRepoForSessionTests(t))
}

func TestPostgresOwnedErrandParentContinuationReservesBeforeSupersedingResult(t *testing.T) {
	exerciseOwnedErrandParentContinuation(t, newPostgresErrandRepo(t))
}

func exerciseOwnedErrandParentContinuation(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	row := seedOwnedErrandCompletion(t, repo)
	_, err := repo.TransitionErrandTeardownIntent(ctx, row.ID, row.TeardownIntent.ID, row.Version, models.ErrandTeardownIntentCompleted)
	require.NoError(t, err)
	request := models.ErrandContinuation{
		TaskID: row.TaskID, ErrandID: row.ID, TurnID: "parent-follow-up", SubmissionID: "prompt:errand:parent-follow-up",
		Content: "New parent instruction", AuthorID: row.ParentSessionID, ParentSessionID: row.ParentSessionID,
	}
	foreign := request
	foreign.ParentSessionID = "other-parent"
	foreign.AuthorID = foreign.ParentSessionID
	_, err = repo.PrepareErrandContinuation(ctx, foreign)
	require.ErrorIs(t, err, models.ErrInvalidErrand)
	_, err = repo.GetErrandTurn(ctx, row.ID, request.TurnID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	prepared, err := repo.PrepareErrandContinuation(ctx, request)
	require.NoError(t, err)
	state, err := repo.ReadOwnedErrandParentAdmission(ctx, request.ParentAdmission())
	require.NoError(t, err)
	require.Equal(t, models.ErrandAdmissionReserved, state)
	reserved, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, reserved.ResultState)
	require.False(t, reserved.AwaitingResult)
	require.NoError(t, repo.DiscardUnallocatedErrandContinuation(ctx, request, prepared.Generation))
	state, err = repo.ReadOwnedErrandParentAdmission(ctx, request.ParentAdmission())
	require.NoError(t, err)
	require.Empty(t, state)

	prepared, err = repo.PrepareErrandContinuation(ctx, request)
	require.NoError(t, err)
	evidence, owned, err := repo.ReadOwnedErrandContinuationAcceptance(ctx, request.ParentAdmission())
	require.NoError(t, err)
	require.True(t, owned)
	require.Equal(t, models.ErrandAdmissionUnknown, evidence)
	bindAcceptedErrandContinuation(t, repo, request, prepared)
	evidence, owned, err = repo.ReadOwnedErrandContinuationAcceptance(ctx, request.ParentAdmission())
	require.NoError(t, err)
	require.True(t, owned)
	require.Equal(t, models.ErrandAdmissionAccepted, evidence)
	require.NoError(t, repo.DecideErrandParentAdmission(ctx, request.ParentAdmission(), models.ErrandAdmissionAccepted))
	accepted, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultSuperseded, accepted.ResultState)
	require.True(t, accepted.AwaitingResult)
	require.Equal(t, row.ResultRevision, accepted.ResultRevision)
	require.EqualValues(t, 1, accepted.ParentAdmissionGeneration)
}

func exerciseOwnedErrandContinuationCapacity(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	row := seedOwnedErrandCompletion(t, repo)
	_, err := repo.TransitionErrandTeardownIntent(ctx, row.ID, row.TeardownIntent.ID, row.Version, models.ErrandTeardownIntentCompleted)
	require.NoError(t, err)
	for i := range 5 {
		waiting := sampleErrand(fmt.Sprintf("waiting-%d", i), row.TaskID, "parent", errandScenarioBase)
		require.NoError(t, repo.CreateErrand(ctx, waiting))
	}
	request := models.ErrandContinuation{
		TaskID: row.TaskID, ErrandID: row.ID, TurnID: "at-capacity", SubmissionID: "next", Content: "Continue",
	}
	_, err = repo.PrepareErrandContinuation(ctx, request)
	require.Error(t, err, "continuation cannot take a sixth live slot")
	_, err = repo.GetErrandTurn(ctx, row.ID, request.TurnID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, repo.DeleteErrand(ctx, "waiting-0"))
	prepared, err := repo.PrepareErrandContinuation(ctx, request)
	require.NoError(t, err)
	live, err := repo.CountLiveErrandsByTask(ctx, row.TaskID)
	require.NoError(t, err)
	require.Equal(t, 5, live, "reserved continuation must hold a slot before prompt dispatch")
	waiting := sampleErrand("replacement", row.TaskID, "parent", errandScenarioBase)
	require.ErrorAs(t, repo.CreateErrand(ctx, waiting), new(*models.ErrandCapacityError))
	require.NoError(t, repo.DiscardUnallocatedErrandContinuation(ctx, request, prepared.Generation))
	_, err = repo.GetErrandTurn(ctx, row.ID, request.TurnID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.NoError(t, repo.CreateErrand(ctx, waiting))
}

func exerciseOwnedErrandContinuation(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	row := seedOwnedErrandCompletion(t, repo)
	working, err := repo.HasWorkingErrands(ctx, row.TaskID)
	require.NoError(t, err)
	require.False(t, working, "projected terminal work no longer holds the workspace move guard")
	request := models.ErrandContinuation{
		TaskID: row.TaskID, ErrandID: row.ID, TurnID: "follow-up", SubmissionID: "prompt:errand:next-submission", AuthorID: "reader", Content: "Review the output",
	}
	_, err = repo.PrepareErrandContinuation(ctx, request)
	require.ErrorIs(t, err, db.ErrErrandSettlementPending)
	_, err = repo.GetErrandTurn(ctx, row.ID, request.TurnID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	current, err := repo.GetErrandExecutionDescriptor(ctx, row.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, current.Generation)

	_, err = repo.TransitionErrandTeardownIntent(ctx, row.ID, row.TeardownIntent.ID, row.Version, models.ErrandTeardownIntentCompleted)
	require.NoError(t, err)
	prepared, err := repo.PrepareErrandContinuation(ctx, request)
	require.NoError(t, err)
	require.Equal(t, &models.PreparedErrandContinuation{
		Generation: 2, NativeID: "native", Predecessor: row.TeardownIntent.Target,
	}, prepared)
	working, err = repo.HasWorkingErrands(ctx, row.TaskID)
	require.NoError(t, err)
	require.True(t, working, "a reserved successor can allocate the workspace before prompt dispatch")
	again, err := repo.PrepareErrandContinuation(ctx, request)
	require.NoError(t, err)
	require.Equal(t, prepared, again)
	wrong := request
	wrong.Content = "Different instruction"
	_, err = repo.PrepareErrandContinuation(ctx, wrong)
	require.ErrorIs(t, err, models.ErrInvalidErrand)
	wrong = request
	wrong.TurnID = "other-turn"
	_, err = repo.PrepareErrandContinuation(ctx, wrong)
	require.ErrorIs(t, err, db.ErrErrandSettlementPending)
	message, err := repo.GetErrandMessage(ctx, row.ID, request.TurnID)
	require.NoError(t, err)
	require.Equal(t, request.Content, message.Content)
	require.Empty(t, message.TaskSessionID)
	bindAcceptedErrandContinuation(t, repo, request, prepared)
	working, err = repo.HasWorkingErrands(ctx, row.TaskID)
	require.NoError(t, err)
	require.True(t, working, "an admitted successor holds the workspace move guard")
	require.NoError(t, repo.DiscardUnallocatedErrandContinuation(ctx, request, prepared.Generation))
	accepted, err := repo.PrepareErrandContinuation(ctx, request)
	require.NoError(t, err)
	require.True(t, accepted.Accepted)
	require.Equal(t, "next-execution", accepted.ExecutionID)
	stored, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, row.ResultRevision, stored.ResultRevision)
	require.Equal(t, row.ResultTurnID, stored.ResultTurnID)
}

func bindAcceptedErrandContinuation(t *testing.T, repo *Repository, request models.ErrandContinuation, prepared *models.PreparedErrandContinuation) {
	t.Helper()
	ctx := t.Context()
	bound, err := repo.BindErrandExecution(ctx, request.ErrandID, prepared.Generation, "next-execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{
		ErrandID: request.ErrandID, Generation: prepared.Generation, ExecutionID: "next-execution", IncarnationID: "next-incarnation",
		HarnessGeneration: 1, StreamID: "next-stream", NativeSessionID: prepared.NativeID,
	}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	prompt := []byte(request.Content)
	created, err := repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{
		ErrandID: binding.ErrandID, Generation: binding.Generation, ExecutionID: binding.ExecutionID, IncarnationID: binding.IncarnationID,
		StreamID: binding.StreamID, NativeSessionID: binding.NativeSessionID, HarnessGeneration: binding.HarnessGeneration,
		PromptGeneration: 1, TurnID: request.TurnID,
		Submission: &models.AgentDeliverySubmission{
			ID: request.SubmissionID, DispatchAttemptID: binding.ExecutionID, State: models.DeliverySubmissionAccepted,
			Payload: prompt, PayloadHash: journal.SubmissionHash(prompt),
		},
	})
	require.NoError(t, err)
	require.True(t, created)
}

func seedOwnedErrandCompletion(t *testing.T, repo *Repository) *models.Errand {
	t.Helper()
	ctx := t.Context()
	taskID := newErrandTask(t, repo, "owned-follow-up")
	row := sampleErrand("owned-follow-up", taskID, "parent", errandScenarioBase)
	row.Lifecycle = models.ErrandLifecycleLaunched
	mustCreateErrand(t, repo, row)
	_, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, row.ID, 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	binding := &models.ErrandHarnessBinding{
		ErrandID: row.ID, Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", NativeSessionID: "native",
	}
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, binding, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, row.ID, &models.Turn{ID: row.FirstTurnID, TaskID: taskID}))
	prompt := []byte(row.Objective)
	created, err := repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{
		ErrandID: row.ID, Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", NativeSessionID: "native", PromptGeneration: 1, TurnID: row.FirstTurnID,
		Submission: &models.AgentDeliverySubmission{ID: row.SubmissionID, DispatchAttemptID: "execution", State: models.DeliverySubmissionAccepted, Payload: prompt, PayloadHash: journal.SubmissionHash(prompt)},
	})
	require.NoError(t, err)
	require.True(t, created)
	payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeComplete, SessionID: "native", TurnID: row.FirstTurnID, PromptGeneration: 1})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: models.ErrandExecutionIdentity(row.ID), TaskID: taskID, ExecutionID: "execution", IncarnationID: "incarnation",
		HarnessGeneration: 1, OwnerGeneration: 1, StreamID: "stream", ExpectedStreamID: "stream", Sequence: 1, SubmissionID: row.SubmissionID,
		EventType: streams.EventTypeComplete, Payload: payload, Terminal: true,
	}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	_, err = repo.SettleAgentDeliveryTerminal(ctx, "stream", 1, models.DeliverySubmissionCompleted, errandScenarioBase)
	require.NoError(t, err)
	target, _, err := repo.ReadOwnedErrandLatestTerminal(ctx, row.ID)
	require.NoError(t, err)
	row.LatestSettledTurnID, row.ResultTurnID = row.FirstTurnID, row.FirstTurnID
	row.AwaitingResult, row.LastOutcome = false, models.ErrandOutcomeDone
	row.ResultText, row.ResultState, row.ResultKind = "original result", models.ErrandResultPending, models.ErrandResultOutput
	row.TeardownIntent = &models.ErrandTeardownIntent{Version: 1, ID: "close", RequestedAt: errandScenarioBase,
		State: models.ErrandTeardownIntentPending, Purpose: models.ErrandTeardownPurposeCompletion, Target: target}
	require.NoError(t, repo.UpdateErrand(ctx, row))
	return row
}
