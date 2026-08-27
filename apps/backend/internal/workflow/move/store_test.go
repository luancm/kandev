package move

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func TestSQLiteEntryStoreRoundTripsAndDeletesOneMove(t *testing.T) {
	raw, err := sql.Open("sqlite3", "file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	store, err := NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	want := &Entry{
		ID:     "move-1",
		TaskID: "task-1",
		Options: StepEntryOptions{
			ResetContext:   true,
			Instructions:   "Create the PR ready for review, not as a draft.",
			AgentProfileID: "profile-qa",
		},
	}
	ctx := context.Background()
	if err := store.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(ctx, want.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil || got.ID != want.ID || got.TaskID != want.TaskID || got.Options != want.Options {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
	if got.Phase != EntryPhaseTransitionCommitted {
		t.Fatalf("initial phase = %q, want %q", got.Phase, EntryPhaseTransitionCommitted)
	}

	if err := store.Delete(ctx, want.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = store.Load(ctx, want.ID)
	if err != nil {
		t.Fatalf("Load after Delete: %v", err)
	}
	if got != nil {
		t.Fatalf("Load after Delete = %+v, want nil", got)
	}
	if err := store.Delete(ctx, want.ID); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("second Delete error = %v, want ErrEntryNotFound", err)
	}
}

func TestSQLiteEntryStoreClaimsDispatchOnceAndFinalizesMarkerAtomically(t *testing.T) {
	raw, err := sql.Open("sqlite3", "file:workflow-move-lifecycle?mode=memory&cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY,
			metadata TEXT NOT NULL DEFAULT '{}',
			updated_at TIMESTAMP NOT NULL
		);
		INSERT INTO tasks (id, metadata, updated_at) VALUES (
			'task-lifecycle',
			'{"workflow_move_pending":{"from_step_id":"source","move_id":"move-lifecycle"}}',
			CURRENT_TIMESTAMP
		);
	`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	base, err := NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	store, ok := base.(LifecycleStore)
	if !ok {
		t.Fatal("entry store does not implement durable lifecycle operations")
	}
	ctx := context.Background()
	if err := store.Save(ctx, &Entry{
		ID: "move-lifecycle", TaskID: "task-lifecycle", Options: EntryOptions{Instructions: "handoff"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	prepared, err := store.ClaimPhase(
		ctx, "move-lifecycle", EntryPhaseTransitionCommitted, EntryPhaseDispatchReady, "session-target",
	)
	if err != nil || !prepared {
		t.Fatalf("prepare dispatch = (%v, %v), want (true, nil)", prepared, err)
	}
	claimed, err := store.ClaimPhase(
		ctx, "move-lifecycle", EntryPhaseDispatchReady, EntryPhaseDispatchClaimed, "session-target",
	)
	if err != nil || !claimed {
		t.Fatalf("first claim = (%v, %v), want (true, nil)", claimed, err)
	}
	claimed, err = store.ClaimPhase(
		ctx, "move-lifecycle", EntryPhaseDispatchReady, EntryPhaseDispatchClaimed, "session-other",
	)
	if err != nil || claimed {
		t.Fatalf("duplicate claim = (%v, %v), want (false, nil)", claimed, err)
	}
	entry, err := store.Load(ctx, "move-lifecycle")
	if err != nil {
		t.Fatalf("Load claimed entry: %v", err)
	}
	if entry.Phase != EntryPhaseDispatchClaimed || entry.TargetSessionID != "session-target" {
		t.Fatalf("claimed entry = %#v", entry)
	}
	if err := store.Finalize(ctx, "task-lifecycle", "move-lifecycle"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if entry, err := store.Load(ctx, "move-lifecycle"); err != nil || entry != nil {
		t.Fatalf("entry after finalize = (%#v, %v), want nil", entry, err)
	}
	var metadata string
	if err := db.GetContext(ctx, &metadata, `SELECT metadata FROM tasks WHERE id = 'task-lifecycle'`); err != nil {
		t.Fatalf("load finalized metadata: %v", err)
	}
	if metadata != "{}" {
		t.Fatalf("metadata after finalize = %s, want {}", metadata)
	}
	if err := store.Finalize(ctx, "task-lifecycle", "move-lifecycle"); err != nil {
		t.Fatalf("repeated Finalize: %v", err)
	}
}

func TestSQLiteEntryStoreRejectsMalformedOptions(t *testing.T) {
	raw, err := sql.Open("sqlite3", "file:workflow-move-malformed?mode=memory&cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO workflow_move_entries (id, task_id, options_json)
		VALUES ('move-malformed', 'task-malformed', '{bad')
	`); err != nil {
		t.Fatalf("insert malformed entry: %v", err)
	}
	if entry, err := store.Load(context.Background(), "move-malformed"); err == nil || entry != nil {
		t.Fatalf("Load malformed entry = (%#v, %v), want strict error", entry, err)
	}
}

func TestDecodeEntryOptionsJSONRejectsNullPayloads(t *testing.T) {
	tests := []string{
		`null`,
		`{"model":null}`,
		`{"instructions":null}`,
		`{"agent_profile_id":null}`,
		`{"reset_context":null}`,
	}
	for _, encoded := range tests {
		t.Run(encoded, func(t *testing.T) {
			if options, err := DecodeEntryOptionsJSON([]byte(encoded)); err == nil || options != nil {
				t.Fatalf("DecodeEntryOptionsJSON(%s) = (%#v, %v), want strict error", encoded, options, err)
			}
		})
	}
	if options, err := DecodeEntryOptionsJSON([]byte(`{}`)); err != nil || options != nil {
		t.Fatalf("DecodeEntryOptionsJSON({}) = (%#v, %v), want (nil, nil)", options, err)
	}
}

func TestSQLiteEntryStorePersistsRecoverableDispatchReadyPhase(t *testing.T) {
	raw, err := sql.Open("sqlite3", "file:workflow-move-dispatch-ready?mode=memory&cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	base, err := NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	store := base.(LifecycleStore)
	ctx := context.Background()
	if err := store.Save(ctx, &Entry{ID: "move-ready", TaskID: "task-ready", Options: EntryOptions{ResetContext: true}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	ready := EntryPhaseDispatchReady
	claimed, err := store.ClaimPhase(ctx, "move-ready", EntryPhaseTransitionCommitted, EntryPhaseDispatchClaimed, "session-target")
	if err != nil || claimed {
		t.Fatalf("premature uncertain claim = (%v, %v), want (false, nil)", claimed, err)
	}
	advanced, err := store.ClaimPhase(ctx, "move-ready", EntryPhaseTransitionCommitted, ready, "session-target")
	if err != nil || !advanced {
		t.Fatalf("advance recoverable phase = (%v, %v), want (true, nil)", advanced, err)
	}
	entry, err := store.Load(ctx, "move-ready")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if entry.Phase != ready || entry.TargetSessionID != "session-target" {
		t.Fatalf("entry = %#v, want durable dispatch-ready target", entry)
	}
}

func TestSQLiteEntryStorePersistsRecoverableSideEffectPhasesAcrossRestart(t *testing.T) {
	raw, err := sql.Open("sqlite3", "file:workflow-move-side-effect-phases?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	base, err := NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatal(err)
	}
	store := base.(LifecycleStore)
	ctx := context.Background()
	if err := store.Save(ctx, &Entry{ID: "move-restart-phases", TaskID: "task-1", Options: EntryOptions{ResetContext: true}}); err != nil {
		t.Fatal(err)
	}
	phases := []EntryPhase{
		EntryPhaseExitApplied,
		EntryPhaseProfileApplied,
		EntryPhaseResetApplied,
		EntryPhaseConfigApplied,
		EntryPhaseActionsApplied,
		EntryPhaseDispatchReady,
	}
	previous := EntryPhaseTransitionCommitted
	for index, phase := range phases {
		if index == 3 {
			reopened, err := NewSQLiteEntryStore(db, db)
			if err != nil {
				t.Fatal(err)
			}
			store = reopened.(LifecycleStore)
		}
		advanced, err := store.ClaimPhase(ctx, "move-restart-phases", previous, phase, "session-target")
		if err != nil || !advanced {
			t.Fatalf("advance %s -> %s = (%v, %v)", previous, phase, advanced, err)
		}
		entry, err := store.Load(ctx, "move-restart-phases")
		if err != nil || entry == nil || entry.Phase != phase || entry.TargetSessionID != "session-target" {
			t.Fatalf("persisted phase after restart boundary = (%#v, %v), want %s", entry, err, phase)
		}
		previous = phase
	}
}

func TestSQLiteEntryStoreExposesTransactionOwner(t *testing.T) {
	raw, err := sql.Open("sqlite3", "file:workflow-move-owner?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewSQLiteEntryStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteEntryStore: %v", err)
	}
	owner, ok := store.(interface{ WorkflowMoveTransactionOwner() *sql.DB })
	if !ok {
		t.Fatal("SQLite entry store does not expose its transaction owner")
	}
	if owner.WorkflowMoveTransactionOwner() != raw {
		t.Fatal("SQLite entry store returned a different transaction owner")
	}
}
