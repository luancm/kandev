package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	errBrowserHistoryLimit     = 25
	errBrowserCursorOrder      = "created_at_desc_id_asc"
	errBrowserHistoryIndexName = "idx_task_session_errands_browser_history_normalized_v3"
	errBrowserEpochOrderKey    = "1970-01-01 00:00:00.000000000"
)

// GetErrandBrowserSummary returns only live or otherwise-outstanding rows;
// counts are computed in the same read transaction and do not depend on those rows.
func (r *Repository) GetErrandBrowserSummary(ctx context.Context, taskID string) (models.ErrandBrowserSummary, error) {
	if taskID == "" {
		return models.ErrandBrowserSummary{}, errors.New("task id is required")
	}
	tx, err := r.beginErrandBrowserReadSnapshot(ctx)
	if err != nil {
		return models.ErrandBrowserSummary{}, fmt.Errorf("begin errand browser summary: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	counts, err := readErrandBrowserCounts(ctx, tx, r.ro.DriverName(), `e.task_id = ?`, taskID)
	if err != nil {
		return models.ErrandBrowserSummary{}, err
	}
	parentCounts, err := readErrandParentBrowserCounts(ctx, tx, r.ro.DriverName(), taskID)
	if err != nil {
		return models.ErrandBrowserSummary{}, err
	}
	errands, err := readErrandBrowserSummaryRows(ctx, tx, r.ro.DriverName(), taskID)
	if err != nil {
		return models.ErrandBrowserSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ErrandBrowserSummary{}, fmt.Errorf("commit errand browser summary: %w", err)
	}
	return models.ErrandBrowserSummary{TaskID: taskID, Errands: errands, Counts: counts, ParentCounts: parentCounts}, nil
}

func readErrandBrowserSummaryRows(ctx context.Context, tx *sqlx.Tx, driver, taskID string) ([]*models.ErrandBrowserObservation, error) {
	createdAtKey := errandBrowserCreatedAtKey(driver, "e.created_at")
	orderedID := dialect.ByteOrderedText(driver, "e.id")
	rows, err := tx.QueryContext(ctx, tx.Rebind(`SELECT `+errandBrowserColumns(driver)+`
		FROM task_session_errands e
		LEFT JOIN task_sessions s ON s.id = e.id
		WHERE e.task_id = ? AND `+validErrandBrowserIdentity()+` AND `+errandBrowserSummaryMembershipPredicate(driver)+`
		ORDER BY `+createdAtKey+` ASC, `+orderedID+` ASC`), taskID)
	if err != nil {
		return nil, fmt.Errorf("query errand browser summary rows: %w", err)
	}
	errands, err := scanErrandBrowserRows(rows)
	if err != nil {
		return nil, fmt.Errorf("scan errand browser summary rows: %w", err)
	}
	return errands, nil
}

// GetErrandHistoryPage reads at most limit+1 compact rows before scanning them.
func (r *Repository) GetErrandHistoryPage(ctx context.Context, request models.ErrandHistoryPageRequest) (models.ErrandHistoryPage, error) {
	if err := validateErrandBrowserHistoryRequest(request); err != nil {
		return models.ErrandHistoryPage{}, err
	}
	tx, err := r.beginErrandBrowserReadSnapshot(ctx)
	if err != nil {
		return models.ErrandHistoryPage{}, fmt.Errorf("begin errand history page: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	total, err := countFinishedErrandHistory(ctx, tx, r.ro.DriverName(), request)
	if err != nil {
		return models.ErrandHistoryPage{}, err
	}
	page, err := readFinishedErrandHistoryPage(ctx, tx, r.ro.DriverName(), request, total)
	if err != nil {
		return models.ErrandHistoryPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ErrandHistoryPage{}, fmt.Errorf("commit errand history page: %w", err)
	}
	return page, nil
}

func (r *Repository) beginErrandBrowserReadSnapshot(ctx context.Context) (*sqlx.Tx, error) {
	options := &sql.TxOptions{ReadOnly: true}
	if dialect.IsPostgres(r.ro.DriverName()) {
		options.Isolation = sql.LevelRepeatableRead
	}
	return r.ro.BeginTxx(ctx, options)
}

func validateErrandBrowserHistoryRequest(request models.ErrandHistoryPageRequest) error {
	if request.TaskID == "" || request.ParentSessionID == "" || request.Limit < 1 || request.Limit > errBrowserHistoryLimit {
		return errors.New("invalid errand history query")
	}
	cursor := request.Cursor
	if cursor != nil && (cursor.Version != 1 || cursor.TaskID != request.TaskID ||
		cursor.ParentSessionID != request.ParentSessionID || cursor.Order != errBrowserCursorOrder ||
		cursor.ID == "") {
		return errors.New("invalid errand history cursor")
	}
	return nil
}

func countFinishedErrandHistory(ctx context.Context, tx *sqlx.Tx, driver string, request models.ErrandHistoryPageRequest) (int, error) {
	query := `SELECT COUNT(*) FROM task_session_errands e
		LEFT JOIN task_sessions s ON s.id = e.id
		WHERE e.task_id = ? AND e.parent_session_id = ? AND ` + validErrandBrowserIdentity() + ` AND ` + errandBrowserFinishedPredicate(driver)
	var total int
	if err := tx.QueryRowContext(ctx, tx.Rebind(query), request.TaskID, request.ParentSessionID).Scan(&total); err != nil {
		return 0, fmt.Errorf("count finished errand history: %w", err)
	}
	return total, nil
}

func readFinishedErrandHistoryPage(ctx context.Context, tx *sqlx.Tx, driver string, request models.ErrandHistoryPageRequest, total int) (models.ErrandHistoryPage, error) {
	query, args := errandBrowserHistoryPageQuery(driver, request)
	rows, err := tx.QueryContext(ctx, tx.Rebind(query), args...)
	if err != nil {
		return models.ErrandHistoryPage{}, fmt.Errorf("query finished errand history: %w", err)
	}
	items, err := scanErrandBrowserRows(rows)
	if err != nil {
		return models.ErrandHistoryPage{}, fmt.Errorf("scan finished errand history: %w", err)
	}
	page := models.ErrandHistoryPage{TotalFinished: total}
	if len(items) > request.Limit {
		page.HasMore = true
		items = items[:request.Limit]
	}
	page.Errands = items
	if page.HasMore && len(items) > 0 {
		last := items[len(items)-1]
		page.NextCursor = &models.ErrandHistoryCursor{
			Version: 1, TaskID: request.TaskID, ParentSessionID: request.ParentSessionID,
			Order: errBrowserCursorOrder, CreatedAt: last.CreatedAt, ID: last.ID,
		}
	}
	return page, nil
}

func errandBrowserHistoryPageQuery(driver string, request models.ErrandHistoryPageRequest) (string, []interface{}) {
	createdAtKey := errandBrowserCreatedAtKey(driver, "e.created_at")
	orderedID := dialect.ByteOrderedText(driver, "e.id")
	query := `SELECT ` + errandBrowserColumns(driver) + `
		FROM task_session_errands e LEFT JOIN task_sessions s ON s.id = e.id
		WHERE e.task_id = ? AND e.parent_session_id = ? AND ` + validErrandBrowserIdentity() + ` AND ` + errandBrowserFinishedPredicate(driver)
	args := []interface{}{request.TaskID, request.ParentSessionID}
	if request.Cursor != nil {
		cursorKey := errandBrowserHistoryCursorKey(driver, request.Cursor.CreatedAt)
		query += ` AND (` + createdAtKey + ` < ? OR (` + createdAtKey + ` = ? AND ` + orderedID + ` > ?))`
		args = append(args, cursorKey, cursorKey, request.Cursor.ID)
	}
	query += ` ORDER BY ` + createdAtKey + ` DESC, ` + orderedID + ` ASC LIMIT ?`
	args = append(args, request.Limit+1)
	return query, args
}

func errandBrowserHistoryCursorKey(driver string, createdAt time.Time) interface{} {
	createdAt = createdAt.UTC()
	if dialect.IsPostgres(driver) {
		return createdAt
	}
	return createdAt.Format("2006-01-02 15:04:05.000000000")
}

func errandBrowserHistoryIndexDDL(driver string) string {
	if dialect.IsPostgres(driver) {
		return `CREATE INDEX IF NOT EXISTS ` + errBrowserHistoryIndexName + `
			ON task_session_errands(task_id, parent_session_id, created_at DESC, id COLLATE "C" ASC)`
	}
	return `CREATE INDEX IF NOT EXISTS ` + errBrowserHistoryIndexName + `
		ON task_session_errands(task_id, parent_session_id, (` + errandBrowserCreatedAtKey(driver, "created_at") + `) DESC, id ASC)`
}

func errandBrowserCreatedAtKey(driver, expression string) string {
	if dialect.IsPostgres(driver) {
		return expression
	}
	expression = errandBrowserPadSingleDigitHour(expression)
	rest := "substr(" + expression + ", 21)"
	digits := `CASE
		WHEN instr(` + rest + `, '+') > 0 THEN substr(` + expression + `, 21, instr(` + rest + `, '+') - 1)
		WHEN instr(` + rest + `, '-') > 0 THEN substr(` + expression + `, 21, instr(` + rest + `, '-') - 1)
		WHEN instr(` + rest + `, 'Z') > 0 THEN substr(` + expression + `, 21, instr(` + rest + `, 'Z') - 1)
		ELSE ` + rest + ` END`
	fraction := `CASE WHEN substr(` + expression + `, 20, 1) IN ('.', ',') THEN (` + digits + `) ELSE '' END`
	zuluZone := `(substr(` + expression + `, -1, 1) = 'Z' AND (
		length(` + expression + `) = 20 OR (substr(` + expression + `, 20, 1) IN ('.', ',') AND
		length(` + rest + `) > 1 AND substr(` + rest + `, 1, length(` + rest + `) - 1) NOT GLOB '*[^0-9]*')))`
	numericZone := `(
		substr(` + expression + `, -6, 1) IN ('+', '-') AND
		substr(` + expression + `, -5, 2) GLOB '[0-9][0-9]' AND
		substr(` + expression + `, -3, 1) = ':' AND
		substr(` + expression + `, -2, 2) GLOB '[0-9][0-9]' AND
		CAST(substr(` + expression + `, -5, 2) AS INTEGER) <= 24 AND
		CAST(substr(` + expression + `, -2, 2) AS INTEGER) <= 60 AND
		((substr(` + expression + `, 20, 1) IN ('.', ',') AND length(` + rest + `) > 6 AND
			(instr(` + rest + `, '+') = length(` + rest + `) - 5 OR instr(` + rest + `, '-') = length(` + rest + `) - 5)) OR
		 (substr(` + expression + `, 20, 1) NOT IN ('.', ',') AND length(` + expression + `) = 25)))`
	timezone := `(` + zuluZone + ` OR ` + numericZone + `)`
	legacyZoneLess := `(substr(` + expression + `, 11, 1) = ' ' AND (
		length(` + expression + `) = 19 OR (substr(` + expression + `, 20, 1) IN ('.', ',') AND
		length(` + rest + `) > 0 AND (` + rest + `) NOT GLOB '*[^0-9]*')))`
	zone := `(` + timezone + ` OR ` + legacyZoneLess + `)`
	offsetSign := `CASE WHEN substr(` + expression + `, -6, 1) = '+' THEN '-' ELSE '+' END`
	offsetHours := `(` + offsetSign + `) || CAST(substr(` + expression + `, -5, 2) AS INTEGER) || ' hours'`
	offsetMinutes := `(` + offsetSign + `) || CAST(substr(` + expression + `, -2, 2) AS INTEGER) || ' minutes'`
	wholeSecond := `CASE
		WHEN substr(` + expression + `, -1, 1) = 'Z' THEN datetime(substr(` + expression + `, 1, 19))
		WHEN substr(` + expression + `, -6, 1) IN ('+', '-') THEN datetime(substr(` + expression + `, 1, 19), ` + offsetHours + `, ` + offsetMinutes + `)
		ELSE datetime(substr(` + expression + `, 1, 19)) END`
	valid := `typeof(` + expression + `) = 'text' AND
		substr(` + expression + `, 1, 10) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]' AND
		substr(` + expression + `, 11, 1) IN ('T', ' ') AND
		substr(` + expression + `, 12, 8) GLOB '[0-9][0-9]:[0-9][0-9]:[0-9][0-9]' AND
		substr(` + expression + `, 12, 2) BETWEEN '00' AND '23' AND
		substr(` + expression + `, 15, 2) BETWEEN '00' AND '59' AND
		substr(` + expression + `, 18, 2) BETWEEN '00' AND '59' AND
		strftime('%Y-%m-%d', julianday(substr(` + expression + `, 1, 10))) = substr(` + expression + `, 1, 10) AND
		` + zone + ` AND ` + wholeSecond + ` IS NOT NULL AND
		(substr(` + expression + `, 20, 1) NOT IN ('.', ',') OR (length(` + fraction + `) > 0 AND (` + fraction + `) NOT GLOB '*[^0-9]*'))`
	return `CASE WHEN ` + valid + ` THEN substr(` + wholeSecond + `, 1, 19) || '.' || substr((` + fraction + `) || '000000000', 1, 9) ELSE '` + errBrowserEpochOrderKey + `' END`
}

func errandBrowserPadSingleDigitHour(expression string) string {
	return `CASE
		WHEN typeof(` + expression + `) = 'text' AND
			substr(` + expression + `, 1, 10) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]' AND
			substr(` + expression + `, 11, 1) IN ('T', ' ') AND
			substr(` + expression + `, 12, 1) GLOB '[0-9]' AND substr(` + expression + `, 13, 1) = ':'
		THEN substr(` + expression + `, 1, 11) || '0' || substr(` + expression + `, 12)
		ELSE ` + expression + ` END`
}

// GetErrandInspection reads one compact Errand row and its parent's hosting identity.
func (r *Repository) GetErrandInspection(ctx context.Context, taskID, childSessionID string) (models.ErrandBrowserInspection, error) {
	if taskID == "" || childSessionID == "" {
		return models.ErrandBrowserInspection{}, errors.New("task and child ids are required")
	}
	tx, err := r.beginErrandBrowserReadSnapshot(ctx)
	if err != nil {
		return models.ErrandBrowserInspection{}, fmt.Errorf("begin errand inspection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	query := `SELECT ` + errandBrowserColumns(r.ro.DriverName()) + `,
		CASE WHEN ` + ownedErrandExists + ` THEN 1 ELSE 0 END
		FROM task_session_errands e LEFT JOIN task_sessions s ON s.id = e.id
		WHERE e.task_id = ? AND e.id = ? AND ` + validErrandBrowserIdentity()
	row := tx.QueryRowContext(ctx, tx.Rebind(query), taskID, childSessionID)
	var ownsConversation int
	projection, err := scanErrandBrowserProjection(row, &ownsConversation)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrandBrowserInspection{}, models.ErrErrandNotFound
	}
	if err != nil {
		return models.ErrandBrowserInspection{}, fmt.Errorf("read errand inspection row: %w", err)
	}
	parent := models.ErrandBrowserParentIdentity{}
	var parentID, parentTitle, parentRole, environmentID, taskEnvironmentID string
	parentErr := tx.QueryRowContext(ctx, tx.Rebind(`SELECT id, COALESCE(name, ''), COALESCE(role, ''),
		COALESCE(environment_id, ''), COALESCE(task_environment_id, '') FROM task_sessions
		WHERE id = ? AND task_id = ?`), projection.ParentSessionID, taskID).Scan(
		&parentID, &parentTitle, &parentRole, &environmentID, &taskEnvironmentID,
	)
	if parentErr == nil {
		parent = models.ErrandBrowserParentIdentity{
			Available: true, ID: parentID, Title: parentTitle, Role: models.SessionRole(parentRole),
			EnvironmentID: environmentID, TaskEnvironmentID: taskEnvironmentID,
		}
	} else if !errors.Is(parentErr, sql.ErrNoRows) {
		return models.ErrandBrowserInspection{}, fmt.Errorf("read errand parent identity: %w", parentErr)
	}
	if err := tx.Commit(); err != nil {
		return models.ErrandBrowserInspection{}, fmt.Errorf("commit errand inspection: %w", err)
	}
	return models.ErrandBrowserInspection{
		Errand: projection, OwnsConversation: ownsConversation != 0, Parent: parent,
	}, nil
}

func readErrandBrowserCounts(ctx context.Context, tx *sqlx.Tx, driver, scope string, args ...interface{}) (models.ErrandBrowserCounts, error) {
	live := liveErrandPredicate(driver)
	queued := `e.lifecycle = 'queued'`
	outstanding := errandBrowserOutstandingPredicate(driver)
	finished := errandBrowserFinishedPredicate(driver)
	pending := `e.result_state = 'pending'`
	unread := errandBrowserUnreadPredicate(driver)
	query := `SELECT
		COALESCE(SUM(CASE WHEN ` + live + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + queued + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + outstanding + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + finished + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + pending + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + unread + ` THEN 1 ELSE 0 END), 0)
		FROM task_session_errands e LEFT JOIN task_sessions s ON s.id = e.id
		WHERE ` + scope + ` AND ` + validErrandBrowserIdentity()
	var counts models.ErrandBrowserCounts
	err := tx.QueryRowContext(ctx, tx.Rebind(query), args...).Scan(
		&counts.Live, &counts.Queued, &counts.Outstanding, &counts.Finished,
		&counts.PendingResults, &counts.Unread,
	)
	if err != nil {
		return models.ErrandBrowserCounts{}, fmt.Errorf("count errand browser rows: %w", err)
	}
	return counts, nil
}

func readErrandParentBrowserCounts(ctx context.Context, tx *sqlx.Tx, driver, taskID string) ([]models.ErrandParentBrowserCounts, error) {
	query := `SELECT e.parent_session_id,
		COALESCE(SUM(CASE WHEN ` + liveErrandPredicate(driver) + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN e.lifecycle = 'queued' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + errandBrowserOutstandingPredicate(driver) + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + errandBrowserFinishedPredicate(driver) + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN e.result_state = 'pending' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + errandBrowserUnreadPredicate(driver) + ` THEN 1 ELSE 0 END), 0)
		FROM task_session_errands e LEFT JOIN task_sessions s ON s.id = e.id
		WHERE e.task_id = ? AND ` + validErrandBrowserIdentity() + `
		GROUP BY e.parent_session_id ORDER BY e.parent_session_id`
	rows, err := tx.QueryContext(ctx, tx.Rebind(query), taskID)
	if err != nil {
		return nil, fmt.Errorf("query errand parent totals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	counts := make([]models.ErrandParentBrowserCounts, 0)
	for rows.Next() {
		var parent models.ErrandParentBrowserCounts
		if err := rows.Scan(&parent.ParentSessionID, &parent.Counts.Live, &parent.Counts.Queued,
			&parent.Counts.Outstanding, &parent.Counts.Finished, &parent.Counts.PendingResults, &parent.Counts.Unread); err != nil {
			return nil, err
		}
		counts = append(counts, parent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate errand parent totals: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close errand parent totals: %w", err)
	}
	return counts, nil
}

func errandBrowserOutstandingPredicate(driver string) string {
	return `(e.lifecycle = 'queued' OR e.awaiting_result OR e.result_state = 'pending' OR COALESCE((` + recoveryAttentionPredicate(driver) + `), FALSE))`
}

func errandBrowserSummaryMembershipPredicate(driver string) string {
	return `(` + liveErrandPredicate(driver) + ` OR ` + errandBrowserOutstandingPredicate(driver) + `)`
}

func errandBrowserFinishedPredicate(driver string) string {
	return `(NOT (` + liveErrandPredicate(driver) + `) AND NOT (` + errandBrowserOutstandingPredicate(driver) + `))`
}

func validErrandBrowserIdentity() string {
	return `(s.id IS NULL OR (s.task_id = e.task_id AND s.role = 'errand' AND s.parent_session_id = e.parent_session_id))`
}

func errandBrowserColumns(driver string) string {
	createdAtKey := errandBrowserCreatedAtKey(driver, "e.created_at")
	return `e.id, e.task_id, e.parent_session_id, e.title, e.agent_profile_id,
		` + errandBrowserStatusExpression(driver) + `,
		` + errandBrowserAttentionExpression(driver) + `,
		e.result_state, ` + errandAwaitProjectionFor("e") + `, e.result_truncated,
		` + createdAtKey + ` AS created_at, e.updated_at, e.settled_at, e.version,
	` + liveErrandPredicate(driver) + `,
	` + errandBrowserOutstandingPredicate(driver) + `,
	` + errandBrowserUnreadPredicate(driver) + `,
	e.latest_settled_turn_id, e.result_turn_id,
	` + errandContextMetadataColumn
}

func errandBrowserStatusExpression(driver string) string {
	permission := errandBrowserPermissionPredicate(driver)
	recovery := recoveryAttentionPredicate(driver)
	return `CASE
		WHEN e.lifecycle = 'cancelled' THEN 'cancelled'
		WHEN e.lifecycle = 'queued' THEN 'queued'
		WHEN ` + ownedErrandExists + ` THEN CASE
			WHEN ` + ownedErrandWorking(driver) + ` AND ` + ownedErrandPermission(driver) + ` THEN 'needs_attention'
			WHEN ` + ownedErrandRecoveryAttention(driver) + ` THEN 'needs_attention'
			WHEN ` + ownedErrandWorking(driver) + ` THEN 'running'
			ELSE e.last_outcome END
		WHEN s.id IS NULL OR s.state = 'FAILED' THEN 'failed'
		WHEN s.state IN ('STARTING', 'RUNNING') AND ` + permission + ` THEN 'needs_attention'
		WHEN s.state IN ('STARTING', 'RUNNING') THEN 'running'
		WHEN ` + recovery + ` THEN 'needs_attention'
		WHEN e.last_outcome = 'done' THEN 'done'
		WHEN e.last_outcome = 'failed' THEN 'failed'
		ELSE 'running' END`
}

func errandBrowserAttentionExpression(driver string) string {
	return `CASE
		WHEN ` + ownedErrandWorking(driver) + ` AND ` + ownedErrandPermission(driver) + ` THEN 'permission'
		WHEN s.state IN ('STARTING', 'RUNNING') AND ` + errandBrowserPermissionPredicate(driver) + ` THEN 'permission'
		WHEN ` + recoveryAttentionPredicate(driver) + ` THEN 'recovery_unresolved' ELSE '' END`
}

func ownedErrandPermission(driver string) string {
	field := func(path ...string) string { return dialect.JSONExtractPath(driver, "m.payload", path...) }
	return `EXISTS (SELECT 1 FROM errand_execution_generations g
		JOIN errand_messages m ON m.errand_id=g.errand_id AND m.turn_id=g.active_turn_id
		WHERE g.errand_id=e.id AND g.active_submission_id<>'' AND g.generation=(
			SELECT MAX(latest.generation) FROM errand_execution_generations latest WHERE latest.errand_id=e.id)
		AND ` + field("author_id") + `=g.active_submission_id AND ` + field("type") + `='permission_request'
		AND COALESCE(` + field("metadata", "status") + `,'') IN ('','pending')
		AND COALESCE(` + field("metadata", models.PermissionPresentationMetadataKey) + `,'') NOT IN ('automatic','unavailable'))`
}

func errandBrowserPermissionPredicate(driver string) string {
	turnPredicate, turnOrder := currentTurnAuthority(driver, "turn_row")
	status := dialect.JSONExtract(driver, "m.metadata", "status")
	presentation := dialect.JSONExtract(driver, "m.metadata", models.PermissionPresentationMetadataKey)
	permissionOrder := pendingActionMessageOrder(driver, "pm")
	clarificationStatus := dialect.JSONExtract(driver, "clarification.metadata", "status")
	currentTurn := `(SELECT turn_row.id FROM task_session_turns turn_row
		WHERE turn_row.task_session_id = s.id AND ` + turnPredicate + `
		AND ` + nonTerminalSessionPredicate("turn_row") + ` ORDER BY ` + turnOrder + ` LIMIT 1)`
	return `EXISTS (SELECT 1 FROM task_session_messages m
		WHERE m.task_session_id = s.id AND m.type = 'permission_request'
		AND m.turn_id = ` + currentTurn + `
		AND m.id = (SELECT pm.id FROM task_session_messages pm
			WHERE pm.task_session_id = s.id AND pm.turn_id = m.turn_id AND pm.type = 'permission_request'
			ORDER BY pm.created_at DESC, ` + permissionOrder + ` DESC LIMIT 1)
		AND COALESCE(` + status + `, '') IN ('', 'pending')
		AND COALESCE(` + presentation + `, '') NOT IN ('automatic', 'unavailable')
		AND NOT EXISTS (SELECT 1 FROM task_session_messages clarification
			WHERE clarification.task_session_id = s.id AND clarification.type = 'clarification_request'
			AND clarification.turn_id = ` + currentTurn + `
			AND COALESCE(` + clarificationStatus + `, '') IN ('', 'pending')))`
}

func errandAwaitProjectionFor(alias string) string {
	return `CASE WHEN ` + alias + `.result_state='delivered' AND ` + alias + `.delivered_route='await'
	 AND ` + alias + `.result_revision<>'' THEN COALESCE((
	 SELECT CASE
	  WHEN a.state='received' AND a.receipt_sequence>0 THEN 'received'
	  WHEN a.state='uncertain' OR a.parent_incarnation_id='' OR a.harness_generation=0
	   OR (a.state='received' AND a.receipt_sequence=0) THEN 'uncertain'
	  ELSE '' END
	 FROM errand_await_attempt a JOIN errand_await_attempt_item i ON i.token=a.token
	 WHERE a.token=` + alias + `.delivery_token
	  AND a.task_id=` + alias + `.task_id AND a.parent_session_id=` + alias + `.parent_session_id
	  AND i.errand_id=` + alias + `.id AND i.result_revision=` + alias + `.result_revision
	  AND i.parent_admission_generation=` + alias + `.parent_admission_generation
	 ), CASE WHEN NOT EXISTS (SELECT 1 FROM errand_await_attempt a WHERE a.token=` + alias + `.delivery_token)
	 THEN 'uncertain' ELSE '' END) ELSE '' END`
}

func scanErrandBrowserRows(rows *sql.Rows) ([]*models.ErrandBrowserObservation, error) {
	defer func() { _ = rows.Close() }()
	items := make([]*models.ErrandBrowserObservation, 0)
	for rows.Next() {
		item, err := scanErrandBrowserProjection(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return items, nil
}

func scanErrandBrowserProjection(row errandRowScanner, extra ...interface{}) (*models.ErrandBrowserObservation, error) {
	item := &models.ErrandBrowserObservation{}
	var (
		status, attention, resultState, deliveryState string
		rawCreatedAt                                  interface{}
		settledAt                                     sql.NullTime
		live, outstanding, unread                     bool
		latestSettled, resultTurn                     string
		contextMetadata                               sql.NullString
	)
	destinations := []interface{}{
		&item.ID, &item.TaskID, &item.ParentSessionID, &item.Title, &item.AgentProfileID,
		&status, &attention, &resultState, &deliveryState, &item.ResultTruncated,
		&rawCreatedAt, &item.UpdatedAt, &settledAt, &item.Version,
		&live, &outstanding, &unread, &latestSettled, &resultTurn,
		&contextMetadata,
	}
	destinations = append(destinations, extra...)
	if err := row.Scan(destinations...); err != nil {
		return nil, err
	}
	item.CreatedAt = normalizeErrandBrowserCreatedAt(rawCreatedAt)
	item.Status = models.ErrandStatus(status)
	item.AttentionReason = models.ProjectErrandAttentionReason(item.Status, models.ErrandAttentionReason(attention))
	item.ResultState = models.ErrandResultState(resultState)
	item.DeliveryState = models.ErrandDeliveryState(deliveryState)
	item.Live, item.Outstanding, item.Unread = live, outstanding, unread
	if settledAt.Valid {
		item.SettledAt = &settledAt.Time
	}
	item.CanSendResult = (&models.Errand{
		LatestSettledTurnID: latestSettled, ResultTurnID: resultTurn, ResultState: item.ResultState,
	}).CanSendResult(item.Status)
	var err error
	item.ContextMetadata, err = decodeErrandContextMetadata(contextMetadata.String)
	return item, err
}

func normalizeErrandBrowserCreatedAt(value interface{}) time.Time {
	createdAt, err := parseTaskActivityTime(value)
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return createdAt.UTC()
}
