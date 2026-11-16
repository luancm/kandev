package errands

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

const maxTeardownRetriesPerSweep = 32

type teardownEvidenceDiagnostic interface {
	error
	TeardownEvidenceKey() string
}

func pendingTeardown(row *models.Errand) bool {
	if row.TeardownIntent == nil {
		return false
	}
	return row.TeardownIntent.State == models.ErrandTeardownIntentPending ||
		(row.TeardownIntent.Purpose == models.ErrandTeardownPurposeCompletion && row.TeardownIntent.State == models.ErrandTeardownIntentClosed)
}

func pendingCancellation(row *models.Errand) bool {
	return pendingTeardown(row) && row.TeardownIntent.Purpose != models.ErrandTeardownPurposeCompletion
}

// cancelRow publishes a committed decision after the adapter releases ownership,
// even when physical stop or its receipt fails. The callback performs only CAS.
func (s *Service) cancelRow(ctx context.Context, id string, source models.ErrandCancelSource) (*models.Errand, bool, error) {
	for attempt := 0; attempt < maxUpdateAttempts; attempt++ {
		row, err := s.store.GetErrand(ctx, id)
		if err != nil {
			return nil, false, err
		}
		if pendingCancellation(row) {
			return row, false, s.retryTeardown(ctx, row)
		}
		if row.Lifecycle == models.ErrandLifecycleCancelled {
			return row, false, nil
		}
		if !pendingTeardown(row) {
			status, err := s.Project(ctx, row)
			if err != nil || status == models.ErrandStatusDone {
				return row, false, err
			}
		}
		next, changed, err := s.cancelCaptured(ctx, row, source)
		if !changed && errors.Is(err, models.ErrErrandVersionConflict) {
			continue
		}
		return next, changed, err
	}
	return nil, false, models.ErrErrandVersionConflict
}

func (s *Service) cancelCaptured(ctx context.Context, row *models.Errand, source models.ErrandCancelSource) (*models.Errand, bool, error) {
	next := *row
	changed := false
	intentID, requestedAt := uuid.NewString(), s.clock.Now()
	err := s.stopper.CancelErrandWithTarget(ctx, row, func(target ErrandTeardownTarget) error {
		if expected, ok := ctx.Value(cancellationTargetFenceKey{}).(ErrandTeardownTarget); ok && expected != target {
			return models.ErrErrandVersionConflict
		}
		s.applyCancel(&next, source)
		next.TeardownIntent = &models.ErrandTeardownIntent{Version: models.ErrandTeardownIntentVersion,
			ID: intentID, State: models.ErrandTeardownIntentPending, Purpose: models.ErrandTeardownPurposeCancellation,
			RequestedAt: requestedAt, Target: target}
		if err := s.store.UpdateErrand(ctx, &next); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if changed && row.Lifecycle != models.ErrandLifecycleCancelled {
		s.events.PublishErrandUpdated(ctx, &next, models.ErrandStatusCancelled)
		s.wakeAwaits(next.ParentSessionID)
		countSettled(models.ErrandStatusCancelled)
		s.logger.Info("errand.cancelled", s.errandFields(&next, "source", string(source))...)
	}
	return &next, changed && row.Lifecycle != models.ErrandLifecycleCancelled, err
}

func (s *Service) retryTeardown(ctx context.Context, row *models.Errand) error {
	if !pendingTeardown(row) {
		return nil
	}
	intent := *row.TeardownIntent
	outcome, err := s.stopper.RetryErrandTeardown(ctx, intent)
	if err == nil && outcome != TeardownCompleted && outcome != TeardownSuperseded {
		err = errors.New("errands: teardown returned no confirmed outcome")
	}
	if err == nil && outcome == TeardownCompleted && intent.Purpose == models.ErrandTeardownPurposeCompletion {
		latest, readErr := s.store.GetErrand(ctx, row.ID)
		if readErr != nil {
			return readErr
		}
		status, projectErr := s.Project(ctx, latest)
		if projectErr != nil {
			return projectErr
		}
		if s.events != nil {
			s.events.PublishErrandUpdated(ctx, latest, status)
		}
	}
	return err
}

// RetryErrandCompletion resumes one durable success-close obligation. It is
// independent of the new-errand admission flag and safe for explicit admission.
func (s *Service) RetryErrandCompletion(ctx context.Context, id string) error {
	row, err := s.store.GetErrand(ctx, id)
	if errors.Is(err, models.ErrErrandNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if row.TeardownIntent == nil {
		if !legacyCompletionCandidate(row) {
			return nil
		}
		if repair, ok := s.stopper.(interface {
			RepairLegacyErrandCompletion(context.Context, *models.Errand) error
		}); ok {
			return repair.RepairLegacyErrandCompletion(ctx, row)
		}
		return nil
	}
	if row.TeardownIntent.Purpose != models.ErrandTeardownPurposeCompletion {
		return nil
	}
	return s.retryTeardown(ctx, row)
}

// retryPendingTeardowns runs on startup and the existing sweep even when new
// errands are disabled. Each row has a bounded attempt and independent error.
func (s *Service) retryPendingTeardowns(ctx context.Context) error {
	if !s.teardownMu.TryLock() {
		return nil
	}
	defer s.teardownMu.Unlock()
	rows, err := s.store.ListErrands(ctx)
	if err != nil {
		return fmt.Errorf("list errand teardowns: %w", err)
	}
	candidates := rows[:0]
	for _, row := range rows {
		if pendingTeardown(row) || legacyTeardown(row) || legacyCompletionCandidate(row) {
			candidates = append(candidates, row)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	s.pruneTeardownEvidenceWarnings(candidates)
	start := sort.Search(len(candidates), func(i int) bool { return candidates[i].ID > s.teardownCursor })
	var failures []error
	for i := 0; i < min(len(candidates), maxTeardownRetriesPerSweep); i++ {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		row := candidates[(start+i)%len(candidates)]
		s.teardownCursor = row.ID
		attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var err error
		switch {
		case legacyTeardown(row):
			_, _, err = s.cancelCaptured(attemptCtx, row, row.CancelSource)
		case legacyCompletionCandidate(row):
			if repair, ok := s.stopper.(interface {
				RepairLegacyErrandCompletion(context.Context, *models.Errand) error
			}); ok {
				err = repair.RepairLegacyErrandCompletion(attemptCtx, row)
			}
		default:
			err = s.retryTeardown(attemptCtx, row)
		}
		cancel()
		if err != nil {
			if !s.shouldReportTeardownFailure(row.ID, err) {
				continue
			}
			s.logger.Warn("errand teardown remains pending", zap.String("errand_id", row.ID), zap.Error(err))
			failures = append(failures, fmt.Errorf("errand %s teardown: %w", row.ID, err))
		} else {
			delete(s.teardownEvidenceWarnings, row.ID)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) pruneTeardownEvidenceWarnings(candidates []*models.Errand) {
	if len(s.teardownEvidenceWarnings) == 0 {
		return
	}
	active := make(map[string]struct{}, len(candidates))
	for _, row := range candidates {
		active[row.ID] = struct{}{}
	}
	for errandID := range s.teardownEvidenceWarnings {
		if _, ok := active[errandID]; !ok {
			delete(s.teardownEvidenceWarnings, errandID)
		}
	}
}

func (s *Service) shouldReportTeardownFailure(errandID string, err error) bool {
	var diagnostic teardownEvidenceDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.TeardownEvidenceKey() == "" {
		delete(s.teardownEvidenceWarnings, errandID)
		return true
	}
	if s.teardownEvidenceWarnings == nil {
		s.teardownEvidenceWarnings = make(map[string]string)
	}
	key := diagnostic.TeardownEvidenceKey()
	if s.teardownEvidenceWarnings[errandID] == key {
		return false
	}
	s.teardownEvidenceWarnings[errandID] = key
	return true
}

func legacyTeardown(row *models.Errand) bool {
	return row.Lifecycle == models.ErrandLifecycleCancelled && row.TeardownIntent == nil
}

func legacyCompletionCandidate(row *models.Errand) bool {
	return row != nil && row.Lifecycle == models.ErrandLifecycleLaunched && row.TeardownIntent == nil && row.LatestSettledTurnID != ""
}
