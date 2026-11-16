package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// Read reception and the current claim in one SQL snapshot. An old receipt
// cannot attach to a replacement revision, token, or admission generation.
const errandAwaitProjection = `CASE WHEN result_state='delivered' AND delivered_route='await'
 AND result_revision<>'' THEN COALESCE((
 SELECT CASE
  WHEN a.state='received' AND a.receipt_sequence>0 THEN 'received'
  WHEN a.state='uncertain' OR a.parent_incarnation_id='' OR a.harness_generation=0
   OR (a.state='received' AND a.receipt_sequence=0) THEN 'uncertain'
  ELSE '' END
 FROM errand_await_attempt a JOIN errand_await_attempt_item i ON i.token=a.token
 WHERE a.token=task_session_errands.delivery_token
  AND a.task_id=task_session_errands.task_id AND a.parent_session_id=task_session_errands.parent_session_id
  AND i.errand_id=task_session_errands.id AND i.result_revision=task_session_errands.result_revision
  AND i.parent_admission_generation=task_session_errands.parent_admission_generation
 ), CASE WHEN NOT EXISTS (SELECT 1 FROM errand_await_attempt a WHERE a.token=task_session_errands.delivery_token)
 THEN 'uncertain' ELSE '' END) ELSE '' END`

func scanProjectedErrand(row errandRowScanner) (*models.Errand, error) {
	var state models.ErrandDeliveryState
	errand, err := scanErrand(row, &state)
	if err != nil {
		return nil, err
	}
	errand.DeliveryState = state
	return errand, nil
}

// Mutation responses use the same projection as list/detail, inside their
// existing write transaction, rather than a field copied from an older read.
func readWrittenErrandDeliveryState(ctx context.Context, tx *sqlx.Tx, row *models.Errand) error {
	return tx.QueryRowContext(ctx, tx.Rebind(`SELECT `+errandAwaitProjection+` FROM task_session_errands WHERE id=?`), row.ID).Scan(&row.DeliveryState)
}
