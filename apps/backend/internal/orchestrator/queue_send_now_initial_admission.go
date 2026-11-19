package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

var errSendNowInitialPromptUncertain = errors.New("initial Send Now prompt acceptance is uncertain")

func (s *Service) configureSendNowInitialAdmission(
	ctx context.Context, claim *messagequeue.SendNowClaim, options *startCreatedSessionOptions,
) func() error {
	completion := make(chan error, 1)
	outcome := &promptDispatchOutcome{}
	options.initialPromptOutcome = outcome
	var once sync.Once
	const (
		waiting uint32 = iota
		admitting
		closed
	)
	var phase atomic.Uint32
	abort := func(err error) error {
		if phase.CompareAndSwap(waiting, closed) {
			return err
		}
		return errors.Join(errSendNowInitialPromptUncertain, err)
	}
	options.beforeProviderAdmission = func(string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !phase.CompareAndSwap(waiting, admitting) {
			return context.Canceled
		}
		return nil
	}
	finish := func(err error) { once.Do(func() { completion <- err }) }
	options.onInitialPromptAccepted = func(string) {
		if err := s.markSendNowClaimAcceptedWithRetry(ctx, claim); err != nil {
			finish(errors.Join(errSendNowInitialPromptUncertain, err))
			return
		}
		finish(nil)
	}
	options.onInitialPromptAdmissionFailed = func(err error) {
		if outcome.snapshot().refused() {
			finish(err)
			return
		}
		finish(abort(err))
	}
	options.onInitialPromptFailed = func() { finish(abort(errors.New("initial Send Now prompt failed before acceptance"))) }
	return func() error {
		defer phase.Store(closed)
		// Match startup timing; only a worker that never began admission can
		// restore its input when cancelled or timed out.
		timer := time.NewTimer(5 * time.Minute)
		defer timer.Stop()
		select {
		case err := <-completion:
			return err
		case <-ctx.Done():
			return abort(ctx.Err())
		case <-timer.C:
			return abort(context.DeadlineExceeded)
		}
	}
}
