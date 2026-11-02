package errands

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-004.11
func TestSendNowDeliveryRoundTrip(t *testing.T) {
	want := sendNowDeliveryFixture()
	want.QueueGeneration, want.OperationGeneration = 7, 7
	want.Sources[0].QueueGeneration, want.Sources[1].QueueGeneration = 3, 4
	want.Receipts[2].Claims[0].ParentAdmissionGeneration = 17
	want.Receipts[2].Envelope.Items = append(want.Receipts[2].Envelope.Items,
		DeliveryItem{ErrandID: "still-running", Title: "Running sibling", Status: models.ErrandStatusRunning})
	metadata := map[string]interface{}{SendNowDeliveryMetadataKey: want, DeliveryTokenMetadataKey: "token-A"}
	wire, err := json.Marshal(metadata)
	require.NoError(t, err)
	var persisted map[string]interface{}
	require.NoError(t, json.Unmarshal(wire, &persisted))
	for name, input := range map[string]map[string]interface{}{"typed": metadata, "reloaded": persisted} {
		t.Run(name, func(t *testing.T) {
			got, present, err := DecodeSendNowDelivery(input)
			require.NoError(t, err)
			require.True(t, present)
			require.Equal(t, want, got)
			for _, token := range []string{"token-A", "token-B", "token-live"} {
				require.True(t, got.HasToken(token), token)
			}
			for _, token := range []string{"", "unrelated", "source-A", "lookalike-token"} {
				require.False(t, got.HasToken(token), token)
			}
		})
	}
}

func TestSendNowDeliveryLegacyAndUserContentAreNotStructured(t *testing.T) {
	for _, metadata := range []map[string]interface{}{
		nil,
		{DeliveryTokenMetadataKey: "legacy-token"},
		{"content": `{"errand_send_now":{"receipts":[{"envelope":{"token":"lookalike-token"}}]}}`},
	} {
		got, present, err := DecodeSendNowDelivery(metadata)
		require.NoError(t, err)
		require.False(t, present)
		require.Equal(t, SendNowDelivery{}, got)
	}
}

func TestSendNowDeliveryRejectsIncompleteOrContradictoryIdentity(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{"unsupported version", func(m map[string]interface{}) { m["version"] = 2 }},
		{"missing incarnation", func(m map[string]interface{}) { delete(m, "session_incarnation_id") }},
		{"missing queue generation", func(m map[string]interface{}) { delete(m, "queue_generation") }},
		{"missing operation generation", func(m map[string]interface{}) { delete(m, "operation_generation") }},
		{"negative generation", func(m map[string]interface{}) { m["queue_generation"] = -1 }},
		{"different operation generation", func(m map[string]interface{}) { m["operation_generation"] = 9 }},
		{"fractional generation", func(m map[string]interface{}) { m["queue_generation"] = 0.5 }},
		{"missing source generation", func(m map[string]interface{}) { delete(sendNowFixtureSource(m, 1), "queue_generation") }},
		{"different source set", func(m map[string]interface{}) { m["source_ids"] = []string{"user", "source-A"} }},
		{"different source order", func(m map[string]interface{}) { m["source_ids"] = []string{"source-A", "user", "source-B", "signal"} }},
		{"duplicate source", func(m map[string]interface{}) {
			m["source_ids"] = []string{"user", "source-A", "source-A", "signal"}
			sendNowFixtureSource(m, 2)["id"] = "source-A"
		}},
		{"user marker is not provenance", func(m map[string]interface{}) { sendNowFixtureSource(m, 1)["queued_by"] = "user" }},
		{"unknown provenance", func(m map[string]interface{}) { sendNowFixtureSource(m, 1)["provenance"] = "user-claimed" }},
		{"mixed prepared and ordinary content", func(m map[string]interface{}) { sendNowFixtureSource(m, 1)["content"] = "ordinary" }},
		{"missing full prepared block", func(m map[string]interface{}) { delete(sendNowFixtureSource(m, 1), "prepared_block") }},
		{"unrepresented prepared block", func(m map[string]interface{}) { sendNowFixtureSource(m, 1)["prepared_block"] = "absent block" }},
		{"missing effective prompt", func(m map[string]interface{}) { delete(m, "effective_prompt") }},
		{"missing prepared receipt", func(m map[string]interface{}) { m["receipts"] = m["receipts"].([]interface{})[1:] }},
		{"unknown source receipt", func(m map[string]interface{}) { sendNowFixtureReceipt(m, 0)["source_id"] = "foreign" }},
		{"ordinary source receipt", func(m map[string]interface{}) { sendNowFixtureReceipt(m, 0)["source_id"] = "user" }},
		{"missing claim revision", func(m map[string]interface{}) { delete(sendNowFixtureClaim(m), "ResultRevision") }},
		{"missing admission generation", func(m map[string]interface{}) { delete(sendNowFixtureClaim(m), "ParentAdmissionGeneration") }},
		{"negative admission generation", func(m map[string]interface{}) { sendNowFixtureClaim(m)["ParentAdmissionGeneration"] = -1 }},
		{"different claim token", func(m map[string]interface{}) { sendNowFixtureClaim(m)["Token"] = "unrelated" }},
		{"different claim route", func(m map[string]interface{}) { sendNowFixtureClaim(m)["Route"] = "turn_context" }},
		{"missing claim item", func(m map[string]interface{}) { sendNowFixtureClaim(m)["ErrandID"] = "unrelated" }},
		{"missing snapshot items", func(m map[string]interface{}) {
			delete(sendNowFixtureReceipt(m, 0)["envelope"].(map[string]interface{}), "items")
		}},
		{"duplicate snapshot items", func(m map[string]interface{}) {
			envelope := sendNowFixtureReceipt(m, 0)["envelope"].(map[string]interface{})
			items := envelope["items"].([]interface{})
			envelope["items"] = append(items, items[0])
		}},
		{"nonterminal result claim", func(m map[string]interface{}) {
			envelope := sendNowFixtureReceipt(m, 0)["envelope"].(map[string]interface{})
			envelope["items"].([]interface{})[0].(map[string]interface{})["status"] = "running"
		}},
		{"same result under different token", func(m map[string]interface{}) {
			claim := sendNowFixtureReceipt(m, 1)["claims"].([]interface{})[0].(map[string]interface{})
			claim["ErrandID"], claim["ResultRevision"] = "deleted-A", "revision-A"
			envelope := sendNowFixtureReceipt(m, 1)["envelope"].(map[string]interface{})
			envelope["items"].([]interface{})[0].(map[string]interface{})["errand_id"] = "deleted-A"
		}},
		{"missing claims", func(m map[string]interface{}) { delete(sendNowFixtureReceipt(m, 0), "claims") }},
		{"unsupported route", func(m map[string]interface{}) {
			sendNowFixtureReceipt(m, 0)["envelope"].(map[string]interface{})["route"] = "await"
		}},
		{"duplicate receipt", func(m map[string]interface{}) {
			m["receipts"] = append(m["receipts"].([]interface{}), sendNowFixtureReceipt(m, 0))
		}},
		{"unknown field", func(m map[string]interface{}) { m["fabricated_provenance"] = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := json.Marshal(sendNowDeliveryFixture())
			require.NoError(t, err)
			var value map[string]interface{}
			require.NoError(t, json.Unmarshal(wire, &value))
			tc.mutate(value)
			got, present, err := DecodeSendNowDelivery(map[string]interface{}{SendNowDeliveryMetadataKey: value})
			require.ErrorIs(t, err, ErrInvalidSendNowDelivery)
			require.True(t, present)
			require.Equal(t, SendNowDelivery{}, got, "invalid input exposes no partial receipts")
		})
	}
}

func TestSendNowDeliveryRejectsMalformedWithoutLegacyDowngrade(t *testing.T) {
	for _, value := range []interface{}{nil, true, "user content", []string{"token-A"}, json.RawMessage(`{"version":`), make(chan int)} {
		got, present, err := DecodeSendNowDelivery(map[string]interface{}{
			SendNowDeliveryMetadataKey: value, DeliveryTokenMetadataKey: "token-A",
		})
		require.ErrorIs(t, err, ErrInvalidSendNowDelivery)
		require.True(t, present)
		require.Equal(t, SendNowDelivery{}, got)
	}
	_, present, err := DecodeSendNowDelivery(map[string]interface{}{
		SendNowDeliveryMetadataKey: sendNowDeliveryFixture(), DeliveryTokenMetadataKey: "unrelated",
	})
	require.True(t, present)
	require.ErrorIs(t, err, ErrInvalidSendNowDelivery)
}

func TestSendNowDeliveryRequiresExplicitPromptButAllowsEmptyOrdinaryInput(t *testing.T) {
	ordinary := sendNowDeliveryFixture()
	ordinary.SourceIDs, ordinary.Sources = ordinary.SourceIDs[:1], ordinary.Sources[:1]
	ordinary.Sources[0].Content, ordinary.Receipts = "", nil
	for _, prompt := range []string{"", " \n "} {
		ordinary.EffectivePrompt = prompt
		got, present, err := DecodeSendNowDelivery(map[string]interface{}{SendNowDeliveryMetadataKey: ordinary})
		require.NoError(t, err)
		require.True(t, present)
		require.Equal(t, prompt, got.EffectivePrompt)
	}
	for _, tc := range []struct {
		name  string
		value interface{}
	}{
		{"omitted", nil}, {"null", nil}, {"number", 1}, {"bool", true},
		{"object", map[string]interface{}{}}, {"array", []string{"prompt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(ordinary)
			require.NoError(t, err)
			var fields map[string]interface{}
			require.NoError(t, json.Unmarshal(encoded, &fields))
			fields["effective_prompt"] = tc.value
			if tc.name == "omitted" {
				delete(fields, "effective_prompt")
			}
			got, present, err := DecodeSendNowDelivery(map[string]interface{}{SendNowDeliveryMetadataKey: fields})
			require.ErrorIs(t, err, ErrInvalidSendNowDelivery)
			require.True(t, present)
			require.Equal(t, SendNowDelivery{}, got)
		})
	}
	prepared := sendNowDeliveryFixture()
	prepared.EffectivePrompt = ""
	_, _, err := DecodeSendNowDelivery(map[string]interface{}{SendNowDeliveryMetadataKey: prepared})
	// An explicit empty prompt cannot omit a prepared block.
	require.ErrorIs(t, err, ErrInvalidSendNowDelivery)
}

func sendNowDeliveryFixture() SendNowDelivery {
	return SendNowDelivery{
		Version: 1, TaskID: "task", SessionID: "parent", SessionIncarnationID: "incarnation",
		OperationGeneration: 0, QueueGeneration: 0,
		SourceIDs: []string{"user", "source-A", "source-B", "signal"},
		Sources: []SendNowDeliverySource{
			{ID: "user", Kind: SendNowSourceOrdinary, QueuedBy: "user", Content: "  ordinary lookalike-token\n "},
			{ID: "source-A", Kind: SendNowSourcePrepared, QueuedBy: "server", Provenance: SendNowErrandProvenance, PreparedBlock: "full result A"},
			{ID: "source-B", Kind: SendNowSourcePrepared, QueuedBy: "server", Provenance: SendNowErrandProvenance, PreparedBlock: "full result B"},
			{ID: "signal", Kind: SendNowSourceNotification, QueuedBy: "server", Provenance: SendNowErrandProvenance},
		},
		Receipts: []SendNowDeliveryReceipt{
			sendNowDeliveryReceipt("source-A", "token-A", "deleted-A", "revision-A", models.ErrandDeliveryNotification),
			sendNowDeliveryReceipt("source-B", "token-B", "deleted-B", "revision-B", models.ErrandDeliveryNotification),
			sendNowDeliveryReceipt("", "token-live", "live-result", "revision-live", models.ErrandDeliveryTurnContext),
		},
		EffectivePrompt: "  ordinary lookalike-token\n \n\nfull result A\n\nfull result B\n\nnewly composed result",
	}
}

func sendNowDeliveryReceipt(source, token, id, revision string, route models.ErrandDeliveryRoute) SendNowDeliveryReceipt {
	return SendNowDeliveryReceipt{
		SourceID: source,
		Envelope: DeliveryEnvelope{Token: token, Route: route,
			Items: []DeliveryItem{{ErrandID: id, Title: id, Status: models.ErrandStatusCancelled, Preview: "preview"}}},
		Claims: []models.ErrandResultClaim{{ErrandID: id, ResultRevision: revision, ParentAdmissionGeneration: 0, Token: token, Route: route}},
	}
}

func sendNowFixtureSource(m map[string]interface{}, index int) map[string]interface{} {
	return m["sources"].([]interface{})[index].(map[string]interface{})
}

func sendNowFixtureReceipt(m map[string]interface{}, index int) map[string]interface{} {
	return m["receipts"].([]interface{})[index].(map[string]interface{})
}

func sendNowFixtureClaim(m map[string]interface{}) map[string]interface{} {
	return sendNowFixtureReceipt(m, 0)["claims"].([]interface{})[0].(map[string]interface{})
}

func TestSendNowDeliveryEmptyPromptNeedsOrdinarySource(t *testing.T) {
	for _, live := range []bool{false, true} {
		snapshot := sendNowDeliveryFixture()
		snapshot.Sources = snapshot.Sources[3:]
		snapshot.SourceIDs = snapshot.SourceIDs[3:]
		snapshot.Receipts = nil
		if live {
			snapshot.Receipts = sendNowDeliveryFixture().Receipts[2:]
		}
		snapshot.EffectivePrompt = ""
		_, present, err := DecodeSendNowDelivery(map[string]interface{}{SendNowDeliveryMetadataKey: snapshot})
		require.True(t, present)
		require.ErrorIs(t, err, ErrInvalidSendNowDelivery, "an empty server-only dispatch cannot represent a delivered result")
	}
}
