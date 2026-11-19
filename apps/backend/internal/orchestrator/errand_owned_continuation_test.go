package orchestrator

import (
	"context"
	"testing"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-002.8
func TestOwnedErrandContinuationRefusesUnresolvedWork(t *testing.T) {
	for _, sender := range []string{"user", "parent"} {
		t.Run(sender, func(t *testing.T) { assertOwnedErrandContinuationRefusal(t, sender) })
	}
}

func assertOwnedErrandContinuationRefusal(t *testing.T, sender string) {
	t.Helper()
	f := newErrandLaunchFixture(t)
	first := f.request()
	ctx := t.Context()
	seedOwnedLaunchReceipt(t, f, first, models.DeliverySubmissionAccepted, "execution", false)
	row, err := f.repo.GetErrand(ctx, first.ErrandID)
	require.NoError(t, err)
	row.Lifecycle = models.ErrandLifecycleLaunched
	require.NoError(t, f.repo.UpdateErrand(ctx, row))
	before, err := f.repo.GetErrandExecutionDescriptor(ctx, first.ErrandID)
	require.NoError(t, err)
	turns, err := f.repo.ListErrandTurns(ctx, first.ErrandID)
	require.NoError(t, err)
	messages, _, err := f.repo.ListErrandMessages(ctx, first.ErrandID, "", 5)
	require.NoError(t, err)

	adapter := NewErrandAdapter(f.svc)
	adapter.SetOwnedLaunchRuntime(ownedErrandLaunchFixtureRuntime{}, func(context.Context, string, string) (*runtimeapi.WorkspaceInfo, error) {
		t.Fatal("unresolved work must not reach workspace admission")
		return nil, nil
	})
	if sender == "parent" {
		ctx = errands.WithParentPromptOrigin(ctx, errands.ParentPromptOrigin{SenderSessionID: first.ParentSessionID})
	}
	request := errands.ContinuationRequest{TaskID: first.TaskID, ErrandID: first.ErrandID, ClientMessageID: "next", Content: "Follow up"}
	for range 2 {
		turnID, err := adapter.ContinueErrand(ctx, request)
		require.ErrorIs(t, err, db.ErrErrandSettlementPending)
		require.Empty(t, turnID)
	}
	after, err := f.repo.GetErrandExecutionDescriptor(ctx, first.ErrandID)
	require.NoError(t, err)
	require.Equal(t, before, after)
	afterTurns, err := f.repo.ListErrandTurns(ctx, first.ErrandID)
	require.NoError(t, err)
	require.Equal(t, turns, afterTurns)
	afterMessages, _, err := f.repo.ListErrandMessages(ctx, first.ErrandID, "", 5)
	require.NoError(t, err)
	require.Equal(t, messages, afterMessages)
}
