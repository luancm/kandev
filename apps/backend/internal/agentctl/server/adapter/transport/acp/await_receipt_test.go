package acp

import (
	"encoding/json"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/require"
)

const awaitReceiptBody = `{"duration_ms":7,"end_reason":"all_ready","errands":[{"errand_id":"child","status":"done","result":"actual result"}],"await_delivery":{"version":1,"token":"attempt","items":[{"errand_id":"child","result_revision":"revision"}]}}`

func TestCodexACPAwaitReceiptUsesSameCanonicalMCPResult(t *testing.T) {
	a := newTestAdapter()
	a.agentID, a.sessionID = codexAgentID, "parent"
	a.normalizer, a.dialect = NewNormalizer(codexAgentID), newACPDialect(codexAgentID)
	input := map[string]any{"server": "kandev", "tool": "await_errands_kandev", "arguments": map[string]any{}}
	a.convertToolCallUpdate("parent", &sdk.SessionUpdateToolCall{ToolCallId: "native-call", Meta: map[string]any{"is_mcp_tool_call": true}, Kind: "execute", RawInput: input})
	result := map[string]any{"content": []any{map[string]any{"type": "text", "text": awaitReceiptBody}}}
	status := sdk.ToolCallStatus("completed")
	event := a.convertToolCallResultUpdate("parent", &sdk.SessionToolCallUpdate{ToolCallId: "native-call", Status: &status, RawOutput: map[string]any{"error": nil, "result": result}})
	require.Equal(t, result, event.NormalizedPayload.Generic().Output)
	require.Equal(t, streams.ErrandAwaitToolName, event.ToolName, "the actual completion must retain native MCP identity for inbox admission")
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.NotNil(t, wire["errand_await_receipt"], "canonical successful native MCP result must carry bounded serialized receipt")
}

func TestCodexACPAwaitReceiptRejectsUnsupportedObservations(t *testing.T) {
	for _, scenario := range []string{"error", "mcp-error", "preview", "no-result", "status-only", "no-initial", "no-marker", "foreign-server", "foreign-tool", "other-provider", "child-thread", "lineage", "no-id", "foreign-update-input", "failed"} {
		t.Run(scenario, func(t *testing.T) {
			a := newTestAdapter()
			a.agentID, a.sessionID = codexAgentID, "parent"
			a.normalizer, a.dialect = NewNormalizer(codexAgentID), newACPDialect(codexAgentID)
			input := map[string]any{"server": "kandev", "tool": "await_errands_kandev", "arguments": map[string]any{}}
			meta := map[string]any{"is_mcp_tool_call": true}
			call, session := "native-call", "parent"
			result := map[string]any{"content": []any{map[string]any{"type": "text", "text": awaitReceiptBody}}}
			envelope := map[string]any{"error": nil, "result": result}
			status := sdk.ToolCallStatus("completed")
			switch scenario {
			case "error":
				envelope["error"] = map[string]any{"message": "provider failure"}
			case "mcp-error":
				result["isError"] = true
			case "preview":
				envelope["result"] = map[string]any{"truncated": true, "preview": awaitReceiptBody}
			case "no-result":
				delete(envelope, "result")
			case "no-marker":
				meta = nil
			case "foreign-server":
				input["server"] = "external"
			case "foreign-tool":
				input["tool"] = "shell"
			case "other-provider":
				a.agentID, a.dialect = "claude", newACPDialect("claude")
			case "child-thread":
				session = "child"
			case "lineage":
				meta["claudeCode"] = map[string]any{"parentToolUseId": "parent-call"}
			case "no-id":
				call = ""
			case "failed":
				status = "failed"
			}
			if scenario != "no-initial" {
				a.convertToolCallUpdate(session, &sdk.SessionUpdateToolCall{ToolCallId: sdk.ToolCallId(call), Kind: "execute", Meta: meta, RawInput: input})
			}
			update := &sdk.SessionToolCallUpdate{ToolCallId: sdk.ToolCallId(call), Status: &status, RawOutput: envelope}
			if scenario == "status-only" {
				update.RawOutput = nil
			}
			if scenario == "foreign-update-input" {
				update.RawInput = map[string]any{"server": "other", "tool": "await_errands_kandev", "arguments": map[string]any{}}
			}
			event := a.convertToolCallResultUpdate(session, update)
			require.NotNil(t, event)
			require.Nil(t, event.ErrandAwaitReceipt, "unsupported evidence cannot mint a receipt")
		})
	}
}

func TestCodexACPAwaitReceiptDiagnosesMissingProvenance(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	a := newTestAdapter()
	a.logger = log
	a.agentID, a.sessionID = codexAgentID, "parent"
	a.normalizer, a.dialect = NewNormalizer(codexAgentID), newACPDialect(codexAgentID)
	input := map[string]any{"server": "kandev", "tool": "await_errands_kandev", "arguments": map[string]any{}}
	status := sdk.ToolCallStatus("completed")
	event := a.convertToolCallResultUpdate("parent", &sdk.SessionToolCallUpdate{ToolCallId: "native-call", Status: &status, RawInput: input, RawOutput: map[string]any{"error": nil, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": awaitReceiptBody}}}}})
	require.Nil(t, event.ErrandAwaitReceipt)
	require.Equal(t, 1, logs.FilterMessage("native await result has no usable receipt").Len(), "a result without initial implementation provenance must be diagnosed")
}
