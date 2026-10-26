package sqlite

import "testing"

// @covers AC-TASKS-ERRANDS-005.2
func TestSetSessionPrimaryRefusesErrandSQLite(t *testing.T) {
	exerciseErrandNeverPrimary(t, newRepoForSessionTests(t))
}

// @covers AC-TASKS-ERRANDS-005.3
func TestErrandSkippedByActiveSessionLookupSQLite(t *testing.T) {
	exerciseActiveSessionLookupSkipsErrands(t, newRepoForSessionTests(t))
}

// @covers AC-TASKS-ERRANDS-005.2
func TestErrandNeverClaimsTaskTitleSQLite(t *testing.T) {
	exerciseErrandNeverClaimsTaskTitle(t, newRepoForSessionTests(t))
}
