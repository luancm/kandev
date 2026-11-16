package acp

import (
	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

func (a *Adapter) trackAwaitReceiptCall(session, call string, initial *sdk.SessionUpdateToolCall, parent string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.awaitReceiptCalls, call)
	if a.agentID != codexAgentID || call == "" || parent != "" || session == "" || session != a.sessionID {
		return
	}
	frame, ok := parseCodexMCPToolCall(initial.Meta, initial.RawInput)
	if !ok || frame.name != streams.ErrandAwaitToolName {
		return
	}
	if a.awaitReceiptCalls == nil {
		a.awaitReceiptCalls = make(map[string]string)
	}
	a.awaitReceiptCalls[call] = session
}

func (a *Adapter) codexAwaitReceiptLocked(session, call, parent, status string, output, input any) *streams.ErrandAwaitReceipt {
	if a.agentID != codexAgentID || status != toolStatusComplete || output == nil {
		return nil
	}
	if !a.ownsAwaitReceiptCallLocked(session, call, parent) {
		if canonicalAwaitInput(input) {
			a.diagnoseCodexAwaitReceipt(call, "missing initial parent call provenance")
		}
		return nil
	}
	if input != nil && !canonicalAwaitInput(input) {
		a.diagnoseCodexAwaitReceipt(call, "conflicting call identity")
		return nil
	}
	result := successfulCodexMCPResult(output)
	receipt := streams.ParseErrandAwaitReceipt(result)
	if receipt == nil {
		a.diagnoseCodexAwaitReceipt(call, "missing or unsuccessful canonical result")
	}
	return receipt
}

func (a *Adapter) ownsAwaitReceiptCallLocked(session, call, parent string) bool {
	return call != "" && parent == "" && session != "" && session == a.sessionID && a.awaitReceiptCalls[call] == session
}

func canonicalAwaitInput(input any) bool {
	frame, ok := parseCodexMCPToolCall(map[string]any{"is_mcp_tool_call": true}, input)
	return ok && frame.name == streams.ErrandAwaitToolName
}

func (a *Adapter) diagnoseCodexAwaitReceipt(call, reason string) {
	a.logger.Warn("native await result has no usable receipt", zap.String("tool_call_id", call), zap.String("reason", reason))
}

// Inspect the original outer error before display normalization unwraps it.
func successfulCodexMCPResult(output any) any {
	envelope, ok := output.(map[string]any)
	if !ok || envelope["error"] != nil {
		return nil
	}
	return envelope["result"]
}
