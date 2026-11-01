package sqlite

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

var errandTeardownRequestedAt = time.Date(2026, time.November, 1, 12, 30, 0, 0, time.UTC)

func TestErrandTeardownIntentRoundTripsAndSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teardown-intent.db")
	repo := newFileErrandRepo(t, path)
	row := persistErrandTeardownIntent(t, repo, "reopen")
	if err := repo.db.Close(); err != nil {
		t.Fatalf("close file database: %v", err)
	}

	reopened := newFileErrandRepo(t, path)
	got, err := reopened.GetErrand(t.Context(), row.ID)
	if err != nil {
		t.Fatalf("GetErrand after reopen: %v", err)
	}
	assertErrandFields(t, got, row)
}

func TestErrandTeardownIntentMigrationReplays(t *testing.T) {
	exerciseErrandTeardownIntentMigration(t, newRepoForSessionTests(t))
}

func TestErrandTeardownIntentMalformedJSONAndExplicitEmpty(t *testing.T) {
	exerciseErrandTeardownIntentMalformedJSONAndExplicitEmpty(t, newRepoForSessionTests(t))
}

func exerciseErrandTeardownIntentMalformedJSONAndExplicitEmpty(t *testing.T, repo *Repository) {
	t.Helper()
	row := createTeardownIntentErrand(t, repo, "malformed")
	legacy, err := repo.GetErrand(t.Context(), row.ID)
	if err != nil || legacy.TeardownIntent != nil {
		t.Fatalf("empty legacy intent = %+v, err = %v", legacy, err)
	}
	if err := setRawTeardownIntent(t, repo, row.ID, "{"); err != nil {
		t.Fatalf("write malformed JSON: %v", err)
	}
	if _, err := repo.GetErrand(t.Context(), row.ID); err == nil {
		t.Fatal("malformed teardown intent JSON was accepted")
	}

	partial := pendingSessionTeardownIntent(row)
	partial.Target.SessionIncarnation = ""
	encoded, err := json.Marshal(partial)
	if err != nil {
		t.Fatalf("marshal partial target: %v", err)
	}
	if err := setRawTeardownIntent(t, repo, row.ID, string(encoded)); err != nil {
		t.Fatalf("write partial target: %v", err)
	}
	if _, err := repo.GetErrand(t.Context(), row.ID); err == nil {
		t.Fatal("partial teardown target was accepted")
	}
}

func TestErrandCancellationAndStopIntentCommitTogether(t *testing.T) {
	exerciseErrandCancellationAndStopIntentCommitTogether(t, newRepoForSessionTests(t))
}

func TestErrandTeardownIntentCASDoesNotOverwriteSuccessor(t *testing.T) {
	exerciseErrandTeardownIntentCASDoesNotOverwriteSuccessor(t, newRepoForSessionTests(t))
}

func exerciseErrandTeardownIntentCASDoesNotOverwriteSuccessor(t *testing.T, repo *Repository) {
	t.Helper()
	row := createTeardownIntentErrand(t, repo, "cas")
	row.Lifecycle, row.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
	row.TeardownIntent = pendingSessionTeardownIntent(row)
	if err := repo.UpdateErrand(t.Context(), row); err != nil {
		t.Fatalf("commit pending intent: %v", err)
	}
	stale := *row
	successor := *row
	successor.ResultText, successor.ResultKind, successor.ResultState = "successor result", models.ErrandResultOutput, models.ErrandResultPending
	successor.ResultTurnID, successor.LatestSettledTurnID = "turn-successor", "turn-successor"
	if err := repo.UpdateErrand(t.Context(), &successor); err != nil {
		t.Fatalf("store successor result: %v", err)
	}

	if _, err := repo.TransitionErrandTeardownIntent(t.Context(), row.ID, row.TeardownIntent.ID, stale.Version, models.ErrandTeardownIntentCompleted); !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("stale teardown CAS = %v, want ErrErrandVersionConflict", err)
	}
	current, err := repo.GetErrand(t.Context(), row.ID)
	if err != nil {
		t.Fatalf("GetErrand after stale CAS: %v", err)
	}
	if current.ResultText != "successor result" || current.ResultTurnID != "turn-successor" || current.TeardownIntent.State != models.ErrandTeardownIntentPending {
		t.Fatalf("stale teardown CAS changed successor state: %+v", current)
	}
	if _, err := repo.TransitionErrandTeardownIntent(t.Context(), row.ID, "wrong-intent", current.Version, models.ErrandTeardownIntentCompleted); !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("wrong intent ID CAS = %v, want ErrErrandVersionConflict", err)
	}
	completed, err := repo.TransitionErrandTeardownIntent(t.Context(), row.ID, current.TeardownIntent.ID, current.Version, models.ErrandTeardownIntentCompleted)
	if err != nil {
		t.Fatalf("complete exact teardown intent: %v", err)
	}
	if completed.Version != current.Version+1 || completed.TeardownIntent.State != models.ErrandTeardownIntentCompleted ||
		completed.ResultText != "successor result" || completed.ResultTurnID != "turn-successor" {
		t.Fatalf("exact intent transition changed successor result: %+v", completed)
	}

	mutated := *completed
	intent := *completed.TeardownIntent
	intent.Target.TurnID = "replacement-turn"
	mutated.TeardownIntent = &intent
	mutated.ResultText = "must stay unchanged"
	if err := repo.UpdateErrand(t.Context(), &mutated); !errors.Is(err, models.ErrInvalidErrand) {
		t.Fatalf("same-intent target mutation = %v, want ErrInvalidErrand", err)
	}
	unchanged, err := repo.GetErrand(t.Context(), row.ID)
	if err != nil || unchanged.ResultText != "successor result" || unchanged.TeardownIntent.Target.TurnID != "turn-errand-teardown-cas" {
		t.Fatalf("immutable target mutation changed stored row: %+v, err = %v", unchanged, err)
	}
}

func exerciseErrandCancellationAndStopIntentCommitTogether(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	row := createTeardownIntentErrand(t, repo, "atomic")
	row.ResultText, row.ResultKind, row.ResultState, row.ResultTurnID = "original result", models.ErrandResultOutput, models.ErrandResultPending, "turn-original"
	if err := repo.UpdateErrand(ctx, row); err != nil {
		t.Fatalf("seed pending result: %v", err)
	}
	update, err := repo.GetErrand(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetErrand before cancellation: %v", err)
	}
	update.Lifecycle, update.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceParent
	update.ResultState = models.ErrandResultDiscarded
	update.TeardownIntent = pendingSessionTeardownIntent(update)
	removeFault := failErrandUpdate(t, repo, "NEW.lifecycle = 'cancelled' AND NEW.cancel_source = 'parent'")
	if err := repo.UpdateErrand(ctx, update); err == nil {
		t.Fatal("injected cancellation write failure did not abort the update")
	}
	unchanged, err := repo.GetErrand(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetErrand after rollback: %v", err)
	}
	if unchanged.Lifecycle != models.ErrandLifecycleQueued || unchanged.CancelSource != "" ||
		unchanged.TeardownIntent != nil || unchanged.ResultState != models.ErrandResultPending || unchanged.ResultText != "original result" {
		t.Fatalf("failed cancellation partially changed row: %+v", unchanged)
	}
	removeFault()
	if err := repo.UpdateErrand(ctx, update); err != nil {
		t.Fatalf("commit cancellation and teardown intent together: %v", err)
	}
	committed, err := repo.GetErrand(ctx, row.ID)
	if err != nil {
		t.Fatalf("GetErrand after commit: %v", err)
	}
	if committed.Lifecycle != models.ErrandLifecycleCancelled || committed.CancelSource != models.ErrandCancelSourceParent ||
		committed.TeardownIntent == nil || committed.TeardownIntent.State != models.ErrandTeardownIntentPending ||
		committed.ResultState != models.ErrandResultDiscarded || committed.ResultText != "original result" {
		t.Fatalf("cancellation, intent, and discard decision were not committed together: %+v", committed)
	}
}

func exerciseErrandTeardownIntentMigration(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	row := createTeardownIntentErrand(t, repo, "migration")
	assertTeardownIntentColumn(t, repo)
	if _, err := repo.db.ExecContext(ctx, `ALTER TABLE task_session_errands DROP COLUMN teardown_intent`); err != nil {
		t.Fatalf("remove new column to construct pre-migration schema: %v", err)
	}
	upgraded, err := NewWithDB(repo.db, repo.ro, nil)
	if err != nil {
		t.Fatalf("upgrade existing errand table: %v", err)
	}
	assertTeardownIntentColumn(t, upgraded)
	legacy, err := upgraded.GetErrand(ctx, row.ID)
	if err != nil || legacy.TeardownIntent != nil {
		t.Fatalf("legacy row after expansion = %+v, err = %v", legacy, err)
	}
	var encoded string
	if err := upgraded.db.GetContext(ctx, &encoded, upgraded.db.Rebind(`SELECT teardown_intent FROM task_session_errands WHERE id = ?`), row.ID); err != nil || encoded != "" {
		t.Fatalf("legacy teardown value = %q, err = %v, want explicit empty", encoded, err)
	}
	if _, err := NewWithDB(upgraded.db, upgraded.ro, nil); err != nil {
		t.Fatalf("replay expanded schema: %v", err)
	}
	legacy, err = upgraded.GetErrand(ctx, row.ID)
	if err != nil || legacy.TeardownIntent != nil {
		t.Fatalf("legacy row after schema replay = %+v, err = %v", legacy, err)
	}
}

func newFileErrandRepo(t *testing.T, path string) *Repository {
	t.Helper()
	connection, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite file: %v", err)
	}
	sqlDB := sqlx.NewDb(connection, "sqlite3")
	t.Cleanup(func() { _ = sqlDB.Close() })
	repo, err := NewWithDB(sqlDB, sqlDB, nil)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	return repo
}

func createTeardownIntentErrand(t *testing.T, repo *Repository, suffix string) *models.Errand {
	t.Helper()
	taskID := newErrandTask(t, repo, "teardown-"+suffix)
	row := sampleErrand("errand-teardown-"+suffix, taskID, "parent-teardown-"+suffix, errandScenarioBase)
	mustCreateErrand(t, repo, row)
	return row
}

func persistErrandTeardownIntent(t *testing.T, repo *Repository, suffix string) *models.Errand {
	t.Helper()
	row := createTeardownIntentErrand(t, repo, suffix)
	row.TeardownIntent = pendingSessionTeardownIntent(row)
	row.Lifecycle, row.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
	if err := repo.UpdateErrand(t.Context(), row); err != nil {
		t.Fatalf("commit teardown intent: %v", err)
	}
	return row
}

func pendingSessionTeardownIntent(row *models.Errand) *models.ErrandTeardownIntent {
	return &models.ErrandTeardownIntent{
		Version: models.ErrandTeardownIntentVersion, ID: "intent-" + row.ID,
		State: models.ErrandTeardownIntentPending, RequestedAt: errandTeardownRequestedAt,
		Target: models.ErrandTeardownTarget{
			TaskID: row.TaskID, SessionID: row.ID, SessionIncarnation: "incarnation-" + row.ID,
			ExecutionID: "execution-" + row.ID, TurnID: "turn-" + row.ID, SubmissionID: "submission-" + row.ID,
		},
	}
}

func assertTeardownIntentColumn(t *testing.T, repo *Repository) {
	t.Helper()
	exists, err := db.ColumnExistsContext(t.Context(), repo.db, "task_session_errands", "teardown_intent")
	if err != nil || !exists {
		t.Fatalf("task_session_errands.teardown_intent exists = %v, err = %v", exists, err)
	}
}

func setRawTeardownIntent(t *testing.T, repo *Repository, errandID, value string) error {
	t.Helper()
	_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE task_session_errands
		SET lifecycle = ?, cancel_source = ?, teardown_intent = ? WHERE id = ?`),
		string(models.ErrandLifecycleCancelled), string(models.ErrandCancelSourceUser), value, errandID)
	return err
}
