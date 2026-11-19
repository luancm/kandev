package acp

import acpsdk "github.com/coder/acp-go-sdk"

type toolCallOwner struct {
	sessionID        string
	parentToolCallID string
	promptGeneration uint64
}

func initiallyTerminalTool(status string) bool {
	switch status {
	case toolStatusCompleted, toolStatusComplete, string(acpsdk.ToolCallStatusFailed), toolStatusError, toolStatusCancelled:
		return true
	default:
		return false
	}
}

// Cleanup carries the tool's original prompt, even after that prompt retires.
func (a *Adapter) trackActiveToolOwner(toolCallID string, owner toolCallOwner) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, active := a.activeToolCalls[toolCallID]; !active {
		a.forgetPromptHandoffToolLocked(toolCallID)
		return
	}
	if a.activeToolOwners == nil {
		a.activeToolOwners = make(map[string]toolCallOwner)
	}
	if _, exists := a.activeToolOwners[toolCallID]; !exists {
		a.activeToolOwners[toolCallID] = owner
	}
}
