package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/gitremote"
	"go.uber.org/zap"
)

// SetComparisonContextsRequest is presence-aware. An omitted field retains
// the currently applied observations; an explicit null or empty object clears
// all observations. A non-empty object updates only the listed worktrees.
type SetComparisonContextsRequest struct {
	ComparisonContexts json.RawMessage `json:"comparison_contexts"`
}

func (s *Server) handleSetComparisonContexts(c *gin.Context) {
	var req SetComparisonContextsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn("set comparison-contexts request rejected: malformed json", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid JSON body"})
		return
	}
	if len(req.ComparisonContexts) == 0 {
		// The additive field was omitted. Retain all existing observations.
		c.JSON(http.StatusOK, gin.H{"ok": true, "retained": true})
		return
	}
	if bytes.Equal(bytes.TrimSpace(req.ComparisonContexts), []byte("null")) {
		if err := s.procMgr.UpdateComparisonContexts(c.Request.Context(), nil); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{errKey: err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	var contexts map[string]gitremote.ComparisonContext
	if err := json.Unmarshal(req.ComparisonContexts, &contexts); err != nil || contexts == nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "comparison_contexts must be an object"})
		return
	}
	for key, context := range contexts {
		if key != "" && (filepath.IsAbs(key) || filepath.Clean(key) != key || key == "." || strings.HasPrefix(key, ".."+string(filepath.Separator)) || strings.ContainsRune(key, '\x00')) {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "comparison context " + key + ": invalid worktree key"})
			return
		}
		if err := context.Validate(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "comparison context " + key + ": " + err.Error()})
			return
		}
	}
	if err := s.procMgr.UpdateComparisonContexts(c.Request.Context(), contexts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "updated": len(contexts)})
}
