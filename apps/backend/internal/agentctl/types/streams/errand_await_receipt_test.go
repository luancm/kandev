package streams

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAwaitReceiptCanonicalBodyValidation(t *testing.T) {
	valid := `{"await_delivery":{"version":1,"token":"attempt","items":[{"errand_id":"child","result_revision":"rev"}]}}`
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", valid, true},
		{"zero", `{"await_delivery":{"version":1,"token":"zero","items":[]}}`, true},
		{"nested", `{"output":` + valid + `}`, false},
		{"version", strings.Replace(valid, `"version":1`, `"version":2`, 1), false},
		{"null-items", `{"await_delivery":{"version":1,"token":"attempt","items":null}}`, false},
		{"missing-revision", strings.Replace(valid, `,"result_revision":"rev"`, "", 1), false},
		{"duplicate", `{"await_delivery":{"version":1,"token":"attempt","items":[{"errand_id":"child","result_revision":"r1"},{"errand_id":"child","result_revision":"r2"}]}}`, false},
		{"duplicate-field", strings.Replace(valid, `"token":"attempt"`, `"token":"attempt","token":"other"`, 1), false},
		{"partial", valid[:len(valid)-1], false},
		{"trailing", valid + ` {}`, false},
		{"oversized-token", strings.Replace(valid, "attempt", strings.Repeat("a", 129), 1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := map[string]any{"content": []any{map[string]any{"type": "text", "text": test.body}}}
			receipt := ParseErrandAwaitReceipt(result)
			require.Equal(t, test.valid, receipt != nil)
		})
	}
}

func TestAwaitReceiptDisplayBoundsSnapshotAndJSON(t *testing.T) {
	rows := make([]any, 0, 5)
	items := make([]any, 0, 5)
	for _, id := range []string{"child", "second", "third", "fourth", "fifth"} {
		revision := "revision"
		if id != "child" {
			revision += "-" + id
		}
		rows = append(rows, map[string]any{"errand_id": id, "status": "done", "result": strings.Repeat("r", 16*1024)})
		items = append(items, map[string]any{"errand_id": id, "result_revision": revision})
	}
	body, err := json.Marshal(map[string]any{"duration_ms": 7, "end_reason": "all_ready", "errands": rows, "await_delivery": map[string]any{"version": 1, "token": "attempt", "items": items}})
	require.NoError(t, err)
	native := map[string]any{"content": []any{map[string]any{"type": "text", "text": string(body)}}}
	original, err := json.Marshal(native)
	require.NoError(t, err)
	receipt := ParseErrandAwaitReceipt(native)
	require.NotNil(t, receipt)
	payload := NewMCPTool(ErrandAwaitToolName, map[string]any{})
	payload.Generic().Output = native
	event := AgentEvent{Type: EventTypeToolUpdate, ToolCallID: "native-call", ToolStatus: "complete", ToolName: ErrandAwaitToolName, NormalizedPayload: payload, ErrandAwaitReceipt: receipt}
	prepared := PrepareToolEventWithLimit(event, 16<<10)
	require.NotSame(t, receipt, prepared.ErrandAwaitReceipt)
	receipt.Items[0].ResultRevision = "mutated-source"
	require.Equal(t, "revision", prepared.ErrandAwaitReceipt.Items[0].ResultRevision)
	encoded, err := json.Marshal(prepared)
	require.NoError(t, err)
	require.Less(t, len(encoded), 16<<10)
	var restored AgentEvent
	require.NoError(t, json.Unmarshal(encoded, &restored))
	require.Equal(t, prepared.ErrandAwaitReceipt, restored.ErrandAwaitReceipt)
	require.Equal(t, true, restored.NormalizedPayload.Generic().Output.(map[string]any)["truncated"])
	after, err := json.Marshal(native)
	require.NoError(t, err)
	require.Equal(t, original, after, "display reduction must never mutate actual native result")
}
