package mcp

import (
	"context"
	"errors"
	"fmt"
	"time"

	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// AwaitAbortStore is backed by the existing owner journal. Pending intents
// represent a selected error, not evidence that a successful call was lost.
type AwaitAbortStore interface {
	CaptureAwaitInvocation(context.Context, string) (models.ErrandAwaitIdentity, error)
	RecordAwaitAbort(context.Context, models.ErrandAwaitIdentity) error
	PendingAwaitAborts(context.Context) ([]models.ErrandAwaitIdentity, error)
	ConfirmAwaitAbort(context.Context, models.ErrandAwaitIdentity) error
}

func (s *Server) SetAwaitAbortStore(store AwaitAbortStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.awaitAbortStore = store
}
func (s *Server) awaitStore() AwaitAbortStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.awaitAbortStore
}
func (s *Server) captureAwait(ctx context.Context, token string) (context.Context, error) {
	store := s.awaitStore()
	if store == nil {
		return ctx, nil
	}
	id, err := store.CaptureAwaitInvocation(ctx, token)
	if err == nil && (!id.Valid() || id.TaskID != s.taskID || id.ParentSessionID != s.sessionID || id.Token != token) {
		err = fmt.Errorf("await invocation owner is unavailable")
	}
	return mcporigin.WithAwaitInvocation(ctx, id), err
}

func (s *Server) abortErrandAwait(ctx context.Context, token string) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), errandAwaitReleaseTimeout)
	defer cancel()
	store := s.awaitStore()
	id, hasOwner := mcporigin.AwaitInvocation(ctx)
	if hasOwner {
		s.rememberAwaitAbort(id)
	}
	var journalErr error
	if store == nil || !hasOwner {
		journalErr = errors.New("await abort journal evidence is unavailable")
	} else {
		journalErr = store.RecordAwaitAbort(cleanup, id)
	}
	confirmed, err := s.sendAwaitAbort(cleanup, token)
	if err == nil && confirmed && journalErr == nil {
		err = store.ConfirmAwaitAbort(cleanup, id)
		if err == nil {
			s.forgetAwaitAbort(id.Token)
		}
	}
	return errors.Join(journalErr, err)
}

func (s *Server) sendAwaitAbort(ctx context.Context, token string) (bool, error) {
	payload := s.errandSenderPayload()
	payload[errandAwaitTokenArg] = token
	var result map[string]interface{}
	err := s.backend.RequestPayload(mcporigin.WithTrustedInternalCall(ctx), ws.ActionMCPAwaitErrandsRelease, payload, &result)
	confirmed, _ := result["released"].(bool)
	if err == nil && !confirmed {
		err = errors.New("await abort tombstone was not confirmed")
	}
	return confirmed, err
}

// ReplayAwaitAborts retries the same private abort intents. The API stream
// owns and cancels this loop; it never repeats await or sends result text.
func (s *Server) ReplayAwaitAborts(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		s.replayAwaitAbortsOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) rememberAwaitAbort(id models.ErrandAwaitIdentity) {
	if !id.Valid() || id.TaskID != s.taskID || id.ParentSessionID != s.sessionID {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.awaitAbortPending == nil {
		s.awaitAbortPending = make(map[string]models.ErrandAwaitIdentity)
	}
	if _, exists := s.awaitAbortPending[id.Token]; !exists {
		s.awaitAbortPending[id.Token] = id
	}
}
func (s *Server) forgetAwaitAbort(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.awaitAbortPending, token)
}
func (s *Server) replayAwaitAbortsOnce(ctx context.Context) {
	store := s.awaitStore()
	if store == nil || ctx.Err() != nil {
		return
	}
	pending, err := store.PendingAwaitAborts(ctx)
	if err == nil {
		for _, id := range pending {
			s.rememberAwaitAbort(id)
		}
	}
	s.mu.RLock()
	pending = make([]models.ErrandAwaitIdentity, 0, len(s.awaitAbortPending))
	for _, id := range s.awaitAbortPending {
		pending = append(pending, id)
	}
	s.mu.RUnlock()
	for _, id := range pending {
		if ctx.Err() != nil {
			return
		}
		attempt, cancel := context.WithTimeout(mcporigin.WithAwaitInvocation(ctx, id), errandAwaitReleaseTimeout)
		journalErr := store.RecordAwaitAbort(attempt, id)
		confirmed, sendErr := s.sendAwaitAbort(attempt, id.Token)
		if sendErr == nil && confirmed && journalErr == nil {
			if err := store.ConfirmAwaitAbort(attempt, id); err == nil {
				s.forgetAwaitAbort(id.Token)
			}
		}
		cancel()
	}
}
