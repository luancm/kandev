package sqlite

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestErrandBrowserReadRepositoryExposesAdditiveMethods(t *testing.T) {
	repo := newRepoForSessionTests(t)
	for _, name := range []string{"GetErrandBrowserSummary", "GetErrandHistoryPage", "GetErrandInspection"} {
		if method := reflect.ValueOf(repo).MethodByName(name); !method.IsValid() {
			t.Errorf("repository does not expose %s", name)
		}
	}
}

func TestErrandBrowserHistoryIsBoundedAndCompleteListStaysComplete(t *testing.T) {
	exerciseErrandBrowserHistory(t, newRepoForSessionTests(t))
}

func TestPostgresErrandBrowserHistoryIsBoundedAndCompleteListStaysComplete(t *testing.T) {
	exerciseErrandBrowserHistory(t, newPostgresErrandRepo(t))
}

func TestErrandBrowserHistoryNormalizesMalformedTimesAcrossPages(t *testing.T) {
	repo := newRepoForSessionTests(t)
	taskID := newErrandTask(t, repo, "browser-history-time-normalization")
	const parentID = "browser-history-time-normalization-parent"
	rows := []struct {
		id        string
		createdAt string
	}{
		{id: "history-newer", createdAt: "2025-01-01T00:00:00Z"},
		{id: "history-older", createdAt: "2024-01-01T00:00:00Z"},
		{id: "history-invalid-a", createdAt: "not-a-timestamp"},
		{id: "history-epoch-b", createdAt: "1970-01-01T00:00:00Z"},
		{id: "history-year-one", createdAt: "0001-01-01T00:00:00Z"},
	}
	for _, row := range rows {
		errand := sampleErrand(row.id, taskID, parentID, errandScenarioBase)
		markErrandFinished(errand)
		mustCreateErrand(t, repo, errand)
		_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(
			`UPDATE task_session_errands SET created_at = ? WHERE id = ?`,
		), row.createdAt, row.id)
		require.NoError(t, err)
	}

	want := []string{"history-newer", "history-older", "history-epoch-b", "history-invalid-a", "history-year-one"}
	var got []string
	var cursor *models.ErrandHistoryCursor
	for len(got) < len(want) {
		page, err := repo.GetErrandHistoryPage(t.Context(), models.ErrandHistoryPageRequest{
			TaskID: taskID, ParentSessionID: parentID, Limit: 1, Cursor: cursor,
		})
		require.NoError(t, err)
		require.Len(t, page.Errands, 1)
		got = append(got, page.Errands[0].ID)
		cursor = page.NextCursor
		if !page.HasMore {
			break
		}
		require.NotNil(t, cursor)
	}
	require.Equal(t, want, got)

	malformed, err := repo.GetErrandHistoryPage(t.Context(), models.ErrandHistoryPageRequest{
		TaskID: taskID, ParentSessionID: parentID, Limit: 1,
		Cursor: &models.ErrandHistoryCursor{
			Version: 1, TaskID: taskID, ParentSessionID: parentID,
			Order: errBrowserCursorOrder, CreatedAt: time.Unix(0, 0).UTC(), ID: "history-epoch-b",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "history-invalid-a", malformed.Errands[0].ID)
	require.True(t, malformed.Errands[0].CreatedAt.Equal(time.Unix(0, 0).UTC()), "invalid UI timestamps use epoch ordering")
}

func TestErrandBrowserHistoryOffsetFractionRolloverDoesNotDoubleShift(t *testing.T) {
	repo := newRepoForSessionTests(t)
	taskID := newErrandTask(t, repo, "browser-history-offset-fraction-rollover")
	const parentID = "browser-history-offset-fraction-rollover-parent"
	rows := []struct {
		id        string
		createdAt string
	}{
		{id: "positive-rollover", createdAt: "2024-01-02T00:00:00.999999999+01:00"},
		{id: "positive-comparison", createdAt: "2024-01-01T23:00:01.500000000Z"},
		{id: "negative-rollover", createdAt: "2024-01-01T23:00:00.999999999-01:00"},
		{id: "negative-comparison", createdAt: "2024-01-02T00:00:01.500000000Z"},
	}
	for _, row := range rows {
		errand := sampleErrand(row.id, taskID, parentID, errandScenarioBase)
		markErrandFinished(errand)
		mustCreateErrand(t, repo, errand)
		_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(
			`UPDATE task_session_errands SET created_at = ? WHERE id = ?`,
		), row.createdAt, row.id)
		require.NoError(t, err)
		parsed, err := parseTaskActivityTime(row.createdAt)
		require.NoError(t, err)
		var actualKey string
		err = repo.ro.GetContext(t.Context(), &actualKey,
			`SELECT `+errandBrowserCreatedAtKey(repo.ro.DriverName(), "created_at")+` FROM task_session_errands WHERE id = ?`, row.id)
		require.NoError(t, err)
		require.Equal(t, parsed.UTC().Format("2006-01-02 15:04:05.000000000"), actualKey,
			"SQL history key must retain the original fraction after timezone normalization for %q", row.createdAt)
	}

	want := []string{
		"negative-comparison", "negative-rollover",
		"positive-comparison", "positive-rollover",
	}
	var got []string
	var cursor *models.ErrandHistoryCursor
	for range len(want) {
		page, err := repo.GetErrandHistoryPage(t.Context(), models.ErrandHistoryPageRequest{
			TaskID: taskID, ParentSessionID: parentID, Limit: 1, Cursor: cursor,
		})
		require.NoError(t, err)
		require.Len(t, page.Errands, 1)
		got = append(got, page.Errands[0].ID)
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
		require.NotNil(t, cursor)
	}
	require.Equal(t, want, got)
}

func TestErrandBrowserHistoryOrderingMatchesActivityParserAcrossPages(t *testing.T) {
	repo := newRepoForSessionTests(t)
	taskID := newErrandTask(t, repo, "browser-history-supported-time-forms")
	const parentID = "browser-history-supported-time-forms-parent"
	rows := []struct {
		id                string
		createdAt         string
		parserAcceptsTime bool
	}{
		{id: "positive-offset-rollover", createdAt: "2024-01-02T00:00:00.999999999+01:00"},
		{id: "positive-offset-comparison", createdAt: "2024-01-01T23:00:01.500000000Z"},
		{id: "negative-offset-rollover", createdAt: "2024-01-01T23:00:00.999999999-01:00"},
		{id: "negative-offset-comparison", createdAt: "2024-01-02T00:00:01.500000000Z"},
		{id: "hour-boundary-offset", createdAt: "2024-01-08T00:00:00.999999999+24:00"},
		{id: "minute-boundary-offset", createdAt: "2024-01-08T00:00:00.999999999+23:60"},
		{id: "invalid-offset-hour", createdAt: "2024-01-08T00:00:00+25:00"},
		{id: "invalid-offset-minute", createdAt: "2024-01-08T00:00:00+23:61"},
		{id: "space-offset-fraction", createdAt: "2024-01-04 00:00:00.123456789-02:00"},
		{id: "space-offset-comma-fraction", createdAt: "2024-01-04 00:00:00,123456789-02:00"},
		{id: "zone-less-fraction", createdAt: "2024-01-05 00:00:00.123456789"},
		{id: "zone-less-comma-fraction", createdAt: "2024-01-05 00:00:00,234567890"},
		{id: "zone-less-six-digit-fraction", createdAt: "2024-01-05 00:00:00.123456"},
		{id: "zone-less-one-digit-fraction", createdAt: "2024-01-05 00:00:00.1"},
		{id: "zone-less-whole-second", createdAt: "2024-01-05 00:00:00"},
		{id: "single-digit-hour-zone-less-whole-second", createdAt: "2024-01-05 1:00:00", parserAcceptsTime: true},
		{id: "single-digit-hour-zone-less-fraction", createdAt: "2024-01-05 1:00:00.123456789", parserAcceptsTime: true},
		{id: "single-digit-hour-zone-less-comma-fraction", createdAt: "2024-01-05 1:00:00,234567890", parserAcceptsTime: true},
		{id: "single-digit-hour-rfc3339-utc", createdAt: "2024-01-05T1:00:00.345678901Z", parserAcceptsTime: true},
		{id: "single-digit-hour-rfc3339-offset", createdAt: "2024-01-05T1:00:00.456789012+01:30", parserAcceptsTime: true},
		{id: "single-digit-hour-space-offset", createdAt: "2024-01-05 1:00:00.567890123-02:00", parserAcceptsTime: true},
		{id: "single-digit-hour-space-comma-offset", createdAt: "2024-01-05 1:00:00,678901234+02:30", parserAcceptsTime: true},
		{id: "invalid-single-digit-hour-clock", createdAt: "2024-01-05 1x:00:00.123"},
		{id: "invalid-single-digit-hour-range", createdAt: "2024-01-05 24:00:00.123"},
		{id: "rfc3339-utc", createdAt: "2024-01-03T12:34:56Z"},
		{id: "fraction-truncated-to-nanoseconds", createdAt: "2024-01-06T00:00:00.123456789123Z"},
		{id: "invalid-date", createdAt: "2024-02-30T00:00:00Z"},
		{id: "invalid-fraction", createdAt: "2024-01-06T00:00:00.Z"},
		{id: "garbage-zone", createdAt: "2024-01-06T00:00:00Zgarbage"},
		{id: "garbage-after-space-fraction", createdAt: "2024-01-06 00:00:00.123+garbage"},
		{id: "unsupported-t-no-zone", createdAt: "2024-01-07T00:00:00"},
		{id: "year-one", createdAt: "0001-01-01 00:00:00"},
		{id: "year-one-offset-fraction", createdAt: "0001-01-01T00:00:00.999999999+01:00"},
	}
	type expectedRow struct {
		id        string
		createdAt time.Time
	}
	expected := make([]expectedRow, 0, len(rows))
	for _, row := range rows {
		errand := sampleErrand(row.id, taskID, parentID, errandScenarioBase)
		markErrandFinished(errand)
		mustCreateErrand(t, repo, errand)
		_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(
			`UPDATE task_session_errands SET created_at = ? WHERE id = ?`,
		), row.createdAt, row.id)
		require.NoError(t, err)

		createdAt, err := parseTaskActivityTime(row.createdAt)
		if row.parserAcceptsTime {
			require.NoError(t, err, "single-digit-hour fixture %q must be accepted by parseTaskActivityTime", row.createdAt)
		}
		if err != nil {
			createdAt = time.Unix(0, 0).UTC()
		} else {
			createdAt = createdAt.UTC()
		}
		expected = append(expected, expectedRow{id: row.id, createdAt: createdAt})
	}
	sort.Slice(expected, func(i, j int) bool {
		if expected[i].createdAt.Equal(expected[j].createdAt) {
			return expected[i].id < expected[j].id
		}
		return expected[i].createdAt.After(expected[j].createdAt)
	})

	var got []*models.ErrandBrowserObservation
	var cursor *models.ErrandHistoryCursor
	for pageNumber := 0; pageNumber <= len(rows); pageNumber++ {
		page, err := repo.GetErrandHistoryPage(t.Context(), models.ErrandHistoryPageRequest{
			TaskID: taskID, ParentSessionID: parentID, Limit: 2, Cursor: cursor,
		})
		require.NoError(t, err)
		require.Equal(t, len(rows), page.TotalFinished)
		got = append(got, page.Errands...)
		if !page.HasMore {
			require.Nil(t, page.NextCursor)
			break
		}
		require.NotNil(t, page.NextCursor)
		cursor = page.NextCursor
	}
	require.Len(t, got, len(expected), "history cursor pages must return every row exactly once")
	for i, want := range expected {
		require.Equal(t, want.id, got[i].ID, "row %d ordering", i)
		require.True(t, want.createdAt.Equal(got[i].CreatedAt), "row %q projected timestamp: got %s, want %s", want.id, got[i].CreatedAt, want.createdAt)
	}
}

func markErrandFinished(item *models.Errand) {
	item.Lifecycle = models.ErrandLifecycleLaunched
	item.LastOutcome = models.ErrandOutcomeDone
	item.AwaitingResult = false
	item.ResultState = models.ErrandResultDelivered
}

func TestErrandBrowserUnreadMatchesCanonicalMicrosecondRank(t *testing.T) {
	exerciseErrandBrowserUnreadMatchesCanonicalMicrosecondRank(t, newRepoForSessionTests(t))
}

func TestPostgresErrandBrowserUnreadMatchesCanonicalMicrosecondRank(t *testing.T) {
	exerciseErrandBrowserUnreadMatchesCanonicalMicrosecondRank(t, newPostgresErrandRepo(t))
}

func exerciseErrandBrowserUnreadMatchesCanonicalMicrosecondRank(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	taskID := newErrandTask(t, repo, "browser-unread-microsecond")
	const (
		parentID = "browser-unread-parent"
		childID  = "browser-unread-child"
		turnID   = "browser-unread-turn"
	)
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: parentID, TaskID: taskID, Role: "primary",
		State: models.TaskSessionStateWaitingForInput,
	})
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: childID, TaskID: taskID, Role: models.SessionRoleErrand, ParentSessionID: parentID,
		State: models.TaskSessionStateRunning,
	})
	item := sampleErrand(childID, taskID, parentID, errandScenarioBase)
	item.Lifecycle = models.ErrandLifecycleLaunched
	item.AwaitingResult = false
	mustCreateErrand(t, repo, item)
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), turnID, childID, taskID, errandScenarioBase, errandScenarioBase, errandScenarioBase)
	require.NoError(t, err)
	for _, message := range []struct {
		id        string
		createdAt time.Time
	}{
		{id: "z-read-cursor", createdAt: time.Date(2026, 11, 12, 9, 0, 0, 123456100, time.UTC)},
		{id: "a-same-microsecond", createdAt: time.Date(2026, 11, 12, 9, 0, 0, 123456900, time.UTC)},
	} {
		_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
			INSERT INTO task_session_messages
				(id, task_session_id, task_id, turn_id, author_type, author_id, content,
				 requests_input, type, metadata, created_at)
			VALUES (?, ?, ?, ?, 'agent', '', '', 0, 'message', '{}', ?)
		`), message.id, childID, taskID, turnID, message.createdAt)
		require.NoError(t, err)
	}
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE task_sessions SET last_read_message_id = ? WHERE id = ?`,
	), "z-read-cursor", childID)
	require.NoError(t, err)

	authority, err := repo.ListSessionUnreadState(ctx, []string{childID})
	require.NoError(t, err)
	require.False(t, authority[childID].Unread(), "same-microsecond lower ID is not after the read cursor")
	summary, err := repo.GetErrandBrowserSummary(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, summary.Errands, 1)
	require.Equal(t, authority[childID].Unread(), summary.Errands[0].Unread)
}

func TestPostgresErrandHistoryAcceptsZeroTimestampCursor(t *testing.T) {
	repo := newPostgresErrandRepo(t)
	taskID := newErrandTask(t, repo, "browser-history-zero-cursor")
	repo.clockNow = func() time.Time { return time.Time{} }
	for _, id := range []string{"a-zero-created-at", "z-zero-created-at"} {
		item := sampleErrand(id, taskID, "browser-history-zero-parent", time.Time{})
		markErrandFinished(item)
		mustCreateErrand(t, repo, item)
	}

	request := models.ErrandHistoryPageRequest{TaskID: taskID, ParentSessionID: "browser-history-zero-parent", Limit: 1}
	first, err := repo.GetErrandHistoryPage(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "a-zero-created-at", first.Errands[0].ID)
	require.True(t, first.NextCursor.CreatedAt.IsZero())
	request.Cursor = first.NextCursor
	second, err := repo.GetErrandHistoryPage(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, []string{"z-zero-created-at"}, browserErrandIDs(second.Errands))
}

func TestPostgresErrandHistoryCountAndMembershipUseOneSnapshot(t *testing.T) {
	repo := newPostgresErrandBrowserRepoWithWriter(t)
	taskID := newErrandTask(t, repo, "browser-history-snapshot")
	const parentID = "browser-history-snapshot-parent"
	item := sampleErrand("browser-history-snapshot-row", taskID, parentID, errandScenarioBase)
	markErrandFinished(item)
	mustCreateErrand(t, repo, item)

	ctx := t.Context()
	request := models.ErrandHistoryPageRequest{TaskID: taskID, ParentSessionID: parentID, Limit: 25}
	tx, err := repo.beginErrandBrowserReadSnapshot(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	total, err := countFinishedErrandHistory(ctx, tx, repo.ro.DriverName(), request)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_session_errands SET awaiting_result = TRUE, result_state = 'pending' WHERE id = ?
	`), item.ID)
	require.NoError(t, err)
	page, err := readFinishedErrandHistoryPage(ctx, tx, repo.ro.DriverName(), request, total)
	require.NoError(t, err)
	require.Equal(t, 1, page.TotalFinished)
	require.Len(t, page.Errands, 1, "the membership read must observe the same committed state as the count")
}

func TestPostgresErrandSummaryAndParentCountsUseOneSnapshot(t *testing.T) {
	repo := newPostgresErrandBrowserRepoWithWriter(t)
	taskID := newErrandTask(t, repo, "browser-summary-snapshot")
	const parentID = "browser-summary-snapshot-parent"
	item := sampleErrand("browser-summary-snapshot-row", taskID, parentID, errandScenarioBase)
	markErrandFinished(item)
	mustCreateErrand(t, repo, item)

	ctx := t.Context()
	tx, err := repo.beginErrandBrowserReadSnapshot(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	counts, err := readErrandBrowserCounts(ctx, tx, repo.ro.DriverName(), "e.task_id = ?", taskID)
	require.NoError(t, err)
	parentCounts, err := readErrandParentBrowserCounts(ctx, tx, repo.ro.DriverName(), taskID)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_session_errands SET awaiting_result = TRUE, result_state = 'pending' WHERE id = ?
	`), item.ID)
	require.NoError(t, err)
	items, err := readErrandBrowserSummaryRows(ctx, tx, repo.ro.DriverName(), taskID)
	require.NoError(t, err)
	require.Zero(t, counts.Outstanding)
	require.Zero(t, parentCounts[0].Counts.Outstanding)
	require.Empty(t, items, "summary membership must observe the same committed state as task and parent counts")
}

func newPostgresErrandBrowserRepoWithWriter(t *testing.T) *Repository {
	t.Helper()
	repo := newPostgresErrandRepo(t)
	var schema string
	require.NoError(t, repo.ro.Get(&schema, "SELECT current_schema()"))
	dsn := testutil.PostgresDSNWithSearchPath(t, testutil.PostgresDSNFromEnv(t), schema)
	writer, err := db.OpenPostgres(dsn, 2, 1)
	require.NoError(t, err)
	writerDB := sqlx.NewDb(writer, "pgx")
	t.Cleanup(func() { _ = writerDB.Close() })
	repo.db = writerDB
	return repo
}

func seedErrandHistoryContext(t *testing.T, repo *Repository, row *models.Errand) {
	t.Helper()
	ctx := t.Context()
	generation, err := repo.ReserveErrandExecutionGeneration(ctx, row.ID, 0)
	require.NoError(t, err)
	binding := models.ErrandHarnessBinding{
		ErrandID: row.ID, Generation: generation.Generation, ExecutionID: "history-execution",
		IncarnationID: "history-incarnation", HarnessGeneration: 1, StreamID: "history-stream", NativeSessionID: "history-native",
	}
	bound, err := repo.BindErrandExecution(ctx, row.ID, generation.Generation, binding.ExecutionID)
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &binding, 0))
	projectErrandContextWindow(t, repo, row.TaskID, binding, 1, 120000, 3000)
}

func exerciseErrandBrowserHistory(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	taskID := newErrandTask(t, repo, "browser-history")
	parentID := "browser-history-parent"
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: parentID, TaskID: taskID, Name: "Parent conversation", Role: "primary",
		State: models.TaskSessionStateWaitingForInput,
	})
	base := errandScenarioBase.Add(6 * time.Hour)
	for index := 0; index < 27; index++ {
		id := fmt.Sprintf("browser-history-%02d", index)
		row := sampleErrand(id, taskID, parentID, base)
		row.Lifecycle, row.LastOutcome = models.ErrandLifecycleLaunched, models.ErrandOutcomeDone
		row.AwaitingResult, row.ResultState, row.ResultKind = false, models.ErrandResultDelivered, models.ErrandResultOutput
		row.ResultText = "private result body " + id
		row.SettledAt = &base
		createErrandScenarioSession(t, repo, &models.TaskSession{
			ID: id, TaskID: taskID, ParentSessionID: parentID, Role: models.SessionRoleErrand,
			State: models.TaskSessionStateCompleted, StartedAt: base, TokensIn: int64(index + 1),
		})
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_sessions SET tokens_in = ? WHERE id = ?`), index+1, id); err != nil {
			t.Fatalf("set token usage for %s: %v", id, err)
		}
		mustCreateErrand(t, repo, row)
		if index == 0 {
			seedErrandHistoryContext(t, repo, row)
		}
	}
	pending := sampleErrand("browser-history-pending", taskID, parentID, base.Add(time.Minute))
	pending.Lifecycle, pending.LastOutcome = models.ErrandLifecycleLaunched, models.ErrandOutcomeDone
	pending.AwaitingResult, pending.ResultState, pending.ResultKind = false, models.ErrandResultPending, models.ErrandResultOutput
	pending.ResultText = "still owed"
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: pending.ID, TaskID: taskID, ParentSessionID: parentID, Role: models.SessionRoleErrand,
		State: models.TaskSessionStateCompleted,
	})
	mustCreateErrand(t, repo, pending)
	queued := sampleErrand("browser-history-queued", taskID, parentID, base.Add(2*time.Minute))
	queued.AwaitingResult = false
	mustCreateErrand(t, repo, queued)
	running := sampleErrand("browser-history-running", taskID, parentID, base.Add(3*time.Minute))
	running.Lifecycle, running.AwaitingResult = models.ErrandLifecycleLaunched, false
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: running.ID, TaskID: taskID, ParentSessionID: parentID, Role: models.SessionRoleErrand,
		State: models.TaskSessionStateRunning, TokensIn: 80,
	})
	mustCreateErrand(t, repo, running)
	failed := sampleErrand("browser-history-failed", taskID, parentID, base.Add(-time.Minute))
	failed.Lifecycle, failed.LastOutcome, failed.AwaitingResult = models.ErrandLifecycleLaunched, models.ErrandOutcomeFailed, false
	failed.ResultState, failed.ResultKind = models.ErrandResultNone, models.ErrandResultFailure
	createErrandScenarioSession(t, repo, &models.TaskSession{
		ID: failed.ID, TaskID: taskID, ParentSessionID: parentID, Role: models.SessionRoleErrand,
		State: models.TaskSessionStateFailed,
	})
	mustCreateErrand(t, repo, failed)

	first, err := repo.GetErrandHistoryPage(ctx, models.ErrandHistoryPageRequest{TaskID: taskID, ParentSessionID: parentID, Limit: 25})
	require.NoError(t, err)
	require.Len(t, first.Errands, 25)
	require.Equal(t, 28, first.TotalFinished)
	require.True(t, first.HasMore)
	require.NotNil(t, first.NextCursor)
	require.Equal(t, "browser-history-24", first.Errands[len(first.Errands)-1].ID)
	require.False(t, first.Errands[0].Unread)
	require.EqualValues(t, 1, first.Errands[0].TokensIn)
	require.Equal(t, 120000.0, first.Errands[0].ContextMetadata[models.SessionMetaKeyContextWindow].(map[string]interface{})["size"])

	second, err := repo.GetErrandHistoryPage(ctx, models.ErrandHistoryPageRequest{
		TaskID: taskID, ParentSessionID: parentID, Limit: 25, Cursor: first.NextCursor,
	})
	require.NoError(t, err)
	require.Len(t, second.Errands, 3)
	require.False(t, second.HasMore)
	require.Nil(t, second.NextCursor)
	require.Equal(t, []string{"browser-history-25", "browser-history-26", "browser-history-failed"}, browserErrandIDs(second.Errands))

	foreignParent, err := repo.GetErrandHistoryPage(ctx, models.ErrandHistoryPageRequest{TaskID: taskID, ParentSessionID: "other-parent", Limit: 25})
	require.NoError(t, err)
	require.Empty(t, foreignParent.Errands)
	require.Zero(t, foreignParent.TotalFinished)
	foreignTask, err := repo.GetErrandHistoryPage(ctx, models.ErrandHistoryPageRequest{TaskID: "other-task", ParentSessionID: parentID, Limit: 25})
	require.NoError(t, err)
	require.Empty(t, foreignTask.Errands)
	require.Zero(t, foreignTask.TotalFinished)
	_, err = repo.GetErrandHistoryPage(ctx, models.ErrandHistoryPageRequest{TaskID: taskID, ParentSessionID: parentID, Limit: 26})
	require.Error(t, err)

	summary, err := repo.GetErrandBrowserSummary(ctx, taskID)
	require.NoError(t, err)
	require.Equal(t, []string{"browser-history-pending", "browser-history-queued", "browser-history-running"}, browserErrandIDs(summary.Errands))
	require.Equal(t, models.ErrandStatusDone, summary.Errands[0].Status, "a terminal pending result keeps its terminal status")
	require.True(t, summary.Errands[0].Outstanding)
	require.False(t, summary.Errands[0].Live)
	require.Equal(t, 1, summary.Counts.Queued)
	require.Equal(t, 2, summary.Counts.Live, "queued work consumes one capacity slot")
	require.Equal(t, 2, summary.Counts.Outstanding)
	require.Equal(t, 28, summary.Counts.Finished)
	require.Equal(t, 1, summary.Counts.PendingResults)
	require.Len(t, summary.ParentCounts, 1)
	require.Equal(t, parentID, summary.ParentCounts[0].ParentSessionID)
	require.Equal(t, summary.Counts.Live, summary.ParentCounts[0].Counts.Live)

	inspection, err := repo.GetErrandInspection(ctx, taskID, queued.ID)
	require.NoError(t, err)
	require.False(t, inspection.OwnsConversation)
	require.True(t, inspection.Parent.Available)
	require.Equal(t, parentID, inspection.Parent.ID)
	require.Equal(t, "Parent conversation", inspection.Parent.Title)
	inspection, err = repo.GetErrandInspection(ctx, taskID, "browser-history-00")
	require.NoError(t, err)
	require.Equal(t, "browser-history-00", inspection.Errand.ID)
	require.True(t, inspection.Parent.Available)
	_, err = repo.GetErrandInspection(ctx, "other-task", "browser-history-00")
	require.ErrorIs(t, err, models.ErrErrandNotFound)

	all, err := repo.ListErrandsByTask(ctx, taskID)
	require.NoError(t, err)
	require.Len(t, all, 31, "legacy complete list retains history and summary children")
}

func browserErrandIDs(rows []*models.ErrandBrowserObservation) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func TestErrandBrowserSummaryIncludesUnresolvedRecovery(t *testing.T) {
	exerciseErrandBrowserRecoverySummary(t, newRepoForSessionTests(t))
}

func TestPostgresErrandBrowserSummaryIncludesUnresolvedRecovery(t *testing.T) {
	exerciseErrandBrowserRecoverySummary(t, newPostgresErrandRepo(t))
}

func exerciseErrandBrowserRecoverySummary(t *testing.T, repo *Repository) {
	t.Helper()
	row, _ := seedRecoveryAttention(t, repo)
	row.AwaitingResult = false
	require.NoError(t, repo.UpdateErrand(t.Context(), row))

	summary, err := repo.GetErrandBrowserSummary(t.Context(), row.TaskID)
	require.NoError(t, err)
	require.Len(t, summary.Errands, 1)
	require.Equal(t, row.ID, summary.Errands[0].ID)
	require.Equal(t, models.ErrandStatusNeedsAttention, summary.Errands[0].Status)
	require.Equal(t, models.ErrandAttentionRecoveryUnresolved, summary.Errands[0].AttentionReason)
	require.True(t, summary.Errands[0].Outstanding)
	require.True(t, summary.Errands[0].Live)
	require.Equal(t, 1, summary.Counts.Live)
	require.Equal(t, 1, summary.Counts.Outstanding)
}

func TestErrandBrowserHistoryIndexSupportsScopedKeysetOrder(t *testing.T) {
	exerciseErrandBrowserHistoryIndex(t, newRepoForSessionTests(t))
}

func TestPostgresErrandBrowserHistoryIndexSupportsScopedKeysetOrder(t *testing.T) {
	exerciseErrandBrowserHistoryIndex(t, newPostgresErrandRepo(t))
}

func exerciseErrandBrowserHistoryIndex(t *testing.T, repo *Repository) {
	t.Helper()
	indexExists, err := db.IndexExistsContext(t.Context(), repo.ro, errBrowserHistoryIndexName)
	require.NoError(t, err)
	require.True(t, indexExists, "history ordering needs an index on the normalized SQLite key or typed PostgreSQL timestamp")
	taskID := newErrandTask(t, repo, "browser-plan")
	const parentID = "browser-plan-parent-00"
	for i := 0; i < 1024; i++ {
		row := sampleErrand(fmt.Sprintf("browser-plan-%04d", i), taskID,
			fmt.Sprintf("browser-plan-parent-%02d", i%32), errandScenarioBase.Add(time.Duration(i)*time.Second))
		row.Lifecycle, row.LastOutcome, row.AwaitingResult = models.ErrandLifecycleLaunched, models.ErrandOutcomeDone, false
		row.ResultState = models.ErrandResultDelivered
		mustCreateErrand(t, repo, row)
	}
	_, err = repo.db.ExecContext(t.Context(), "ANALYZE task_session_errands")
	require.NoError(t, err)
	request := models.ErrandHistoryPageRequest{TaskID: taskID, ParentSessionID: parentID, Limit: 25}
	first, err := repo.GetErrandHistoryPage(t.Context(), request)
	require.NoError(t, err)
	require.Len(t, first.Errands, 25)
	require.True(t, first.HasMore)
	require.NotNil(t, first.NextCursor)
	for _, current := range []models.ErrandHistoryPageRequest{
		request,
		{TaskID: taskID, ParentSessionID: parentID, Limit: 25, Cursor: first.NextCursor},
	} {
		plan := explainErrandBrowserHistoryQuery(t, repo, current)
		t.Logf("production history plan (driver=%s, continuation=%t):\n%s",
			repo.ro.DriverName(), current.Cursor != nil, plan)
		require.Contains(t, plan, errBrowserHistoryIndexName, "production history query plan: %s", plan)
		if dialect.IsPostgres(repo.ro.DriverName()) {
			if strings.Contains(plan, "Sort Key: e.created_at") {
				require.Contains(t, plan, `Sort Key: e.created_at DESC, e.id COLLATE "C"`, "production history query plan: %s", plan)
			}
		} else {
			require.NotContains(t, plan, "0/0 USE TEMP B-TREE FOR ORDER BY", "outer history query plan: %s", plan)
		}
	}
}

func explainErrandBrowserHistoryQuery(
	t *testing.T,
	repo *Repository,
	request models.ErrandHistoryPageRequest,
) string {
	t.Helper()
	query, args := errandBrowserHistoryPageQuery(repo.ro.DriverName(), request)
	prefix := "EXPLAIN "
	if !dialect.IsPostgres(repo.ro.DriverName()) {
		prefix = "EXPLAIN QUERY PLAN "
	}
	rows, err := repo.ro.QueryxContext(t.Context(), repo.ro.Rebind(prefix+query), args...)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	var lines []string
	for rows.Next() {
		if dialect.IsPostgres(repo.ro.DriverName()) {
			var line string
			require.NoError(t, rows.Scan(&line))
			lines = append(lines, line)
			continue
		}
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		lines = append(lines, fmt.Sprintf("%d/%d %s", id, parent, detail))
	}
	require.NoError(t, rows.Err())
	return strings.Join(lines, "\n")
}
