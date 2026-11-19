package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type sweepingErrandController struct {
	settlingErrandController
	sweeps   int
	sweepErr error
}

func (c *sweepingErrandController) SweepDelivery(context.Context) error {
	c.sweeps++
	return c.sweepErr
}

// @covers AC-TASKS-ERRANDS-008.2
func TestSweepErrandDeliveryRunsSweepAndToleratesFailure(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &sweepingErrandController{sweepErr: errors.New("list failed")}
	svc.SetErrandController(controller)

	require.NotPanics(t, func() { svc.sweepErrandDelivery(context.Background()) })
	require.Equal(t, 1, controller.sweeps)

	svc.SetErrandController(nil)
	require.NotPanics(t, func() { svc.sweepErrandDelivery(context.Background()) })
}
