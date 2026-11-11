package errands

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDeliveryDurationCompositionUsesRecordedSettlement(t *testing.T) {
	for _, route := range []models.ErrandDeliveryRoute{models.ErrandDeliveryTurnContext, models.ErrandDeliveryNotification} {
		t.Run(string(route), func(t *testing.T) {
			h, errandID := pendingResultHarness(t, "the answer")
			createdAt := time.Date(2026, time.October, 26, 8, 0, 0, 0, time.UTC)
			settledAt := createdAt.Add(2*time.Minute + 14*time.Second)
			setDeliverySnapshotTimes(t, h, errandID, createdAt, &settledAt, settledAt.Add(3*time.Hour))

			composition, err := h.service.ComposeParentTurn(t.Context(), testParent, route)
			require.NoError(t, err)
			require.Len(t, composition.Items, 1)
			requireDeliveryDuration(t, composition.Items[0], 134000)

			laterSettlement := settledAt.Add(time.Hour)
			setDeliverySnapshotTimes(t, h, errandID, createdAt, &laterSettlement, laterSettlement)
			requireDeliveryDuration(t, composition.Items[0], 134000)
		})
	}
}

func TestDeliveryDurationPreservesZeroAndOmitsUnknownOrInvalidTimestamps(t *testing.T) {
	createdAt := time.Date(2026, time.October, 26, 8, 0, 0, 0, time.UTC)
	zeroTime := time.Time{}
	cases := []struct {
		name        string
		createdAt   time.Time
		settledAt   *time.Time
		wantPresent bool
		want        float64
	}{
		{name: "zero interval", createdAt: createdAt, settledAt: &createdAt, wantPresent: true, want: 0},
		{name: "missing creation", createdAt: time.Time{}, settledAt: &createdAt},
		{name: "missing settlement", createdAt: createdAt},
		{name: "zero settlement timestamp", createdAt: createdAt, settledAt: &zeroTime},
		{name: "reversed interval", createdAt: createdAt, settledAt: deliveryTimeRef(createdAt.Add(-time.Second))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, errandID := pendingResultHarness(t, "the answer")
			setDeliverySnapshotTimes(t, h, errandID, tc.createdAt, tc.settledAt, createdAt.Add(4*time.Hour))
			composition, err := h.service.ComposeParentTurn(t.Context(), testParent, models.ErrandDeliveryTurnContext)
			require.NoError(t, err)
			require.Len(t, composition.Items, 1)
			got, present := deliveryItemDuration(t, composition.Items[0])
			require.Equal(t, tc.wantPresent, present)
			if present {
				require.Equal(t, tc.want, got)
			}
		})
	}
}

func TestDeliveryDurationPreservedCancellationNoticeSnapshot(t *testing.T) {
	h := newHarness(t)
	errands := h.mustStart("audit").Errand
	created, err := h.store.GetErrand(t.Context(), errands.ID)
	require.NoError(t, err)
	require.NoError(t, h.service.PreserveCancellationNotice(t.Context(), created.ID))

	settled, err := h.store.GetErrand(t.Context(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, settled.SettledAt)
	require.Len(t, h.deliverer.preparedMetadata, 1)
	metadata := h.deliverer.preparedMetadata[0]
	envelope, ok := metadata[DeliveryEnvelopeMetadataKey].(DeliveryEnvelope)
	require.True(t, ok)
	require.Len(t, envelope.Items, 1)
	want := float64(settled.SettledAt.Sub(settled.CreatedAt).Milliseconds())
	requireDeliveryDuration(t, envelope.Items[0], want)

	encoded, err := json.Marshal(metadata)
	require.NoError(t, err)
	var persisted map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &persisted))
	reloaded := persisted[DeliveryEnvelopeMetadataKey].(map[string]interface{})
	items := reloaded["items"].([]interface{})
	require.Equal(t, want, items[0].(map[string]interface{})["duration_ms"])
}

func TestDeliveryDurationSendNowCodecPreservesMeasuredZeroAndAbsentPreparedItems(t *testing.T) {
	metadata := map[string]interface{}{SendNowDeliveryMetadataKey: sendNowDeliveryFixture()}
	wire, err := json.Marshal(metadata)
	require.NoError(t, err)
	var persisted map[string]interface{}
	require.NoError(t, json.Unmarshal(wire, &persisted))
	snapshot := persisted[SendNowDeliveryMetadataKey].(map[string]interface{})
	receipts := snapshot["receipts"].([]interface{})
	items := make([]map[string]interface{}, len(receipts))
	for i, value := range receipts {
		envelope := value.(map[string]interface{})["envelope"].(map[string]interface{})
		items[i] = envelope["items"].([]interface{})[0].(map[string]interface{})
	}
	items[0]["duration_ms"] = float64(134000)
	items[1]["duration_ms"] = float64(0)
	delete(items[2], "duration_ms")

	decoded, present, err := DecodeSendNowDelivery(persisted)
	require.NoError(t, err)
	require.True(t, present)
	roundTrip, err := json.Marshal(decoded)
	require.NoError(t, err)
	var reloaded map[string]interface{}
	require.NoError(t, json.Unmarshal(roundTrip, &reloaded))
	reloadedReceipts := reloaded["receipts"].([]interface{})
	for i, wantPresent := range []bool{true, true, false} {
		envelope := reloadedReceipts[i].(map[string]interface{})["envelope"].(map[string]interface{})
		reloadedItem := envelope["items"].([]interface{})[0].(map[string]interface{})
		got, gotPresent := reloadedItem["duration_ms"]
		require.Equal(t, wantPresent, gotPresent)
		if wantPresent {
			want := float64(134000)
			if i == 1 {
				want = 0
			}
			require.Equal(t, want, got)
		}
	}
	require.Equal(t, sendNowDeliveryFixture().SourceIDs, decoded.SourceIDs, "codec retains replay identity")
}

func setDeliverySnapshotTimes(t *testing.T, h *harness, errandID string, createdAt time.Time, settledAt *time.Time, updatedAt time.Time) {
	t.Helper()
	_, changed, err := h.service.update(t.Context(), errandID, func(row *models.Errand) bool {
		row.CreatedAt, row.SettledAt, row.UpdatedAt = createdAt, settledAt, updatedAt
		return true
	})
	require.NoError(t, err)
	require.True(t, changed)
}

func deliveryItemDuration(t *testing.T, item DeliveryItem) (float64, bool) {
	t.Helper()
	encoded, err := json.Marshal(item)
	require.NoError(t, err)
	var fields map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &fields))
	value, present := fields["duration_ms"]
	if !present {
		return 0, false
	}
	duration, ok := value.(float64)
	require.True(t, ok, "duration_ms must encode as a JSON number")
	return duration, true
}

func requireDeliveryDuration(t *testing.T, item DeliveryItem, want float64) {
	t.Helper()
	got, present := deliveryItemDuration(t, item)
	require.True(t, present)
	require.Equal(t, want, got)
}

func deliveryTimeRef(value time.Time) *time.Time { return &value }
