package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

func (m *ControlServer) handleStopErrandInstance(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var owner agentctltypes.ErrandMCPOwner
	if err := c.ShouldBindJSON(&owner); err != nil || owner.TaskID == "" || owner.ErrandID == "" || owner.Generation == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid errand owner"})
		return
	}

	// Once admitted, teardown runs to its own deadline even if the caller disconnects.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	id := c.Param("id")
	err := m.instMgr.StopErrandInstance(ctx, id, owner)
	switch {
	case errors.Is(err, instance.ErrErrandInstanceConflict):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case err != nil:
		m.logger.Error("failed to stop errand instance", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "errand instance stop unconfirmed"})
	default:
		c.Status(http.StatusNoContent)
	}
}
