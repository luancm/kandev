package config

// MCPIdentity returns the logical MCP caller while retaining internal transport grouping.
func (cfg *InstanceConfig) MCPIdentity() (string, string) {
	if cfg.ErrandMCPOwner != nil {
		return cfg.ErrandMCPOwner.TaskID, ""
	}
	return cfg.TaskID, cfg.SessionID
}
