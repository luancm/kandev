package messagequeue

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// QueueServerMessageOnceForSession admits an immutable server source using
// the existing durable admission receipt. Server output never auto-merges.
func (s *Service) QueueServerMessageOnceForSession(ctx context.Context, identity QueueSessionIdentity, key, content string, metadata map[string]interface{}) (*QueuedMessage, error) {
	writer, ok := s.repo.(queueAdmissionRepository)
	if !ok {
		return nil, ErrQueueAdmissionUnavailable
	}
	candidate := &QueuedMessage{ID: key, TaskID: identity.TaskID, SessionID: identity.SessionID,
		Content: content, QueuedBy: QueuedByServer, Metadata: copyMessageMetadata(metadata, 0)}
	var admitted *QueuedMessage
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var err error
		admitted, _, err = writer.AdmitQueueMessage(admittedCtx, identity, key, candidate, nil, s.MaxPerSession(), nil, nil)
		return err
	})
	return admitted, err
}

// PreparedServerAdmission commits through the owning task repository. The queue
// service keeps its existing outer session admission lock around that commit.
type PreparedServerAdmission interface {
	AdmitPreparedErrand(context.Context, QueueSessionIdentity, *QueuedMessage, int) (*QueuedMessage, error)
}

func (s *Service) QueuePreparedServerMessageForSession(ctx context.Context, identity QueueSessionIdentity, key, content string, metadata map[string]interface{}, owner PreparedServerAdmission) (*QueuedMessage, error) {
	candidate := &QueuedMessage{ID: key, TaskID: identity.TaskID, SessionID: identity.SessionID,
		Content: content, QueuedBy: QueuedByServer, Metadata: copyMessageMetadata(metadata, 0)}
	var admitted *QueuedMessage
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		queue, ok := s.repo.(*sqliteRepository)
		if !ok {
			return ErrQueueAdmissionUnavailable
		}
		unlock := queue.withSessionLock(identity.SessionID)
		defer unlock()
		var err error
		admitted, err = owner.AdmitPreparedErrand(admittedCtx, identity, candidate, s.MaxPerSession())
		return err
	})
	return admitted, err
}

// AdmitServerMessageInTransaction reuses ordinary queue receipt, capacity and
// identity checks. The caller holds GuardServerAdmissionInTransaction and owns
// the transaction, committing its result claim before publishing readiness.
func AdmitServerMessageInTransaction(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, identity QueueSessionIdentity, candidate *QueuedMessage, maxPerSession int) (*QueuedMessage, bool, error) {
	if err := validateQueueAdmissionInput(identity, candidate.ID, candidate); err != nil {
		return nil, false, err
	}
	if candidate.QueuedBy != QueuedByServer {
		return nil, false, ErrQueueIDConflict
	}
	fingerprint, err := queueAdmissionFingerprint(identity, candidate, nil)
	if err != nil {
		return nil, false, err
	}
	r := &sqliteRepository{db: db, tasksTablePresent: true, taskSessionsTablePresent: true, taskSessionsTaskIDPresent: true}
	return r.admitQueueMessageTx(ctx, tx, identity, candidate.ID, cloneQueuedMessage(candidate), nil, maxPerSession, nil, nil, fingerprint)
}

// GuardServerAdmissionInTransaction keeps the task-row, delivery and queue
// session lock order shared with ordinary admission.
func GuardServerAdmissionInTransaction(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, identity QueueSessionIdentity) error {
	r := &sqliteRepository{db: db, tasksTablePresent: true, taskSessionsTablePresent: true, taskSessionsTaskIDPresent: true}
	return r.guardQueueAdmissionTx(ctx, tx, identity, nil)
}

func SourceGenerationsInTransaction(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) (int64, int64, error) {
	task, err := getLifecycleGenerationTx(ctx, tx, tx, taskID)
	if err != nil {
		return 0, 0, err
	}
	session, err := sessionGenerationInTx(ctx, tx, sessionID)
	return task, session, err
}
