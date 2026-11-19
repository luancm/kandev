package sqlite

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestOwnedErrandInterruptionRequiresExactReceipt(t *testing.T) {
	exerciseOwnedErrandInterruptionReceipt(t, newRepoForSessionTests(t))
}

func TestPostgresOwnedErrandInterruptionRequiresExactReceipt(t *testing.T) {
	exerciseOwnedErrandInterruptionReceipt(t, newPostgresErrandRepo(t))
}

func exerciseOwnedErrandInterruptionReceipt(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	row, sample := prepareOwnedErrandRecoveryAttention(t, repo)
	before, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	submission, err := repo.GetAgentDeliverySubmission(ctx, sample.SubmissionID)
	require.NoError(t, err)

	_, _, err = repo.ReadOwnedErrandLatestTerminal(ctx, row.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Empty(t, mustReadInterruptionAdmissions(t, repo), "without recorded process identity the prompt cannot recover")
	recordFixtureInterruptionProcess(t, repo, submission.ID)
	admissions := mustReadInterruptionAdmissions(t, repo)
	require.Len(t, admissions, 1)
	admission := admissions[0]
	require.Equal(t, sample.TurnID, admission.TurnID)

	wrong := admission
	wrong.Binding.StreamID = "foreign-stream"
	changed, err := repo.CommitOwnedErrandInterruption(ctx, wrong, 0)
	require.NoError(t, err)
	require.False(t, changed)
	changed, err = repo.CommitOwnedErrandInterruption(ctx, admission, 4)
	require.ErrorIs(t, err, sql.ErrNoRows, "an unreconciled journal prefix cannot close the turn")
	require.False(t, changed)
	_, _, err = repo.ReadOwnedErrandLatestTerminal(ctx, row.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	changed, err = repo.CommitOwnedErrandInterruption(ctx, admission, 0)
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = repo.CommitOwnedErrandInterruption(ctx, admission, 0)
	require.NoError(t, err)
	require.False(t, changed)
	require.Empty(t, mustReadInterruptionAdmissions(t, repo))

	target, outcome, err := repo.ReadOwnedErrandLatestTerminal(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, sample.TurnID, target.TurnID)
	require.Equal(t, sample.SubmissionID, target.SubmissionID)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, outcome)
	stored, err := repo.GetAgentDeliverySubmission(ctx, sample.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, submission.Payload, stored.Payload)
	require.Equal(t, submission.Outcome, stored.Outcome)
	require.Zero(t, stored.TerminalSequence)
	after, err := repo.GetErrand(ctx, row.ID)
	require.NoError(t, err)
	require.Equal(t, before, after, "recovering interruption evidence does not deliver a result")
}

func mustReadInterruptionAdmissions(t *testing.T, repo *Repository) []models.ErrandInterruptedAdmission {
	t.Helper()
	admissions, err := repo.ListOwnedErrandInterruptionAdmissions(t.Context())
	require.NoError(t, err)
	return admissions
}

func recordFixtureInterruptionProcess(t *testing.T, repo *Repository, submissionID string) {
	t.Helper()
	ctx := t.Context()
	key := errandPromptAdmissionEffect + ":" + submissionID
	var raw string
	require.NoError(t, repo.ro.GetContext(ctx, &raw, repo.ro.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=?`), key))
	var prompt errandPromptProof
	require.NoError(t, json.Unmarshal([]byte(raw), &prompt))
	prompt.OriginalRuntime = processidentity.Identity{PID: 123, BirthToken: "fixture"}
	newContent, err := json.Marshal(prompt)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_effects SET outcome=? WHERE effect_key=?`), string(newContent), key)
	require.NoError(t, err)
}
