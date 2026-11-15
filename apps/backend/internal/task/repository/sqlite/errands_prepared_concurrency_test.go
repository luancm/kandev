package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPreparedErrandRestoreSerializesOwnershipRelease(t *testing.T) {
	for _, mode := range []string{"fifo", "send_now"} {
		t.Run(mode, func(t *testing.T) {
			exercisePreparedRestoreRelease(t, newRepoForSessionTests(t), mode)
		})
	}
}

func TestPostgresPreparedErrandRestoreSerializesOwnershipRelease(t *testing.T) {
	testutil.PostgresDSNFromEnv(t)
	for _, mode := range []string{"fifo", "send_now"} {
		t.Run(mode, func(t *testing.T) {
			exercisePreparedRestoreRelease(t, newPostgresErrandRepo(t), mode)
		})
	}
}

func exercisePreparedRestoreRelease(t *testing.T, repo *Repository, mode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	row, identity, source := preparedOwnershipSource(t, repo)
	_, err := repo.AdmitPreparedErrand(ctx, identity, source, 10)
	require.NoError(t, err)
	claims, err := repo.ListErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, "prepared-source-token")
	require.NoError(t, err)
	require.Len(t, claims, 1)
	restoreGate := newErrandSQLGate("unused", false)
	restorer := independentErrandRepository(t, repo, restoreGate)
	queueRepo, err := messagequeue.NewSQLiteRepository(restorer.db, restorer.ro)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, 10, logger.Default())
	restore := reservePreparedForRestore(t, ctx, queue, identity, mode)
	restoreGate.prefix = "INSERT INTO queue_session_locks"
	readPrefix := "SELECT claim_id, claim_json FROM queue_send_now_claims"
	if mode == "fifo" {
		restoreGate.prefix = "UPDATE tasks SET updated_at = updated_at"
		readPrefix = "SELECT entry_id, attempt_id, message_json FROM queue_dispatch_claims"
	}
	// Pending sources have already been read. A concurrent restore must not
	// remove the claim before this reader observes that half of the authority.
	readGate := newErrandSQLGate(readPrefix, true)
	releaser := independentErrandRepository(t, repo, readGate)
	var holderPID, waiterPID int
	if dialect.IsPostgres(repo.db.DriverName()) {
		require.NoError(t, releaser.db.GetContext(ctx, &holderPID, `SELECT pg_backend_pid()`))
		require.NoError(t, restorer.db.GetContext(ctx, &waiterPID, `SELECT pg_backend_pid()`))
	}
	type releaseResult struct {
		rows []*models.Errand
		err  error
	}
	released := make(chan releaseResult, 1)
	restored := make(chan error, 1)
	releaseDone, restoreDone := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		cancel()
		readGate.unblock()
		awaitSignal(t, releaseDone, "release goroutine exited")
		awaitSignal(t, restoreDone, "restore goroutine exited")
	})
	go func() {
		defer close(releaseDone)
		rows, err := releaser.ReleaseErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, claims)
		released <- releaseResult{rows, err}
	}()
	// Start restore even on an early release failure, so cleanup can join both.
	go func() {
		defer close(restoreDone)
		select {
		case <-readGate.entered:
			restored <- restore(ctx)
		case <-ctx.Done():
			restored <- ctx.Err()
		}
	}()
	awaitSignal(t, readGate.entered, "release read pending sources before claim inspection")
	awaitSignal(t, restoreGate.entered, "independent refusal restore attempts queue ownership")
	if holderPID != 0 {
		waitPreparedRestorePostgresLock(t, ctx, repo, holderPID, waiterPID, restored)
	}
	readGate.unblock()
	result := <-released
	require.NoError(t, result.err)
	require.Empty(t, result.rows, "release must observe either the live claim or its restored source")
	require.NoError(t, awaitError(t, restored, "refusal restores after ownership observation commits"))
	assertAdmissionResult(t, repo, row.ID, row.ParentAdmissionGeneration, models.ErrandResultDelivered)
	entries := queue.GetStatus(ctx, identity.SessionID).Entries
	require.Len(t, entries, 1)
	require.Equal(t, source.ID, entries[0].ID)
	require.Equal(t, source.Content, entries[0].Content)
	reopened := independentErrandRepository(t, repo, newErrandSQLGate("unused", false))
	changed, err := reopened.ReleaseErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, claims)
	require.NoError(t, err)
	require.Empty(t, changed, "restored full source retains exact ownership after reopen")
}

func reservePreparedForRestore(t *testing.T, ctx context.Context, queue *messagequeue.Service, identity messagequeue.QueueSessionIdentity, mode string) func(context.Context) error {
	t.Helper()
	if mode == "send_now" {
		claim, err := queue.ClaimSendNowForSession(ctx, identity, queue.GetStatus(ctx, identity.SessionID).Entries)
		require.NoError(t, err)
		return func(ctx context.Context) error { return queue.RestoreSendNowClaim(ctx, claim) }
	}
	require.NoError(t, queue.SetAutoRun(ctx, identity.SessionID, true))
	reserved, exists, _, err := queue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, exists)
	return func(ctx context.Context) error { _, err := queue.RestoreMessage(ctx, reserved); return err }
}

func waitPreparedRestorePostgresLock(t *testing.T, ctx context.Context, repo *Repository, holderPID, waiterPID int, restored <-chan error) {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var blocked bool
		require.NoError(t, repo.db.QueryRowContext(ctx, `SELECT $1=ANY(pg_blocking_pids($2))`, holderPID, waiterPID).Scan(&blocked))
		if blocked {
			t.Logf("restore PID %d waits for ownership reader PID %d", waiterPID, holderPID)
			return
		}
		select {
		case err := <-restored:
			t.Fatalf("restore crossed the unfinished ownership observation: %v", err)
		case <-ctx.Done():
			t.Fatal("restore never waited for the ownership reader", ctx.Err())
		case <-tick.C:
		}
	}
}
