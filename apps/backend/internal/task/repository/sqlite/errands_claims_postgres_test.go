package sqlite

import "testing"

func TestPostgresErrandResultRevisionTracksReplacement(t *testing.T) {
	exerciseErrandRevision(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandClaimIdentityCannotBeReused(t *testing.T) {
	exerciseErrandClaimIdentityCannotBeReused(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandBatchClaimAtomicOnSecondWriteFailure(t *testing.T) {
	exerciseErrandBatchClaimAtomic(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandBatchClaimRejectsInvalidSnapshotAtomically(t *testing.T) {
	exerciseErrandInvalidClaim(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandClaimWaitsForAdmissionDecision(t *testing.T) {
	exerciseErrandClaimAdmissionFence(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandParentAdmissionDecisions(t *testing.T) {
	exerciseErrandParentAdmissionDecisions(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandParentAdmissionPreservesPostReservationResult(t *testing.T) {
	exerciseErrandAdmissionPreservesReplacement(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandAdmissionResolutionRetriesDeferredRelease(t *testing.T) {
	exerciseErrandAdmissionDeferredRelease(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandParentAdmissionRejectsStaleIdentityAndIgnoresUser(t *testing.T) {
	exerciseErrandAdmissionIdentity(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandClaimLegacyUpdateCannotBypassAdmissionFence(t *testing.T) {
	exerciseErrandLegacyRelease(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandClaimReleaseRequiresExactIdentity(t *testing.T) {
	exerciseErrandReleaseIdentity(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandClaimAbortTombstonePreventsDelayedClaim(t *testing.T) {
	exerciseErrandAbortTombstone(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandClaimDoesNotReplayUnknownExposure(t *testing.T) {
	exerciseErrandClaimUnknownExposure(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandClaimReceivedAttemptCannotBeAborted(t *testing.T) {
	exerciseErrandReceivedAbort(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandAdmissionDecisionAndDeferredReleaseRollbackTogether(t *testing.T) {
	exerciseErrandAdmissionRollback(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandClaimAbortBatchRollsBackOnSecondWriteFailure(t *testing.T) {
	exerciseErrandAbortRollback(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandAdmissionGenerationIsRepositoryOwned(t *testing.T) {
	exerciseErrandAdmissionGenerationOwnership(t, newPostgresErrandRepo(t))
}

func TestPostgresErrandAdmissionStorageMigrationReplaysAndRetainsEvidence(t *testing.T) {
	exerciseErrandDeliveryMigration(t, newPostgresErrandRepo(t))
}
