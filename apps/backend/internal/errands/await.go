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

var errAwaitSnapshotNotReady = errors.New("errand await snapshot is no longer ready")

// AwaitRequest is one in-turn join. Token identifies the claim so a later
// release can return it. MaxWaitSeconds is 0 to 600.
type AwaitRequest struct {
	Owner           *models.ErrandAwaitIdentity
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
	if req.Token == "" {
		req.Token = uuid.NewString()
	}
	bound := req.Owner == nil
	defer func() {
		result.Token = req.Token
		if returnErr == nil || !bound {
			return
		}
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		var abortErr error
		if req.Owner != nil {
			abortErr = s.AbortOwnedAwait(abortCtx, *req.Owner)
		} else {
			abortErr = s.AbortAwait(abortCtx, req.ParentSessionID, req.Token)
		}
		returnErr = errors.Join(returnErr, abortErr)
	}()
	if req.Owner != nil {
		if req.Owner.Token != req.Token || req.Owner.ParentSessionID != req.ParentSessionID {
			return AwaitResult{}, models.ErrInvalidErrand
		}
		if err := s.store.BeginErrandAwait(ctx, *req.Owner); err != nil {
			return AwaitResult{}, err
		}
		bound = true
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
	result, err = s.awaitLoop(ctx, req, rows, waiter, deadline)
	if err == nil {
		result.DurationMS = time.Since(started).Milliseconds()
	}
	return result, err
}

// ReleaseAwait returns results still delivered under token to pending.
func (s *Service) ReleaseAwait(ctx context.Context, token string) error {
	return s.releaseClaim(ctx, token, models.ErrandDeliveryAwait)
}

// AbortAwait records a parent-scoped abort even before a delayed result claim.
// The caller has irrevocably selected an error response rather than success.
func (s *Service) AbortAwait(ctx context.Context, parentSessionID, token string) error {
	parent, err := s.store.GetTaskSession(ctx, parentSessionID)
	if err != nil {
		return err
	}
	if parent == nil || models.IsErrandSession(parent) {
		return models.ErrInvalidErrand
	}
	released, err := s.store.AbortAwait(ctx, parent.TaskID, parent.ID, token)
	if err != nil {
		return fmt.Errorf("abort errand await: %w", err)
	}
	for _, row := range released {
		s.publish(ctx, row)
	}
	s.wakeAwaits(parent.ID)
	s.deliverPending(ctx, parent.ID)
	return nil
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
	return AwaitResult{Token: req.Token}, fmt.Errorf("await snapshot retry exhausted: %w", models.ErrErrandVersionConflict)
}

func (s *Service) finishAwaitSnapshot(ctx context.Context, req AwaitRequest, timedOut, ended bool) (AwaitResult, error) {
	rows, inventory, err := s.awaitSnapshot(ctx, req)
	if err != nil {
		return AwaitResult{}, err
	}
	items, err := s.awaitItems(ctx, req.Token, rows, timedOut, ended)
	if err != nil {
		return AwaitResult{}, err
	}
	if !timedOut && !ended && !awaitSnapshotReady(req.Mode, rows, items) {
		if req.MaxWaitSeconds > 0 {
			return AwaitResult{}, errAwaitSnapshotNotReady
		}
		timedOut = true
		for i := range items {
			if !errandSettled(models.ErrandStatus(items[i].Status)) {
				items[i].Status = awaitPending
			}
		}
	}
	delivery := models.ErrandAwaitDelivery{Version: 1, Token: req.Token, Items: make([]models.ErrandAwaitDeliveryItem, 0, 5)}
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
				fillAwaitResult(&items[i], snapshot, req.Token)
				delivery.Items = append(delivery.Items, models.ErrandAwaitDeliveryItem{ErrandID: snapshot.ID, ResultRevision: snapshot.ResultRevision})
			}
		}
	}

	return AwaitResult{Delivery: delivery, Token: req.Token, Errands: items, Ended: ended, EndReason: awaitEndReason(req.Mode, timedOut, ended)}, nil
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
	unlock := s.parentLocks.lock(req.ParentSessionID)
	defer unlock()
	included := make([]composedErrand, 0, len(rows))
	for _, row := range rows {
		included = append(included, composedErrand{errand: row})
	}
	return s.claimPending(ctx, included, models.ErrandDeliveryAwait, req.Token, inventory)
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

func (s *Service) awaitSnapshot(ctx context.Context, req AwaitRequest) ([]*models.Errand, []models.ErrandClaimCandidate, error) {
	inventory, err := s.store.ListErrandsByParent(ctx, req.ParentSessionID)
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
