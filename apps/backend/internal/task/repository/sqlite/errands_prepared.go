package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// preparedErrandSource reads only the private ownership fields. The queue
// receipt preserves the complete original source and display snapshot.
type preparedErrandSource struct {
	Identity        *messagequeue.QueueSessionIdentity `json:"identity"`
	Provenance      string                             `json:"provenance"`
	Kind            string                             `json:"kind"`
	PreparedBlock   string                             `json:"prepared_block"`
	QueueGeneration *int64                             `json:"queue_generation,omitempty"`
	Receipt         struct {
		SourceID string `json:"source_id"`
		Envelope struct {
			Token string                     `json:"token"`
			Route models.ErrandDeliveryRoute `json:"route"`
			Items []struct {
				ErrandID string `json:"errand_id"`
			} `json:"items"`
		} `json:"envelope"`
		Claims []models.ErrandResultClaim `json:"claims"`
	} `json:"receipt"`
}

func readPreparedErrandSource(source *messagequeue.QueuedMessage) (preparedErrandSource, error) {
	var stamp preparedErrandSource
	encoded, err := json.Marshal(source.Metadata["errand_source"])
	if err != nil {
		return stamp, err
	}
	if err := json.Unmarshal(encoded, &stamp); err != nil {
		return stamp, err
	}
	if !validPreparedSourceIdentity(source, stamp) || !validPreparedSourceReceipt(source, stamp) || !preparedClaimGenerationsPresent(encoded) {
		return stamp, models.ErrInvalidErrand
	}
	return stamp, nil
}

func validPreparedSourceIdentity(source *messagequeue.QueuedMessage, stamp preparedErrandSource) bool {
	return source.QueuedBy == messagequeue.QueuedByServer && stamp.Provenance == "errands" && stamp.Kind == "prepared" &&
		stamp.Identity != nil && stamp.Identity.TaskID == source.TaskID && stamp.Identity.SessionID == source.SessionID &&
		stamp.Identity.SessionIncarnationID != "" && stamp.PreparedBlock != "" && stamp.PreparedBlock == source.Content
}

func validPreparedSourceReceipt(source *messagequeue.QueuedMessage, stamp preparedErrandSource) bool {
	token, ok := source.Metadata["errand_delivery_token"].(string)
	if !ok || token == "" || token != stamp.Receipt.Envelope.Token || stamp.Receipt.SourceID != source.ID ||
		stamp.Receipt.Envelope.Route != models.ErrandDeliveryNotification || len(stamp.Receipt.Claims) == 0 ||
		len(stamp.Receipt.Envelope.Items) != len(stamp.Receipt.Claims) {
		return false
	}
	return validPreparedClaims(stamp, token)
}

func validPreparedClaims(stamp preparedErrandSource, token string) bool {
	seen := map[string]bool{}
	for i, claim := range stamp.Receipt.Claims {
		if claim.ErrandID == "" || seen[claim.ErrandID] || claim.ResultRevision == "" || claim.ParentAdmissionGeneration < 0 ||
			claim.Token != token || claim.Route != models.ErrandDeliveryNotification || stamp.Receipt.Envelope.Items[i].ErrandID != claim.ErrandID {
			return false
		}
		seen[claim.ErrandID] = true
	}
	return true
}

func preparedClaimGenerationsPresent(encoded []byte) bool {
	var source struct {
		Receipt struct {
			Claims []struct {
				Generation *int64 `json:"ParentAdmissionGeneration"`
			} `json:"claims"`
		} `json:"receipt"`
	}
	if json.Unmarshal(encoded, &source) != nil {
		return false
	}
	for _, claim := range source.Receipt.Claims {
		if claim.Generation == nil {
			return false
		}
	}
	return len(source.Receipt.Claims) > 0
}

// AdmitPreparedErrand transfers the exact result to its row-independent queue
// source in one transaction. Queue admission obtains task/delivery/session locks
// in their established order; no readiness or provider work runs before commit.
func (r *Repository) AdmitPreparedErrand(ctx context.Context, identity messagequeue.QueueSessionIdentity, source *messagequeue.QueuedMessage, maxPerSession int) (*messagequeue.QueuedMessage, error) {
	stamp, err := readPreparedErrandSource(source)
	if err != nil {
		return nil, err
	}
	if *stamp.Identity != identity {
		return nil, models.ErrErrandVersionConflict
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// The queue's admission guard takes the task row before the delivery guard.
	if err := messagequeue.GuardServerAdmissionInTransaction(ctx, tx, r.db, identity); err != nil {
		return nil, err
	}
	if err := stampPreparedQueueGenerations(ctx, tx, source); err != nil {
		return nil, err
	}
	admitted, sourceReplay, err := messagequeue.AdmitServerMessageInTransaction(ctx, tx, r.db, identity, source, maxPerSession)
	if err != nil {
		return nil, err
	}
	if err := claimPreparedErrandResults(ctx, tx, identity, stamp.Receipt.Claims, sourceReplay, r.nowUTC()); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%w (token %s): %v", models.ErrErrandClaimCommitUnknown, stamp.Receipt.Envelope.Token, err)
	}
	return admitted, nil
}

func stampPreparedQueueGenerations(ctx context.Context, tx *sqlx.Tx, source *messagequeue.QueuedMessage) error {
	task, _, err := messagequeue.SourceGenerationsInTransaction(ctx, tx, source.TaskID, source.SessionID)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(source.Metadata["errand_source"])
	if err != nil {
		return err
	}
	var stamp map[string]interface{}
	if err := json.Unmarshal(encoded, &stamp); err != nil {
		return err
	}
	stamp["queue_generation"] = task
	source.Metadata["errand_source"] = stamp
	return nil
}

func claimPreparedErrandResults(ctx context.Context, tx *sqlx.Tx, identity messagequeue.QueueSessionIdentity, claims []models.ErrandResultClaim, sourceReplay bool, at time.Time) error {
	ordered := append([]models.ErrandResultClaim(nil), claims...)
	slices.SortFunc(ordered, func(a, b models.ErrandResultClaim) int { return strings.Compare(a.ErrandID, b.ErrandID) })
	candidates := make([]models.ErrandClaimCandidate, 0, len(ordered))
	for _, claim := range ordered {
		row, err := scanErrand(tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandColumns+` FROM task_session_errands WHERE id = ?`), claim.ErrandID))
		if err != nil {
			return err
		}
		if row.TaskID != identity.TaskID || row.ParentSessionID != identity.SessionID || row.ResultRevision != claim.ResultRevision || row.ParentAdmissionGeneration != claim.ParentAdmissionGeneration {
			return models.ErrErrandVersionConflict
		}
		candidates = append(candidates, models.ErrandClaimCandidate{ID: row.ID, Version: row.Version})
	}
	token := ordered[0].Token
	replay, err := prepareErrandClaim(ctx, tx, identity.TaskID, identity.SessionID, models.ErrandDeliveryNotification, token, candidates, at)
	if err != nil {
		return err
	}
	if replay {
		// A result token can replay only its original immutable queue admission.
		if !sourceReplay {
			return models.ErrErrandVersionConflict
		}
		return validatePreparedClaimReplay(ctx, tx, identity, ordered)
	}
	rows := make([]*models.Errand, 0, len(candidates))
	for _, candidate := range candidates {
		row, err := claimErrandResult(ctx, tx, identity.TaskID, identity.SessionID, models.ErrandDeliveryNotification, token, candidate, at)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	return saveErrandClaimSnapshot(ctx, tx, token, candidates, candidates, rows)
}

func validatePreparedClaimReplay(ctx context.Context, tx *sqlx.Tx, identity messagequeue.QueueSessionIdentity, claims []models.ErrandResultClaim) error {
	stored, err := readErrandReleaseClaims(ctx, tx, identity.TaskID, identity.SessionID, "i.token = ? AND i.state = 'claimed'", claims[0].Token)
	if err != nil {
		return err
	}
	if !slices.Equal(stored, claims) {
		return models.ErrErrandVersionConflict
	}
	return nil
}
