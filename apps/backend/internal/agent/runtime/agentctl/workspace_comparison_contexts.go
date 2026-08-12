package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kandev/kandev/internal/common/gitremote"
)

// UpdateComparisonContexts sends a presence-aware observation. A nil pointer
// omits the field and retains the server's last observation; a pointer to an
// empty map explicitly clears every worktree; a populated map updates only
// those keys and preserves siblings.
func (c *Client) UpdateComparisonContexts(ctx context.Context, contexts *map[string]gitremote.ComparisonContext) error {
	body, err := json.Marshal(struct {
		ComparisonContexts *map[string]gitremote.ComparisonContext `json:"comparison_contexts,omitempty"`
	}{ComparisonContexts: contexts})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/workspace/comparison-contexts", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := readResponseBody(resp)
		return fmt.Errorf("update comparison contexts failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// SetComparisonContexts replaces the supplied worktree observations. It is
// the common full-observation call used by lifecycle refreshes.
func (c *Client) SetComparisonContexts(ctx context.Context, contexts map[string]gitremote.ComparisonContext) error {
	return c.UpdateComparisonContexts(ctx, &contexts)
}
