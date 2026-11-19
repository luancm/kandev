package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type errandShellService struct {
	*fakeErrandService
	reads int
}

func (s *errandShellService) ReadErrandMessageDetail(_ context.Context, taskID, errandID, messageID string) (*models.Message, error) {
	s.reads++
	if taskID != "task-b" || errandID != "errand-1" || messageID != "command" {
		return nil, models.ErrErrandNotFound
	}
	return &models.Message{
		ID: messageID, Type: models.MessageTypeToolExecute,
		Metadata: map[string]any{"status": "complete", "normalized": map[string]any{
			"shell_exec": map[string]any{"output": map[string]any{"stdout": "owned stdout", "has_output": true}},
		}},
	}, nil
}

func TestErrandShellOutputRouteChecksTaskScope(t *testing.T) {
	svc := &errandShellService{fakeErrandService: &fakeErrandService{}}
	router := gin.New()
	errandHandlers(t, svc, false).registerHTTP(router)
	get := func(user, errandID string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-b/errands/"+errandID+"/messages/command/shell-output", nil)
		router.ServeHTTP(recorder, req.WithContext(errandCallerAs(user)))
		return recorder
	}

	owned := get("user-b", "errand-1")
	require.Equal(t, http.StatusOK, owned.Code)
	require.Contains(t, owned.Body.String(), "owned stdout")
	wrongOwner := get("user-b", "other-errand")
	forbidden := get("user-a", "errand-1")
	require.Equal(t, http.StatusNotFound, wrongOwner.Code)
	require.Equal(t, wrongOwner.Code, forbidden.Code)
	require.JSONEq(t, wrongOwner.Body.String(), forbidden.Body.String())
	require.Equal(t, 2, svc.reads, "foreign task callers cannot reach message detail")
}
