package sqlite

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

var errandScenarioBase = time.Date(2026, time.October, 26, 9, 0, 0, 0, time.UTC)

func newErrandTask(t *testing.T, repo *Repository, suffix string) string {
	t.Helper()
	ctx := context.Background()
	workspaceID := "ws-errand-" + suffix
	taskID := "task-errand-" + suffix
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Errands " + suffix}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Errands " + suffix}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return taskID
}

func createErrandScenarioSession(t *testing.T, repo *Repository, session *models.TaskSession) {
	t.Helper()
	if err := repo.CreateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("CreateTaskSession %s: %v", session.ID, err)
	}
}

func sampleErrand(id, taskID, parentID string, created time.Time) *models.Errand {
	return &models.Errand{
		ID: id, TaskID: taskID, ParentSessionID: parentID,
		Title: "Title " + id, Objective: "Objective " + id, AgentProfileID: "profile-" + id,
		MCPServerIDs: []string{"mcp-a", "mcp-b"}, FirstTurnID: "turn-" + id, SubmissionID: "submission-" + id,
		Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone,
		AwaitingResult: true, CreatedAt: created, UpdatedAt: created,
	}
}

func mustCreateErrand(t *testing.T, repo *Repository, errand *models.Errand) {
	t.Helper()
	if err := repo.CreateErrand(context.Background(), errand); err != nil {
		t.Fatalf("CreateErrand %s: %v", errand.ID, err)
	}
}

func errandIDs(errands []*models.Errand) []string {
	ids := make([]string, 0, len(errands))
	for _, errand := range errands {
		ids = append(ids, errand.ID)
	}
	return ids
}

func equalOptionalTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func assertErrandFields(t *testing.T, got, want *models.Errand) {
	t.Helper()
	gotCopy, wantCopy := *got, *want
	if !gotCopy.CreatedAt.Equal(wantCopy.CreatedAt) || !equalOptionalTime(gotCopy.DeliveredAt, wantCopy.DeliveredAt) ||
		!equalOptionalTime(gotCopy.SettledAt, wantCopy.SettledAt) {
		t.Fatalf("errand timestamps = (%v, %v, %v), want (%v, %v, %v)", gotCopy.CreatedAt, gotCopy.DeliveredAt,
			gotCopy.SettledAt, wantCopy.CreatedAt, wantCopy.DeliveredAt, wantCopy.SettledAt)
	}
	for _, errand := range []*models.Errand{&gotCopy, &wantCopy} {
		errand.CreatedAt, errand.UpdatedAt, errand.DeliveredAt, errand.SettledAt = time.Time{}, time.Time{}, nil, nil
	}
	if !reflect.DeepEqual(gotCopy, wantCopy) {
		t.Fatalf("errand = %+v\nwant %+v", gotCopy, wantCopy)
	}
}

func assertErrandSchema(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	for _, column := range []string{"role", "parent_session_id"} {
		exists, err := db.ColumnExistsContext(ctx, repo.db, "task_sessions", column)
		if err != nil || !exists {
			t.Fatalf("task_sessions.%s exists = %v, err = %v", column, exists, err)
		}
	}
	if exists, err := db.TableExistsContext(ctx, repo.db, "task_session_errands"); err != nil || !exists {
		t.Fatalf("task_session_errands exists = %v, err = %v", exists, err)
	}
	for _, index := range []string{
		"idx_task_sessions_parent_session_id",
		"idx_task_session_errands_task_lifecycle",
		"idx_task_session_errands_parent",
	} {
		if exists, err := db.IndexExistsContext(ctx, repo.db, index); err != nil || !exists {
			t.Fatalf("index %s exists = %v, err = %v", index, exists, err)
		}
	}
}

// exerciseErrandSchemaReplay proves that the columns, index, and table are
// created on a fresh database, survive a same-database replay with their
// data, and are restored on a database that predates them.
func exerciseErrandSchemaReplay(t *testing.T, repo *Repository, reopen func() error) {
	t.Helper()
	ctx := context.Background()
	assertErrandSchema(t, repo)
	taskID := newErrandTask(t, repo, "schema")
	createErrandScenarioSession(t, repo, &models.TaskSession{ID: "parent-schema", TaskID: taskID})
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: "errand-schema", TaskID: taskID, Role: models.SessionRoleErrand, ParentSessionID: "parent-schema",
	})
	mustCreateErrand(t, repo, sampleErrand("errand-schema", taskID, "parent-schema", errandScenarioBase))

	if err := reopen(); err != nil {
		t.Fatalf("same-database replay: %v", err)
	}
	assertErrandSchema(t, repo)
	session, err := repo.GetTaskSession(ctx, "errand-schema")
	if err != nil || session.Role != models.SessionRoleErrand || session.ParentSessionID != "parent-schema" {
		t.Fatalf("errand session after replay = %+v, err = %v", session, err)
	}
	if _, err := repo.GetErrand(ctx, "errand-schema"); err != nil {
		t.Fatalf("errand row after replay: %v", err)
	}

	for _, statement := range []string{
		`DROP INDEX idx_task_sessions_parent_session_id`,
		`ALTER TABLE task_sessions DROP COLUMN parent_session_id`,
		`ALTER TABLE task_sessions DROP COLUMN role`,
		`DROP TABLE task_session_errands`,
	} {
		if _, err := repo.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("build legacy schema with %q: %v", statement, err)
		}
	}
	if err := reopen(); err != nil {
		t.Fatalf("legacy database replay: %v", err)
	}
	assertErrandSchema(t, repo)
	session, err = repo.GetTaskSession(ctx, "errand-schema")
	if err != nil || session.Role != "" || session.ParentSessionID != "" {
		t.Fatalf("legacy session after upgrade = %+v, err = %v; want an ordinary session", session, err)
	}
}

func readSessionByID(sessions []*models.TaskSession, id string) *models.TaskSession {
	for _, session := range sessions {
		if session.ID == id {
			return session
		}
	}
	return nil
}

func errandSessionReaders(repo *Repository, taskID, sessionID string) map[string]func() (*models.TaskSession, error) {
	ctx := context.Background()
	return map[string]func() (*models.TaskSession, error){
		"get": func() (*models.TaskSession, error) { return repo.GetTaskSession(ctx, sessionID) },
		"list": func() (*models.TaskSession, error) {
			sessions, err := repo.ListTaskSessions(ctx, taskID)
			return readSessionByID(sessions, sessionID), err
		},
		"batch": func() (*models.TaskSession, error) {
			byTask, err := repo.BatchGetSessionsByTaskIDs(ctx, []string{taskID})
			return readSessionByID(byTask[taskID], sessionID), err
		},
		"summary": func() (*models.TaskSession, error) {
			observations, err := repo.ListTaskSessionSummaryObservations(ctx, taskID)
			sessions := make([]*models.TaskSession, 0, len(observations))
			for _, observation := range observations {
				sessions = append(sessions, observation.ToTaskSession())
			}
			return readSessionByID(sessions, sessionID), err
		},
	}
}

// exerciseSessionRolePersistence proves that role and parent survive every
// session read path and that full-row writes never change them.
func exerciseSessionRolePersistence(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	taskID := newErrandTask(t, repo, "role")
	createErrandScenarioSession(t, repo, &models.TaskSession{ID: "parent-role", TaskID: taskID})
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: "errand-role", TaskID: taskID, Role: models.SessionRoleErrand, ParentSessionID: "parent-role",
	})
	assertRole := func(stage string) {
		t.Helper()
		for name, read := range errandSessionReaders(repo, taskID, "errand-role") {
			session, err := read()
			if err != nil || session == nil {
				t.Fatalf("%s %s read = %+v, err = %v", stage, name, session, err)
			}
			if session.Role != models.SessionRoleErrand || session.ParentSessionID != "parent-role" {
				t.Fatalf("%s %s read role = (%q, %q), want (errand, parent-role)", stage, name, session.Role, session.ParentSessionID)
			}
		}
	}
	assertRole("after insert")

	current, err := repo.GetTaskSession(ctx, "errand-role")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	full := *current
	full.Role, full.ParentSessionID, full.ErrorMessage = "", "", "full-row write"
	if err := repo.UpdateTaskSession(ctx, &full); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}
	if written, err := repo.GetTaskSession(ctx, "errand-role"); err != nil || written.ErrorMessage != "full-row write" {
		t.Fatalf("full-row update did not persist: %+v, err = %v", written, err)
	}
	assertRole("after full-row update")

	guarded := *current
	guarded.Role, guarded.ParentSessionID = models.SessionRole("other"), "another-parent"
	if changed, err := repo.UpdateTaskSessionIfCurrentState(ctx, &guarded, current.State); err != nil || !changed {
		t.Fatalf("UpdateTaskSessionIfCurrentState = %v, %v", changed, err)
	}
	assertRole("after guarded update")

	parent, err := repo.GetTaskSession(ctx, "parent-role")
	if err != nil || parent.Role != "" || parent.ParentSessionID != "" {
		t.Fatalf("ordinary session role = %+v, err = %v", parent, err)
	}
}

// exerciseErrandRoundTrip proves that every errand field round-trips and
// that updates are compare-and-set on the version.
func exerciseErrandRoundTrip(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	taskID := newErrandTask(t, repo, "roundtrip")
	errand := sampleErrand("errand-roundtrip", taskID, "parent-roundtrip", errandScenarioBase)
	mustCreateErrand(t, repo, errand)
	if errand.Version != 1 {
		t.Fatalf("inserted version = %d, want 1", errand.Version)
	}
	stored, err := repo.GetErrand(ctx, errand.ID)
	if err != nil {
		t.Fatalf("GetErrand: %v", err)
	}
	assertErrandFields(t, stored, errand)

	if err := repo.CreateErrand(ctx, sampleErrand(errand.ID, taskID, "parent-roundtrip", errandScenarioBase)); err == nil {
		t.Fatal("duplicate CreateErrand succeeded")
	}
	invalid := sampleErrand("errand-invalid", taskID, "parent-roundtrip", errandScenarioBase)
	invalid.Lifecycle = ""
	if err := repo.CreateErrand(ctx, invalid); !errors.Is(err, models.ErrInvalidErrand) {
		t.Fatalf("invalid CreateErrand = %v, want ErrInvalidErrand", err)
	}

	settled := errandScenarioBase.Add(5 * time.Minute)
	delivered := errandScenarioBase.Add(6 * time.Minute)
	update := *stored
	update.Lifecycle, update.LastOutcome, update.AwaitingResult = models.ErrandLifecycleLaunched, models.ErrandOutcomeDone, false
	update.ResultText, update.ResultTruncated, update.ResultTurnID = "result é", true, "turn-result"
	update.ResultKind, update.ResultState = models.ErrandResultOutput, models.ErrandResultDelivered
	update.DeliveredRoute, update.DeliveryToken, update.DeliveredAt = models.ErrandDeliveryNotification, "token-1", &delivered
	update.LatestSettledTurnID, update.SettledAt = "turn-result", &settled
	if err := repo.UpdateErrand(ctx, &update); err != nil {
		t.Fatalf("UpdateErrand: %v", err)
	}
	if update.Version != 2 {
		t.Fatalf("updated version = %d, want 2", update.Version)
	}
	after, err := repo.GetErrand(ctx, errand.ID)
	if err != nil {
		t.Fatalf("GetErrand after update: %v", err)
	}
	assertErrandFields(t, after, &update)

	immutable := *after
	immutable.Title, immutable.Objective, immutable.ParentSessionID = "changed", "changed", "other-parent"
	immutable.MCPServerIDs, immutable.FirstTurnID, immutable.SubmissionID = []string{"other"}, "other", "other"
	if err := repo.UpdateErrand(ctx, &immutable); err != nil {
		t.Fatalf("UpdateErrand with immutable changes: %v", err)
	}
	if fixed, err := repo.GetErrand(ctx, errand.ID); err != nil || fixed.Title != errand.Title ||
		fixed.Objective != errand.Objective || fixed.ParentSessionID != errand.ParentSessionID ||
		!reflect.DeepEqual(fixed.MCPServerIDs, errand.MCPServerIDs) || fixed.FirstTurnID != errand.FirstTurnID ||
		fixed.SubmissionID != errand.SubmissionID {
		t.Fatalf("immutable errand fields changed: %+v, err = %v", fixed, err)
	}

	stale := *stored
	stale.ResultState = models.ErrandResultSuperseded
	if err := repo.UpdateErrand(ctx, &stale); !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("stale UpdateErrand = %v, want ErrErrandVersionConflict", err)
	}
	if current, err := repo.GetErrand(ctx, errand.ID); err != nil || current.ResultState != models.ErrandResultDelivered {
		t.Fatalf("stale update changed the row: %+v, err = %v", current, err)
	}

	missing := sampleErrand("errand-missing", taskID, "parent-roundtrip", errandScenarioBase)
	missing.Version = 1
	if err := repo.UpdateErrand(ctx, missing); !errors.Is(err, models.ErrErrandNotFound) {
		t.Fatalf("missing UpdateErrand = %v, want ErrErrandNotFound", err)
	}
	if _, err := repo.GetErrand(ctx, missing.ID); !errors.Is(err, models.ErrErrandNotFound) {
		t.Fatalf("missing GetErrand = %v, want ErrErrandNotFound", err)
	}
}

// exerciseErrandConcurrentUpdate proves that exactly one of two writers
// holding the same version wins.
func exerciseErrandConcurrentUpdate(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	taskID := newErrandTask(t, repo, "race")
	mustCreateErrand(t, repo, sampleErrand("errand-race", taskID, "parent-race", errandScenarioBase))
	base, err := repo.GetErrand(ctx, "errand-race")
	if err != nil {
		t.Fatalf("GetErrand: %v", err)
	}
	results := make([]error, 2)
	var wg sync.WaitGroup
	for index, state := range []models.ErrandResultState{models.ErrandResultPending, models.ErrandResultDiscarded} {
		writer := *base
		writer.ResultState = state
		wg.Add(1)
		go func(index int, writer models.Errand) {
			defer wg.Done()
			results[index] = repo.UpdateErrand(ctx, &writer)
		}(index, writer)
	}
	wg.Wait()
	wins, conflicts := 0, 0
	for _, result := range results {
		switch {
		case result == nil:
			wins++
		case errors.Is(result, models.ErrErrandVersionConflict):
			conflicts++
		default:
			t.Fatalf("concurrent UpdateErrand returned %v", result)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("concurrent updates: %d wins, %d conflicts; want exactly one of each", wins, conflicts)
	}
	if final, err := repo.GetErrand(ctx, "errand-race"); err != nil || final.Version != 2 {
		t.Fatalf("final errand = %+v, err = %v; want version 2", final, err)
	}
}
