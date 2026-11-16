package process

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestAwaitReceiptSurvivesToolDisplayBoundsAndJournalReplay(t *testing.T) {
	manager := newAllToolManager(t, 32<<10)
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
	payload := streams.NewMCPTool(streams.ErrandAwaitToolName, map[string]any{})
	payload.Generic().Output = native
	observation := streams.ParseErrandAwaitReceipt(native)
	require.NotNil(t, observation)
	update := streams.AgentEvent{Type: streams.EventTypeToolUpdate, SessionID: "native-parent", ToolCallID: "native-call", ToolName: streams.ErrandAwaitToolName, ToolStatus: "complete", NormalizedPayload: payload, ErrandAwaitReceipt: observation}
	committed, err := manager.persistDeliveryEvent(update)
	require.NoError(t, err)
	require.EqualValues(t, 1, committed.DeliverySequence)
	observation.Items[0].ResultRevision = "source-mutated"
	require.Equal(t, "revision", committed.ErrandAwaitReceipt.Items[0].ResultRevision)
	require.NoError(t, manager.closeDeliveryJournal())
	reopened, err := journal.Open(journal.Config{Path: manager.cfg.DurableJournalPath, MaxEventBytes: 32 << 10})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	events, _, err := reopened.Replay(t.Context(), "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Less(t, len(events[0].Payload), 32<<10)
	var replay streams.AgentEvent
	require.NoError(t, json.Unmarshal(events[0].Payload, &replay))
	require.Equal(t, committed.ErrandAwaitReceipt, replay.ErrandAwaitReceipt)
	require.Equal(t, true, replay.NormalizedPayload.Generic().Output.(map[string]any)["truncated"])
	after, err := json.Marshal(native)
	require.NoError(t, err)
	require.Equal(t, original, after, "display and journal preparation cannot mutate native body")
}

func TestAwaitReceiptJournalAppendFailureCannotClaimDurableProof(t *testing.T) {
	manager := newAllToolManager(t, 32<<10)
	require.NoError(t, manager.deliveryJournal.Close())
	native := map[string]any{"content": []any{map[string]any{"type": "text", "text": `{"await_delivery":{"version":1,"token":"attempt","items":[]}}`}}}
	event := streams.AgentEvent{Type: streams.EventTypeToolUpdate, ToolName: streams.ErrandAwaitToolName, ToolCallID: "native-call", ToolStatus: "complete", ErrandAwaitReceipt: streams.ParseErrandAwaitReceipt(native)}
	persisted, err := manager.persistDeliveryEvent(event)
	require.Error(t, err)
	require.Zero(t, persisted.DeliverySequence, "uncommitted observation cannot become durable evidence")
	require.NoError(t, manager.closeDeliveryJournal())
	reopened, err := journal.Open(journal.Config{Path: manager.cfg.DurableJournalPath})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	events, _, err := reopened.Replay(t.Context(), "stream", 0, 10)
	require.ErrorIs(t, err, journal.ErrStreamNotFound)
	require.Empty(t, events)
}
