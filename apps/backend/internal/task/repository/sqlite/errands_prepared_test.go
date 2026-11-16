package sqlite

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func preparedOwnershipSource(t *testing.T, repo *Repository) (*models.Errand, messagequeue.QueueSessionIdentity, *messagequeue.QueuedMessage) {
	t.Helper()
	row := newAdmissionResult(t, repo, "prepared-source")
	parent := &models.TaskSession{ID: row.ParentSessionID, TaskID: row.TaskID}
	createErrandScenarioSession(t, repo, parent)
	parent, err := repo.GetTaskSession(t.Context(), parent.ID)
	require.NoError(t, err)
	_, err = messagequeue.NewSQLiteRepository(repo.db, repo.ro)
	require.NoError(t, err)
	identity := messagequeue.QueueSessionIdentity{TaskID: row.TaskID, SessionID: parent.ID, SessionIncarnationID: parent.QueueIncarnationID}
	claim := models.ErrandResultClaim{ErrandID: row.ID, ResultRevision: row.ResultRevision, ParentAdmissionGeneration: row.ParentAdmissionGeneration,
		Token: "prepared-source-token", Route: models.ErrandDeliveryNotification}
	envelope := map[string]interface{}{"token": claim.Token, "route": claim.Route,
		"items": []map[string]interface{}{{"errand_id": row.ID, "title": row.Title, "status": "cancelled", "preview": "preview"}}}
	source := &messagequeue.QueuedMessage{ID: "prepared-source-id", TaskID: row.TaskID, SessionID: parent.ID, Content: "complete bounded source", QueuedBy: messagequeue.QueuedByServer,
		Metadata: map[string]interface{}{"errand_delivery_token": claim.Token, "errand_delivery": envelope,
			"errand_source": map[string]interface{}{"identity": identity, "provenance": "errands", "kind": "prepared", "prepared_block": "complete bounded source",
				"receipt": map[string]interface{}{"source_id": "prepared-source-id", "envelope": envelope, "claims": []models.ErrandResultClaim{claim}}}}}
	return row, identity, source
}

func TestPreparedErrandAdmissionRollsBackBothOwners(t *testing.T) {
	for _, table := range []string{"queued_messages", "queue_admission_receipts", "errand_result_claim", "errand_result_claim_item"} {
		t.Run(table, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			row, identity, source := preparedOwnershipSource(t, repo)
			_, err := repo.db.Exec(`CREATE TRIGGER refuse_prepared BEFORE INSERT ON ` + table + ` BEGIN SELECT RAISE(ABORT, 'injected prepared write failure'); END`)
			require.NoError(t, err)
			_, err = repo.AdmitPreparedErrand(t.Context(), identity, source, 10)
			require.ErrorContains(t, err, "injected prepared write failure")
			assertAdmissionResult(t, repo, row.ID, row.ParentAdmissionGeneration, models.ErrandResultPending)
			for _, verify := range []string{"queued_messages", "queue_admission_receipts", "errand_result_claim", "errand_result_claim_item"} {
				var count int
				require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM `+verify))
				require.Zero(t, count, verify+" must roll back with the paired ownership writes")
			}
		})
	}
}

func TestPreparedErrandAdmissionAmbiguousCommitRetainsIdentity(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "rolled_back", true: "committed"}[committed], func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			row, identity, source := preparedOwnershipSource(t, repo)
			gate := newErrandSQLGate("unused", false)
			gate.commitFault = func(tx driver.Tx) error {
				if committed {
					require.NoError(t, tx.Commit())
				} else {
					require.NoError(t, tx.Rollback())
				}
				return errors.New("commit acknowledgement lost")
			}
			unknown := independentErrandRepository(t, repo, gate)
			_, err := unknown.AdmitPreparedErrand(t.Context(), identity, source, 10)
			require.ErrorIs(t, err, models.ErrErrandClaimCommitUnknown)
			accepted, err := repo.AdmitPreparedErrand(t.Context(), identity, source, 10)
			require.NoError(t, err)
			require.Equal(t, source.ID, accepted.ID)
			after, err := repo.GetErrand(t.Context(), row.ID)
			require.NoError(t, err)
			require.Equal(t, row.Version+1, after.Version, "retry must not claim a revision twice")
			require.Equal(t, "prepared-source-token", after.DeliveryToken)
			for _, table := range []string{"queued_messages", "queue_admission_receipts", "errand_result_claim", "errand_result_claim_item"} {
				var count int
				require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM `+table))
				require.Equal(t, 1, count, table)
			}
		})
	}
}

func TestPreparedErrandAdmissionSerializesCompositionAcrossConnections(t *testing.T) {
	repo := newRepoForSessionTests(t)
	row, identity, source := preparedOwnershipSource(t, repo)
	gate := newErrandSQLGate("INSERT INTO queued_messages", true)
	defer gate.unblock()
	writer := independentErrandRepository(t, repo, gate)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	admitted := make(chan error, 1)
	go func() { _, err := writer.AdmitPreparedErrand(ctx, identity, source, 10); admitted <- err }()
	awaitSignal(t, gate.entered, "prepared source insertion owns the task guard")
	guard := newErrandSQLGate("UPDATE tasks SET id = id WHERE 0", false)
	composer := independentErrandRepository(t, repo, guard)
	claimed := make(chan error, 1)
	go func() {
		_, err := composer.ClaimResults(ctx, row.TaskID, row.ParentSessionID, models.ErrandDeliveryTurnContext, "competing-composition",
			[]models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}}, errandScenarioBase, []models.ErrandClaimCandidate{{ID: row.ID, Version: row.Version}})
		claimed <- err
	}()
	awaitSignal(t, guard.entered, "independent composition attempts the same delivery guard")
	gate.unblock()
	require.NoError(t, awaitError(t, admitted, "prepared admission commit"))
	require.ErrorIs(t, awaitError(t, claimed, "composition rechecks after admission"), models.ErrErrandVersionConflict)
	claims, err := repo.ListErrandResultClaims(ctx, row.TaskID, row.ParentSessionID, "competing-composition")
	require.NoError(t, err)
	require.Empty(t, claims)
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.ro)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, 10, logger.Default())
	require.Len(t, queue.GetStatus(ctx, row.ParentSessionID).Entries, 1)
}

func TestPreparedErrandLiveSourceOrClaimRetainsExactResult(t *testing.T) {
	for _, state := range []string{"pending", "fifo_uncertain", "fifo_accepted", "fifo_refused", "send_now_uncertain", "send_now_accepted", "send_now_refused", "historical_receipt"} {
		t.Run(state, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			row, identity, source := preparedOwnershipSource(t, repo)
			_, err := repo.AdmitPreparedErrand(t.Context(), identity, source, 10)
			require.NoError(t, err)
			qr, err := messagequeue.NewSQLiteRepository(repo.db, repo.ro)
			require.NoError(t, err)
			queue := messagequeue.NewService(qr, 10, logger.Default())
			require.NoError(t, queue.SetAutoRun(t.Context(), identity.SessionID, true))
			switch state {
			case "fifo_uncertain", "fifo_accepted", "fifo_refused":
				reserved, exists, _, err := queue.ReserveQueuedWithAutoRunForSession(t.Context(), identity)
				require.NoError(t, err)
				require.True(t, exists)
				if state == "fifo_accepted" {
					require.NoError(t, queue.MarkPendingQueueDispatchAccepted(t.Context(), reserved))
				}
				if state == "fifo_refused" {
					_, err = queue.RestoreMessage(t.Context(), reserved)
					require.NoError(t, err)
				}
			case "send_now_uncertain", "send_now_accepted", "send_now_refused":
				claim, err := queue.ClaimSendNowForSession(t.Context(), identity, queue.GetStatus(t.Context(), identity.SessionID).Entries)
				require.NoError(t, err)
				if state == "send_now_accepted" {
					require.NoError(t, queue.MarkPendingSendNowClaimAccepted(t.Context(), claim))
				}
				if state == "send_now_refused" {
					require.NoError(t, queue.RestoreSendNowClaim(t.Context(), claim))
				}
			case "historical_receipt":
				_, err := repo.db.Exec(`DELETE FROM queued_messages`)
				require.NoError(t, err)
			}
			// Read and release through an independent connection after queue/claim commit.
			reopened := independentErrandRepository(t, repo, newErrandSQLGate("unused", false))
			claims, err := reopened.ListErrandResultClaims(t.Context(), row.TaskID, row.ParentSessionID, "prepared-source-token")
			require.NoError(t, err)
			require.Len(t, claims, 1)
			changed, err := reopened.ReleaseErrandResultClaims(t.Context(), row.TaskID, row.ParentSessionID, claims)
			require.NoError(t, err)
			if state == "historical_receipt" {
				require.Len(t, changed, 1, "an admission receipt survives removal and cannot prove current ownership")
				require.Equal(t, models.ErrandResultPending, changed[0].ResultState)
			} else {
				require.Empty(t, changed, "live queue ownership must retain the result across release and reopen")
				assertAdmissionResult(t, repo, row.ID, row.ParentAdmissionGeneration, models.ErrandResultDelivered)
			}
		})
	}
}

func TestPreparedErrandOwnershipRejectsStaleOrIncompleteEvidence(t *testing.T) {
	for _, mismatch := range []string{"task", "parent", "incarnation", "source_id", "revision", "generation", "missing_generation", "token", "route", "queue_generation", "missing_queue_generation", "user_source", "content"} {
		t.Run(mismatch, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			row, identity, source := preparedOwnershipSource(t, repo)
			_, err := repo.AdmitPreparedErrand(t.Context(), identity, source, 10)
			require.NoError(t, err)
			encoded, err := json.Marshal(source.Metadata)
			require.NoError(t, err)
			var metadata map[string]interface{}
			require.NoError(t, json.Unmarshal(encoded, &metadata))
			stamp := metadata["errand_source"].(map[string]interface{})
			target := stamp["identity"].(map[string]interface{})
			receipt := stamp["receipt"].(map[string]interface{})
			claim := receipt["claims"].([]interface{})[0].(map[string]interface{})
			content, sender := source.Content, source.QueuedBy
			switch mismatch {
			case "task":
				target["task_id"] = "other-task"
			case "parent":
				target["session_id"] = "other-parent"
			case "incarnation":
				target["session_incarnation_id"] = "previous-incarnation"
			case "source_id":
				receipt["source_id"] = "other-source"
			case "revision":
				claim["ResultRevision"] = "previous-revision"
			case "generation":
				claim["ParentAdmissionGeneration"] = 1
			case "missing_generation":
				delete(claim, "ParentAdmissionGeneration")
			case "token":
				claim["Token"] = "previous-token"
			case "route":
				claim["Route"] = string(models.ErrandDeliveryTurnContext)
			case "queue_generation":
				stamp["queue_generation"] = 1
			case "missing_queue_generation":
				delete(stamp, "queue_generation")
			case "user_source":
				sender = messagequeue.QueuedByUser
			case "content":
				content = "other source content"
			}
			encoded, err = json.Marshal(metadata)
			require.NoError(t, err)
			_, err = repo.db.Exec(`UPDATE queued_messages SET metadata_json = ?, content = ?, queued_by = ? WHERE id = ?`, string(encoded), content, sender, source.ID)
			require.NoError(t, err)
			claims, err := repo.ListErrandResultClaims(t.Context(), row.TaskID, row.ParentSessionID, "prepared-source-token")
			require.NoError(t, err)
			changed, err := repo.ReleaseErrandResultClaims(t.Context(), row.TaskID, row.ParentSessionID, claims)
			require.NoError(t, err)
			require.Len(t, changed, 1, "stale or incomplete source identity cannot own this exact revision")
			require.Equal(t, models.ErrandResultPending, changed[0].ResultState)
		})
	}
}

func TestPreparedErrandSourceCannotOwnOrReleaseSuccessorRevision(t *testing.T) {
	repo := newRepoForSessionTests(t)
	row, identity, source := preparedOwnershipSource(t, repo)
	_, err := repo.AdmitPreparedErrand(t.Context(), identity, source, 10)
	require.NoError(t, err)
	old, err := repo.ListErrandResultClaims(t.Context(), row.TaskID, row.ParentSessionID, "prepared-source-token")
	require.NoError(t, err)
	current, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	current.ResultText = "successor result content"
	current.ResultState, current.DeliveryToken, current.DeliveredRoute, current.DeliveredAt = models.ErrandResultPending, "", "", nil
	require.NoError(t, repo.UpdateErrand(t.Context(), current))
	_, err = repo.ClaimResults(t.Context(), row.TaskID, row.ParentSessionID, models.ErrandDeliveryNotification, "successor-token",
		[]models.ErrandClaimCandidate{{ID: current.ID, Version: current.Version}}, errandScenarioBase, claimInventory(t, repo, row.ParentSessionID))
	require.NoError(t, err)
	before, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.NotEqual(t, row.ResultRevision, before.ResultRevision)
	changed, err := repo.ReleaseErrandResultClaims(t.Context(), row.TaskID, row.ParentSessionID, old)
	require.NoError(t, err)
	require.Empty(t, changed, "the stale source cannot release its successor")
	after, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, before.Version, after.Version)
	require.Equal(t, "successor-token", after.DeliveryToken)
	successor, err := repo.ListErrandResultClaims(t.Context(), row.TaskID, row.ParentSessionID, "successor-token")
	require.NoError(t, err)
	changed, err = repo.ReleaseErrandResultClaims(t.Context(), row.TaskID, row.ParentSessionID, successor)
	require.NoError(t, err)
	require.Len(t, changed, 1, "the old prepared source cannot acquire a different revision's ownership")
	require.Equal(t, before.ResultRevision, changed[0].ResultRevision)
}

func TestPreparedErrandAdmissionCannotReuseClaimAcrossIncarnations(t *testing.T) {
	repo := newRepoForSessionTests(t)
	row, identity, source := preparedOwnershipSource(t, repo)
	_, err := repo.AdmitPreparedErrand(t.Context(), identity, source, 10)
	require.NoError(t, err)
	_, err = repo.db.Exec(`UPDATE task_sessions SET queue_incarnation_id = 'replacement-incarnation' WHERE id = ?`, identity.SessionID)
	require.NoError(t, err)
	identity.SessionIncarnationID = "replacement-incarnation"
	source.ID = "replacement-incarnation-source"
	stamp := source.Metadata["errand_source"].(map[string]interface{})
	stamp["identity"] = identity
	stamp["receipt"].(map[string]interface{})["source_id"] = source.ID
	_, err = repo.AdmitPreparedErrand(t.Context(), identity, source, 10)
	require.Error(t, err, "a claim owned by the previous incarnation cannot admit another notice")
	var count int
	require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM queued_messages`))
	require.Equal(t, 1, count, "the new queue write must roll back with the mismatched claim replay")
	after, err := repo.GetErrand(t.Context(), row.ID)
	require.NoError(t, err)
	require.Equal(t, row.Version+1, after.Version)
}
