package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// Bridge the orchestrator's message interface to the real task service. The
// shared fixture otherwise inherits no-op streaming methods from its mock.
// Request shapes match backendapp's private messageCreatorAdapter.
type warningStreamMessageCreator struct {
	*serviceBackedMessageCreator
}

func (m *warningStreamMessageCreator) CreateAgentMessageStreaming(ctx context.Context, messageID, taskID, content, sessionID, turnID string) error {
	_, err := m.svc.CreateMessageWithID(ctx, messageID, &taskservice.CreateMessageRequest{
		TaskSessionID: sessionID, TaskID: taskID, TurnID: turnID,
		Content: content, AuthorType: "agent",
	})
	return err
}

func (m *warningStreamMessageCreator) CreateThinkingMessageStreaming(ctx context.Context, messageID, taskID, content, sessionID, turnID string) error {
	_, err := m.svc.CreateMessageWithID(ctx, messageID, &taskservice.CreateMessageRequest{
		TaskSessionID: sessionID, TaskID: taskID, TurnID: turnID,
		AuthorType: "agent", Type: "thinking", Metadata: map[string]interface{}{"thinking": content},
	})
	return err
}

func (m *warningStreamMessageCreator) CreateToolCallMessage(ctx context.Context, taskID, toolCallID, parentToolCallID, title, status, sessionID, turnID string, normalized *streams.NormalizedPayload) error {
	metadata := map[string]interface{}{"tool_call_id": toolCallID, "title": title, "status": status}
	if parentToolCallID != "" {
		metadata["parent_tool_call_id"] = parentToolCallID
	}
	messageType := "tool_call"
	if normalized != nil {
		metadata["normalized"] = normalized
		messageType = normalized.Kind().ToMessageType()
	}
	return m.CreateSessionMessage(ctx, taskID, title, sessionID, messageType, turnID, metadata, false)
}
