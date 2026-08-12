package gitlab

import (
	"context"
	"testing"
)

func TestTaskMRSourceIdentitySchemaReplay(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.db.Exec(`ALTER TABLE gitlab_task_mrs RENAME TO gitlab_task_mrs_before_identity`); err != nil {
		t.Fatalf("rename current task MR table: %v", err)
	}
	if _, err := store.db.Exec(`DROP INDEX idx_gitlab_task_mrs_task_id`); err != nil {
		t.Fatalf("drop renamed task MR index: %v", err)
	}
	if _, err := store.db.Exec(`
		CREATE TABLE gitlab_task_mrs (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL DEFAULT '',
			host TEXT NOT NULL DEFAULT '',
			project_path TEXT NOT NULL,
			mr_iid INTEGER NOT NULL,
			mr_url TEXT NOT NULL,
			mr_title TEXT NOT NULL,
			head_branch TEXT NOT NULL,
			base_branch TEXT NOT NULL,
			author_username TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT 'open',
			approval_state TEXT NOT NULL DEFAULT '',
			pipeline_state TEXT NOT NULL DEFAULT '',
			merge_status TEXT NOT NULL DEFAULT '',
			draft INTEGER NOT NULL DEFAULT 0,
			approval_count INTEGER DEFAULT 0,
			required_approvals INTEGER DEFAULT 0,
			pipeline_jobs_total INTEGER DEFAULT 0,
			pipeline_jobs_pass INTEGER DEFAULT 0,
			detailed_merge_status TEXT NOT NULL DEFAULT '',
			reviewer_count INTEGER NOT NULL DEFAULT 0,
			unapproved_reviewers INTEGER NOT NULL DEFAULT 0,
			unresolved_discussions INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			merged_at DATETIME,
			closed_at DATETIME,
			last_synced_at DATETIME,
			updated_at DATETIME NOT NULL,
			UNIQUE(task_id, repository_id, project_path, mr_iid)
		)`); err != nil {
		t.Fatalf("create legacy task MR table: %v", err)
	}

	store, err := NewStore(store.db, store.db)
	if err != nil {
		t.Fatalf("apply additive source identity migration: %v", err)
	}
	row := newTestMR("task-source", "repo-source", "group/base/project", 7)
	row.SourceHost = "https://gitlab.example"
	row.SourceProjectPath = "forks/team/project"
	row.SourceProjectID = 42
	row.TargetHost = "https://gitlab.example"
	row.TargetProjectPath = "group/base/project"
	row.TargetProjectID = 99
	if err := store.UpsertTaskMR(context.Background(), row); err != nil {
		t.Fatalf("seed migrated task MR: %v", err)
	}

	replayed, err := NewStore(store.db, store.db)
	if err != nil {
		t.Fatalf("replay source identity migration: %v", err)
	}
	got, err := replayed.GetTaskMR(context.Background(), row.TaskID, row.RepositoryID, row.ProjectPath, row.MRIID)
	if err != nil {
		t.Fatalf("read replayed task MR: %v", err)
	}
	if got == nil || got.SourceProjectPath != row.SourceProjectPath || got.SourceProjectID != row.SourceProjectID ||
		got.TargetProjectPath != row.TargetProjectPath || got.TargetProjectID != row.TargetProjectID {
		t.Fatalf("source/target identity did not survive replay: %+v", got)
	}
	var indexName string
	if err := store.db.Get(&indexName, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'idx_gitlab_task_mrs_task_id'`); err != nil {
		t.Fatalf("task MR index missing after replay: %v", err)
	}
}
