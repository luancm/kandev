package journal

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/types/errandawait"
	bolt "go.etcd.io/bbolt"
)

var bucketAwaitAborts = []byte("await_aborts")

// RecordAwaitAbort commits only a pre-success error disposition. It retains
// its exact owner until the authenticated backend confirms the tombstone.
func (j *Journal) RecordAwaitAbort(ctx context.Context, id errandawait.Identity) error {
	if !id.Valid() {
		return ErrOwnerMismatch
	}
	raw, err := json.Marshal(id)
	if err != nil {
		return err
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	db, err := j.dbLocked()
	if err != nil {
		return err
	}
	reservation, err := j.checkDiskCapacity(ctx, db, int64(len(raw)), false)
	if err != nil {
		return err
	}
	changed := false
	err = j.updateLocked(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := tx.CreateBucketIfNotExists(bucketAwaitAborts)
		if err != nil {
			return err
		}
		if stored := b.Get([]byte(id.Token)); stored != nil {
			if !bytes.Equal(stored, raw) {
				return ErrOwnerMismatch
			}
			return nil
		}
		if b.Stats().KeyN >= MaxStreamSubmissions {
			return ErrJournalFull
		}
		total, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
		if err != nil {
			return err
		}
		if exceedsLimit(total, int64(len(raw)), j.config.MaxJournalBytes) {
			return ErrJournalFull
		}
		if err := b.Put([]byte(id.Token), raw); err != nil {
			return err
		}
		changed = true
		return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(total+int64(len(raw))))
	})
	j.finishDiskCapacityReservation(reservation, err == nil && changed)
	if err == nil {
		j.refreshMetricsLocked()
	}
	return err
}

func (j *Journal) PendingAwaitAborts(ctx context.Context) ([]errandawait.Identity, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	pending := make([]errandawait.Identity, 0)
	err := j.viewLocked(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAwaitAborts)
		if b == nil {
			return ctx.Err()
		}
		return b.ForEach(func(_, raw []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			var id errandawait.Identity
			if json.Unmarshal(raw, &id) != nil || !id.Valid() {
				return ErrJournalCorrupt
			}
			pending = append(pending, id)
			return nil
		})
	})
	return pending, err
}

func (j *Journal) ConfirmAwaitAbort(ctx context.Context, id errandawait.Identity) error {
	if !id.Valid() {
		return ErrOwnerMismatch
	}
	raw, err := json.Marshal(id)
	if err != nil {
		return err
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	err = j.updateLocked(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		b := tx.Bucket(bucketAwaitAborts)
		if b == nil {
			return nil
		}
		stored := b.Get([]byte(id.Token))
		if stored == nil {
			return nil
		}
		if !bytes.Equal(raw, stored) {
			return ErrOwnerMismatch
		}
		total, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
		if err != nil {
			return err
		}
		if err := tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(total-int64(len(stored)))); err != nil {
			return err
		}
		return b.Delete([]byte(id.Token))
	})
	if err == nil {
		j.refreshMetricsLocked()
	}
	return err
}

// CaptureAwaitInvocation validates the journal and any exact originating
// submission under one read. Absence of an association stays unknown.
func (j *Journal) CaptureAwaitInvocation(ctx context.Context, id errandawait.Identity) (errandawait.Identity, error) {
	if !id.Valid() {
		return id, ErrOwnerMismatch
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	err := j.viewLocked(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if id.SubmissionID == "" {
			return nil
		}
		raw := tx.Bucket(bucketSubmissions).Get([]byte(id.SubmissionID))
		if raw == nil {
			return ErrSubmissionNotFound
		}
		var submission Submission
		if json.Unmarshal(raw, &submission) != nil {
			return ErrJournalCorrupt
		}
		if submission.ID != id.SubmissionID || submission.SessionID != id.ParentSessionID || submission.IncarnationID != id.ParentIncarnationID || int64(submission.HarnessGeneration) != id.HarnessGeneration {
			return ErrOwnerMismatch
		}
		return nil
	})
	return id, err
}
