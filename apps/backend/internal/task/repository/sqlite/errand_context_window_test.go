package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestOwnedErrandContextWindowProjectsToFinishedRow(t *testing.T) {
	exerciseOwnedErrandContextWindow(t, newRepoForSessionTests(t))
}

func TestPostgresOwnedErrandContextWindowProjectsToFinishedRow(t *testing.T) {
	exerciseOwnedErrandContextWindow(t, newPostgresErrandRepo(t))
}

func exerciseOwnedErrandContextWindow(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	row := seedOwnedErrandCompletion(t, repo)
	before, err := repo.GetErrandInspection(ctx, row.TaskID, row.ID)
	require.NoError(t, err)
	require.NotContains(t, before.Errand.ContextMetadata, models.SessionMetaKeyContextWindow)

	binding := models.ErrandHarnessBinding{
		ErrandID: row.ID, Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation",
		HarnessGeneration: 1, StreamID: "stream", NativeSessionID: "native",
	}
	for index, used := range []int64{0, 640, 1024} {
		projectErrandContextWindow(t, repo, row.TaskID, binding, int64(index+2), 1024, used)
		inspection, err := repo.GetErrandInspection(ctx, row.TaskID, row.ID)
		require.NoError(t, err)
		sample, ok := inspection.Errand.ContextMetadata[models.SessionMetaKeyContextWindow].(map[string]interface{})
		require.True(t, ok, "an owned context event must appear on the finished Errand row")
		require.EqualValues(t, used, sample["used"])
		require.EqualValues(t, 1024, sample["size"])
		require.Equal(t, before.Errand.Status, inspection.Errand.Status)
		require.Equal(t, before.Errand.ResultState, inspection.Errand.ResultState)
	}

	projectErrandContextWindow(t, repo, row.TaskID, binding, 5, 0, 0)
	cleared, err := repo.GetErrandInspection(ctx, row.TaskID, row.ID)
	require.NoError(t, err)
	require.Contains(t, cleared.Errand.ContextMetadata, models.SessionMetaKeyContextWindow)
	require.Nil(t, cleared.Errand.ContextMetadata[models.SessionMetaKeyContextWindow])
	statuses, err := repo.ReadErrandStatuses(ctx, row.TaskID, row.ID)
	require.NoError(t, err)
	require.Equal(t, cleared.Errand.ContextMetadata, statuses[row.ID].ContextMetadata)

	replacement := binding
	replacement.HarnessGeneration++
	replacement.StreamID = "replacement-stream"
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &replacement, 1))
	projectErrandContextWindow(t, repo, row.TaskID, replacement, 1, 2048, 800)
	projectErrandContextWindow(t, repo, row.TaskID, binding, 6, 1024, 300)
	inspection, err := repo.GetErrandInspection(ctx, row.TaskID, row.ID)
	require.NoError(t, err)
	sample := inspection.Errand.ContextMetadata[models.SessionMetaKeyContextWindow].(map[string]interface{})
	require.EqualValues(t, 2048, sample["size"], "a late event cannot replace the current binding's reading")
}

func projectErrandContextWindow(t *testing.T, repo *Repository, taskID string, binding models.ErrandHarnessBinding, sequence, size, used int64) {
	t.Helper()
	ctx := t.Context()
	providerEvent := streams.AgentEvent{
		Type: streams.EventTypeContextWindow, SessionID: binding.NativeSessionID,
		ContextWindowSize: size, ContextWindowUsed: used,
	}
	payload, err := json.Marshal(providerEvent)
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: models.ErrandExecutionIdentity(binding.ErrandID), TaskID: taskID, ExecutionID: binding.ExecutionID,
		IncarnationID: binding.IncarnationID, HarnessGeneration: binding.HarnessGeneration, OwnerGeneration: binding.HarnessGeneration,
		StreamID: binding.StreamID, ExpectedStreamID: binding.StreamID, Sequence: sequence,
		EventType: providerEvent.Type, Payload: payload,
	}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, sequence)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
}
