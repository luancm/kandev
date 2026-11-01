package codexappserver

import (
	"context"
	"encoding/json"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

const nativeAwaitBody = `{"duration_ms":7,"end_reason":"all_ready","errands":[{"errand_id":"child","status":"done","result":"actual result"}],"await_delivery":{"version":1,"token":"attempt","items":[{"errand_id":"child","result_revision":"revision"}]}}`

func TestCodexAppServerAwaitReceiptPreservesActualResult(t *testing.T) {
	a := NewAdapter(&shared.Config{}, logger.Default())
	t.Cleanup(func() { _ = a.Close() })
	a.threadID = "parent"
	item := map[string]any{"type": "mcpToolCall", "id": "native-call", "server": "kandev", "tool": "await_errands_kandev", "status": "completed", "arguments": map[string]any{}, "error": nil, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": nativeAwaitBody}}}}
	raw, err := json.Marshal(map[string]any{"threadId": "parent", "turnId": "turn", "item": item})
	require.NoError(t, err)
	a.handleNotification(context.Background(), "item/completed", raw)
	event := <-a.Updates()
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.NotNil(t, wire["errand_await_receipt"], "pinned native mcpToolCall result must carry bounded receipt")
	require.Equal(t, "kandev/await_errands_kandev", event.ToolName)
	require.NotNil(t, event.NormalizedPayload)
	require.Equal(t, item["result"], event.NormalizedPayload.Generic().Output)
}

func TestCodexAppServerAwaitReceiptRejectsUnsupportedObservations(t *testing.T) {
	for _, scenario := range []string{"error", "mcp-error", "preview", "no-result", "no-id", "missing-thread", "foreign-server", "foreign-tool", "toolName-only", "child-thread", "mapped-child-thread", "not-completed", "failed", "assistant-text"} {
		t.Run(scenario, func(t *testing.T) {
			a := NewAdapter(&shared.Config{}, logger.Default())
			t.Cleanup(func() { _ = a.Close() })
			a.threadID = "parent"
			result := map[string]any{"content": []any{map[string]any{"type": "text", "text": nativeAwaitBody}}}
			item := map[string]any{"type": "mcpToolCall", "id": "native-call", "server": "kandev", "tool": "await_errands_kandev", "status": "completed", "arguments": map[string]any{}, "error": nil, "result": result}
			params := map[string]any{"threadId": "parent", "turnId": "turn", "item": item}
			method := "item/completed"
			switch scenario {
			case "error":
				item["error"] = map[string]any{"message": "failed"}
			case "mcp-error":
				result["isError"] = true
			case "preview":
				item["result"] = map[string]any{"truncated": true, "preview": nativeAwaitBody}
			case "no-result":
				delete(item, "result")
			case "no-id":
				delete(item, "id")
			case "missing-thread":
				delete(params, "threadId")
			case "foreign-server":
				item["server"] = "other"
			case "foreign-tool":
				item["tool"] = "other"
			case "toolName-only":
				delete(item, "tool")
				item["toolName"] = "await_errands_kandev"
			case "child-thread":
				params["threadId"] = "child"
			case "mapped-child-thread":
				params["threadId"] = "child"
				a.children["child"] = childBinding{toolCallID: "parent-call", parentThreadID: "parent"}
			case "not-completed":
				method = "item/started"
			case "failed":
				item["status"] = "failed"
			case "assistant-text":
				item["type"] = "agentMessage"
			}
			raw, err := json.Marshal(params)
			require.NoError(t, err)
			a.handleNotification(context.Background(), method, raw)
			select {
			case event := <-a.Updates():
				require.Nil(t, event.ErrandAwaitReceipt)
				if scenario == "mapped-child-thread" {
					require.Equal(t, "parent", event.SessionID)
					require.Equal(t, "parent-call", event.ParentToolCallID)
				}
			default:
			}
		})
	}
}

func TestCodexAppServerAwaitReceiptDiagnosesMissingOrigin(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	a := NewAdapter(&shared.Config{}, log)
	t.Cleanup(func() { _ = a.Close() })
	a.threadID = "parent"
	item := map[string]any{"type": "mcpToolCall", "id": "native-call", "server": "kandev", "tool": "await_errands_kandev", "status": "completed", "arguments": map[string]any{}, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": nativeAwaitBody}}}}
	raw, err := json.Marshal(map[string]any{"item": item, "turnId": "turn"})
	require.NoError(t, err)
	a.handleNotification(context.Background(), "item/completed", raw)
	event := <-a.Updates()
	require.Nil(t, event.ErrandAwaitReceipt)
	require.Equal(t, 1, logs.Len(), "missing native origin must be diagnosed")
}

func TestCodexAppServerAwaitReceiptDiagnosesSchemaMismatch(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	a := NewAdapter(&shared.Config{}, log)
	t.Cleanup(func() { _ = a.Close() })
	a.threadID = "parent"
	item := map[string]any{"type": "mcpToolCall", "id": "native-call", "server": "kandev", "toolName": "await_errands_kandev", "status": "completed", "arguments": map[string]any{}, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": nativeAwaitBody}}}}
	raw, err := json.Marshal(map[string]any{"item": item, "turnId": "turn", "threadId": "parent"})
	require.NoError(t, err)
	a.handleNotification(context.Background(), "item/completed", raw)
	event := <-a.Updates()
	require.Nil(t, event.ErrandAwaitReceipt)
	require.Equal(t, 1, logs.FilterMessage("native MCP item missing schema identity").Len(), "a protocol field mismatch must be diagnosed without using toolName as authority")
}
