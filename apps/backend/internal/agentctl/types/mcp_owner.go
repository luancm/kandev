package types

// ErrandMCPOwnerCapability requires both owner projection and question refusal.
const ErrandMCPOwnerCapability = "errand-mcp-owner.v2"

// ErrandMCPOwner is the backend-projected owner used to configure the MCP surface.
// Backend operations validate the execution owner and current binding independently.
type ErrandMCPOwner struct {
	TaskID     string `json:"task_id"`
	ErrandID   string `json:"errand_id"`
	Generation uint64 `json:"generation"`
}
