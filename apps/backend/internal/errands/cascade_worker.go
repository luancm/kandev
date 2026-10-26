package errands

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// cascadeJob names one cascade: every errand of a parent session, or every
// errand of a task.
type cascadeJob struct {
	taskWide bool
	id       string
}

// cascadeWorker runs cascades off the session state transition or task
// archive that requested them, so a runtime stop never blocks or re-enters the
// caller. It has a single goroutine, started and stopped by its owner.
type cascadeWorker struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	pending []cascadeJob
	queued  map[cascadeJob]struct{}
	wake    chan struct{}
	started bool
}

func (w *cascadeWorker) start(parent context.Context, run func(ctx context.Context, job cascadeJob)) {
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
	w.queued = make(map[cascadeJob]struct{})
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

// enqueue schedules a cascade. A job already waiting is not queued twice. It
// reports false when the worker is not running.
func (w *cascadeWorker) enqueue(job cascadeJob) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.started {
		return false
	}
	if _, waiting := w.queued[job]; !waiting {
		w.queued[job] = struct{}{}
		w.pending = append(w.pending, job)
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return true
}

func (w *cascadeWorker) next() (cascadeJob, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return cascadeJob{}, false
	}
	job := w.pending[0]
	w.pending = w.pending[1:]
	delete(w.queued, job)
	return job, true
}

func (w *cascadeWorker) loop(
	ctx context.Context, wake <-chan struct{}, done chan<- struct{},
	run func(ctx context.Context, job cascadeJob),
) {
	defer close(done)
	for {
		for {
			job, ok := w.next()
			if !ok {
				break
			}
			run(ctx, job)
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
	return s.cascades.enqueue(cascadeJob{id: parentSessionID})
}

// CascadeTask asks the worker to cancel every errand of an archived task,
// queued ones included. It reports whether the request was queued.
func (s *Service) CascadeTask(taskID string) bool {
	return s.cascades.enqueue(cascadeJob{taskWide: true, id: taskID})
}

func (s *Service) runCascade(ctx context.Context, job cascadeJob) {
	var cancelled int
	var err error
	if job.taskWide {
		cancelled, err = s.CancelForTask(ctx, job.id)
	} else {
		cancelled, err = s.CancelForParent(ctx, job.id)
	}
	if err != nil {
		s.logger.Warn("errand cascade left runtimes running",
			zap.String("cascade_id", job.id), zap.Bool("task_wide", job.taskWide),
			zap.Int("cancelled", cancelled), zap.Error(err))
	}
}
