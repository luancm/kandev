package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type recoveryRefusalOrchestrator struct{ firstTurnCaptureOrchestrator }

func (*recoveryRefusalOrchestrator) ValidateErrandContinuation(context.Context, string) error {
	return db.ErrErrandSettlementPending
}

// @covers AC-TASKS-ERRANDS-002.8
func TestMessageAdmissionRefusesErrandBeforeConversationWrite(t *testing.T) {
	for _, state := range []models.TaskSessionState{models.TaskSessionStateWaitingForInput, models.TaskSessionStateCompleted} {
		t.Run(string(state), func(t *testing.T) { assertErrandMessageRecoveryRefusal(t, state) })
	}
}

func assertErrandMessageRecoveryRefusal(t *testing.T, state models.TaskSessionState) {
	t.Helper()
	repo := &messageAddSwitchRepo{tasks: map[string]*models.Task{"t1": {ID: "t1"}}, sessions: map[string]*models.TaskSession{"s1": {ID: "s1", TaskID: "t1", Role: models.SessionRoleErrand, State: state}}, primaryID: "s1"}
	svc := service.NewService(service.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo, TaskEnvironments: repo, Reviews: repo}, nil, logger.Default(), service.RepositoryDiscoveryConfig{})
	h := NewMessageHandlers(svc, &recoveryRefusalOrchestrator{}, logger.Default())
	for range 2 {
		request, err := ws.NewRequest("refused", ws.ActionMessageAdd, map[string]interface{}{"task_id": "t1", "session_id": "s1", "content": "new instruction"})
		require.NoError(t, err)
		response, err := h.wsAddMessage(t.Context(), request)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeError, response.Type)
		var payload struct {
			Details map[string]interface{} `json:"details"`
		}
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		require.Equal(t, "errand_recovery_unresolved", payload.Details["reason"])
	}
	require.Empty(t, repo.messages)
	require.Empty(t, repo.turns)
	require.Zero(t, repo.taskStateUpdateCalls)
}
