package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

const (
	gitPushAlertTaskRepositoryID = "task-repo-push"
	gitPushAlertRepositoryID     = "repo-push"
)

// newGitPushAlertTestService creates the smallest real task/session tree on
// which the alert projection can be exercised. The task repository ID is
// intentionally different from the global repository ID: the former is the
// operation's task-scoped identity.
func newGitPushAlertTestService(t *testing.T) (*Service, *sqliterepo.Repository) {
	t.Helper()
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID:          gitPushAlertRepositoryID,
		WorkspaceID: "ws-msg",
		Name:        "push-repository",
		SourceType:  "local",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID:           gitPushAlertTaskRepositoryID,
		TaskID:       "task-msg",
		RepositoryID: gitPushAlertRepositoryID,
		BaseBranch:   "main",
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	return svc, repo
}

func addGitPushAlertRepository(t *testing.T, repo *sqliterepo.Repository, taskRepositoryID, repositoryID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID:          repositoryID,
		WorkspaceID: "ws-msg",
		Name:        repositoryID,
		SourceType:  "local",
	}); err != nil {
		t.Fatalf("create repository %s: %v", repositoryID, err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID:           taskRepositoryID,
		TaskID:       "task-msg",
		RepositoryID: repositoryID,
		BaseBranch:   "main",
		Position:     1,
	}); err != nil {
		t.Fatalf("create task repository %s: %v", taskRepositoryID, err)
	}
}

func readGitPushAlertState(t *testing.T, repo *sqliterepo.Repository) models.GitPushAlertState {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), "sess-msg")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	state, ok := models.LoadGitPushAlert(session.Metadata)
	if !ok {
		t.Fatal("session does not contain a git push alert projection")
	}
	return state
}

func synchronizedGitPushObservation(taskRepositoryID string, observedAt time.Time) GitPushStatusObservation {
	return GitPushStatusObservation{
		TaskRepositoryID:  taskRepositoryID,
		Branch:            "main",
		RemoteBranch:      "origin/main",
		HeadCommit:        "abc123",
		RemoteHeadCommit:  "abc123",
		RemoteAhead:       0,
		RemoteBehind:      0,
		RemoteAheadKnown:  true,
		RemoteBehindKnown: true,
		ObservedAt:        observedAt,
	}
}

func recordGitPushFailure(t *testing.T, svc *Service, at time.Time, taskRepositoryID, diagnostic string) {
	t.Helper()
	if err := svc.RecordGitPushFailure(context.Background(), "sess-msg", GitPushFailure{
		TaskRepositoryID: taskRepositoryID,
		Diagnostic:       diagnostic,
		Remote:           "origin",
		Branch:           "main",
		OccurredAt:       at,
	}); err != nil {
		t.Fatalf("record failure: %v", err)
	}
}

func TestService_GitPushFailureReusesCurrentMessageAndAdvancesRevision(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	firstAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Minute)

	recordGitPushFailure(t, svc, firstAt, gitPushAlertTaskRepositoryID, "first diagnostic")
	first := readGitPushAlertState(t, repo)
	if !first.Active || first.Revision != 1 {
		t.Fatalf("first state = %+v, want active revision 1", first)
	}
	if first.MessageID == "" {
		t.Fatal("first state has no presentation message")
	}

	recordGitPushFailure(t, svc, secondAt, gitPushAlertTaskRepositoryID, "latest diagnostic")
	second := readGitPushAlertState(t, repo)
	if !second.Active || second.Revision != 2 {
		t.Fatalf("second state = %+v, want active revision 2", second)
	}
	if second.MessageID != first.MessageID {
		t.Fatalf("message ID changed from %q to %q for same-repository failure", first.MessageID, second.MessageID)
	}
	if second.Diagnostic != "latest diagnostic" || !second.OccurredAt.Equal(secondAt) {
		t.Fatalf("second state = %+v, want latest diagnostic and occurrence", second)
	}

	messages, err := repo.ListMessages(context.Background(), "sess-msg")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want one current alert message", len(messages))
	}
	message := messages[0]
	if message.ID != second.MessageID || message.Content != "Git push failed" {
		t.Fatalf("current message = %+v, want reused push alert", message)
	}
	if got := message.Metadata["error_output"]; got != "latest diagnostic" {
		t.Fatalf("message diagnostic = %v, want latest diagnostic", got)
	}
	if got := message.Metadata["git_push_alert_revision"]; got != float64(2) && got != uint64(2) {
		t.Fatalf("message revision = %#v, want 2", got)
	}
}

func TestService_GitPushFailureIsIdempotentForDuplicateEvent(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	at := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	failure := GitPushFailure{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		Diagnostic:       "same diagnostic",
		Remote:           "origin",
		Branch:           "main",
		OccurredAt:       at,
	}
	if err := svc.RecordGitPushFailure(context.Background(), "sess-msg", failure); err != nil {
		t.Fatalf("record first failure: %v", err)
	}
	if err := svc.RecordGitPushFailure(context.Background(), "sess-msg", failure); err != nil {
		t.Fatalf("record duplicate failure: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if state.Revision != 1 || !state.Active {
		t.Fatalf("state after duplicate failure = %+v, want unchanged revision 1", state)
	}
	messages, err := repo.ListMessages(context.Background(), "sess-msg")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want one current alert", len(messages))
	}
}

func TestService_GitPushFailureImportsLegacyCurrentMessage(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	createdAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	seedMessage(t, repo, &models.Message{
		ID:        "legacy-before-new-failure",
		Content:   "legacy failure",
		CreatedAt: createdAt,
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
			"error_output":        "legacy diagnostic",
		},
	})
	recordGitPushFailure(t, svc, createdAt.Add(time.Minute), gitPushAlertTaskRepositoryID, "new diagnostic")
	state := readGitPushAlertState(t, repo)
	if !state.Active || state.MessageID != "legacy-before-new-failure" || state.Revision != 2 {
		t.Fatalf("state after legacy failure replacement = %+v, want reused current message", state)
	}
	messages, err := repo.ListMessages(context.Background(), "sess-msg")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want one current alert", len(messages))
	}
	if messages[0].Metadata["error_output"] != "new diagnostic" {
		t.Fatalf("current message metadata = %#v, want new diagnostic", messages[0].Metadata)
	}
}

func TestService_GitPushSuccessUsesCanonicalTaskRepositoryID(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	addGitPushAlertRepository(t, repo, "task-repo-other", "repo-other")
	failureAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	successAt := failureAt.Add(time.Minute)
	recordGitPushFailure(t, svc, failureAt, gitPushAlertTaskRepositoryID, "failed")

	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertRepositoryID,
		OccurredAt:       successAt,
	}); err != nil {
		t.Fatalf("record global-repository success: %v", err)
	}
	foreignIdentity := readGitPushAlertState(t, repo)
	if !foreignIdentity.Active {
		t.Fatalf("global repository ID incorrectly cleared task-scoped alert: %+v", foreignIdentity)
	}

	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		OccurredAt:       successAt,
	}); err != nil {
		t.Fatalf("record task-repository success: %v", err)
	}
	cleared := readGitPushAlertState(t, repo)
	if cleared.Active || cleared.Revision != 2 {
		t.Fatalf("cleared state = %+v, want inactive revision 2", cleared)
	}

	message, err := repo.GetMessage(context.Background(), cleared.MessageID)
	if err != nil {
		t.Fatalf("get resolved message: %v", err)
	}
	if got := message.Metadata["git_push_alert_active"]; got != false {
		t.Fatalf("resolved message active marker = %#v, want false", got)
	}
	resolvedAt, err := time.Parse(time.RFC3339Nano, message.Metadata["git_operation_resolved_at"].(string))
	if err != nil {
		t.Fatalf("parse resolved timestamp: %v", err)
	}
	if !resolvedAt.Equal(successAt) {
		t.Fatalf("resolved timestamp = %s, want authoritative success time %s", resolvedAt, successAt)
	}
}

func TestService_GitPushSuccessIgnoresForeignAndStaleEvents(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	addGitPushAlertRepository(t, repo, "task-repo-other", "repo-other")
	failureAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	recordGitPushFailure(t, svc, failureAt, gitPushAlertTaskRepositoryID, "failed")

	for _, test := range []struct {
		name string
		id   string
		at   time.Time
	}{
		{name: "foreign", id: "task-repo-other", at: failureAt.Add(time.Minute)},
		{name: "stale", id: gitPushAlertTaskRepositoryID, at: failureAt.Add(-time.Nanosecond)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
				TaskRepositoryID: test.id,
				OccurredAt:       test.at,
			}); err != nil {
				t.Fatalf("record success: %v", err)
			}
			state := readGitPushAlertState(t, repo)
			if !state.Active || state.Revision != 1 {
				t.Fatalf("state after %s success = %+v, want unchanged active revision 1", test.name, state)
			}
		})
	}
}

func TestService_GitPushStatusRequiresCanonicalRepositoryAndFreshSynchronization(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	addGitPushAlertRepository(t, repo, "task-repo-other", "repo-other")
	failureAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	recordGitPushFailure(t, svc, failureAt, gitPushAlertTaskRepositoryID, "failed")

	foreign := synchronizedGitPushObservation("task-repo-other", failureAt.Add(time.Minute))
	if err := svc.ReconcileGitPushStatus(context.Background(), "sess-msg", foreign); err != nil {
		t.Fatalf("reconcile foreign status: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if !state.Active {
		t.Fatal("foreign synchronized status cleared current alert")
	}

	stale := synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, failureAt)
	if err := svc.ReconcileGitPushStatus(context.Background(), "sess-msg", stale); err != nil {
		t.Fatalf("reconcile stale status: %v", err)
	}
	state = readGitPushAlertState(t, repo)
	if !state.Active {
		t.Fatal("stale synchronized status cleared current alert")
	}

	fresh := synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, failureAt.Add(time.Minute))
	if err := svc.ReconcileGitPushStatus(context.Background(), "sess-msg", fresh); err != nil {
		t.Fatalf("reconcile fresh status: %v", err)
	}
	state = readGitPushAlertState(t, repo)
	if state.Active || state.Revision != 2 {
		t.Fatalf("fresh status state = %+v, want inactive revision 2", state)
	}
}

func TestService_GitPushStatusRejectsUnprovenSynchronization(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	failureAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	recordGitPushFailure(t, svc, failureAt, gitPushAlertTaskRepositoryID, "failed")

	observation := synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, failureAt.Add(time.Minute))
	observation.RemoteBehindKnown = false
	if err := svc.ReconcileGitPushStatus(context.Background(), "sess-msg", observation); err != nil {
		t.Fatalf("reconcile incomplete status: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if !state.Active || state.Revision != 1 {
		t.Fatalf("incomplete status state = %+v, want unchanged active revision 1", state)
	}
}

func TestService_GitPushStatusImportsNewestLegacyPushAlertOnce(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	ctx := context.Background()
	firstAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	latestAt := firstAt.Add(time.Minute)
	seedMessage(t, repo, &models.Message{
		ID:        "legacy-first",
		Content:   "first failure",
		CreatedAt: firstAt,
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
			"error_output":        "first diagnostic",
		},
	})
	seedMessage(t, repo, &models.Message{
		ID:        "legacy-latest",
		Content:   "latest failure",
		CreatedAt: latestAt,
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
			"error_output":        "latest diagnostic",
		},
	})

	observation := synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, latestAt.Add(time.Minute))
	if err := svc.ReconcileGitPushStatus(ctx, "sess-msg", observation); err != nil {
		t.Fatalf("reconcile legacy status: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if state.Active || state.MessageID != "legacy-latest" || state.Diagnostic != "latest diagnostic" {
		t.Fatalf("legacy import state = %+v, want latest message then clear", state)
	}

	latest, err := repo.GetMessage(ctx, "legacy-latest")
	if err != nil {
		t.Fatalf("get latest legacy message: %v", err)
	}
	if latest.Metadata["git_operation_resolved"] != true {
		t.Fatalf("latest legacy message metadata = %#v, want resolved", latest.Metadata)
	}
	first, err := repo.GetMessage(ctx, "legacy-first")
	if err != nil {
		t.Fatalf("get first legacy message: %v", err)
	}
	if first.Metadata["git_push_alert_active"] != nil || first.Metadata["git_operation_resolved"] != true {
		t.Fatalf("legacy import marked older historical message as current: %#v", first.Metadata)
	}

	// Once a projection exists, a later status event must use it directly and
	// must not rescan the transcript for a newly inserted legacy row.
	seedMessage(t, repo, &models.Message{
		ID:        "legacy-after-import",
		Content:   "late historical failure",
		CreatedAt: latestAt.Add(2 * time.Minute),
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
			"error_output":        "late diagnostic",
		},
	})
	if err := svc.ReconcileGitPushStatus(ctx, "sess-msg", observation); err != nil {
		t.Fatalf("reconcile repeated legacy status: %v", err)
	}
	state = readGitPushAlertState(t, repo)
	if state.MessageID != "legacy-latest" || state.Active {
		t.Fatalf("state after repeated legacy scan = %+v, want unchanged inactive projection", state)
	}
}

func TestService_GitPushStatusDoesNotClearNewestLegacyAlertWithOlderObservation(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	ctx := context.Background()
	firstAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	latestAt := firstAt.Add(time.Minute)
	seedMessage(t, repo, &models.Message{
		ID:        "legacy-older-observation-first",
		Content:   "first failure",
		CreatedAt: firstAt,
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
		},
	})
	seedMessage(t, repo, &models.Message{
		ID:        "legacy-older-observation-latest",
		Content:   "latest failure",
		CreatedAt: latestAt,
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
		},
	})

	if err := svc.ReconcileGitPushStatus(ctx, "sess-msg", synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, firstAt.Add(30*time.Second))); err != nil {
		t.Fatalf("reconcile older status: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if !state.Active || state.MessageID != "legacy-older-observation-latest" || !state.OccurredAt.Equal(latestAt) {
		t.Fatalf("state after older status = %+v, want newest legacy alert active", state)
	}
	latest, err := repo.GetMessage(ctx, "legacy-older-observation-latest")
	if err != nil {
		t.Fatalf("get newest legacy message: %v", err)
	}
	if latest.Metadata["git_operation_resolved"] == true {
		t.Fatal("older status resolved the newest legacy alert")
	}
}

func TestService_GitPushStatusDoesNotImportAmbiguousLegacyAlert(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	addGitPushAlertRepository(t, repo, "task-repo-other", "repo-other")
	ctx := context.Background()
	seedMessage(t, repo, &models.Message{
		ID:        "legacy-ambiguous",
		Content:   "ambiguous failure",
		CreatedAt: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC),
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
		},
	})

	observation := synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, time.Date(2026, 9, 16, 12, 1, 0, 0, time.UTC))
	if err := svc.ReconcileGitPushStatus(ctx, "sess-msg", observation); err != nil {
		t.Fatalf("reconcile ambiguous legacy status: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if state.Active || state.MessageID != "" {
		t.Fatalf("ambiguous legacy state = %+v, want inactive marker without imported message", state)
	}
	message, err := repo.GetMessage(ctx, "legacy-ambiguous")
	if err != nil {
		t.Fatalf("get ambiguous legacy message: %v", err)
	}
	if message.Metadata["git_operation_resolved"] == true {
		t.Fatal("ambiguous legacy message was resolved without repository evidence")
	}
}

func TestService_GitPushFailureReplacesOneCurrentAlertAcrossRepositories(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	addGitPushAlertRepository(t, repo, "task-repo-other", "repo-other")
	firstAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Minute)

	recordGitPushFailure(t, svc, firstAt, gitPushAlertTaskRepositoryID, "first")
	first := readGitPushAlertState(t, repo)
	recordGitPushFailure(t, svc, secondAt, "task-repo-other", "second")
	second := readGitPushAlertState(t, repo)
	if !second.Active || second.TaskRepositoryID != "task-repo-other" || second.Diagnostic != "second" {
		t.Fatalf("replacement state = %+v, want active foreign-repository scope", second)
	}
	if second.MessageID != first.MessageID {
		t.Fatalf("replacement message ID = %q, want bounded current message %q", second.MessageID, first.MessageID)
	}
	messages, err := repo.ListMessages(context.Background(), "sess-msg")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want one current alert", len(messages))
	}
}

func TestService_GitPushResolutionRecomputesLegacyScopeAfterRepositoryAdded(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	failureAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	successAt := failureAt.Add(time.Minute)
	recordGitPushFailure(t, svc, failureAt, gitPushAlertTaskRepositoryID, "failed")
	state := readGitPushAlertState(t, repo)
	addGitPushAlertRepository(t, repo, "task-repo-other", "repo-other")
	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		OccurredAt:       successAt,
	}); err != nil {
		t.Fatalf("record success after repository drift: %v", err)
	}
	message, err := repo.GetMessage(context.Background(), state.MessageID)
	if err != nil {
		t.Fatalf("get resolved message: %v", err)
	}
	if message.Metadata["git_push_alert_legacy_single_repository"] != false {
		t.Fatalf("resolved message retained single-repository compatibility scope: %#v", message.Metadata)
	}
}

func TestService_GitPushStatusRequiresNamedUpstream(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	failureAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	recordGitPushFailure(t, svc, failureAt, gitPushAlertTaskRepositoryID, "failed")

	observation := synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, failureAt.Add(time.Minute))
	observation.RemoteBranch = "main"
	if err := svc.ReconcileGitPushStatus(context.Background(), "sess-msg", observation); err != nil {
		t.Fatalf("reconcile unnamed upstream: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if !state.Active || state.Revision != 1 {
		t.Fatalf("state after unnamed upstream = %+v, want unchanged active alert", state)
	}
}

func TestService_GitPushFailureCannotResurrectAfterNewerSuccess(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	failureAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	successAt := failureAt.Add(time.Minute)
	recordGitPushFailure(t, svc, failureAt, gitPushAlertTaskRepositoryID, "failed")
	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		OccurredAt:       successAt,
	}); err != nil {
		t.Fatalf("record success: %v", err)
	}
	if err := svc.RecordGitPushFailure(context.Background(), "sess-msg", GitPushFailure{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		Diagnostic:       "delayed failure",
		OccurredAt:       failureAt.Add(30 * time.Second),
	}); err != nil {
		t.Fatalf("record delayed failure: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if state.Active || state.Revision != 2 {
		t.Fatalf("state after delayed failure = %+v, want inactive revision 2", state)
	}
}

func TestService_GitPushSuccessMarkerBlocksOlderDelayedFailure(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	successAt := time.Date(2026, 9, 16, 12, 1, 0, 0, time.UTC)
	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		OccurredAt:       successAt,
	}); err != nil {
		t.Fatalf("record success: %v", err)
	}
	if err := svc.RecordGitPushFailure(context.Background(), "sess-msg", GitPushFailure{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		Diagnostic:       "delayed failure",
		OccurredAt:       successAt.Add(-time.Second),
	}); err != nil {
		t.Fatalf("record delayed failure: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if state.Active || state.Revision != 1 {
		t.Fatalf("state after delayed failure = %+v, want inactive success marker", state)
	}
}

func TestService_GitPushStatusAdvancesInactiveWatermarkForSameRepository(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	firstAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	statusAt := firstAt.Add(time.Minute)
	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		OccurredAt:       firstAt,
	}); err != nil {
		t.Fatalf("record initial success: %v", err)
	}
	if err := svc.ReconcileGitPushStatus(context.Background(), "sess-msg", synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, statusAt)); err != nil {
		t.Fatalf("reconcile inactive status: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if state.Active || state.TaskRepositoryID != gitPushAlertTaskRepositoryID || state.Revision != 1 || !state.OccurredAt.Equal(statusAt) {
		t.Fatalf("state after routine inactive status = %+v, want advanced watermark", state)
	}
	if err := svc.RecordGitPushFailure(context.Background(), "sess-msg", GitPushFailure{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		Diagnostic:       "delayed failure",
		OccurredAt:       firstAt.Add(30 * time.Second),
	}); err != nil {
		t.Fatalf("record delayed failure: %v", err)
	}
	state = readGitPushAlertState(t, repo)
	if state.Active || state.Revision != 1 || !state.OccurredAt.Equal(statusAt) {
		t.Fatalf("state after delayed failure = %+v, want unchanged inactive watermark", state)
	}
	if err := svc.ReconcileGitPushStatus(context.Background(), "sess-msg", synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, statusAt.Add(time.Minute))); err != nil {
		t.Fatalf("repeat inactive status: %v", err)
	}
	state = readGitPushAlertState(t, repo)
	if state.Revision != 1 || !state.OccurredAt.Equal(statusAt.Add(time.Minute)) {
		t.Fatalf("state after newer inactive status = %+v, want latest watermark", state)
	}
}

func TestService_GitPushSuccessDoesNotMoveInactiveMarkerToForeignRepository(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	addGitPushAlertRepository(t, repo, "task-repo-other", "repo-other")
	firstAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		OccurredAt:       firstAt,
	}); err != nil {
		t.Fatalf("record initial success: %v", err)
	}
	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: "task-repo-other",
		OccurredAt:       firstAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("record foreign success: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if state.Active || state.TaskRepositoryID != gitPushAlertTaskRepositoryID || state.Revision != 1 {
		t.Fatalf("state after foreign success = %+v, want original inactive scope", state)
	}
}

func TestService_GitPushFailureDoesNotUseForeignInactiveMarkerForOrdering(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	addGitPushAlertRepository(t, repo, "task-repo-other", "repo-other")
	statusAt := time.Date(2026, 9, 16, 12, 2, 0, 0, time.UTC)
	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		OccurredAt:       statusAt,
	}); err != nil {
		t.Fatalf("record repository A success: %v", err)
	}
	if err := svc.RecordGitPushFailure(context.Background(), "sess-msg", GitPushFailure{
		TaskRepositoryID: "task-repo-other",
		Diagnostic:       "repository B failure",
		OccurredAt:       statusAt.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("record delayed repository B failure: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if !state.Active || state.TaskRepositoryID != "task-repo-other" || state.Diagnostic != "repository B failure" {
		t.Fatalf("state after foreign delayed failure = %+v, want active repository B alert", state)
	}
}

func TestService_GitPushFailureRecreatesMissingCurrentMessage(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	firstAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Minute)
	recordGitPushFailure(t, svc, firstAt, gitPushAlertTaskRepositoryID, "first diagnostic")
	first := readGitPushAlertState(t, repo)
	if err := repo.DeleteMessage(context.Background(), first.MessageID); err != nil {
		t.Fatalf("delete current message: %v", err)
	}
	recordGitPushFailure(t, svc, secondAt, gitPushAlertTaskRepositoryID, "latest diagnostic")
	state := readGitPushAlertState(t, repo)
	if state.MessageID == "" || state.MessageID == first.MessageID || state.Diagnostic != "latest diagnostic" {
		t.Fatalf("state after recreating message = %+v, want new current message", state)
	}
	message, err := repo.GetMessage(context.Background(), state.MessageID)
	if err != nil {
		t.Fatalf("get recreated message: %v", err)
	}
	if message.Metadata["error_output"] != "latest diagnostic" {
		t.Fatalf("recreated message metadata = %#v, want latest diagnostic", message.Metadata)
	}
}

func TestService_GitPushSuccessClearsMissingCurrentMessage(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	failureAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	recordGitPushFailure(t, svc, failureAt, gitPushAlertTaskRepositoryID, "missing before success")
	state := readGitPushAlertState(t, repo)
	if state.MessageID == "" {
		t.Fatal("failure did not create a presentation message")
	}
	if err := repo.DeleteMessage(context.Background(), state.MessageID); err != nil {
		t.Fatalf("delete current alert message: %v", err)
	}

	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		Remote:           "origin",
		Branch:           "main",
		OccurredAt:       failureAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("record success after message deletion: %v", err)
	}
	state = readGitPushAlertState(t, repo)
	if state.Active {
		t.Fatalf("missing-message success left alert active: %+v", state)
	}
}

func TestService_GitPushSuccessImportsAndClearsLegacyPushAlert(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	createdAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	successAt := createdAt.Add(time.Minute)
	seedMessage(t, repo, &models.Message{
		ID:        "legacy-direct-success",
		Content:   "legacy failure",
		CreatedAt: createdAt,
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
			"error_output":        "legacy diagnostic",
		},
	})
	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		Remote:           "origin",
		Branch:           "main",
		OccurredAt:       successAt,
	}); err != nil {
		t.Fatalf("record success: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if state.Active || state.MessageID != "legacy-direct-success" || state.Revision != 2 {
		t.Fatalf("state after legacy direct success = %+v, want cleared imported alert", state)
	}
	message, err := repo.GetMessage(context.Background(), "legacy-direct-success")
	if err != nil {
		t.Fatalf("get legacy message: %v", err)
	}
	if message.Metadata["git_push_alert_active"] != false || message.Metadata["git_operation_resolved"] != true {
		t.Fatalf("legacy message metadata = %#v, want inactive resolved marker", message.Metadata)
	}
}

func TestService_GitPushSuccessDoesNotClearNewerLegacyPushAlert(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	successAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	createdAt := successAt.Add(time.Minute)
	seedMessage(t, repo, &models.Message{
		ID:        "legacy-newer-than-success",
		Content:   "newer legacy failure",
		CreatedAt: createdAt,
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
			"error_output":        "newer diagnostic",
		},
	})
	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		Remote:           "origin",
		Branch:           "main",
		OccurredAt:       successAt,
	}); err != nil {
		t.Fatalf("record success: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if !state.Active || state.MessageID != "legacy-newer-than-success" {
		t.Fatalf("state after stale success = %+v, want active newer legacy alert", state)
	}
	message, err := repo.GetMessage(context.Background(), "legacy-newer-than-success")
	if err != nil {
		t.Fatalf("get legacy message: %v", err)
	}
	if message.Metadata["git_push_alert_active"] == false {
		t.Fatal("stale success hid a newer legacy push alert")
	}
}

func TestService_GitPushSuccessDoesNotStampStaleLegacyDestination(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	createdAt := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	seedMessage(t, repo, &models.Message{
		ID:        "legacy-unstamped-destination",
		Content:   "legacy failure",
		CreatedAt: createdAt,
		Metadata: map[string]interface{}{
			"git_operation_error": true,
			"operation":           "push",
			"error_output":        "legacy diagnostic",
		},
	})

	if err := svc.RecordGitPushSuccess(context.Background(), "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		Remote:           "stale-remote",
		Branch:           "stale-branch",
		OccurredAt:       createdAt.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("record stale success: %v", err)
	}
	state := readGitPushAlertState(t, repo)
	if !state.Active || state.Remote != "" || state.Branch != "" {
		t.Fatalf("stale legacy import state = %+v, want active and unscoped", state)
	}

	if err := svc.ReconcileGitPushStatus(context.Background(), "sess-msg", synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, createdAt.Add(time.Minute))); err != nil {
		t.Fatalf("reconcile fresh status: %v", err)
	}
	state = readGitPushAlertState(t, repo)
	if state.Active {
		t.Fatalf("fresh status could not clear unscoped legacy alert: %+v", state)
	}
}
