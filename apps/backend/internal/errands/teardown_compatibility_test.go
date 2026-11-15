package errands

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

type legacyEvidenceRefusal string

func (e legacyEvidenceRefusal) Error() string {
	return "legacy errand lacks positive initial admission authority: " + string(e)
}

func (e legacyEvidenceRefusal) TeardownEvidenceKey() string { return string(e) }

type legacyEvidenceStopper struct {
	*fakeStopper
	err      error
	attempts int
}

func (s *legacyEvidenceStopper) CancelErrandWithTarget(ctx context.Context, row *models.Errand, commit func(ErrandTeardownTarget) error) error {
	s.attempts++
	if s.err != nil {
		return s.err
	}
	return s.fakeStopper.CancelErrandWithTarget(ctx, row, commit)
}

// @covers AC-TASKS-ERRAND-COMPLETION-001.4
func TestErrandTeardownStableLegacyDiagnosticIsDeduplicatedButRetried(t *testing.T) {
	h, healthyID := pendingResultHarness(t, "healthy sibling result")
	h.deliverer.mu.Lock()
	h.deliverer.parents = nil
	h.deliverer.mu.Unlock()
	core, logs := observer.New(zapcore.DebugLevel)
	zapLogger := zap.New(core)
	wrapped, err := logger.NewFromZap(zapLogger)
	require.NoError(t, err)
	h.service.logger = wrapped.WithFields(zap.String("component", "errands"))
	t.Cleanup(func() { _ = zapLogger.Sync() })

	row := h.mustStart("legacy").Errand
	row.Lifecycle, row.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
	require.NoError(t, h.store.UpdateErrand(t.Context(), row))
	stopper := &legacyEvidenceStopper{fakeStopper: h.stopper, err: legacyEvidenceRefusal("initial admission marker is missing")}
	h.service.stopper = stopper

	require.Error(t, h.service.SweepDelivery(t.Context()))
	require.Equal(t, 1, stopper.attempts)
	require.Len(t, logs.FilterMessage("errand teardown remains pending").All(), 1)
	require.Equal(t, []string{testParent}, h.deliverer.delivered(), "healthy result delivery proceeds beside the refusal")

	// A suppressed diagnostic must not suppress the authoritative retry.
	require.NoError(t, h.service.SweepDelivery(t.Context()))
	require.Equal(t, 2, stopper.attempts)
	require.Len(t, logs.FilterMessage("errand teardown remains pending").All(), 1)
	healthy, err := h.store.GetErrand(t.Context(), healthyID)
	require.NoError(t, err)
	require.Equal(t, models.ErrandResultPending, healthy.ResultState)
	stillLegacy, err := h.store.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Nil(t, stillLegacy.TeardownIntent, "missing admission proof remains protected")

	stopper.err = legacyEvidenceRefusal("initial sender authority conflicts")
	require.Error(t, h.service.SweepDelivery(t.Context()))
	require.Equal(t, 3, stopper.attempts)
	require.Len(t, logs.FilterMessage("errand teardown remains pending").All(), 2,
		"changed legacy evidence gets a new diagnostic")

	stopper.err = nil
	require.NoError(t, h.service.SweepDelivery(t.Context()))
	require.Equal(t, 4, stopper.attempts)
	resolved, err := h.store.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.NotNil(t, resolved.TeardownIntent)
	require.Equal(t, models.ErrandTeardownIntentCompleted, resolved.TeardownIntent.State)

	// Changed evidence may make the row eligible again later; stale dedupe state
	// must not hide that new refusal.
	resolved.TeardownIntent = nil
	resolved.Lifecycle = models.ErrandLifecycleCancelled
	resolved.CancelSource = models.ErrandCancelSourceUser
	require.NoError(t, h.store.UpdateErrand(t.Context(), resolved))
	stopper.err = fmt.Errorf("%w", legacyEvidenceRefusal("initial admission marker is missing"))
	require.Error(t, h.service.SweepDelivery(t.Context()))
	require.Equal(t, 5, stopper.attempts)
	require.Len(t, logs.FilterMessage("errand teardown remains pending").All(), 3)
}
