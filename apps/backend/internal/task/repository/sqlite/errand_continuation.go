package sqlite

import (
	"context"
	"database/sql"

	"github.com/kandev/kandev/internal/db"
)

// ValidateErrandContinuation is a read-only preflight. The reservation writer
// repeats this predicate under the owning transaction before creating a turn.
func (r *Repository) ValidateErrandContinuation(ctx context.Context, sessionID string) error {
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	return db.RequireErrandTurnSettlements(ctx, tx, sessionID, "")
}
