package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// deletingErrandController adds the deletion and sweep methods to the
// settlement fake.
type deletingErrandController struct {
	settlingErrandController
	preserved []string
	preserveE error
	sweeps    int
	sweepErr  error
}

func (c *deletingErrandController) PreserveCancellationNotice(_ context.Context, errandID string) error {
	c.preserved = append(c.preserved, errandID)
	return c.preserveE
}

func (c *deletingErrandController) SweepDelivery(context.Context) error {
	c.sweeps++
	return c.sweepErr
}

// @covers AC-TASKS-ERRANDS-007.3
func TestPreserveErrandNoticeOnDeleteKeepsRowWhenAdmissionRefused(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &deletingErrandController{preserveE: errors.New("queue full")}
	svc.SetErrandController(controller)

	err := svc.preserveErrandNoticeOnDelete(context.Background(), "errand-1")
	require.Error(t, err, "a refused notice must stop the delete so the notice is not lost")
	require.Equal(t, []string{"errand-1"}, controller.preserved)
}

func TestPreserveErrandNoticeOnDeleteIgnoresSessionThatIsNotAnErrand(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &deletingErrandController{preserveE: models.ErrErrandNotFound}
	svc.SetErrandController(controller)

	require.NoError(t, svc.preserveErrandNoticeOnDelete(context.Background(), "session-1"))

	svc.SetErrandController(nil)
	require.NoError(t, svc.preserveErrandNoticeOnDelete(context.Background(), "session-1"))
}

func TestPreserveErrandNoticeOnDeleteFallsBackToCancelWithoutNotifier(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &recordingErrandController{}
	svc.SetErrandController(controller)

	require.NoError(t, svc.preserveErrandNoticeOnDelete(context.Background(), "session-1"))
	require.Equal(t, []string{"cancel-on-stop:session-1"}, controller.seen())
}

// @covers AC-TASKS-ERRANDS-008.2
func TestSweepErrandDeliveryRunsSweepAndToleratesFailure(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &deletingErrandController{sweepErr: errors.New("list failed")}
	svc.SetErrandController(controller)

	require.NotPanics(t, func() { svc.sweepErrandDelivery(context.Background()) })
	require.Equal(t, 1, controller.sweeps)

	svc.SetErrandController(nil)
	require.NotPanics(t, func() { svc.sweepErrandDelivery(context.Background()) })
}
