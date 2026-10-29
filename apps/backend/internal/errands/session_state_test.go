package errands

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAwaitAllWakesWhenCapturedResultSessionBecomesIdle(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	row := h.mustStart("audit").Errand
	h.service.SetTurnReader(&fakeTurns{views: map[string]TurnView{
		row.FirstTurnID: {LastAgentText: "Settled result", EarliestUserSender: testParent},
	}})
	req := AwaitRequest{ParentSessionID: testParent, ErrandIDs: []string{row.ID}, Mode: awaitModeAll, Token: "state-wake", MaxWaitSeconds: 600}
	rows, err := h.service.loadAwaitErrands(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	waiter := h.service.awaits.register(testParent, req.Token)
	defer h.service.awaits.remove(waiter)
	if err := h.service.OnErrandTurnSettled(ctx, TurnSettlement{
		SessionID: row.ID, TurnID: row.FirstTurnID, Outcome: models.ErrandOutcomeDone,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waiter.wake:
	default:
		t.Fatal("result capture did not wake await")
	}
	ready, err := h.service.awaitReady(ctx, req, rows)
	if err != nil || ready {
		t.Fatalf("still-running result readiness = %v, %v", ready, err)
	}
	h.store.setSessionState(row.ID, models.TaskSessionStateWaitingForInput)
	if err := h.service.OnSessionStateChanged(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waiter.wake:
	default:
		t.Fatal("authoritative idle state did not wake await")
	}
	deadline := time.NewTimer(time.Minute)
	defer deadline.Stop()
	result, err := h.service.awaitLoop(ctx, req, rows, waiter, deadline)
	if err != nil || len(result.Errands) != 1 || result.Errands[0].Result != "Settled result" || result.Token != req.Token {
		t.Fatalf("awaited settled result = %+v, %v", result, err)
	}
}
