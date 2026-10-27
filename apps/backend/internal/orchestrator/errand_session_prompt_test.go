package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errTestErrandLookup = errors.New("errand lookup unavailable")

// errandRowRepo overlays GetErrand on the real repository so a test controls
// the durable errand row and counts the lookups.
type errandRowRepo struct {
	sessionExecutorStore
	row   *models.Errand
	err   error
	calls int
}

func (r *errandRowRepo) GetErrand(_ context.Context, _ string) (*models.Errand, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return r.row, nil
}

// plainRowRepo exposes only the session store, so no errand row reader exists.
type plainRowRepo struct {
	sessionExecutorStore
}

const (
	errandReplayTaskID        = "task-1"
	errandReplayParentSession = "parent-session"
	errandReplayErrandID      = "errand-1"
)

// seedErrandReplayFixture creates a parent session and one errand session on
// the same task, and returns the errand session with its task.
func seedErrandReplayFixture(t *testing.T, repo *sqliterepo.Repository) (*models.TaskSession, *models.Task) {
	t.Helper()
	ctx := context.Background()
	seedSession(t, repo, errandReplayTaskID, errandReplayParentSession, "")
	now := time.Now().UTC()
	errandSession := &models.TaskSession{
		ID: errandReplayErrandID, TaskID: errandReplayTaskID, Role: models.SessionRoleErrand,
		ParentSessionID: errandReplayParentSession, State: models.TaskSessionStateStarting,
		StartedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, errandSession))
	dbTask, err := repo.GetTask(ctx, errandReplayTaskID)
	require.NoError(t, err)
	return errandSession, dbTask
}

func errandReplayRow() *models.Errand {
	return &models.Errand{
		ID: errandReplayErrandID, TaskID: errandReplayTaskID, ParentSessionID: errandReplayParentSession,
		Title: "Check logs",
	}
}

// A replayed errand first submission must carry the same trusted errand block
// that first delivery does, looked up from the durable row.
func TestInitialSubmissionReplayCarriesErrandBlock(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	session, _ := seedErrandReplayFixture(t, repo)
	rows := &errandRowRepo{sessionExecutorStore: repo, row: errandReplayRow()}
	svc := &Service{repo: rows}

	got, err := svc.wrapInitialSubmissionReplayPrompt(
		ctx, errandReplayTaskID, errandReplayErrandID, "Find the failing test", nil, session,
	)

	require.NoError(t, err)
	assert.Equal(t, 1, rows.calls)
	assert.Contains(t, got, sysprompt.Wrap(sysprompt.FormatErrandContext(errandReplayParentSession, "Check logs")))
	assert.Contains(t, got, "Find the failing test")
	assert.Contains(t, got, "errand of session "+errandReplayParentSession)
	assert.NotContains(t, got, "start_errand_kandev")
	assert.NotContains(t, got, "ask_user_question_kandev")
}

// The replayed errand block must match first delivery byte for byte, so the
// two prompts can be compared directly.
func TestInitialSubmissionReplayErrandBlockMatchesFirstDelivery(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	session, _ := seedErrandReplayFixture(t, repo)
	svc := &Service{repo: &errandRowRepo{sessionExecutorStore: repo, row: errandReplayRow()}}

	replayed, err := svc.wrapInitialSubmissionReplayPrompt(
		ctx, errandReplayTaskID, errandReplayErrandID, "Find the failing test", nil, session,
	)
	require.NoError(t, err)

	first := (&Service{}).applyLaunchPromptContext(ctx, launchPromptContext{
		prompt:        "Find the failing test",
		taskID:        errandReplayTaskID,
		sessionID:     errandReplayErrandID,
		errand:        &ErrandLaunch{ErrandID: errandReplayErrandID, ParentSessionID: errandReplayParentSession, Title: "Check logs"},
		configMode:    false,
		isPassthrough: false,
	})
	assert.Contains(t, first, sysprompt.Wrap(sysprompt.FormatErrandContext(errandReplayParentSession, "Check logs")))
	assert.Contains(t, replayed, sysprompt.Wrap(sysprompt.FormatErrandContext(errandReplayParentSession, "Check logs")))
}

// An ordinary session replays exactly as before and never reads the errand
// table, so the feature-off path is unchanged.
func TestInitialSubmissionReplayOrdinarySessionIsUnchanged(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-ordinary", "session-ordinary", "")
	session, err := repo.GetTaskSession(ctx, "session-ordinary")
	require.NoError(t, err)
	rows := &errandRowRepo{sessionExecutorStore: repo}
	svc := &Service{repo: rows}

	got, err := svc.wrapInitialSubmissionReplayPrompt(ctx, "task-ordinary", "session-ordinary", "hello", nil, session)

	require.NoError(t, err)
	assert.Zero(t, rows.calls)
	want := sysprompt.InjectKandevContextWithOptions("task-ordinary", "session-ordinary", "hello", sysprompt.KandevContextOptions{
		IncludeCoordinatorTaskControls: true,
		IncludeUserQuestionTool:        true,
	})
	assert.Equal(t, want, got)
}

// Without a row reader, or with a row that disagrees with its session, an
// errand replay fails closed instead of running without its errand block.
func TestInitialSubmissionReplayFailsClosedWithoutTrustedErrandRow(t *testing.T) {
	ctx := context.Background()
	mismatched := errandReplayRow()
	mismatched.ParentSessionID = "other-parent"
	cases := map[string]sessionExecutorStore{
		"no row reader":  nil,
		"lookup error":   &errandRowRepo{err: errTestErrandLookup},
		"mismatched row": &errandRowRepo{row: mismatched},
		"missing row":    &errandRowRepo{row: nil},
	}
	for name, store := range cases {
		t.Run(name, func(t *testing.T) {
			repo := setupTestRepo(t)
			session, _ := seedErrandReplayFixture(t, repo)
			if store == nil {
				store = plainRowRepo{sessionExecutorStore: repo}
			} else if overlay, ok := store.(*errandRowRepo); ok {
				overlay.sessionExecutorStore = repo
			}
			svc := &Service{repo: store}

			_, err := svc.wrapInitialSubmissionReplayPrompt(
				ctx, errandReplayTaskID, errandReplayErrandID, "Find the failing test", nil, session,
			)

			require.Error(t, err)
		})
	}
}
