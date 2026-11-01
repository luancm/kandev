package codexappserver

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// nativeAwaitReceipt uses the pinned mcpToolCall item fields, before any display
// preparation or synthetic fallback ID. A remapped child cannot attest receipt.
func (a *Adapter) nativeAwaitReceipt(item map[string]any, source, root, parent string, completed bool) *streams.ErrandAwaitReceipt {
	if !completed || stringField(item, "server") != "kandev" || stringField(item, "tool") != "await_errands_kandev" {
		return nil
	}
	var receipt *streams.ErrandAwaitReceipt
	if source != "" && source == root && root == a.GetSessionID() && parent == "" && stringField(item, "id") != "" && stringField(item, "status") == "completed" && item["error"] == nil {
		receipt = streams.ParseErrandAwaitReceipt(item["result"])
	}
	if receipt == nil {
		a.log.Warn("native await result has no usable receipt", zap.String("tool_call_id", stringField(item, "id")))
	}

	return receipt
}

func (a *Adapter) emitMCPItem(item map[string]any, root, source, parent, turn string, completed bool) {
	call := stringField(item, "id")
	server, tool := stringField(item, "server"), stringField(item, "tool")
	if server == "" || tool == "" {
		a.log.Warn("native MCP item missing schema identity", zap.String("tool_call_id", call))
	}
	name := server + "/" + tool
	payload := streams.NewMCPTool(name, item["arguments"])
	payload.Generic().Output = item["result"]
	status, kind := "in_progress", streams.EventTypeToolCall
	if completed {
		status, kind = "complete", streams.EventTypeToolUpdate
		if stringField(item, "status") != "completed" || item["error"] != nil {
			status = "error"
		}
	}
	a.emit(streams.AgentEvent{Type: kind, SessionID: root, OperationID: turn, ParentToolCallID: parent,
		ToolCallID: call, ToolName: name, ToolStatus: status, NormalizedPayload: payload,
		ErrandAwaitReceipt: a.nativeAwaitReceipt(item, source, root, parent, completed)})
}
