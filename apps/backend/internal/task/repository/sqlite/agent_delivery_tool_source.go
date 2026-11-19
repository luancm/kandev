package sqlite

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

type toolSource struct {
	Sequence         int64  `json:"sequence"`
	ToolID           string `json:"tool_id"`
	ParentID         string `json:"parent_id"`
	PromptGeneration uint64 `json:"prompt_generation"`
	ProtocolID       string `json:"protocol_id"`
	ControlTurnID    int64  `json:"control_turn_id"`
	TurnID           string `json:"turn_id"`
}

func (source toolSource) matches(event *models.AgentDeliveryEvent, agent streams.AgentEvent) bool {
	return source.Sequence < event.Sequence && source.ToolID == agent.ToolCallID &&
		source.ParentID == agent.ParentToolCallID && source.PromptGeneration == agent.PromptGeneration &&
		(source.ControlTurnID == 0 || agent.ControlTurnID == 0 || source.ControlTurnID == agent.ControlTurnID)
}
