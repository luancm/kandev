package orchestrator

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// TestErrandRecoveryBrowserFixture marks unresolved recovery on the exact
// active owned prompt. It leaves both the provider and journal untouched.
func TestErrandRecoveryBrowserFixture(t *testing.T) {
	root := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_ROOT")
	if root == "" {
		t.Skip("used by the isolated Errand recovery browser test")
	}
	root = validateErrandHistoryFixtureRoot(t, root)
	id := os.Getenv("KANDEV_E2E_ERRAND_RECOVERY_ID")
	require.NotEmpty(t, id)
	conn, err := db.OpenSQLite(filepath.Join(root, "kandev.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	sqlConn := sqlx.NewDb(conn, "sqlite3")
	repo, err := tasksqlite.NewWithDB(sqlConn, sqlConn, nil)
	require.NoError(t, err)
	ctx := t.Context()
	current, err := repo.GetErrandExecutionDescriptor(ctx, id)
	require.NoError(t, err)
	var binding models.ErrandHarnessBinding
	binding.ErrandID, binding.Generation, binding.ExecutionID = id, current.Generation, current.ExecutionID
	err = sqlConn.QueryRowContext(ctx, `SELECT incarnation_id,harness_generation,stream_id,native_session_id
		FROM errand_harness_bindings WHERE errand_id=? AND owner_generation=? AND execution_id=?
		ORDER BY binding_sequence DESC LIMIT 1`, id, current.Generation, current.ExecutionID).
		Scan(&binding.IncarnationID, &binding.HarnessGeneration, &binding.StreamID, &binding.NativeSessionID)
	require.NoError(t, err)
	stored, err := repo.GetAgentDeliverySubmissionMetadata(ctx, current.ActiveSubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionAccepted, stored.State)
	changed, err := repo.RecordErrandRecoveryObservation(ctx, models.ErrandRecoveryObservation{
		TaskID: current.TaskID, Binding: binding, TurnID: current.ActiveTurnID,
		SubmissionID: current.ActiveSubmissionID, PromptGeneration: current.PromptGeneration, Unresolved: true,
	})
	require.NoError(t, err)
	require.True(t, changed)
	_, err = repo.GetTaskSession(ctx, id)
	require.ErrorIs(t, err, models.ErrTaskSessionNotFound)
	_, _, err = repo.ReadOwnedErrandLatestTerminal(ctx, id)
	require.ErrorIs(t, err, sql.ErrNoRows)
	fmt.Println("KANDEV_E2E_ERRAND_RECOVERY_FIXTURE: attention")
}
