package dialect_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

func TestIsPostgres(t *testing.T) {
	if !dialect.IsPostgres(dialect.PGX) {
		t.Error("expected pgx to be postgres")
	}
	if dialect.IsPostgres(dialect.SQLite3) {
		t.Error("expected sqlite3 to not be postgres")
	}
}

func TestTimestampType(t *testing.T) {
	if got := dialect.TimestampType(dialect.SQLite3); got != "DATETIME" {
		t.Errorf("sqlite: got %q", got)
	}
	if got := dialect.TimestampType(dialect.PGX); got != "TIMESTAMPTZ" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestByteOrderedText(t *testing.T) {
	if got := dialect.ByteOrderedText(dialect.SQLite3, "t.id"); got != "t.id" {
		t.Errorf("sqlite: expected bare expression, got %q", got)
	}
	if got := dialect.ByteOrderedText(dialect.PGX, "t.id"); got != `t.id COLLATE "C"` {
		t.Errorf("pgx: expected explicit C collation, got %q", got)
	}
}

func TestBoolToInt(t *testing.T) {
	if dialect.BoolToInt(true) != 1 {
		t.Error("expected 1 for true")
	}
	if dialect.BoolToInt(false) != 0 {
		t.Error("expected 0 for false")
	}
}

func TestBlobType(t *testing.T) {
	if dialect.BlobType(dialect.SQLite3) != "BLOB" {
		t.Errorf("sqlite: got %q", dialect.BlobType(dialect.SQLite3))
	}
	if dialect.BlobType(dialect.PGX) != "BYTEA" {
		t.Errorf("pgx: got %q", dialect.BlobType(dialect.PGX))
	}
}

func TestByteLength(t *testing.T) {
	if got := dialect.ByteLength(dialect.SQLite3, "name"); got != "length(CAST(name AS BLOB))" {
		t.Errorf("sqlite: got %q", got)
	}
	if got := dialect.ByteLength(dialect.PGX, "name"); got != "octet_length(name)" {
		t.Errorf("postgres: got %q", got)
	}
}

func TestJSONExtract(t *testing.T) {
	got := dialect.JSONExtract(dialect.SQLite3, "metadata", "status")
	if got != "json_extract(metadata, '$.status')" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.JSONExtract(dialect.PGX, "metadata", "status")
	if got != "metadata::jsonb->>'status'" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestJSONExtractPath(t *testing.T) {
	got := dialect.JSONExtractPath(dialect.SQLite3, "m.metadata", "question_id")
	if got != "json_extract(m.metadata, '$.question_id')" {
		t.Errorf("sqlite single segment: got %q", got)
	}
	got = dialect.JSONExtractPath(dialect.PGX, "m.metadata", "question_id")
	if got != "m.metadata::jsonb->>'question_id'" {
		t.Errorf("pgx single segment: got %q", got)
	}

	got = dialect.JSONExtractPath(dialect.SQLite3, "m.metadata", "question", "id")
	if got != "json_extract(m.metadata, '$.question.id')" {
		t.Errorf("sqlite nested: got %q", got)
	}
	got = dialect.JSONExtractPath(dialect.PGX, "m.metadata", "question", "id")
	if got != "m.metadata::jsonb->'question'->>'id'" {
		t.Errorf("pgx nested: got %q", got)
	}
}

func TestJSONTypeIsBoolean(t *testing.T) {
	if got := dialect.JSONTypeIsBoolean(dialect.SQLite3, "summary", "has_running_session"); got != "json_type(summary, '$.has_running_session') IN ('true', 'false')" {
		t.Errorf("sqlite: got %q", got)
	}
	if got := dialect.JSONTypeIsBoolean(dialect.PGX, "summary", "has_running_session"); got != "jsonb_typeof(summary::jsonb->'has_running_session') = 'boolean'" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestJSONTypeAndNumber(t *testing.T) {
	if got := dialect.JSONType(dialect.SQLite3, "metadata", "context_window"); got != "json_type(metadata, '$.context_window')" {
		t.Errorf("sqlite root type: got %q", got)
	}
	if got := dialect.JSONType(dialect.PGX, "metadata", "context_window"); got != "jsonb_typeof(metadata::jsonb #> '{context_window}')" {
		t.Errorf("postgres root type: got %q", got)
	}
	if got := dialect.JSONTypeIsNumber(dialect.SQLite3, "metadata", "context_window", "size"); got != "json_type(metadata, '$.context_window.size') IN ('integer', 'real')" {
		t.Errorf("sqlite number type: got %q", got)
	}
	if got := dialect.JSONTypeIsNumber(dialect.PGX, "metadata", "context_window", "size"); got != "jsonb_typeof(metadata::jsonb #> '{context_window,size}') = 'number'" {
		t.Errorf("postgres number type: got %q", got)
	}
	if got := dialect.JSONTypeIsStringPath(dialect.SQLite3, "metadata", "context_window", "source"); got != "json_type(metadata, '$.context_window.source') = 'text'" {
		t.Errorf("sqlite string type: got %q", got)
	}
	if got := dialect.JSONTypeIsStringPath(dialect.PGX, "metadata", "context_window", "source"); got != "jsonb_typeof(metadata::jsonb #> '{context_window,source}') = 'string'" {
		t.Errorf("postgres string type: got %q", got)
	}
}

func TestJSONExtractIsNotNull(t *testing.T) {
	got := dialect.JSONExtractIsNotNull(dialect.SQLite3, "m", "id")
	if got != "json_extract(m, '$.id') IS NOT NULL" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.JSONExtractIsNotNull(dialect.PGX, "m", "id")
	if got != "m::jsonb->>'id' IS NOT NULL" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestJSONSet(t *testing.T) {
	got := dialect.JSONSet(dialect.SQLite3, "metadata", "status", "complete")
	if got != "json_set(metadata, '$.status', 'complete')" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.JSONSet(dialect.PGX, "metadata", "status", "complete")
	if got != `jsonb_set(metadata::jsonb, '{status}', '"complete"')::text` {
		t.Errorf("pgx: got %q", got)
	}
}

func TestExcludeConfigModePredicate(t *testing.T) {
	got := dialect.ExcludeConfigModePredicate(dialect.SQLite3, "metadata")
	if got != "json_extract(metadata, '$.config_mode') IS NOT 1" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.ExcludeConfigModePredicate(dialect.PGX, "metadata")
	if got != "COALESCE(metadata::jsonb->>'config_mode', '') NOT IN ('true', '1')" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestExcludeTruthyMetadataPredicate(t *testing.T) {
	got := dialect.ExcludeTruthyMetadataPredicate(dialect.SQLite3, "m.metadata", "parent_question")
	if got != "json_extract(m.metadata, '$.parent_question') IS NOT 1" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.ExcludeTruthyMetadataPredicate(dialect.PGX, "m.metadata", "parent_question")
	if got != "COALESCE(m.metadata::jsonb->>'parent_question', '') NOT IN ('true', '1')" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestDurationMs(t *testing.T) {
	got := dialect.DurationMs(dialect.SQLite3, "completed_at", "started_at")
	if got != "(julianday(completed_at) - julianday(started_at)) * 86400000" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.DurationMs(dialect.PGX, "completed_at", "started_at")
	if got != "EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestDateOf(t *testing.T) {
	got := dialect.DateOf(dialect.SQLite3, "created_at")
	if got != "date(created_at)" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.DateOf(dialect.PGX, "created_at")
	if got != "(created_at)::date" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestDateText(t *testing.T) {
	if got := dialect.DateText(dialect.SQLite3, "date"); got != "date(date)" {
		t.Errorf("sqlite: got %q", got)
	}
	if got := dialect.DateText(dialect.PGX, "date"); got != "to_char(date, 'YYYY-MM-DD')" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestDateTimeOf(t *testing.T) {
	got := dialect.DateTimeOf(dialect.SQLite3, "activation.value")
	if got != "datetime(activation.value)" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.DateTimeOf(dialect.PGX, "activation.value")
	if got != "(activation.value)::timestamptz" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestNullableTimestamp(t *testing.T) {
	if got := dialect.NullableTimestamp(dialect.SQLite3, "?"); got != "?" {
		t.Errorf("sqlite: got %q", got)
	}
	if got := dialect.NullableTimestamp(dialect.PGX, "?"); got != "(?)::timestamptz" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestNaiveUTCTimestampOf(t *testing.T) {
	got := dialect.NaiveUTCTimestampOf(dialect.SQLite3, "ts.started_at")
	if got != "datetime(ts.started_at)" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.NaiveUTCTimestampOf(dialect.PGX, "ts.started_at")
	if got != "(ts.started_at AT TIME ZONE 'UTC')" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestSecondPrecisionText(t *testing.T) {
	got := dialect.SecondPrecisionText(dialect.SQLite3, "MAX(c.updated_at)")
	if got != "strftime('%Y-%m-%d %H:%M:%S', MAX(c.updated_at))" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.SecondPrecisionText(dialect.PGX, "MAX(c.updated_at)")
	if got != "to_char(MAX(c.updated_at), 'YYYY-MM-DD HH24:MI:SS')" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestNow(t *testing.T) {
	if dialect.Now(dialect.SQLite3) != "datetime('now')" {
		t.Errorf("sqlite: got %q", dialect.Now(dialect.SQLite3))
	}
	if dialect.Now(dialect.PGX) != "NOW()" {
		t.Errorf("pgx: got %q", dialect.Now(dialect.PGX))
	}
}

func TestNowMinusHours(t *testing.T) {
	got := dialect.NowMinusHours(dialect.SQLite3, "ws.hours")
	if got != "datetime('now', '-' || ws.hours || ' hours')" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.NowMinusHours(dialect.PGX, "ws.hours")
	if got != "NOW() - (ws.hours || ' hours')::interval" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestGreatestTimestamp(t *testing.T) {
	got := dialect.GreatestTimestamp(dialect.SQLite3, "t.updated_at", "MAX(ts.updated_at)")
	if got != "max(t.updated_at, MAX(ts.updated_at))" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.GreatestTimestamp(dialect.PGX, "t.updated_at", "MAX(ts.updated_at)")
	if got != "GREATEST(t.updated_at, MAX(ts.updated_at))" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestCurrentDate(t *testing.T) {
	if dialect.CurrentDate(dialect.SQLite3) != "date('now')" {
		t.Errorf("sqlite: got %q", dialect.CurrentDate(dialect.SQLite3))
	}
	if dialect.CurrentDate(dialect.PGX) != "(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date" {
		t.Errorf("pgx: got %q", dialect.CurrentDate(dialect.PGX))
	}
}

func TestDateNowMinusDays(t *testing.T) {
	got := dialect.DateNowMinusDays(dialect.SQLite3, "?")
	if got != "date('now', '-' || ? || ' days')" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.DateNowMinusDays(dialect.PGX, "?")
	if got != "(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date - (?::int)" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestDatePlusOneDay(t *testing.T) {
	got := dialect.DatePlusOneDay(dialect.SQLite3, "date")
	if got != "date(date, '+1 day')" {
		t.Errorf("sqlite: got %q", got)
	}
	got = dialect.DatePlusOneDay(dialect.PGX, "date")
	if got != "(date)::date + 1" {
		t.Errorf("pgx: got %q", got)
	}
}

func TestLike(t *testing.T) {
	if dialect.Like(dialect.SQLite3) != "LIKE" {
		t.Errorf("sqlite: got %q", dialect.Like(dialect.SQLite3))
	}
	if dialect.Like(dialect.PGX) != "ILIKE" {
		t.Errorf("pgx: got %q", dialect.Like(dialect.PGX))
	}
}

func TestInsertReturningID_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	rawDB, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlxDB := sqlx.NewDb(rawDB, dialect.SQLite3)
	t.Cleanup(func() { _ = sqlxDB.Close() })

	_, err = sqlxDB.Exec(`CREATE TABLE test_insert (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	id, err := dialect.InsertReturningID(context.Background(), sqlxDB, `INSERT INTO test_insert (name) VALUES (?)`, "hello")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id != 1 {
		t.Errorf("expected id 1, got %d", id)
	}

	id, err = dialect.InsertReturningID(context.Background(), sqlxDB, `INSERT INTO test_insert (name) VALUES (?)`, "world")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if id != 2 {
		t.Errorf("expected id 2, got %d", id)
	}
}
