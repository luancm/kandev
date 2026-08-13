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
			Model:          "gpt-5.6-sol",
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
