package sqlite

import "testing"

func TestErrandSchemaReplaysOnFreshAndLegacySQLite(t *testing.T) {
	repo := newRepoForSessionTests(t)
	exerciseErrandSchemaReplay(t, repo, func() error {
		_, err := NewWithDB(repo.db, repo.ro, nil)
		return err
	})
}

// @covers AC-TASKS-ERRANDS-001.1
func TestSessionRoleSurvivesEveryReadAndFullRowWriteSQLite(t *testing.T) {
	exerciseSessionRolePersistence(t, newRepoForSessionTests(t))
}

func TestErrandRowRoundTripsWithVersionGuardSQLite(t *testing.T) {
	exerciseErrandRoundTrip(t, newRepoForSessionTests(t))
}

func TestErrandConcurrentUpdateHasOneWinnerSQLite(t *testing.T) {
	exerciseErrandConcurrentUpdate(t, newRepoForSessionTests(t))
}

func TestErrandListingAndLiveCountSQLite(t *testing.T) {
	exerciseErrandListingAndLiveCount(t, newRepoForSessionTests(t))
}

func TestErrandRowIsDeletedWithItsSessionAndTaskSQLite(t *testing.T) {
	exerciseErrandDeletion(t, newRepoForSessionTests(t))
}
