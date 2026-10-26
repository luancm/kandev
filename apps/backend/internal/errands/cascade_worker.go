package errands

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// cascadeWorker runs parent cascades off the session state transition that
// requested them, so a runtime stop never blocks or re-enters the transition.
// It has a single goroutine, started and stopped by its owner.
type cascadeWorker struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	pending []string
	queued  map[string]struct{}
	wake    chan struct{}
	started bool
}

func (w *cascadeWorker) start(parent context.Context, run func(ctx context.Context, parentSessionID string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.done = make(chan struct{})
	w.wake = make(chan struct{}, 1)
	w.queued = make(map[string]struct{})
	w.pending = nil
	w.started = true
	go w.loop(ctx, w.wake, w.done, run)
}

func (w *cascadeWorker) stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	cancel, done := w.cancel, w.done
	w.started = false
	w.mu.Unlock()
	cancel()
	<-done
}

// enqueue schedules a cascade for a parent session. A parent already waiting
// is not queued twice. It reports false when the worker is not running.
func (w *cascadeWorker) enqueue(parentSessionID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.started {
		return false
	}
	if _, waiting := w.queued[parentSessionID]; !waiting {
		w.queued[parentSessionID] = struct{}{}
		w.pending = append(w.pending, parentSessionID)
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return true
}

func (w *cascadeWorker) next() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return "", false
	}
	parentSessionID := w.pending[0]
	w.pending = w.pending[1:]
	delete(w.queued, parentSessionID)
	return parentSessionID, true
}

func (w *cascadeWorker) loop(
	ctx context.Context, wake <-chan struct{}, done chan<- struct{},
	run func(ctx context.Context, parentSessionID string),
) {
	defer close(done)
	for {
		for {
			parentSessionID, ok := w.next()
			if !ok {
				break
			}
			run(ctx, parentSessionID)
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		}
	}
}

// CascadeParent asks the worker to cancel the errands of a parent session that
// stopped, failed, or was deleted. It reports whether the request was queued.
func (s *Service) CascadeParent(parentSessionID string) bool {
	return s.cascades.enqueue(parentSessionID)
}

func (s *Service) runCascade(ctx context.Context, parentSessionID string) {
	cancelled, err := s.CancelForParent(ctx, parentSessionID)
	if err != nil {
		s.logger.Warn("errand cascade left runtimes running",
			zap.String("parent_session_id", parentSessionID), zap.Int("cancelled", cancelled), zap.Error(err))
	}
}
