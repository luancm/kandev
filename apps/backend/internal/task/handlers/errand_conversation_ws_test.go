package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type ownedConversationService struct {
	*fakeErrandService
	requests     int
	acknowledged [][3]string
}

func (s *ownedConversationService) MarkErrandRead(_ context.Context, taskID, errandID, messageID string) error {
	s.acknowledged = append(s.acknowledged, [3]string{taskID, errandID, messageID})
	return nil
}

func (s *ownedConversationService) ReadErrandMessages(_ context.Context, taskID, errandID, before string, limit int) ([]*models.Message, bool, error) {
	s.requests++
	if taskID != "task-b" || errandID != "errand-1" || before != "older-page" || limit != 20 {
		return nil, false, models.ErrInvalidErrand
	}
	return []*models.Message{{
		ID: "answer", TaskID: taskID, TurnID: "turn-errand",
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "owned answer",
	}}, true, nil
}

func TestErrandConversationRouteIsTaskScoped(t *testing.T) {
	svc := &ownedConversationService{fakeErrandService: &fakeErrandService{errand: sampleErrand()}}
	h := errandHandlers(t, svc, false)
	dispatcher := ws.NewDispatcher()
	h.registerErrandWS(dispatcher)
	message := errandMessage(t, "errand.messages", map[string]any{
		"task_id": "task-b", "errand_id": "errand-1", "before_message_id": "older-page", "limit": 20,
	})
	response, err := dispatcher.Dispatch(errandCallerAs("user-b"), message)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var page struct {
		TaskID   string           `json:"task_id"`
		ErrandID string           `json:"errand_id"`
		Messages []models.Message `json:"messages"`
		HasMore  bool             `json:"has_more"`
	}
	require.NoError(t, response.ParsePayload(&page))
	require.Equal(t, "task-b", page.TaskID)
	require.Equal(t, "errand-1", page.ErrandID)
	require.True(t, page.HasMore)
	require.Len(t, page.Messages, 1)
	require.Equal(t, "owned answer", page.Messages[0].Content)
	require.Empty(t, page.Messages[0].TaskSessionID)

	response, err = dispatcher.Dispatch(errandCallerAs("user-a"), message)
	require.NoError(t, err)
	require.Equal(t, ws.ErrorCodeNotFound, decodeError(t, response).Code)
	require.Equal(t, 1, svc.requests, "foreign requests must be refused before reading the transcript")
}

func TestErrandMarkReadRouteRequiresTaskWriteAccess(t *testing.T) {
	svc := &ownedConversationService{fakeErrandService: &fakeErrandService{errand: sampleErrand()}}
	dispatcher := ws.NewDispatcher()
	errandHandlers(t, svc, true).registerErrandWS(dispatcher)
	message := errandMessage(t, "errand.mark_read", map[string]any{
		"task_id": "task-b", "errand_id": "errand-1", "message_id": "observed-output",
	})
	response, err := dispatcher.Dispatch(errandCallerAs("user-b"), message)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)

	response, err = dispatcher.Dispatch(errandCallerAs("user-a"), message)
	require.NoError(t, err)
	require.Equal(t, ws.ErrorCodeNotFound, decodeError(t, response).Code)
	require.Equal(t, [][3]string{{"task-b", "errand-1", "observed-output"}}, svc.acknowledged)
}
