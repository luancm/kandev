package errands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

type ownedSettlementTranscript struct {
	repo *sqlite.Repository
}

func (r ownedSettlementTranscript) ReadTurn(ctx context.Context, errandID, turnID string) (TurnView, error) {
	turn, err := r.repo.GetErrandTurn(ctx, errandID, turnID)
	if err != nil {
		return TurnView{}, err
	}
	message, err := r.repo.GetErrandLastAssistantMessage(ctx, errandID, turnID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return TurnView{}, err
	}
	view := TurnView{Completed: turn.CompletedAt != nil}
	if message != nil {
		view.LastAgentText = message.Content
	}
	return view, nil
}

func newOwnedSettlementFixture(t *testing.T, outcome models.ErrandOutcome, answer string) (*harness, *sqlite.Repository, *models.Errand) {
	t.Helper()
	ctx := t.Context()
	conn := serviceClaimDB(t, false)
	repo, err := sqlite.NewWithDB(conn, conn, nil)
	require.NoError(t, err)
	const workspaceID = "owned-race-workspace"
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Settlement race"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: testTask, WorkspaceID: workspaceID, Title: "Settlement race"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: testParent, TaskID: testTask, AgentProfileID: "parent-profile", State: models.TaskSessionStateWaitingForInput,
	}))
	row := &models.Errand{
		ID: "owned-race", TaskID: testTask, ParentSessionID: testParent,
		Title: "Audit", AgentProfileID: "profile-1", FirstTurnID: "owned-race-turn", SubmissionID: "prompt:owned-race",
		Lifecycle: models.ErrandLifecycleLaunched, AwaitingResult: true, ResultState: models.ErrandResultNone,
	}
	require.NoError(t, repo.CreateErrand(ctx, row))
	_, err = repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
	require.NoError(t, err)
	binding := admitOwnedErrandFixture(t, repo, row)
	if answer != "" {
		require.NoError(t, repo.CreateErrandMessage(ctx, row.ID, &models.Message{
			ID: "first-answer", TurnID: row.FirstTurnID, AuthorType: models.MessageAuthorAgent, Content: answer,
		}))
	}
	if outcome != "" {
		projectOwnedSettlementTerminal(t, repo, row, binding, outcome)
	}
	execution, err := repo.GetErrandExecutionDescriptor(ctx, row.ID)
	require.NoError(t, err)
	h := newHarness(t)
	h.service.store = repo
	h.service.SetTurnReader(ownedSettlementTranscript{repo})
	h.stopper.store = repo
	h.stopper.target = &ErrandTeardownTarget{
		TaskID: row.TaskID, ErrandID: row.ID, OwnerGeneration: execution.Generation,
		ExecutionID: execution.ExecutionID, PromptGeneration: execution.PromptGeneration, NoTurn: execution.ActiveTurnID == "",
		TurnID: execution.ActiveTurnID, SubmissionID: execution.ActiveSubmissionID,
	}
	return h, repo, row
}

func projectOwnedSettlementTerminal(t *testing.T, repo *sqlite.Repository, row *models.Errand, binding models.ErrandHarnessBinding, outcome models.ErrandOutcome) {
	t.Helper()
	ctx := t.Context()
	kind, state := streams.EventTypeComplete, models.DeliverySubmissionCompleted
	if outcome == models.ErrandOutcomeFailed {
		kind, state = streams.EventTypeError, models.DeliverySubmissionFailed
	}
	payload, err := json.Marshal(streams.AgentEvent{
		Type: kind, SessionID: binding.NativeSessionID, TurnID: row.FirstTurnID, PromptGeneration: 1,
	})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: models.ErrandExecutionIdentity(row.ID), TaskID: row.TaskID, ExecutionID: binding.ExecutionID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: 1, OwnerGeneration: 1,
		ExpectedStreamID: binding.StreamID, StreamID: binding.StreamID, Sequence: 1,
		SubmissionID: row.SubmissionID, EventType: kind, Payload: payload, Terminal: true,
	}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	_, err = repo.SettleAgentDeliveryTerminal(ctx, binding.StreamID, 1, state, row.CreatedAt)
	require.NoError(t, err)
}
