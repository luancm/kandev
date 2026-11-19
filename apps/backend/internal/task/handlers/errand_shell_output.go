package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

func (h *TaskHandlers) httpErrandShellOutput(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, taskID := c.Request.Context(), c.Param("id")
	if h.errands == nil || h.authorizeErrandTask(ctx, taskID, authz.ScopeWorkspaceRead) != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	message, err := h.errands.ReadErrandMessageDetail(ctx, taskID, c.Param("errand_id"), c.Param("message_id"))
	if err != nil {
		reason, _ := errands.ReasonOf(err)
		if reason == errands.ReasonNotFound || errors.Is(err, models.ErrErrandNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
			return
		}
		handleNotFound(c, h.logger, err, "failed to load shell output")
		return
	}
	if message.Type != models.MessageTypeToolExecute {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}
	respondShellOutputSnapshot(c, message)
}
