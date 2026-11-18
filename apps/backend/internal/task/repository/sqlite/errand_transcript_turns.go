package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const errandTurnColumns = `t.id, '', e.task_id, t.execution_profile_id, t.route_generation,
 t.started_at, t.completed_at, t.metadata, t.created_at, t.updated_at`

// errandTranscriptOwner validates value-type ownership without creating a TaskSession.
func (r *Repository) errandTranscriptOwner(ctx context.Context, errandID, taskID, sessionID string) (string, error) {
	if sessionID != "" {
		return "", fmt.Errorf("errand transcript cannot reference a task session")
	}
	var owner string
	if err := r.ro.GetContext(ctx, &owner, r.ro.Rebind(`SELECT task_id FROM task_session_errands WHERE id = ?`), errandID); err != nil {
		return "", err
	}
	if taskID != "" && taskID != owner {
		return "", fmt.Errorf("errand transcript task ownership mismatch")
	}
	return owner, nil
}

func (r *Repository) CreateErrandTurn(ctx context.Context, errandID string, turn *models.Turn) error {
	if turn == nil || turn.ID == "" {
		return fmt.Errorf("errand turn identity is required")
	}
	taskID, err := r.errandTranscriptOwner(ctx, errandID, turn.TaskID, turn.TaskSessionID)
	if err != nil {
		return err
	}
	row := *turn
	row.TaskID = taskID
	if err = insertErrandTurn(ctx, r.db, errandID, &row); err == nil {
		*turn = row
	}
	return err
}

type errandTurnWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	Rebind(string) string
}

func insertErrandTurn(ctx context.Context, writer errandTurnWriter, errandID string, row *models.Turn) error {
	stampTurnDefaults(row)
	normalizeErrandTurnTimestamps(row)
	metadata, err := serializeTurnMetadata(row.Metadata)
	if err != nil {
		return err
	}
	_, err = writer.ExecContext(ctx, writer.Rebind(`INSERT INTO errand_turns
 (id,errand_id,execution_profile_id,route_generation,started_at,completed_at,metadata,created_at,updated_at)
 VALUES (?,?,?,?,?,?,?,?,?)`), row.ID, errandID, row.ExecutionProfileID, row.RouteGeneration, row.StartedAt, utcTimeArg(row.CompletedAt), metadata, row.CreatedAt, row.UpdatedAt)
	return err
}

func (r *Repository) GetErrandTurn(ctx context.Context, errandID, turnID string) (*models.Turn, error) {
	return scanTurn(r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT `+errandTurnColumns+`
 FROM errand_turns t JOIN task_session_errands e ON e.id=t.errand_id WHERE t.errand_id=? AND t.id=?`), errandID, turnID))
}

func (r *Repository) UpdateErrandTurn(ctx context.Context, errandID string, turn *models.Turn) error {
	if turn == nil {
		return fmt.Errorf("errand turn is required")
	}
	if _, err := r.errandTranscriptOwner(ctx, errandID, turn.TaskID, turn.TaskSessionID); err != nil {
		return err
	}
	row, err := r.GetErrandTurn(ctx, errandID, turn.ID)
	if err != nil {
		return err
	}
	row.CompletedAt = turn.CompletedAt
	row.Metadata = turn.Metadata
	row.UpdatedAt = r.nowUTC()
	normalizeErrandTurnTimestamps(row)
	metadata, err := serializeTurnMetadata(row.Metadata)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE errand_turns SET completed_at=?,metadata=?,updated_at=? WHERE errand_id=? AND id=?`), utcTimeArg(row.CompletedAt), metadata, row.UpdatedAt, errandID, row.ID)
	if err != nil {
		return err
	}
	if err = errandTranscriptUpdated(result); err == nil {
		*turn = *row
	}
	return err
}

func errandTranscriptUpdated(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListErrandTurns(ctx context.Context, errandID string) ([]*models.Turn, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT `+errandTurnColumns+`
 FROM errand_turns t JOIN task_session_errands e ON e.id=t.errand_id WHERE t.errand_id=? ORDER BY t.created_at,t.id`), errandID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	turns := make([]*models.Turn, 0)
	for rows.Next() {
		turn, scanErr := scanTurn(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		turns = append(turns, turn)
	}
	return turns, rows.Err()
}

func errandTranscriptTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}
func normalizeErrandTurnTimestamps(turn *models.Turn) {
	turn.StartedAt = errandTranscriptTime(turn.StartedAt)
	turn.CreatedAt = errandTranscriptTime(turn.CreatedAt)
	turn.UpdatedAt = errandTranscriptTime(turn.UpdatedAt)
	if turn.CompletedAt != nil {
		completed := errandTranscriptTime(*turn.CompletedAt)
		turn.CompletedAt = &completed
	}
}
