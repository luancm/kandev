package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandClaimLegacyUpdateCannotBypassAdmissionFence(t *testing.T) {
	exerciseErrandLegacyRelease(t, newRepoForSessionTests(t))
}

func exerciseErrandLegacyRelease(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	store := admissionRepository(t, repo)
	row := newAdmissionResult(t, repo, "legacy-release")
	claimAdmissionResult(t, repo, row, models.ErrandDeliveryTurnContext)
	admission := parentAdmission(row, "reserved")
	if _, err := store.ReserveErrandParentAdmission(ctx, admission); err != nil {
		t.Fatal(err)
	}
	row.ResultState, row.DeliveredRoute, row.DeliveryToken, row.DeliveredAt = models.ErrandResultPending, "", "", nil
	if err := repo.UpdateErrand(ctx, row); err != nil {
		t.Fatal(err)
	}
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultDelivered)
	if err := store.DecideErrandParentAdmission(ctx, admission, models.ErrandAdmissionRefused); err != nil {
		t.Fatal(err)
	}
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultPending)
}

func TestErrandClaimReleaseRequiresExactIdentity(t *testing.T) {
	exerciseErrandReleaseIdentity(t, newRepoForSessionTests(t))
}

func exerciseErrandReleaseIdentity(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	store := admissionRepository(t, repo)
	row := newAdmissionResult(t, repo, "release-identity")
	claim := claimAdmissionResult(t, repo, row, models.ErrandDeliveryTurnContext)
	stored, readErr := repo.ListErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, claim.Token)
	if readErr != nil || len(stored) != 1 || stored[0] != claim {
		t.Fatalf("immutable release receipt: %+v, %v", stored, readErr)
	}
	for _, field := range []string{"revision", "generation", "token", "route", "errand"} {
		t.Run(field, func(t *testing.T) {
			wrong := claim
			switch field {
			case "revision":
				wrong.ResultRevision = "other-revision"
			case "generation":
				wrong.ParentAdmissionGeneration++
			case "token":
				wrong.Token = "other-token"
			case "route":
				wrong.Route = models.ErrandDeliveryNotification
			case "errand":
				wrong.ErrandID = "other-errand"
			}
			changed, err := store.ReleaseErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, []models.ErrandResultClaim{wrong})
			if err != nil || len(changed) != 0 {
				t.Fatalf("wrong identity released a claim: %+v, %v", changed, err)
			}
			assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultDelivered)
		})
	}
	// A newer content revision with another token must survive the old abort.
	row.ResultText, row.ResultState = "replacement", models.ErrandResultPending
	row.DeliveredRoute, row.DeliveryToken, row.DeliveredAt = "", "", nil
	if err := repo.UpdateErrand(ctx, row); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryNotification, "new-token",
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, claimInventory(t, repo, row.ParentSessionID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReleaseErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, []models.ErrandResultClaim{claim}); err != nil {
		t.Fatal(err)
	}
	current, err := repo.GetErrand(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertErrandFields(t, current, rows[0])
	newClaim := models.ErrandResultClaim{ErrandID: current.ID, ResultRevision: current.ResultRevision,
		ParentAdmissionGeneration: current.ParentAdmissionGeneration, Token: current.DeliveryToken, Route: current.DeliveredRoute}
	if _, err := store.ReleaseErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, []models.ErrandResultClaim{newClaim}); err != nil {
		t.Fatal(err)
	}
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultPending)
}

func TestErrandClaimAbortTombstonePreventsDelayedClaim(t *testing.T) {
	exerciseErrandAbortTombstone(t, newRepoForSessionTests(t))
}

func exerciseErrandAbortTombstone(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	store := admissionRepository(t, repo)
	row := newAdmissionResult(t, repo, "abort-tombstone")
	if _, err := store.AbortAwait(ctx, row.TaskID, row.ParentSessionID, "late-token"); err != nil {
		t.Fatal(err)
	}
	_, err := repo.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryAwait, "late-token",
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, claimInventory(t, repo, row.ParentSessionID))
	if !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("late claim escaped abort: %v", err)
	}
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultPending)
	if _, err := store.AbortAwait(ctx, row.TaskID, "wrong-parent", "late-token"); !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("foreign parent consumed tombstone: %v", err)
	}
}

func TestErrandClaimDoesNotReplayUnknownExposure(t *testing.T) {
	exerciseErrandClaimUnknownExposure(t, newRepoForSessionTests(t))
}

func TestErrandClaimReceivedAttemptCannotBeAborted(t *testing.T) {
	exerciseErrandReceivedAbort(t, newRepoForSessionTests(t))
}

func exerciseErrandReceivedAbort(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	store := admissionRepository(t, repo)
	row := newAdmissionResult(t, repo, "received")
	claim := claimAdmissionResult(t, repo, row, models.ErrandDeliveryAwait)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE errand_await_attempt SET state = 'received' WHERE token = ?`), claim.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AbortAwait(ctx, row.TaskID, row.ParentSessionID, claim.Token); !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("received attempt was aborted: %v", err)
	}
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultDelivered)
	if _, err := store.ReleaseErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, []models.ErrandResultClaim{claim}); !errors.Is(err, models.ErrInvalidErrand) {
		t.Fatalf("generic release bypassed the await attempt: %v", err)
	}
	current, err := repo.GetErrand(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	row.DeliveryState = models.ErrandDeliveryUncertain
	assertErrandFields(t, current, row)
}

func exerciseErrandClaimUnknownExposure(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	row := newAdmissionResult(t, repo, "exposure")
	claimAdmissionResult(t, repo, row, models.ErrandDeliveryAwait)
	if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE errand_await_attempt SET state = 'uncertain' WHERE token = ?`), row.DeliveryToken); err != nil {
		t.Fatal(err)
	}
	_, err := repo.ClaimResults(ctx, row.TaskID, row.ParentSessionID, row.DeliveredRoute, row.DeliveryToken,
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, claimInventory(t, repo, row.ParentSessionID))
	if !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("unknown exposure was replayed: %v", err)
	}
}
