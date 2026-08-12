package github

import (
	"context"
	"database/sql"
	"testing"
)

func TestPRWatchSchemaIncludesRuntimeHeadColumns(t *testing.T) {
	store := newTestStore(t)

	rows, err := store.db.QueryContext(context.Background(), `PRAGMA table_info(github_pr_watches)`)
	if err != nil {
		t.Fatalf("inspect github_pr_watches schema: %v", err)
	}
	defer func() { _ = rows.Close() }()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan github_pr_watches schema: %v", err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read github_pr_watches schema: %v", err)
	}

	for _, name := range []string{"head_host", "head_owner", "head_repo", "head_branch"} {
		if !columns[name] {
			t.Errorf("github_pr_watches missing runtime head column %q", name)
		}
	}
}

func TestPRWatchRuntimeHeadUpdateIsSearchingOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	watch := &PRWatch{
		ID:           "watch-searching",
		WorkspaceID:  "workspace-1",
		SessionID:    "session-1",
		TaskID:       "task-1",
		RepositoryID: "repository-1",
		Owner:        "fork",
		Repo:         "project",
		Branch:       "local-feature",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create searching watch: %v", err)
	}

	if err := store.UpdatePRWatchSearchTargetIfSearching(
		ctx, watch.ID, "local-feature-renamed", "github.com", "fork", "project", "review-feature",
	); err != nil {
		t.Fatalf("update searching target: %v", err)
	}
	got, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("get searching watch: %v", err)
	}
	if got.Branch != "local-feature-renamed" || got.HeadHost != "github.com" || got.HeadOwner != "fork" || got.HeadRepo != "project" || got.HeadBranch != "review-feature" {
		t.Fatalf("searching target = %+v, want renamed local and exact remote head", got)
	}

	resolved, err := store.ResolvePRWatch(ctx, watch.ID, "upstream", "project", 42)
	if err != nil {
		t.Fatalf("resolve watch: %v", err)
	}
	if !resolved {
		t.Fatal("resolve watch = false, want true")
	}

	if err := store.UpdatePRWatchSearchTargetIfSearching(
		ctx, watch.ID, "should-not-change", "github.com", "other", "project", "other-feature",
	); err != nil {
		t.Fatalf("update resolved target: %v", err)
	}
	got, err = store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("get resolved watch: %v", err)
	}
	if got.Owner != "upstream" || got.Repo != "project" || got.PRNumber != 42 {
		t.Fatalf("resolved canonical base = %+v, want upstream/project#42", got)
	}
	if got.Branch != "local-feature-renamed" || got.HeadOwner != "fork" || got.HeadBranch != "review-feature" {
		t.Fatalf("resolved runtime head was rewritten = %+v", got)
	}

	resolved, err = store.ResolvePRWatch(ctx, watch.ID, "wrong", "repo", 99)
	if err != nil {
		t.Fatalf("resolve watch twice: %v", err)
	}
	if resolved {
		t.Fatal("second resolve watch = true, want false")
	}
}

func TestPRWatchRuntimeHeadUpdateDropsSearchingBranchCollision(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for _, watch := range []*PRWatch{
		{ID: "watch-owner", WorkspaceID: "workspace-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repository-1", Owner: "owner", Repo: "repo", Branch: "feature-a"},
		{ID: "watch-source", WorkspaceID: "workspace-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repository-1", Owner: "owner", Repo: "repo", Branch: "feature-b"},
	} {
		if err := store.CreatePRWatch(ctx, watch); err != nil {
			t.Fatalf("create watch %s: %v", watch.ID, err)
		}
	}

	if err := store.UpdatePRWatchSearchTargetIfSearching(
		ctx, "watch-source", "feature-a", "github.com", "owner", "repo", "feature-a",
	); err != nil {
		t.Fatalf("update colliding target: %v", err)
	}
	got, err := store.GetPRWatch(ctx, "watch-source")
	if err != nil {
		t.Fatalf("get source watch: %v", err)
	}
	if got != nil {
		t.Fatalf("colliding searching source still exists: %+v", got)
	}
	owner, err := store.GetPRWatch(ctx, "watch-owner")
	if err != nil {
		t.Fatalf("get destination watch: %v", err)
	}
	if owner == nil || owner.Branch != "feature-a" {
		t.Fatalf("destination watch = %+v, want feature-a", owner)
	}
}

func TestPRWatchBranchOnlyMutationClearsRuntimeHead(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	watch := &PRWatch{
		ID: "watch-branch-only", WorkspaceID: "workspace-1", SessionID: "session-1", TaskID: "task-1",
		RepositoryID: "repository-1", Owner: "attached", Repo: "project", Branch: "local-feature",
		HeadHost: "github.com", HeadOwner: "fork", HeadRepo: "project", HeadBranch: "review-feature",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	if err := store.UpdatePRWatchBranchIfSearching(ctx, watch.ID, "local-feature-renamed"); err != nil {
		t.Fatalf("update branch: %v", err)
	}
	got, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if got == nil {
		t.Fatal("watch disappeared")
	}
	if got.Branch != "local-feature-renamed" {
		t.Fatalf("branch = %q, want local-feature-renamed", got.Branch)
	}
	if got.HeadHost != "" || got.HeadOwner != "" || got.HeadRepo != "" || got.HeadBranch != "" {
		t.Fatalf("branch-only mutation retained stale runtime head: %+v", got)
	}
}

func TestPRWatchBranchResetClearsRuntimeHeadAtomically(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	watch := &PRWatch{
		ID: "watch-reset", WorkspaceID: "workspace-1", SessionID: "session-1", TaskID: "task-1",
		RepositoryID: "repository-1", Owner: "canonical", Repo: "project", PRNumber: 0, Branch: "old-local",
		HeadHost: "github.com", HeadOwner: "fork", HeadRepo: "project", HeadBranch: "review-feature",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	if err := store.ResetPRWatch(ctx, watch.ID, "new-local"); err != nil {
		t.Fatalf("reset watch: %v", err)
	}
	got, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if got == nil || got.Branch != "new-local" || got.PRNumber != 0 {
		t.Fatalf("reset watch = %+v, want searching new-local", got)
	}
	if got.HeadHost != "" || got.HeadOwner != "" || got.HeadRepo != "" || got.HeadBranch != "" {
		t.Fatalf("branch reset retained stale runtime head: %+v", got)
	}
}

func TestPRWatchBranchResetDoesNotStealNumberedWatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	watch := &PRWatch{
		ID: "watch-numbered-reset", WorkspaceID: "workspace-1", SessionID: "session-1", TaskID: "task-1",
		RepositoryID: "repository-1", Owner: "canonical", Repo: "project", PRNumber: 42, Branch: "old-local",
		HeadHost: "github.com", HeadOwner: "fork", HeadRepo: "project", HeadBranch: "review-feature",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	if err := store.ResetPRWatch(ctx, watch.ID, "new-local"); err != nil {
		t.Fatalf("reset numbered watch: %v", err)
	}
	got, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if got.Branch != "old-local" || got.PRNumber != 42 || got.HeadBranch != "review-feature" {
		t.Fatalf("numbered watch changed during branch reset: %+v", got)
	}
}

func TestPRWatchTerminalResetRetainsRuntimeHeadForSameBranch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	watch := &PRWatch{
		ID: "watch-terminal", WorkspaceID: "workspace-1", SessionID: "session-1", TaskID: "task-1",
		RepositoryID: "repository-1", Owner: "canonical", Repo: "project", PRNumber: 42, Branch: "local-feature",
		HeadHost: "github.com", HeadOwner: "fork", HeadRepo: "project", HeadBranch: "review-feature",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	if err := store.UpdatePRWatchPRNumber(ctx, watch.ID, 0); err != nil {
		t.Fatalf("terminal reset: %v", err)
	}
	got, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if got == nil || got.PRNumber != 0 || got.Branch != "local-feature" {
		t.Fatalf("terminal reset watch = %+v, want same searching branch", got)
	}
	if got.HeadHost != "github.com" || got.HeadOwner != "fork" || got.HeadRepo != "project" || got.HeadBranch != "review-feature" {
		t.Fatalf("terminal reset lost observed runtime head: %+v", got)
	}
}

func TestPRWatchNumberedIdentityCannotBeOverwritten(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	watch := &PRWatch{
		ID: "watch-numbered-immutable", WorkspaceID: "workspace-1", SessionID: "session-1", TaskID: "task-1",
		RepositoryID: "repository-1", Owner: "canonical", Repo: "project", PRNumber: 42, Branch: "feature",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}

	if err := store.UpdatePRWatchPRNumber(ctx, watch.ID, 99); err != nil {
		t.Fatalf("attempt overwrite: %v", err)
	}
	got, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if got.PRNumber != 42 {
		t.Fatalf("numbered watch identity changed to %d, want 42", got.PRNumber)
	}
}

func TestPRWatchTerminalResetDoesNotClearNewAssociation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	watch := &PRWatch{
		ID: "watch-stale-reset", WorkspaceID: "workspace-1", SessionID: "session-1", TaskID: "task-1",
		RepositoryID: "repository-1", Owner: "canonical", Repo: "project", PRNumber: 42, Branch: "feature",
	}
	if err := store.CreatePRWatch(ctx, watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	reset, err := store.ResetPRWatchPRNumberIfCurrent(ctx, watch.ID, 99)
	if err != nil {
		t.Fatalf("stale terminal reset: %v", err)
	}
	if reset {
		t.Fatal("stale terminal reset cleared a different PR association")
	}
	got, err := store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("get watch: %v", err)
	}
	if got.PRNumber != 42 {
		t.Fatalf("stale terminal reset changed PR number to %d, want 42", got.PRNumber)
	}
}
