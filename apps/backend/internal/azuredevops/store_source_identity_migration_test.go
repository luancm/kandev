package azuredevops

import (
	"context"
	"testing"
)

func TestTaskPRSourceIdentitySchemaReplay(t *testing.T) {
	db := newTestDB(t)
	if _, err := NewStore(db, db); err != nil {
		t.Fatalf("initial store: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE azure_devops_task_prs RENAME TO azure_devops_task_prs_before_identity`); err != nil {
		t.Fatalf("rename current task PR table: %v", err)
	}
	if _, err := db.Exec(`DROP INDEX idx_azure_devops_task_prs_task_id`); err != nil {
		t.Fatalf("drop renamed task PR index: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE azure_devops_task_prs (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL,
			organization_url TEXT NOT NULL,
			project_id TEXT NOT NULL,
			azure_repository_id TEXT NOT NULL,
			pull_request_id INTEGER NOT NULL,
			pull_request_url TEXT NOT NULL,
			title TEXT NOT NULL,
			source_branch TEXT NOT NULL,
			target_branch TEXT NOT NULL,
			author_id TEXT NOT NULL,
			author_name TEXT NOT NULL,
			status TEXT NOT NULL,
			review_state TEXT NOT NULL DEFAULT '',
			policy_state TEXT NOT NULL DEFAULT '',
			is_draft BOOLEAN NOT NULL DEFAULT 0,
			last_synced_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(task_id, repository_id, azure_repository_id, pull_request_id)
		)`); err != nil {
		t.Fatalf("create legacy task PR table: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("apply additive source identity migration: %v", err)
	}
	row := testTaskPR("task-source", "repo-source", 42)
	row.SourceOrganizationURL = "https://dev.azure.com/fork"
	row.SourceProjectID = "source-project"
	row.SourceProjectName = "Contributors"
	row.SourceRepositoryID = "source-repo"
	row.SourceRepositoryName = "product-fork"
	row.TargetOrganizationURL = "https://dev.azure.com/base"
	row.TargetProjectID = "target-project"
	row.TargetProjectName = "Platform"
	row.TargetRepositoryID = "target-repo"
	row.TargetRepositoryName = "product"
	if err := store.UpsertTaskPR(context.Background(), row); err != nil {
		t.Fatalf("seed migrated task PR: %v", err)
	}

	replayed, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("replay source identity migration: %v", err)
	}
	gotRows, err := replayed.ListTaskPRsByTask(context.Background(), row.TaskID)
	if err != nil {
		t.Fatalf("read replayed task PR: %v", err)
	}
	if len(gotRows) != 1 || gotRows[0].SourceRepositoryID != row.SourceRepositoryID ||
		gotRows[0].TargetRepositoryID != row.TargetRepositoryID || gotRows[0].SourceProjectID != row.SourceProjectID ||
		gotRows[0].TargetProjectID != row.TargetProjectID {
		t.Fatalf("source/target identity did not survive replay: %+v", gotRows)
	}
	var indexName string
	if err := db.Get(&indexName, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'idx_azure_devops_task_prs_task_id'`); err != nil {
		t.Fatalf("task PR index missing after replay: %v", err)
	}
}
