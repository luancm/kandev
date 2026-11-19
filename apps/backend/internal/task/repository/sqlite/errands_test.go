package sqlite

import "testing"

func TestOwnedErrandSchemaReplaysSQLite(t *testing.T) {
	exerciseErrandSchemaReplay(t, newRepoForSessionTests(t))
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

func TestErrandsSummaryByTaskSQLite(t *testing.T) {
	exerciseErrandsSummary(t, newRepoForSessionTests(t))
}

func TestErrandListingAndLiveCountSQLite(t *testing.T) {
	exerciseErrandListingAndLiveCount(t, newRepoForSessionTests(t))
}

func TestErrandRowIsDeletedWithItsSessionAndTaskSQLite(t *testing.T) {
	exerciseErrandDeletion(t, newRepoForSessionTests(t))
}
