package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

func newPostgresErrandRepo(t *testing.T) *Repository {
	t.Helper()
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := conn.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}
	repo, err := NewWithDB(conn, conn, nil)
	if err != nil {
		t.Fatalf("NewWithDB on postgres: %v", err)
	}
	return repo
}

func TestPostgresErrandSchemaReplaysOnFreshAndLegacyDatabases(t *testing.T) {
	repo := newPostgresErrandRepo(t)
	exerciseErrandSchemaReplay(t, repo, func() error {
		_, err := NewWithDB(repo.db, repo.ro, nil)
		return err
	})
}

// @covers AC-TASKS-ERRANDS-001.1
func TestPostgresSessionRoleSurvivesEveryReadAndFullRowWrite(t *testing.T) {
	exerciseSessionRolePersistence(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandRowRoundTripsWithVersionGuard(t *testing.T) {
	exerciseErrandRoundTrip(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandTeardownIntentRoundTrips(t *testing.T) {
	repo := newPostgresErrandRepo(t)
	row := persistErrandTeardownIntent(t, repo, "postgres-roundtrip")
	got, err := repo.GetErrand(t.Context(), row.ID)
	if err != nil {
		t.Fatalf("GetErrand: %v", err)
	}
	assertErrandFields(t, got, row)
}

func TestPostgresErrandTeardownIntentMigrationReplays(t *testing.T) {
	exerciseErrandTeardownIntentMigration(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandTeardownIntentMalformedJSONAndExplicitEmpty(t *testing.T) {
	exerciseErrandTeardownIntentMalformedJSONAndExplicitEmpty(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandCancellationAndStopIntentCommitTogether(t *testing.T) {
	exerciseErrandCancellationAndStopIntentCommitTogether(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandTeardownIntentCASDoesNotOverwriteSuccessor(t *testing.T) {
	exerciseErrandTeardownIntentCASDoesNotOverwriteSuccessor(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandConcurrentUpdateHasOneWinner(t *testing.T) {
	exerciseErrandConcurrentUpdate(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandsSummaryByTask(t *testing.T) {
	exerciseErrandsSummary(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandListingAndLiveCount(t *testing.T) {
	exerciseErrandListingAndLiveCount(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandRowIsDeletedWithItsSessionAndTask(t *testing.T) {
	exerciseErrandDeletion(t, newPostgresErrandRepo(t))
}

// @covers AC-TASKS-ERRANDS-005.2
func TestPostgresSetSessionPrimaryRefusesErrand(t *testing.T) {
	exerciseErrandNeverPrimary(t, newPostgresErrandRepo(t))
}

// @covers AC-TASKS-ERRANDS-005.3
func TestPostgresErrandSkippedByActiveSessionLookup(t *testing.T) {
	exerciseActiveSessionLookupSkipsErrands(t, newPostgresErrandRepo(t))
}

// @covers AC-TASKS-ERRANDS-005.2
func TestPostgresErrandNeverClaimsTaskTitle(t *testing.T) {
	exerciseErrandNeverClaimsTaskTitle(t, newPostgresErrandRepo(t))
}
