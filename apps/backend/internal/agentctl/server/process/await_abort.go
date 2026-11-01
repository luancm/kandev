package process

import (
	"context"
	"github.com/kandev/kandev/internal/agentctl/journal"

	"github.com/kandev/kandev/internal/agentctl/types/errandawait"
)

// CaptureAwaitInvocation snapshots this launched owner's authority once. A
// missing originating submission stays unknown; it is never the latest turn.
func (m *Manager) CaptureAwaitInvocation(ctx context.Context, token string) (errandawait.Identity, error) {
	session, incarnation, generation := m.DeliverySubmissionIdentity()
	id := errandawait.Identity{TaskID: m.cfg.TaskID, ParentSessionID: session, Token: token, ParentIncarnationID: incarnation, HarnessGeneration: int64(generation)}
	j, err := m.DeliveryJournal()
	if err != nil {
		return id, err
	}
	id.SubmissionID = m.activeDeliverySubmissionID()
	if id.SubmissionID == "" {
		m.deliveryPromptMu.Lock()
		if len(m.deliveryPromptByGeneration) == 1 {
			for _, submission := range m.deliveryPromptByGeneration {
				id.SubmissionID = submission
			}
		}
		m.deliveryPromptMu.Unlock()
	}
	return j.CaptureAwaitInvocation(ctx, id)
}

func (m *Manager) RecordAwaitAbort(ctx context.Context, id errandawait.Identity) error {
	j, err := m.DeliveryJournal()
	if err != nil {
		return err
	}
	if id.TaskID != m.cfg.TaskID || id.ParentSessionID != m.cfg.SessionID {
		return journal.ErrOwnerMismatch
	}
	return j.RecordAwaitAbort(ctx, id)
}
func (m *Manager) PendingAwaitAborts(ctx context.Context) ([]errandawait.Identity, error) {
	j, err := m.DeliveryJournal()
	if err != nil {
		return nil, err
	}
	return j.PendingAwaitAborts(ctx)
}
func (m *Manager) ConfirmAwaitAbort(ctx context.Context, id errandawait.Identity) error {
	j, err := m.DeliveryJournal()
	if err != nil {
		return err
	}
	return j.ConfirmAwaitAbort(ctx, id)
}
