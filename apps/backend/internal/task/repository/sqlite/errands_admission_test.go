package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type errandAdmissionRepository interface {
	ReserveErrandParentAdmission(context.Context, models.ErrandParentAdmission) (models.ErrandAdmissionReservation, error)
	DecideErrandParentAdmission(context.Context, models.ErrandParentAdmission, models.ErrandAdmissionState) error
	ReleaseErrandResultClaims(context.Context, string, string, []models.ErrandResultClaim) ([]*models.Errand, error)
	AbortAwait(context.Context, string, string, string) ([]*models.Errand, error)
}

func admissionRepository(t *testing.T, repo *Repository) errandAdmissionRepository {
	t.Helper()
	store, ok := any(repo).(errandAdmissionRepository)
	if !ok {
		t.Fatal("repository cannot reserve/resolve durable parent admissions and release exact claims")
	}
	return store
}

func TestErrandParentAdmissionDecisions(t *testing.T) {
	exerciseErrandParentAdmissionDecisions(t, newRepoForSessionTests(t))
}

func exerciseErrandParentAdmissionDecisions(t *testing.T, repo *Repository) {
	t.Helper()
	store := admissionRepository(t, repo)
	ctx := context.Background()
	for _, decision := range []models.ErrandAdmissionState{models.ErrandAdmissionAccepted, models.ErrandAdmissionRefused, models.ErrandAdmissionUnknown} {
		t.Run(string(decision), func(t *testing.T) {
			row := newAdmissionResult(t, repo, string(decision))
			admission := parentAdmission(row, "successor")
			reservation, err := store.ReserveErrandParentAdmission(ctx, admission)
			if err != nil || !reservation.Created || reservation.State != models.ErrandAdmissionReserved {
				t.Fatalf("new reservation=%+v, err=%v", reservation, err)
			}
			assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultPending)
			pending, readErr := repo.ListUnresolvedErrandParentAdmissions(ctx, row.ID)
			if readErr != nil || len(pending) != 1 || pending[0] != admission {
				t.Fatalf("recovery lost exact admission identity: %+v, %v", pending, readErr)
			}
			if _, err := store.ReserveErrandParentAdmission(ctx, parentAdmission(row, "competing")); !errors.Is(err, models.ErrErrandVersionConflict) {
				t.Fatalf("competing dispatch was not fenced: %v", err)
			}
			if err := store.DecideErrandParentAdmission(ctx, admission, decision); err != nil {
				t.Fatal(err)
			}
			generation, state := int64(0), models.ErrandResultPending
			if decision == models.ErrandAdmissionAccepted {
				generation, state = 1, models.ErrandResultSuperseded
			}
			assertAdmissionResult(t, repo, row.ID, generation, state)
			pending, readErr = repo.ListUnresolvedErrandParentAdmissions(ctx, row.ID)
			wantPending := 0
			if decision == models.ErrandAdmissionUnknown {
				wantPending = 1
			}
			if readErr != nil || len(pending) != wantPending {
				t.Fatalf("resolved recovery inventory: %+v, %v", pending, readErr)
			}
			// Retry from another repository instance, with the same durable identity.
			reopened := &Repository{db: repo.db, ro: repo.ro, clockNow: repo.clockNow}
			retry := admissionRepository(t, reopened)
			reservation, err = retry.ReserveErrandParentAdmission(ctx, admission)
			if err != nil || reservation.Created || reservation.State != decision {
				t.Fatalf("replay cannot authorize redispatch: reservation=%+v, err=%v", reservation, err)
			}
			if err := retry.DecideErrandParentAdmission(ctx, admission, decision); err != nil {
				t.Fatal(err)
			}
			assertAdmissionResult(t, repo, row.ID, generation, state)
			if decision == models.ErrandAdmissionUnknown {
				if _, err := retry.ReserveErrandParentAdmission(ctx, parentAdmission(row, "still-fenced")); !errors.Is(err, models.ErrErrandVersionConflict) {
					t.Fatalf("unknown admission lost fence after replay: %v", err)
				}
			} else {
				other := models.ErrandAdmissionAccepted
				if decision == other {
					other = models.ErrandAdmissionRefused
				}
				if err := retry.DecideErrandParentAdmission(ctx, admission, other); !errors.Is(err, models.ErrErrandVersionConflict) {
					t.Fatalf("contradictory terminal decision accepted: %v", err)
				}
			}
		})
	}
}

func TestErrandAdmissionResolutionRetriesDeferredRelease(t *testing.T) {
	exerciseErrandAdmissionDeferredRelease(t, newRepoForSessionTests(t))
}

func exerciseErrandAdmissionDeferredRelease(t *testing.T, repo *Repository) {
	t.Helper()
	store := admissionRepository(t, repo)
	ctx := context.Background()
	for _, route := range []models.ErrandDeliveryRoute{models.ErrandDeliveryAwait, models.ErrandDeliveryTurnContext, models.ErrandDeliveryNotification} {
		for _, decision := range []models.ErrandAdmissionState{models.ErrandAdmissionAccepted, models.ErrandAdmissionRefused} {
			t.Run(string(route)+"/"+string(decision), func(t *testing.T) {
				row := newAdmissionResult(t, repo, string(route)+string(decision))
				claim := claimAdmissionResult(t, repo, row, route)
				admission := parentAdmission(row, "successor")
				if _, err := store.ReserveErrandParentAdmission(ctx, admission); err != nil {
					t.Fatal(err)
				}
				if err := store.DecideErrandParentAdmission(ctx, admission, models.ErrandAdmissionUnknown); err != nil {
					t.Fatal(err)
				}
				if route == models.ErrandDeliveryAwait {
					if _, err := store.AbortAwait(ctx, row.TaskID, row.ParentSessionID, claim.Token); err != nil {
						t.Fatal(err)
					}
				} else if _, err := store.ReleaseErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, []models.ErrandResultClaim{claim}); err != nil {
					t.Fatal(err)
				}
				assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultDelivered)
				if err := store.DecideErrandParentAdmission(ctx, admission, decision); err != nil {
					t.Fatal(err)
				}
				generation, state := int64(0), models.ErrandResultPending
				if decision == models.ErrandAdmissionAccepted {
					generation, state = 1, models.ErrandResultSuperseded
				}
				assertAdmissionResult(t, repo, row.ID, generation, state)
			})
		}
	}
}

func TestErrandParentAdmissionRejectsStaleIdentityAndIgnoresUser(t *testing.T) {
	exerciseErrandAdmissionIdentity(t, newRepoForSessionTests(t))
}

func TestErrandParentAdmissionPreservesPostReservationResult(t *testing.T) {
	exerciseErrandAdmissionPreservesReplacement(t, newRepoForSessionTests(t))
}

func exerciseErrandAdmissionPreservesReplacement(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	store := admissionRepository(t, repo)
	for _, kind := range []models.ErrandResultKind{models.ErrandResultOutput, models.ErrandResultCancelled} {
		row := newAdmissionResult(t, repo, "post-reservation-"+string(kind))
		a := parentAdmission(row, "successor")
		if _, err := store.ReserveErrandParentAdmission(ctx, a); err != nil {
			t.Fatal(err)
		}
		row.ResultText, row.ResultKind, row.AwaitingResult = "replacement", kind, false
		if kind == models.ErrandResultCancelled {
			row.Lifecycle, row.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
		} else {
			row.ResultTurnID, row.LatestSettledTurnID = a.TurnID, a.TurnID
		}
		if err := repo.UpdateErrand(ctx, row); err != nil {
			t.Fatal(err)
		}
		revision := row.ResultRevision
		if err := store.DecideErrandParentAdmission(ctx, a, models.ErrandAdmissionAccepted); err != nil {
			t.Fatal(err)
		}
		assertAdmissionResult(t, repo, row.ID, 1, models.ErrandResultPending)
		current, err := repo.GetErrand(ctx, row.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.ResultRevision != revision || current.AwaitingResult {
			t.Fatalf("admission replaced a settled/cancelled successor: %+v", current)
		}
	}
}

func exerciseErrandAdmissionIdentity(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	store := admissionRepository(t, repo)
	row := newAdmissionResult(t, repo, "identity")
	user := parentAdmission(row, "user-turn")
	user.SenderSessionID = ""
	if _, err := store.ReserveErrandParentAdmission(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := store.DecideErrandParentAdmission(ctx, user, models.ErrandAdmissionAccepted); err != nil {
		t.Fatal(err)
	}
	assertAdmissionResult(t, repo, row.ID, 0, models.ErrandResultPending)
	admission := parentAdmission(row, "first")
	if _, err := store.ReserveErrandParentAdmission(ctx, admission); err != nil {
		t.Fatal(err)
	}
	stale := admission
	stale.TurnID = "wrong-turn"
	if err := store.DecideErrandParentAdmission(ctx, stale, models.ErrandAdmissionAccepted); !errors.Is(err, models.ErrErrandVersionConflict) {
		t.Fatalf("stale turn decided reservation: %v", err)
	}
	if err := store.DecideErrandParentAdmission(ctx, admission, models.ErrandAdmissionAccepted); err != nil {
		t.Fatal(err)
	}
	next := parentAdmission(row, "second")
	if _, err := store.ReserveErrandParentAdmission(ctx, next); err != nil {
		t.Fatal(err)
	}
	if err := store.DecideErrandParentAdmission(ctx, next, models.ErrandAdmissionAccepted); err != nil {
		t.Fatal(err)
	}
	if err := store.DecideErrandParentAdmission(ctx, admission, models.ErrandAdmissionAccepted); err != nil {
		t.Fatal(err)
	}
	assertAdmissionResult(t, repo, row.ID, 2, models.ErrandResultSuperseded)
}

func newAdmissionResult(t *testing.T, repo *Repository, suffix string) *models.Errand {
	t.Helper()
	row := sampleErrand("admission-"+suffix, newErrandTask(t, repo, "admission-"+suffix), "parent", errandScenarioBase)
	row.ResultState, row.ResultText, row.AwaitingResult = models.ErrandResultPending, "original "+suffix, false
	mustCreateErrand(t, repo, row)
	return row
}

func parentAdmission(row *models.Errand, suffix string) models.ErrandParentAdmission {
	return models.ErrandParentAdmission{TaskID: row.TaskID, ParentSessionID: row.ParentSessionID, ErrandID: row.ID,
		SenderSessionID: row.ParentSessionID, TurnID: "turn-" + suffix, SubmissionID: "submission-" + suffix}
}

func claimAdmissionResult(t *testing.T, repo *Repository, row *models.Errand, route models.ErrandDeliveryRoute) models.ErrandResultClaim {
	t.Helper()
	inventory, err := repo.ListErrandsByTask(t.Context(), row.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	var scoped []*models.Errand
	for _, candidate := range inventory {
		if candidate.ParentSessionID == row.ParentSessionID {
			scoped = append(scoped, candidate)
		}
	}
	rows, err := repo.ClaimResults(context.Background(), row.TaskID, row.ParentSessionID, route, "claim-"+row.ID,
		[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, models.ErrandInventory(scoped))
	if err != nil {
		t.Fatal(err)
	}
	*row = *rows[0]
	return models.ErrandResultClaim{ErrandID: row.ID, ResultRevision: row.ResultRevision, ParentAdmissionGeneration: row.ParentAdmissionGeneration,
		Token: row.DeliveryToken, Route: row.DeliveredRoute}
}

func assertAdmissionResult(t *testing.T, repo *Repository, id string, generation int64, state models.ErrandResultState) {
	t.Helper()
	row, err := repo.GetErrand(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ParentAdmissionGeneration != generation || row.ResultState != state {
		t.Fatalf("result generation/state = %d/%s, want %d/%s", row.ParentAdmissionGeneration, row.ResultState, generation, state)
	}
}
