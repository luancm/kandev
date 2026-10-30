package errands

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	awaitMinErrands = 1
	awaitMaxErrands = 5
	awaitMaxSeconds = 600
	awaitPending    = "pending"
	awaitModeAll    = "all"
	awaitModeAny    = "any"
)

// AwaitRequest is one in-turn join. Token identifies the claim so a later
// release can return it. MaxWaitSeconds is 0 to 600.
type AwaitRequest struct {
	ParentSessionID string
	ErrandIDs       []string
	Mode            string
	MaxWaitSeconds  int
	Token           string
}

// AwaitErrand is one named errand in an await response.
type AwaitErrand struct {
	ErrandID     string
	Status       string
	Result       string
	Truncated    bool
	DeliveredVia string
}

// AwaitResult is the await response. Ended means the parent's turn cancelled
// the wait: statuses are present and nothing was claimed.
type AwaitResult struct {
	DurationMS int64
	EndReason  string
	Token      string
	Errands    []AwaitErrand
	Ended      bool
}

// Await waits until the named errands satisfy mode, the deadline passes, or
// the parent turn ends the wait. A cancelled request context claims nothing.
func (s *Service) Await(ctx context.Context, req AwaitRequest) (AwaitResult, error) {
	if !s.featureEnabled() {
		return AwaitResult{}, newError(ReasonFeatureDisabled, "errands are disabled", nil)
	}
	if err := validateAwait(req); err != nil {
		return AwaitResult{}, err
	}
	if req.Token == "" {
		req.Token = uuid.NewString()
	}
	rows, err := s.loadAwaitErrands(ctx, req)
	if err != nil {
		return AwaitResult{}, err
	}
	started := time.Now()
	waiter := s.awaits.register(req.ParentSessionID, req.Token)
	defer s.awaits.remove(waiter)
	deadline := time.NewTimer(time.Duration(req.MaxWaitSeconds) * time.Second)
	defer deadline.Stop()
	result, err := s.awaitLoop(ctx, req, rows, waiter, deadline)
	if err == nil {
		result.DurationMS = time.Since(started).Milliseconds()
	}
	return result, err
}

// ReleaseAwait returns results still delivered under token to pending.
func (s *Service) ReleaseAwait(ctx context.Context, token string) error {
	return s.releaseClaim(ctx, token)
}

// EndParentAwaits ends every in-flight await of a parent. Each returns
// statuses and claims nothing. Errands keep running.
func (s *Service) EndParentAwaits(parentSessionID string) {
	s.awaits.endParent(parentSessionID)
}

func (s *Service) wakeAwaits(parentSessionID string) {
	s.awaits.wake(parentSessionID)
}

func (s *Service) awaitLoop(
	ctx context.Context, req AwaitRequest, rows []*models.Errand, waiter *awaitWaiter, deadline *time.Timer,
) (AwaitResult, error) {
	for {
		if waiter.endedNow() {
			return s.finishAwait(ctx, req, rows, false, true)
		}
		ready, err := s.awaitReady(ctx, req, rows)
		if err != nil {
			return AwaitResult{}, err
		}
		if ready || req.MaxWaitSeconds == 0 {
			return s.finishAwait(ctx, req, rows, !ready, false)
		}
		select {
		case <-ctx.Done():
			return AwaitResult{}, ctx.Err()
		case <-waiter.wake:
			if waiter.endedNow() {
				return s.finishAwait(ctx, req, rows, false, true)
			}
		case <-deadline.C:
			return s.finishAwait(ctx, req, rows, true, false)
		}
	}
}

func (s *Service) awaitReady(ctx context.Context, req AwaitRequest, seeded []*models.Errand) (bool, error) {
	rows, err := s.reloadAwait(ctx, req, seeded)
	if err != nil {
		return false, err
	}
	copy(seeded, rows)
	settled := 0
	pending := 0
	for _, row := range rows {
		status, err := s.Project(ctx, row)
		if err != nil {
			return false, err
		}
		if errandSettled(status) {
			settled++
		}
		if row.ResultState == models.ErrandResultPending {
			pending++
		}
	}
	switch req.Mode {
	case awaitModeAll:
		return settled == len(rows), nil
	default:
		return pending > 0 || settled == len(rows), nil
	}
}

func (s *Service) finishAwait(
	ctx context.Context, req AwaitRequest, seeded []*models.Errand, timedOut, ended bool,
) (AwaitResult, error) {
	rows, err := s.reloadAwait(ctx, req, seeded)
	if err != nil {
		return AwaitResult{}, err
	}
	if !ended {
		if err := s.claimAwait(ctx, req, rows); err != nil {
			return AwaitResult{}, err
		}
		rows, err = s.reloadAwait(ctx, req, seeded)
		if err != nil {
			return AwaitResult{}, err
		}
	}
	items, err := s.awaitItems(ctx, req.Token, rows, timedOut, ended)
	if err != nil {
		return AwaitResult{}, err
	}
	reason := "all_ready"
	if req.Mode == awaitModeAny {
		reason = "any_ready"
	}
	if timedOut {
		reason = "timeout"
	}
	if ended {
		reason = "parent_ended"
	}
	return AwaitResult{Token: req.Token, Errands: items, Ended: ended, EndReason: reason}, nil
}

func (s *Service) claimAwait(ctx context.Context, req AwaitRequest, rows []*models.Errand) error {
	unlock := s.parentLocks.lock(req.ParentSessionID)
	defer unlock()
	now := s.clock.Now()
	for _, row := range rows {
		if row.ResultState != models.ErrandResultPending {
			continue
		}
		updated, changed, err := s.update(ctx, row.ID, func(e *models.Errand) bool {
			return claimResult(e, models.ErrandDeliveryAwait, req.Token, now)
		})
		if err != nil {
			return fmt.Errorf("claim awaited errand result: %w", err)
		}
		if changed {
			countDelivery(models.ErrandDeliveryAwait)
			s.logger.Info("errand.delivered", s.errandFields(updated, "route", string(models.ErrandDeliveryAwait))...)
			s.publish(ctx, updated)
		}
	}
	return nil
}

func (s *Service) awaitItems(
	ctx context.Context, token string, rows []*models.Errand, timedOut, ended bool,
) ([]AwaitErrand, error) {
	items := make([]AwaitErrand, 0, len(rows))
	for _, row := range rows {
		status, err := s.Project(ctx, row)
		if err != nil {
			return nil, err
		}
		item := AwaitErrand{ErrandID: row.ID, Status: string(status)}
		if timedOut && !ended && !errandSettled(status) {
			item.Status = awaitPending
		}
		if !ended {
			fillAwaitResult(&item, row, token)
		}
		items = append(items, item)
	}
	return items, nil
}

func fillAwaitResult(item *AwaitErrand, row *models.Errand, token string) {
	if row.ResultState == models.ErrandResultDelivered {
		if row.DeliveryToken == token && row.DeliveredRoute == models.ErrandDeliveryAwait {
			item.Result = row.ResultText
			item.Truncated = row.ResultTruncated
			return
		}
		item.DeliveredVia = string(row.DeliveredRoute)
		return
	}
	if row.ResultState == models.ErrandResultPending {
		item.Result = row.ResultText
		item.Truncated = row.ResultTruncated
	}
}

func validateAwait(req AwaitRequest) error {
	switch {
	case req.ParentSessionID == "":
		return fmt.Errorf("%w: parent session is required", ErrInvalidRequest)
	case len(req.ErrandIDs) < awaitMinErrands || len(req.ErrandIDs) > awaitMaxErrands:
		return fmt.Errorf("%w: errand_ids must contain 1 to 5 ids", ErrInvalidRequest)
	case req.Mode != awaitModeAll && req.Mode != awaitModeAny:
		return fmt.Errorf("%w: mode must be all or any", ErrInvalidRequest)
	case req.MaxWaitSeconds < 0 || req.MaxWaitSeconds > awaitMaxSeconds:
		return fmt.Errorf("%w: max_wait_seconds must be from 0 to 600", ErrInvalidRequest)
	case hasDuplicate(req.ErrandIDs):
		return fmt.Errorf("%w: errand_ids must be unique", ErrInvalidRequest)
	default:
		return nil
	}
}

func hasDuplicate(ids []string) bool {
	seen := map[string]struct{}{}
	for _, id := range ids {
		if id == "" {
			return true
		}
		if _, ok := seen[id]; ok {
			return true
		}
		seen[id] = struct{}{}
	}
	return false
}

func (s *Service) loadAwaitErrands(ctx context.Context, req AwaitRequest) ([]*models.Errand, error) {
	rows := make([]*models.Errand, 0, len(req.ErrandIDs))
	for _, id := range req.ErrandIDs {
		row, err := s.store.GetErrand(ctx, id)
		if errors.Is(err, models.ErrErrandNotFound) || (err == nil && row.ParentSessionID != req.ParentSessionID) {
			return nil, notFound()
		}
		if err != nil {
			return nil, fmt.Errorf("load awaited errand: %w", err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *Service) reloadAwait(ctx context.Context, req AwaitRequest, seeded []*models.Errand) ([]*models.Errand, error) {
	rows := make([]*models.Errand, len(seeded))
	for i, row := range seeded {
		current, err := s.store.GetErrand(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("reload awaited errand: %w", err)
		}
		if current.ParentSessionID != req.ParentSessionID {
			return nil, notFound()
		}
		rows[i] = current
	}
	return rows, nil
}

func errandSettled(status models.ErrandStatus) bool {
	return slices.Contains([]models.ErrandStatus{
		models.ErrandStatusDone, models.ErrandStatusFailed, models.ErrandStatusCancelled,
	}, status)
}
