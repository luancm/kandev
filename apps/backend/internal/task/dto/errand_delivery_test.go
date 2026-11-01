package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandDeliveryStateDTOContract(t *testing.T) {
	for _, state := range []models.ErrandDeliveryState{"", models.ErrandDeliveryReceived, models.ErrandDeliveryUncertain, "offered", "claimed", "invalid"} {
		t.Run(string(state), func(t *testing.T) {
			row := &models.Errand{ID: "child", DeliveryState: state, DeliveryToken: "private-token", ResultRevision: "private-revision", ResultState: models.ErrandResultDelivered, DeliveredRoute: models.ErrandDeliveryAwait}
			projection := FromErrand(row, models.ErrandStatusDone)
			for _, value := range []any{projection, ErrandUpdatedEvent{ErrandDTO: projection, WorkspaceID: "workspace"}} {
				body, err := json.Marshal(value)
				require.NoError(t, err)
				var fields map[string]any
				require.NoError(t, json.Unmarshal(body, &fields))
				if state != models.ErrandDeliveryReceived && state != models.ErrandDeliveryUncertain {
					require.NotContains(t, fields, "delivery_state", "omission must remain unavailable/in progress")
				} else {
					require.Equal(t, string(state), fields["delivery_state"])
				}
				require.Equal(t, "done", fields["status"])
				require.NotContains(t, string(body), "private-token")
				require.NotContains(t, string(body), "private-revision")
			}
		})
	}
}

func TestErrandDeliveryDTOOmitsProjectionWithoutCurrentAwaitClaim(t *testing.T) {
	for _, state := range []models.ErrandResultState{models.ErrandResultNone, models.ErrandResultPending, models.ErrandResultSuperseded, models.ErrandResultDiscarded} {
		row := &models.Errand{ResultState: state, DeliveredRoute: models.ErrandDeliveryAwait, DeliveryState: models.ErrandDeliveryUncertain}
		require.NotContains(t, encodeToMap(t, FromErrand(row, models.ErrandStatusDone)), "delivery_state")
	}
	for _, route := range []models.ErrandDeliveryRoute{"", models.ErrandDeliveryNotification, models.ErrandDeliveryTurnContext} {
		row := &models.Errand{ResultState: models.ErrandResultDelivered, DeliveredRoute: route, DeliveryState: models.ErrandDeliveryReceived}
		require.NotContains(t, encodeToMap(t, FromErrand(row, models.ErrandStatusDone)), "delivery_state")
	}
}

func TestErrandDeliveryProjectionStaysOutOfClaimSnapshots(t *testing.T) {
	body, err := json.Marshal(&models.Errand{DeliveryState: models.ErrandDeliveryUncertain})
	require.NoError(t, err)
	require.NotContains(t, string(body), "DeliveryState", "read-derived reception must not enter immutable result snapshots")
}
