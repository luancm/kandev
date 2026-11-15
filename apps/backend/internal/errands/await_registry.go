package errands

import (
	"sync"
	"sync/atomic"
)

type awaitWaiter struct {
	parentID string
	token    string
	wake     chan struct{}
	ended    atomic.Bool
}

type awaitRegistry struct {
	mu       sync.Mutex
	byParent map[string][]*awaitWaiter
}

func (r *awaitRegistry) register(parentID, token string) *awaitWaiter {
	waiter := &awaitWaiter{parentID: parentID, token: token, wake: make(chan struct{}, 1)}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byParent == nil {
		r.byParent = map[string][]*awaitWaiter{}
	}
	r.byParent[parentID] = append(r.byParent[parentID], waiter)
	return waiter
}

func (r *awaitRegistry) remove(waiter *awaitWaiter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	waiters := r.byParent[waiter.parentID]
	kept := waiters[:0]
	for _, candidate := range waiters {
		if candidate != waiter {
			kept = append(kept, candidate)
		}
	}
	if len(kept) == 0 {
		delete(r.byParent, waiter.parentID)
		return
	}
	r.byParent[waiter.parentID] = kept
}

func (r *awaitRegistry) wake(parentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, waiter := range r.byParent[parentID] {
		signalAwait(waiter)
	}
}

func (r *awaitRegistry) endParent(parentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, waiter := range r.byParent[parentID] {
		waiter.ended.Store(true)
		signalAwait(waiter)
	}
}

func (w *awaitWaiter) endedNow() bool {
	return w != nil && w.ended.Load()
}

func signalAwait(waiter *awaitWaiter) {
	select {
	case waiter.wake <- struct{}{}:
	default:
	}
}
