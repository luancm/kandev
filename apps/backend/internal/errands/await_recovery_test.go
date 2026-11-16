package errands

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type awaitProjectionBus struct {
	bus.EventBus
	fail   bool
	values []dto.ErrandUpdatedEvent
	check  func(dto.ErrandUpdatedEvent)
}

func (b *awaitProjectionBus) Publish(_ context.Context, subject string, event *bus.Event) error {
	if b.fail {
		return errors.New("publication unavailable")
	}
	if subject == events.ErrandUpdated {
		value := event.Data.(dto.ErrandUpdatedEvent)
		if b.check != nil {
			b.check(value)
		}
		b.values = append(b.values, value)
	}
	return nil
}

func serviceAwaitRecoveryFixture(t *testing.T, postgres bool) (*Service, *sqlite.Repository, *awaitProjectionBus, *models.AgentDeliveryEvent) {
	t.Helper()
	conn := serviceClaimDB(t, postgres)
	r, err := sqlite.NewWithDB(conn, conn, nil)
	require.NoError(t, err)
	seedServiceClaimRows(t, r)
	parent, err := r.GetTaskSession(t.Context(), testParent)
	require.NoError(t, err)
	require.NoError(t, r.UpdateTaskSessionState(t.Context(), testParent, models.TaskSessionStateRunning, ""))
	require.NoError(t, r.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "running", SessionID: testParent, TaskID: testTask, AgentExecutionID: "execution", Status: models.ExecutorRunningStatusRunning}))
	require.NoError(t, r.CreateHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{SessionID: testParent, IncarnationID: parent.QueueIncarnationID, Generation: 1, NativeSessionID: "native-parent", CreationReason: "initial"}))
	id := models.ErrandAwaitIdentity{TaskID: testTask, ParentSessionID: testParent, ParentIncarnationID: parent.QueueIncarnationID, HarnessGeneration: 1, Token: "attempt"}
	require.NoError(t, r.BeginErrandAwait(t.Context(), id))
	rows, err := r.ListErrandsByParent(t.Context(), testParent)
	require.NoError(t, err)
	inventory := models.ErrandInventory(rows)
	claimed, err := r.ClaimResults(t.Context(), testTask, testParent, models.ErrandDeliveryAwait, id.Token, inventory[:1], time.Now(), inventory)
	require.NoError(t, err)
	items := []models.ErrandAwaitDeliveryItem{{ErrandID: claimed[0].ID, ResultRevision: claimed[0].ResultRevision}}
	require.NoError(t, r.OfferErrandAwait(t.Context(), id, items))
	body, err := json.Marshal(map[string]any{"type": streams.EventTypeToolUpdate, "tool_name": streams.ErrandAwaitToolName, "tool_call_id": "native-call", "tool_status": "complete", "errand_await_receipt": models.ErrandAwaitDelivery{Version: 1, Token: id.Token, Items: items}})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{TaskID: testTask, SessionID: testParent, IncarnationID: parent.QueueIncarnationID, HarnessGeneration: 1, ExecutionID: "execution", OwnerGeneration: 1, ExpectedStreamID: "stream", StreamID: "stream", Sequence: 1, EventType: streams.EventTypeToolUpdate, Payload: body}
	h := newHarness(t)
	h.service.store, h.service.deliverer = r, nil
	h.stopper.store = r
	h.service.SetEnabled(func() bool { return false })
	publication := &awaitProjectionBus{}
	h.service.events = NewBusPublisher(publication, func(context.Context, string) (string, error) { return "claim-ws", nil }, logger.Default())
	return h.service, r, publication, event
}

// @covers AC-TASKS-ERRANDS-004.13
func TestAwaitRecoveryRetainedFlagOffAndListRepairsMissedPublication(t *testing.T) {
	exerciseAwaitRecoveryPublication(t, false)
}

func TestPostgresAwaitRecoveryRetainedFlagOffAndListRepairsMissedPublication(t *testing.T) {
	exerciseAwaitRecoveryPublication(t, true)
}

func exerciseAwaitRecoveryPublication(t *testing.T, postgres bool) {
	t.Helper()
	s, r, publication, receipt := serviceAwaitRecoveryFixture(t, postgres)
	before, err := s.List(t.Context(), testTask)
	require.NoError(t, err)
	require.Empty(t, before[0].Errand.DeliveryState, "ordinary live offered await is silent")
	publication.fail = true
	require.NoError(t, s.Reconcile(t.Context()))
	require.Empty(t, publication.values)
	reconnected, err := s.List(t.Context(), testTask)
	require.NoError(t, err)
	require.Equal(t, models.ErrandDeliveryUncertain, reconnected[0].Errand.DeliveryState)
	require.Empty(t, reconnected[1].Errand.DeliveryState, "a named but unclaimed result cannot be warned")
	require.Equal(t, before[0].Status, reconnected[0].Status)
	require.Equal(t, before[0].Errand.ResultRevision, reconnected[0].Errand.ResultRevision)
	_, err = r.ReceiveAgentDeliveryEvent(t.Context(), receipt, 1)
	require.NoError(t, err)
	refresh, ok := any(s).(interface{ RefreshAwaitDelivery(context.Context, string) })
	require.True(t, ok, "lifecycle needs a postcommit current-evidence refresh")
	refresh.RefreshAwaitDelivery(t.Context(), testParent)
	reconnected, err = s.List(t.Context(), testTask)
	require.NoError(t, err)
	require.Equal(t, models.ErrandDeliveryReceived, reconnected[0].Errand.DeliveryState)
	publication.fail = false
	publication.check = func(value dto.ErrandUpdatedEvent) {
		row, err := r.GetErrand(t.Context(), value.ErrandID)
		require.NoError(t, err)
		require.Equal(t, row.DeliveryState, value.DeliveryState, "publication follows committed current evidence")
	}
	refresh.RefreshAwaitDelivery(t.Context(), testParent)
	require.Len(t, publication.values, 1)
	require.Equal(t, models.ErrandDeliveryReceived, publication.values[0].DeliveryState)
	require.Equal(t, before[0].Status, publication.values[0].Status)
	s.publish(t.Context(), before[0].Errand)
	require.Len(t, publication.values, 2)
	require.Equal(t, models.ErrandDeliveryReceived, publication.values[1].DeliveryState, "an older mutation snapshot cannot publish older reception evidence")
}
