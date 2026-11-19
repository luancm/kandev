package errands

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

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

var errAwaitSnapshotNotReady = errors.New("errand await snapshot is no longer ready")

// AwaitRequest is one owned in-turn join. Owner binds the claim and its
// later receipt or abort. MaxWaitSeconds is 0 to 600.
type AwaitRequest struct {
	Owner          models.ErrandAwaitIdentity
	ErrandIDs      []string
	Mode           string
	MaxWaitSeconds int
}

// AwaitErrand is one named errand in an await response.
type AwaitErrand struct {
	AttentionReason models.ErrandAttentionReason
	ErrandID        string
	Status          string
	Result          string
	Truncated       bool
	DeliveredVia    string
}

// AwaitResult is the await response. Ended means the parent's turn cancelled
// the wait: statuses are present and nothing was claimed.
type AwaitResult struct {
	Delivery   models.ErrandAwaitDelivery
	DurationMS int64
	EndReason  string
	Token      string
	Errands    []AwaitErrand
	Ended      bool
}

// Await waits until the named errands satisfy mode, the deadline passes, or
// the parent turn ends the wait. A cancelled request context claims nothing.
func (s *Service) Await(ctx context.Context, req AwaitRequest) (result AwaitResult, returnErr error) {
	if !s.featureEnabled() {
		return AwaitResult{}, newError(ReasonFeatureDisabled, "errands are disabled", nil)
	}
	if err := validateAwait(req); err != nil {
		return AwaitResult{}, err
	}
	if err := s.store.BeginErrandAwait(ctx, req.Owner); err != nil {
		return AwaitResult{Token: req.Owner.Token}, err
	}
	defer func() {
		result.Token = req.Owner.Token
		if returnErr == nil {
			return
		}
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		returnErr = errors.Join(returnErr, s.AbortOwnedAwait(abortCtx, req.Owner))
	}()
	rows, err := s.loadAwaitErrands(ctx, req)
	if err != nil {
		return AwaitResult{}, err
	}
	started := time.Now()
	waiter := s.awaits.register(req.Owner.ParentSessionID, req.Owner.Token)
	defer s.awaits.remove(waiter)
	deadline := time.NewTimer(time.Duration(req.MaxWaitSeconds) * time.Second)
	defer deadline.Stop()
	result, err = s.awaitLoop(ctx, req, rows, waiter, deadline)
	if err == nil {
		result.DurationMS = time.Since(started).Milliseconds()
	}
	return result, err
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
			result, err := s.finishAwait(ctx, req, rows, !ready, false)
			if !errors.Is(err, errAwaitSnapshotNotReady) {
				return result, err
			}
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
	ctx context.Context, req AwaitRequest, _ []*models.Errand, timedOut, ended bool,
) (AwaitResult, error) {
	for attempt := 0; attempt < claimSnapshotAttempts; attempt++ {
		result, err := s.finishAwaitSnapshot(ctx, req, timedOut, ended)
		if !errors.Is(err, models.ErrErrandVersionConflict) {
			return result, err
		}
	}
	return AwaitResult{Token: req.Owner.Token}, fmt.Errorf("await snapshot retry exhausted: %w", models.ErrErrandVersionConflict)
}

func (s *Service) finishAwaitSnapshot(ctx context.Context, req AwaitRequest, timedOut, ended bool) (AwaitResult, error) {
	rows, inventory, err := s.awaitSnapshot(ctx, req)
	if err != nil {
		return AwaitResult{}, err
	}
	items, err := s.awaitItems(ctx, req.Owner.Token, rows, timedOut, ended)
	if err != nil {
		return AwaitResult{}, err
	}
	if !timedOut && !ended && !awaitSnapshotReady(req.Mode, rows, items) {
		if req.MaxWaitSeconds > 0 {
			return AwaitResult{}, errAwaitSnapshotNotReady
		}
		timedOut = true
		for i := range items {
			if !errandSettled(models.ErrandStatus(items[i].Status)) && items[i].Status != string(models.ErrandStatusNeedsAttention) {
				items[i].Status = awaitPending
			}
		}
	}
	delivery := models.ErrandAwaitDelivery{Version: 1, Token: req.Owner.Token, Items: make([]models.ErrandAwaitDeliveryItem, 0, 5)}
	if !ended {
		claimed, err := s.claimAwait(ctx, req, rows, inventory)
		if err != nil {
			return AwaitResult{}, err
		}
		if err := ctx.Err(); err != nil {
			return AwaitResult{}, err
		}
		for i, row := range rows {
			if snapshot := claimed[row.ID]; snapshot != nil {
				fillAwaitResult(&items[i], snapshot, req.Owner.Token)
				delivery.Items = append(delivery.Items, models.ErrandAwaitDeliveryItem{ErrandID: snapshot.ID, ResultRevision: snapshot.ResultRevision})
			}
		}
	}

	return AwaitResult{Delivery: delivery, Token: req.Owner.Token, Errands: items, Ended: ended, EndReason: awaitEndReason(req.Mode, timedOut, ended)}, nil
}

func awaitEndReason(mode string, timedOut, ended bool) string {
	switch {
	case ended:
		return "parent_ended"
	case timedOut:
		return "timeout"
	case mode == awaitModeAny:
		return "any_ready"
	default:
		return "all_ready"
	}
}

func awaitSnapshotReady(mode string, rows []*models.Errand, items []AwaitErrand) bool {
	settled, pending := 0, 0
	for i, row := range rows {
		if errandSettled(models.ErrandStatus(items[i].Status)) {
			settled++
		}
		if row.ResultState == models.ErrandResultPending {
			pending++
		}
	}
	return settled == len(rows) || (mode == awaitModeAny && pending > 0)
}

func (s *Service) claimAwait(ctx context.Context, req AwaitRequest, rows []*models.Errand, inventory []models.ErrandClaimCandidate) (map[string]*models.Errand, error) {
	unlock := s.parentLocks.lock(req.Owner.ParentSessionID)
	defer unlock()
	included := make([]composedErrand, 0, len(rows))
	for _, row := range rows {
		included = append(included, composedErrand{errand: row})
	}
	return s.claimPending(ctx, included, models.ErrandDeliveryAwait, req.Owner.Token, inventory)
}

func (s *Service) awaitItems(
	ctx context.Context, token string, rows []*models.Errand, timedOut, ended bool,
) ([]AwaitErrand, error) {
	items := make([]AwaitErrand, 0, len(rows))
	for _, row := range rows {
		snapshot, err := s.ProjectSnapshot(ctx, row)
		if err != nil {
			return nil, err
		}
		status := snapshot.Status
		item := AwaitErrand{ErrandID: row.ID, Status: string(status), AttentionReason: snapshot.AttentionReason}
		if timedOut && !ended && !errandSettled(status) && status != models.ErrandStatusNeedsAttention {
			item.Status = awaitPending
		}
		if !ended && row.DeliveryToken != token {
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
}

func validateAwait(req AwaitRequest) error {
	switch {
	case req.Owner.ParentSessionID == "":
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
		if errors.Is(err, models.ErrErrandNotFound) || (err == nil && row.ParentSessionID != req.Owner.ParentSessionID) {
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
		if current.ParentSessionID != req.Owner.ParentSessionID {
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

func (s *Service) awaitSnapshot(ctx context.Context, req AwaitRequest) ([]*models.Errand, []models.ErrandClaimCandidate, error) {
	inventory, err := s.store.ListErrandsByParent(ctx, req.Owner.ParentSessionID)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[string]*models.Errand, len(inventory))
	for _, row := range inventory {
		byID[row.ID] = row
	}
	rows := make([]*models.Errand, 0, len(req.ErrandIDs))
	for _, id := range req.ErrandIDs {
		row := byID[id]
		if row == nil {
			return nil, nil, notFound()
		}
		rows = append(rows, row)
	}
	return rows, models.ErrandInventory(inventory), nil
}
