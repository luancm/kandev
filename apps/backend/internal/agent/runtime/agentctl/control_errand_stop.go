package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

// StopErrandInstance succeeds only when the control server confirms absence
// under its owner-checked stop contract.
func (c *ControlClient) StopErrandInstance(ctx context.Context, id string, owner agentctltypes.ErrandMCPOwner) error {
	body, err := json.Marshal(owner)
	if err != nil {
		return err
	}
	endpoint := c.baseURL + "/api/v1/instances/" + url.PathEscape(id) + "/stop-errand"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.mu.Lock()
	httpClient := *c.httpClient
	c.mu.Unlock()
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("stop errand instance: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("stop errand instance unconfirmed: HTTP %d", resp.StatusCode)
	}
	return nil
}
