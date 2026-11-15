package sqlite

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandClaimSnapshotAmbiguousCommitReadback(t *testing.T) {
	exerciseErrandCommitReadback(t, newRepoForSessionTests)
}

func TestPostgresErrandClaimSnapshotAmbiguousCommitReadback(t *testing.T) {
	exerciseErrandCommitReadback(t, newPostgresErrandRepo)
}

func exerciseErrandCommitReadback(t *testing.T, create func(*testing.T) *Repository) {
	t.Helper()
	for _, outcome := range []string{"committed", "rolled_back", "unreadable", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			repo := create(t)
			row := newAdmissionResult(t, repo, "commit-readback")
			inventory := claimInventory(t, repo, row.ParentSessionID)
			gate := newErrandSQLGate("unused", false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			committed := false
			gate.commitFault = func(tx driver.Tx) error {
				if outcome == "rolled_back" {
					if err := tx.Rollback(); err != nil {
						return err
					}
				} else {
					if err := tx.Commit(); err != nil {
						return err
					}
					committed = true
					if outcome == "cancelled" {
						cancel()
					}
				}
				return errors.New("lost commit acknowledgement")
			}
			gate.queryFault = func(query string) error {
				if committed && outcome == "unreadable" && strings.Contains(query, "errand_result_claim") {
					return errors.New("readback unavailable")
				}
				return nil
			}
			writer := independentErrandRepository(t, repo, gate)
			rows, err := writer.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryTurnContext,
				"retained-token", inventory, errandScenarioBase, inventory)
			if outcome == "committed" || outcome == "cancelled" {
				if err != nil || len(rows) != 1 || rows[0].DeliveryToken != "retained-token" || rows[0].ResultText != row.ResultText {
					t.Fatalf("positive commit was not read back: rows=%+v err=%v", rows, err)
				}
				return
			}
			if len(rows) != 0 || !errors.Is(err, models.ErrErrandClaimCommitUnknown) || !strings.Contains(err.Error(), "retained-token") {
				t.Fatalf("unknown commit became success or lost token: rows=%+v err=%v", rows, err)
			}
			want := models.ErrandResultPending
			if committed {
				want = models.ErrandResultDelivered
			}
			assertAdmissionResult(t, repo, row.ID, 0, want)
		})
	}
}
