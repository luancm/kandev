package github

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestTaskPRReads_ToleratesExtraColumn locks in resilience against a
// specific production incident: the "Link GitHub pull request" dialog
// failed with "associate PR with task: missing destination name workspace_id
// in *github.TaskPR" for a task in a workspace whose github_task_prs table
// had picked up a column (via a schema migration from a newer release, e.g.
// a self-update that was later rolled back) that the running binary's
// TaskPR struct doesn't declare. sqlx's StructScan errors out on any SELECT
// * column with no matching destination field, so every TaskPR read query
// must project an explicit column list rather than `SELECT *` /
// `SELECT gtp.*` — otherwise ANY future schema drift ahead of the binary
// breaks every read path, not just the one column that happened to trigger
// the original report.
func TestTaskPRReads_ToleratesExtraColumn(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Simulate schema drift: a column present in the table that the
	// current TaskPR struct has no field for.
	if _, err := store.db.Exec(
		`ALTER TABLE github_task_prs ADD COLUMN future_only_column TEXT NOT NULL DEFAULT ''`,
	); err != nil {
		t.Fatalf("simulate schema drift: %v", err)
	}
	if _, err := store.db.Exec(
		`INSERT INTO tasks (id, workspace_id) VALUES ('task-drift', 'ws-drift')`,
	); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	tp := &TaskPR{
		WorkspaceID: "ws-drift",
		TaskID:      "task-drift",
		Owner:       "kdlbs",
		Repo:        "kandev",
		PRNumber:    1978,
		PRURL:       "https://github.com/kdlbs/kandev/pull/1978",
		PRTitle:     "drifted schema",
		State:       "open",
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.ReplaceTaskPR(ctx, tp); err != nil {
		t.Fatalf("ReplaceTaskPR: %v", err)
	}

	fatalOnScanError := func(name string, err error) {
		t.Helper()
		if err == nil {
			return
		}
		t.Fatalf("%s: SELECT scan broke on schema drift: %v", name, err)
	}

	got, err := store.GetTaskPR(ctx, "task-drift")
	fatalOnScanError("GetTaskPR", err)
	if got == nil || got.PRNumber != 1978 || got.WorkspaceID != "ws-drift" {
		t.Fatalf("GetTaskPR: expected PR #1978 in ws-drift, got %+v", got)
	}

	gotByRepo, err := store.GetTaskPRByRepository(ctx, "task-drift", "")
	fatalOnScanError("GetTaskPRByRepository", err)
	if gotByRepo == nil || gotByRepo.PRNumber != 1978 {
		t.Fatalf("GetTaskPRByRepository: expected PR #1978, got %+v", gotByRepo)
	}

	gotByNumber, err := store.GetTaskPRByRepoAndNumber(ctx, "task-drift", "", 1978)
	fatalOnScanError("GetTaskPRByRepoAndNumber", err)
	if gotByNumber == nil || gotByNumber.PRTitle != "drifted schema" {
		t.Fatalf("GetTaskPRByRepoAndNumber: expected the drifted-schema PR, got %+v", gotByNumber)
	}

	list, err := store.ListTaskPRsByTask(ctx, "task-drift")
	fatalOnScanError("ListTaskPRsByTask", err)
	if len(list) != 1 || list[0].PRNumber != 1978 {
		t.Fatalf("ListTaskPRsByTask: expected 1 PR (#1978), got %+v", list)
	}

	byIDs, err := store.ListTaskPRsByTaskIDs(ctx, []string{"task-drift"})
	fatalOnScanError("ListTaskPRsByTaskIDs", err)
	if len(byIDs["task-drift"]) != 1 || byIDs["task-drift"][0].PRNumber != 1978 {
		t.Fatalf("ListTaskPRsByTaskIDs: expected 1 PR (#1978) for task-drift, got %+v", byIDs)
	}

	byWorkspace, err := store.ListTaskPRsByWorkspaceID(ctx, "ws-drift")
	fatalOnScanError("ListTaskPRsByWorkspaceID", err)
	if len(byWorkspace["task-drift"]) != 1 || byWorkspace["task-drift"][0].PRNumber != 1978 {
		t.Fatalf("ListTaskPRsByWorkspaceID: expected 1 PR (#1978) for task-drift, got %+v", byWorkspace)
	}
}

func TestTaskPRSourceIdentityRoundTripsAndPartialUpdatePreservesIt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tp := &TaskPR{
		WorkspaceID: "ws-identity", TaskID: "task-identity", Owner: "base-owner", Repo: "base-repo",
		PRNumber: 42, PRURL: "https://github.com/base-owner/base-repo/pull/42", PRTitle: "identity",
		HeadHost: "github.example", HeadOwner: "source-owner", HeadRepo: "source-repo", HeadBranch: "feature/source",
		HeadRepoID: 101, HeadRepoNodeID: "R_source", BaseHost: "github.example", BaseOwner: "base-owner",
		BaseRepo: "base-repo", BaseRepoID: 202, BaseBranch: "main", State: "open", CreatedAt: time.Now().UTC(),
	}
	if err := store.ReplaceTaskPR(ctx, tp); err != nil {
		t.Fatalf("replace task PR: %v", err)
	}

	partial := *tp
	partial.State = "closed"
	partial.HeadHost, partial.HeadOwner, partial.HeadRepo, partial.HeadBranch = "", "", "", ""
	partial.HeadRepoID, partial.HeadRepoNodeID = 0, ""
	partial.BaseHost, partial.BaseOwner, partial.BaseRepo, partial.BaseBranch = "", "", "", ""
	partial.BaseRepoID = 0
	if err := store.UpdateTaskPR(ctx, &partial); err != nil {
		t.Fatalf("partial update task PR: %v", err)
	}

	got, err := store.GetTaskPRByID(ctx, tp.ID)
	if err != nil {
		t.Fatalf("read task PR: %v", err)
	}
	if got == nil || got.HeadOwner != "source-owner" || got.HeadRepo != "source-repo" || got.HeadBranch != "feature/source" ||
		got.BaseOwner != "base-owner" || got.BaseRepo != "base-repo" || got.BaseBranch != "main" || got.HeadRepoID != 101 || got.BaseRepoID != 202 {
		t.Fatalf("partial update lost source/base identity: %+v", got)
	}
}

func TestTaskPRSourceIdentitySurvivesLegacyTableRebuildAndReplay(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`ALTER TABLE github_task_prs RENAME TO github_task_prs_before_rebuild`); err != nil {
		t.Fatalf("rename current task PR table: %v", err)
	}
	_, err := store.db.Exec(`
		CREATE TABLE github_task_prs (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL DEFAULT '',
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			pr_number INTEGER NOT NULL,
			pr_url TEXT NOT NULL,
			pr_title TEXT NOT NULL,
			head_host TEXT NOT NULL DEFAULT '',
			head_owner TEXT NOT NULL DEFAULT '',
			head_repo TEXT NOT NULL DEFAULT '',
			head_repo_id INTEGER NOT NULL DEFAULT 0,
			head_repo_node_id TEXT NOT NULL DEFAULT '',
			base_host TEXT NOT NULL DEFAULT '',
			base_owner TEXT NOT NULL DEFAULT '',
			base_repo TEXT NOT NULL DEFAULT '',
			base_repo_id INTEGER NOT NULL DEFAULT 0,
			head_branch TEXT NOT NULL,
			base_branch TEXT NOT NULL,
			author_login TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'open',
			review_state TEXT NOT NULL DEFAULT '',
			checks_state TEXT NOT NULL DEFAULT '',
			mergeable_state TEXT NOT NULL DEFAULT '',
			review_count INTEGER DEFAULT 0,
			pending_review_count INTEGER DEFAULT 0,
			required_reviews INTEGER,
			comment_count INTEGER DEFAULT 0,
			unresolved_review_threads INTEGER DEFAULT 0,
			checks_total INTEGER DEFAULT 0,
			checks_passing INTEGER DEFAULT 0,
			additions INTEGER DEFAULT 0,
			deletions INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL,
			merged_at DATETIME,
			closed_at DATETIME,
			last_synced_at DATETIME,
			detached_at DATETIME,
			updated_at DATETIME NOT NULL,
			UNIQUE(task_id, pr_number)
		)`)
	if err != nil {
		t.Fatalf("create legacy task PR table: %v", err)
	}
	now := time.Now().UTC()
	_, err = store.db.Exec(`
		INSERT INTO github_task_prs (
			id, workspace_id, task_id, repository_id, owner, repo, pr_number, pr_url, pr_title,
			head_host, head_owner, head_repo, head_repo_id, head_repo_node_id,
			base_host, base_owner, base_repo, base_repo_id, head_branch, base_branch, author_login,
			state, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"legacy-identity", "ws-legacy", "task-legacy", "repo-legacy", "base-owner", "base-repo", 9,
		"https://github.com/base-owner/base-repo/pull/9", "legacy", "github.com", "fork-owner", "fork-repo", 101, "R_fork",
		"github.com", "base-owner", "base-repo", 202, "feature/fork", "main", "alice", "open", now, now)
	if err != nil {
		t.Fatalf("seed legacy task PR: %v", err)
	}

	replayed, err := NewStore(store.db, store.db)
	if err != nil {
		t.Fatalf("rebuild legacy task PR table: %v", err)
	}
	if _, err := NewStore(store.db, store.db); err != nil {
		t.Fatalf("replay rebuilt task PR table: %v", err)
	}
	got, err := replayed.GetTaskPRByID(ctx, "legacy-identity")
	if err != nil {
		t.Fatalf("read rebuilt task PR: %v", err)
	}
	if got == nil || got.HeadOwner != "fork-owner" || got.HeadRepo != "fork-repo" || got.HeadRepoID != 101 ||
		got.BaseOwner != "base-owner" || got.BaseRepo != "base-repo" || got.BaseRepoID != 202 {
		t.Fatalf("rebuild lost source/base identity: %+v", got)
	}
	var tableSQL string
	if err := store.db.Get(&tableSQL, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'github_task_prs'`); err != nil {
		t.Fatalf("read rebuilt table definition: %v", err)
	}
	if !strings.Contains(tableSQL, "UNIQUE(task_id, repository_id, pr_number)") {
		t.Fatalf("rebuilt table lost multi-repo constraint: %s", tableSQL)
	}
}
