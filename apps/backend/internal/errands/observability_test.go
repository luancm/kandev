package errands

import (
	"context"
	"errors"
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestStartLogsOutcomeAndCommittedAdmission(t *testing.T) {
	h := newHarness(t)
	logs := observeErrandLogs(t, h)
	err := h.mustStart("audit").Errand

	started := logs.FilterMessage("errand.started").All()
	if len(started) != 1 {
		t.Fatalf("errand.started logs = %d, want one", len(started))
	}
	assertErrandLogFields(t, started[0], map[string]string{
		"task_id": testTask, "session_id": err.ID, "errand_id": err.ID,
		"parent_session_id": testParent, "outcome": string(StartOutcomeStarted),
	})

	admitted := logs.FilterMessage("errand.admitted").All()
	if len(admitted) != 1 {
		t.Fatalf("errand.admitted logs = %d, want one", len(admitted))
	}
	assertErrandLogFields(t, admitted[0], map[string]string{
		"task_id": testTask, "session_id": err.ID, "errand_id": err.ID, "parent_session_id": testParent,
	})
}

func TestRejectedStartLogsOutcomeWithoutAdmission(t *testing.T) {
	h := newHarness(t)
	logs := observeErrandLogs(t, h)
	_, err := h.service.Start(context.Background(), StartRequest{
		TaskID: testTask, ParentSessionID: testParent, Title: "audit", Objective: " ",
	})
	if err == nil {
		t.Fatal("Start accepted a blank objective")
	}

	started := logs.FilterMessage("errand.started").All()
	if len(started) != 1 {
		t.Fatalf("errand.started logs = %d, want one", len(started))
	}
	assertErrandLogFields(t, started[0], map[string]string{
		"task_id": testTask, "parent_session_id": testParent, "outcome": string(StartOutcomeRejected),
	})
	if got := logs.FilterMessage("errand.admitted").Len(); got != 0 {
		t.Fatalf("errand.admitted logs = %d after a rejected start, want none", got)
	}
}

func TestFailedErrandCreateLogsFailureWithoutAdmission(t *testing.T) {
	h := newHarness(t)
	logs := observeErrandLogs(t, h)
	h.service.store = createErrandFailureStore{Store: h.store, err: errors.New("store unavailable")}

	_, err := h.service.Start(context.Background(), StartRequest{
		TaskID: testTask, ParentSessionID: testParent, Title: "audit", Objective: "review the API",
	})
	if err == nil {
		t.Fatal("Start succeeded when the errand row could not be persisted")
	}

	started := logs.FilterMessage("errand.started").All()
	if len(started) != 1 {
		t.Fatalf("errand.started logs = %d, want one", len(started))
	}
	assertErrandLogFields(t, started[0], map[string]string{
		"task_id": testTask, "parent_session_id": testParent, "outcome": string(StartOutcomeFailed),
	})
	if got := logs.FilterMessage("errand.admitted").Len(); got != 0 {
		t.Fatalf("errand.admitted logs = %d after a failed write, want none", got)
	}
}

func TestSettlementAndCancellationLogsCarryIdentifiers(t *testing.T) {
	h := newHarness(t)
	settled := h.mustStart("settle").Errand
	cancelled := h.mustStart("cancel").Errand
	logs := observeErrandLogs(t, h)

	if err := h.service.OnErrandTurnSettled(context.Background(), TurnSettlement{
		SessionID: settled.ID, TurnID: settled.FirstTurnID, Outcome: models.ErrandOutcomeDone,
	}); err != nil {
		t.Fatalf("OnErrandTurnSettled: %v", err)
	}
	settlement := logs.FilterMessage("errand.settled").All()
	if len(settlement) != 1 {
		t.Fatalf("errand.settled logs = %d, want one", len(settlement))
	}
	assertErrandLogFields(t, settlement[0], map[string]string{
		"task_id": testTask, "session_id": settled.ID, "errand_id": settled.ID,
		"parent_session_id": testParent, "status": string(models.ErrandStatusDone),
	})

	if _, err := h.service.Cancel(context.Background(), CancelRequest{
		ErrandID: cancelled.ID, TaskID: testTask, Source: models.ErrandCancelSourceUser,
	}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	cancellation := logs.FilterMessage("errand.cancelled").All()
	if len(cancellation) != 1 {
		t.Fatalf("errand.cancelled logs = %d, want one", len(cancellation))
	}
	assertErrandLogFields(t, cancellation[0], map[string]string{
		"task_id": testTask, "session_id": cancelled.ID, "errand_id": cancelled.ID,
		"parent_session_id": testParent, "source": string(models.ErrandCancelSourceUser),
	})
}

func TestReconcileLogsOnlyCommittedTransitions(t *testing.T) {
	h := newHarness(t)
	err := h.mustStart("audit").Errand
	logs := observeErrandLogs(t, h)
	h.store.setSessionState(err.ID, models.TaskSessionStateFailed)
	h.service.store = updateErrandFailureStore{Store: h.store, err: errors.New("write failed")}

	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile with a failed errand update: %v", err)
	}
	if got := logs.FilterMessage("errand.reconciled").Len(); got != 0 {
		t.Fatalf("errand.reconciled logs = %d after a failed write, want none", got)
	}
	row, errRead := h.store.GetErrand(context.Background(), err.ID)
	if errRead != nil {
		t.Fatalf("GetErrand after failed reconciliation: %v", errRead)
	}
	if row.ResultState != models.ErrandResultNone || !row.AwaitingResult {
		t.Fatalf("failed reconciliation changed result to %q, awaiting=%v", row.ResultState, row.AwaitingResult)
	}

	h.service.store = h.store
	if err := h.service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile after restoring the store: %v", err)
	}
	reconciled := logs.FilterMessage("errand.reconciled").All()
	if len(reconciled) != 1 {
		t.Fatalf("errand.reconciled logs = %d after a committed write, want one", len(reconciled))
	}
	assertErrandLogFields(t, reconciled[0], map[string]string{
		"task_id": testTask, "session_id": err.ID, "errand_id": err.ID,
		"parent_session_id": testParent, "action": "failed_launched_row",
	})
}

func TestCancellationNoticeLogsAndCountsOnlyAfterAdmission(t *testing.T) {
	h := newHarness(t)
	err := h.mustStart("audit").Errand
	logs := observeErrandLogs(t, h)
	countBefore := errandMetricValue("errands_result_delivery_total", "route=notification")
	h.deliverer.admitErr = errors.New("parent queue is full")

	if err := h.service.PreserveCancellationNotice(context.Background(), err.ID); err == nil {
		t.Fatal("cancellation notice was admitted to a full queue")
	}
	if got := logs.FilterMessage("errand.delivered").Len(); got != 0 {
		t.Fatalf("errand.delivered logs = %d after a refused admission, want none", got)
	}
	if got := errandMetricValue("errands_result_delivery_total", "route=notification"); got != countBefore {
		t.Fatalf("notification delivery count = %d after a refusal, want %d", got, countBefore)
	}

	h.deliverer.admitErr = nil
	if err := h.service.PreserveCancellationNotice(context.Background(), err.ID); err != nil {
		t.Fatalf("retry cancellation notice admission: %v", err)
	}
	delivered := logs.FilterMessage("errand.delivered").All()
	if len(delivered) != 1 {
		t.Fatalf("errand.delivered logs = %d after accepted admission, want one", len(delivered))
	}
	assertErrandLogFields(t, delivered[0], map[string]string{
		"task_id": testTask, "session_id": err.ID, "errand_id": err.ID,
		"parent_session_id": testParent, "route": string(models.ErrandDeliveryNotification),
	})
	if got := errandMetricValue("errands_result_delivery_total", "route=notification"); got != countBefore+1 {
		t.Fatalf("notification delivery count = %d after acceptance, want %d", got, countBefore+1)
	}
}

func observeErrandLogs(t *testing.T, h *harness) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	zapLogger := zap.New(core)
	wrapped, err := logger.NewFromZap(zapLogger)
	if err != nil {
		t.Fatalf("wrap observed logger: %v", err)
	}
	h.service.logger = wrapped.WithFields(zap.String("component", "errands"))
	t.Cleanup(func() { _ = zapLogger.Sync() })
	return logs
}

func assertErrandLogFields(t *testing.T, entry observer.LoggedEntry, want map[string]string) {
	t.Helper()
	fields := entry.ContextMap()
	for key, value := range want {
		if got := fields[key]; got != value {
			t.Errorf("%s field %q = %v, want %q", entry.Message, key, got, value)
		}
	}
}

func errandMetricValue(name, label string) int64 {
	counter, ok := expvar.Get(name).(*expvar.Map)
	if !ok {
		return 0
	}
	value, ok := counter.Get(label).(*expvar.Int)
	if !ok {
		return 0
	}
	return value.Value()
}

type createErrandFailureStore struct {
	Store
	err error
}

func (s createErrandFailureStore) CreateErrand(context.Context, *models.Errand) error {
	return s.err
}

type updateErrandFailureStore struct {
	Store
	err error
}

func (s updateErrandFailureStore) UpdateErrand(context.Context, *models.Errand) error {
	return s.err
}
