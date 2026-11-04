package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type sendNowErrandFixture struct {
	svc     *Service
	repo    *sqliterepo.Repository
	manager *mockAgentManager
	payload chan string
	release chan struct{}
}

func newSendNowErrandFixture(t *testing.T) *sendNowErrandFixture {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	seedExecutorRunning(t, repo, "session-1", "task-1", "exec-1")
	session, err := repo.GetTaskSession(t.Context(), "session-1")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	require.NoError(t, repo.UpdateTaskSession(t.Context(), session))
	f := &sendNowErrandFixture{repo: repo, payload: make(chan string, 1), release: make(chan struct{})}
	f.manager = &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	f.manager.promptAgentFunc = func(_ context.Context, _, prompt string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		f.payload <- prompt
		<-f.release
		return &executor.PromptResult{}, nil
	}
	f.svc = createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), f.manager)
	f.svc.executor = executor.NewExecutor(f.manager, repo, testLogger(), executor.ExecutorConfig{})
	f.svc.messageCreator = &repositoryBackedMessageCreator{mockMessageCreator: &mockMessageCreator{}, repo: repo}
	f.svc.SetTurnService(&repoTurnService{repo: repo})
	f.svc.SetTransientRetryMessageService(repo)
	database := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	f.svc.messageQueue = queue
	f.svc.SetErrandController(&fakeErrandComposer{})
	require.NoError(t, queue.SetAutoRun(t.Context(), "session-1", false))
	t.Cleanup(func() {
		close(f.release)
		f.svc.stopSendNowWorkers()
		if f.svc.sendNowWorkers != nil {
			waitSendNowErrandWorkers(t, f.svc)
		}
	})
	return f
}

func (f *sendNowErrandFixture) queue(t *testing.T, content, sender string, metadata map[string]interface{}) *messagequeue.QueuedMessage {
	t.Helper()
	entry, err := f.svc.messageQueue.QueueMessageWithMetadata(t.Context(), "session-1", "task-1", content, "", sender, false, nil, metadata)
	require.NoError(t, err)
	return entry
}

func (f *sendNowErrandFixture) claimForExecution(t *testing.T) *messagequeue.SendNowClaim {
	t.Helper()
	identity, err := f.svc.parentQueueIdentity(t.Context(), "session-1")
	require.NoError(t, err)
	claim, err := f.svc.messageQueue.ClaimSendNowForSession(t.Context(), identity,
		f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries)
	require.NoError(t, err)
	f.svc.markQueuedDispatchInFlight("session-1", claim.Dispatch.ID)
	return claim
}

func (f *sendNowErrandFixture) executeWithAutoRunPaused(t *testing.T) {
	t.Helper()
	claim := f.claimForExecution(t)
	require.NoError(t, f.svc.messageQueue.SetAutoRun(t.Context(), "session-1", false))
	f.svc.executeSendNowClaim(claim)
}

func preparedSendNowMetadata(token, id string) map[string]interface{} {
	return map[string]interface{}{
		errandDeliveryKey:              errands.DeliveryEnvelope{Token: token, Route: models.ErrandDeliveryNotification, Items: []errands.DeliveryItem{{ErrandID: id, Title: id, Status: models.ErrandStatusCancelled, Preview: "preview " + id}}},
		ErrandDeliveryTokenMetadataKey: token,
	}
}

// @covers AC-TASKS-ERRANDS-004.7 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-004.11
func TestSendNowMixedErrandSourcesPreserveOrdinaryInputAndIdentity(t *testing.T) {
	for _, userFirst := range []bool{true, false} {
		name := "prepared_first"
		if userFirst {
			name = "user_first"
		}
		t.Run(name, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			var sources []string
			addUser := func() {
				sources = append(sources, f.queue(t, "  original user text\n ", messagequeue.QueuedByUser, nil).ID)
			}
			if userFirst {
				addUser()
			}
			sources = append(sources, f.prepared(t, "token-A", "deleted-A", "complete prepared result A").ID)
			sources = append(sources, f.prepared(t, "token-B", "deleted-B", "complete prepared result B").ID)
			if !userFirst {
				addUser()
			}
			n, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
			require.NoError(t, err)
			require.Equal(t, 3, n)
			var prompt string
			select {
			case prompt = <-f.payload:
			case <-time.After(10 * time.Second):
				t.Fatal("provider was not reached")
			}
			require.Equal(t, 1, strings.Count(prompt, "complete prepared result A"))
			require.Equal(t, 1, strings.Count(prompt, "complete prepared result B"))
			require.Contains(t, prompt, "  original user text\n ")
			messages, err := f.svc.repo.ListMessages(t.Context(), "session-1")
			require.NoError(t, err)
			require.Len(t, messages, 1)
			message := messages[0]
			require.Equal(t, queuedTranscriptMessageID(&messagequeue.QueuedMessage{TaskID: "task-1", SessionID: "session-1"}, sources), message.ID)
			pending, err := f.svc.messageQueue.ListPendingSendNowClaims(t.Context())
			require.NoError(t, err)
			require.Len(t, pending, 1)
			encoded, err := json.Marshal(pending[0].Claim.Dispatch.Metadata)
			require.NoError(t, err)
			require.Contains(t, string(encoded), `"effective_prompt":`)
			require.Contains(t, string(encoded), "token-A")
			require.Contains(t, string(encoded), "token-B")
			require.Equal(t, pending[0].Claim.Dispatch.Metadata["errand_send_now"], message.Metadata["errand_send_now"])
		})
	}
}

func waitSendNowErrandWorkers(t *testing.T, svc *Service) {
	t.Helper()
	done := make(chan struct{})
	go func() { svc.sendNowWorkers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Send Now worker did not settle")
	}
}

// @covers AC-TASKS-ERRANDS-004.10 AC-TASKS-ERRANDS-004.12
func TestSendNowStaleNotificationOnlyStartsNoTurn(t *testing.T) {
	f := newSendNowErrandFixture(t)
	f.queue(t, "", messagequeue.QueuedByServer, map[string]interface{}{errandNotificationKey: true})
	_, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
	require.NoError(t, err)
	waitSendNowErrandWorkers(t, f.svc)
	select {
	case p := <-f.payload:
		t.Fatalf("stale notification started provider: %q", p)
	default:
	}
	turns, err := f.repo.ListTurnsBySession(t.Context(), "session-1")
	require.NoError(t, err)
	require.Empty(t, turns)
}

func TestSendNowMixedStaleNotificationPreservesOrdinaryInput(t *testing.T) {
	f := newSendNowErrandFixture(t)
	f.queue(t, "", messagequeue.QueuedByServer, map[string]interface{}{errandNotificationKey: true})
	f.queue(t, "original ordinary instruction", messagequeue.QueuedByUser, nil)
	_, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
	require.NoError(t, err)
	select {
	case p := <-f.payload:
		require.Contains(t, p, "original ordinary instruction")
	case <-time.After(10 * time.Second):
		t.Fatal("mixed stale notification lost ordinary input")
	}
}

func TestSendNowErrandContradictoryLegacyNotificationPreservesSource(t *testing.T) {
	for _, fullBlock := range []bool{false, true} {
		t.Run(fmt.Sprintf("full_block=%t", fullBlock), func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			f.manager.promptAgentFunc = nil
			row := f.result(t, "legacy-result")
			metadata := preparedSendNowMetadata("legacy-token", row.ID)
			metadata[errandNotificationKey] = true
			block := ""
			if fullBlock {
				block = "complete legacy prepared output"
			}
			source := f.queue(t, block, messagequeue.QueuedByServer, metadata)
			claim := f.claimForExecution(t)
			f.svc.SetErrandsEnabled(func() bool { return true })
			f.svc.executeSendNowClaim(claim)
			f.manager.mu.Lock()
			prompts := append([]string(nil), f.manager.capturedPrompts...)
			f.manager.mu.Unlock()
			_, queue := f.reopen(t)
			status := queue.GetStatus(t.Context(), "session-1")
			if fullBlock {
				require.Equal(t, []string{block}, prompts, "proved legacy prepared output must remain dispatchable")
				require.Empty(t, status.Entries)
			} else {
				require.Empty(t, prompts)
				require.Len(t, status.Entries, 1, "missing prepared output must be retained, not consumed as stale")
				require.Equal(t, source.ID, status.Entries[0].ID)
				want, err := json.Marshal(source.Metadata)
				require.NoError(t, err)
				got, err := json.Marshal(status.Entries[0].Metadata)
				require.NoError(t, err)
				require.JSONEq(t, string(want), string(got))
			}
		})
	}
}

func TestSendNowUserErrandMetadataIsNotServerProvenance(t *testing.T) {
	f := newSendNowErrandFixture(t)
	composer := &fakeErrandComposer{}
	f.svc.SetErrandController(composer)
	f.queue(t, "user text with a marker", messagequeue.QueuedByUser, map[string]interface{}{errandNotificationKey: true})
	_, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
	require.NoError(t, err)
	select {
	case p := <-f.payload:
		require.Contains(t, p, "user text with a marker")
	case <-time.After(10 * time.Second):
		t.Fatal("user input was classified as server output")
	}
	require.Equal(t, []models.ErrandDeliveryRoute{models.ErrandDeliveryTurnContext}, composer.routes)
}

func (f *sendNowErrandFixture) result(t *testing.T, id string) *models.Errand {
	t.Helper()
	row := &models.Errand{ID: id, TaskID: "task-1", ParentSessionID: "session-1", Title: id, Objective: "check " + id, AgentProfileID: "profile",
		FirstTurnID: "first-" + id, SubmissionID: "prompt:first-" + id, Lifecycle: models.ErrandLifecycleCancelled, CancelSource: models.ErrandCancelSourceUser,
		ResultText: "full result " + id, ResultKind: models.ErrandResultCancelled, ResultState: models.ErrandResultPending}
	require.NoError(t, f.repo.CreateErrand(t.Context(), row))
	return row
}

func (f *sendNowErrandFixture) prepared(t *testing.T, token, id, block string) *messagequeue.QueuedMessage {
	t.Helper()
	f.result(t, id)
	require.NoError(t, NewErrandParentDeliverer(f.svc).AdmitPrepared(t.Context(), "session-1", block, preparedSendNowMetadata(token, id)))
	status := f.svc.messageQueue.GetStatus(t.Context(), "session-1")
	entry := status.Entries[len(status.Entries)-1]
	require.NoError(t, f.repo.DeleteErrand(t.Context(), id))
	return &entry
}

func (f *sendNowErrandFixture) receivePrompt(t *testing.T) string {
	t.Helper()
	select {
	case prompt := <-f.payload:
		return prompt
	case <-time.After(10 * time.Second):
		t.Fatal("provider was not reached")
		return ""
	}
}

func (f *sendNowErrandFixture) realComposer(t *testing.T) {
	t.Helper()
	adapter := NewErrandAdapter(f.svc)
	service, err := errands.New(errands.Dependencies{Store: f.repo, Launcher: adapter, Stopper: adapter, MCP: adapter,
		Profiles: errandProfileStub{}, Events: &errandEventRecorder{}, Logger: testLogger()})
	require.NoError(t, err)
	f.svc.SetErrandController(service)
}

func (f *sendNowErrandFixture) reopen(t *testing.T) (*sqliterepo.Repository, *messagequeue.Service) {
	t.Helper()
	var seq int
	var name, path string
	require.NoError(t, f.repo.DB().QueryRow("PRAGMA database_list").Scan(&seq, &name, &path))
	require.NotEmpty(t, path)
	db, err := sqlx.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	require.NoError(t, err)
	return sqliterepo.NewWithInitializedDB(db, db, nil), messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
}

// @covers AC-TASKS-ERRANDS-004.7 AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-004.11
func TestSendNowMixedErrandReceiptSnapshotSurvivesReopen(t *testing.T) {
	f := newSendNowErrandFixture(t)
	prepared := f.prepared(t, "prepared-token", "deleted", "complete prepared block\nwith full output beyond preview")
	live := f.result(t, "live")
	f.realComposer(t)
	reference := v1.EntityReference{Version: v1.EntityReferenceVersion, Ref: entityrefs.CanonicalRef("kandev", "task", "ws1", "other"), Provider: "kandev", Kind: "task", ID: "other", Title: "Related task", URL: "/t/other", Scope: "ws1"}
	original := "  original ordinary text\n "
	attachment := messagequeue.MessageAttachment{Type: "image", Name: "diagram.png", MimeType: "image/png", Data: "eA=="}
	user, err := f.svc.messageQueue.QueueMessageWithMetadata(t.Context(), "session-1", "task-1", original, "", messagequeue.QueuedByUser, true,
		[]messagequeue.MessageAttachment{attachment}, map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{reference}, messagequeue.MetadataStepHandoff: "exact handoff"})
	require.NoError(t, err)
	_, err = f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
	require.NoError(t, err)
	prompt := f.receivePrompt(t)
	require.Contains(t, prompt, original)
	require.Contains(t, prompt, "exact handoff")
	require.Contains(t, prompt, reference.Ref)
	require.Equal(t, 1, strings.Count(prompt, prepared.Content))
	require.Equal(t, 1, strings.Count(prompt, live.ResultText))
	reopened, queue := f.reopen(t)
	pending, err := queue.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	claim := pending[0].Claim
	snapshot, present, err := errands.DecodeSendNowDelivery(claim.Dispatch.Metadata)
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, prompt, snapshot.EffectivePrompt)
	require.Equal(t, []string{prepared.ID, user.ID}, snapshot.SourceIDs)
	require.Len(t, snapshot.Receipts, 2)
	require.Equal(t, "prepared-token", snapshot.Receipts[0].Envelope.Token)
	require.Equal(t, prepared.ID, snapshot.Receipts[0].SourceID)
	require.Equal(t, live.ResultRevision, snapshot.Receipts[1].Claims[0].ResultRevision)
	require.Equal(t, live.ParentAdmissionGeneration, snapshot.Receipts[1].Claims[0].ParentAdmissionGeneration)
	require.Equal(t, snapshot.Receipts[1].Envelope.Token, snapshot.Receipts[1].Claims[0].Token)
	require.Equal(t, models.ErrandDeliveryNotification, snapshot.Receipts[1].Claims[0].Route)
	require.Equal(t, prepared.Content, snapshot.Sources[0].PreparedBlock)
	require.Equal(t, original, snapshot.Sources[1].Content)
	require.Equal(t, user.Attachments, claim.Sources[1].Attachments)
	require.Equal(t, user.PlanMode, claim.Sources[1].PlanMode)
	messages, err := reopened.ListMessages(t.Context(), "session-1")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, queuedTranscriptMessageID(&claim.Dispatch, snapshot.SourceIDs), messages[0].ID)
	stored, _, err := errands.DecodeSendNowDelivery(messages[0].Metadata)
	require.NoError(t, err)
	require.Equal(t, snapshot, stored)
}

// @covers AC-TASKS-ERRANDS-004.11
func TestSendNowErrandTranscriptUsesSameSourceIDsForEarlyAndLateSnapshot(t *testing.T) {
	f := newSendNowErrandFixture(t)
	identity, err := f.svc.parentQueueIdentity(t.Context(), "session-1")
	require.NoError(t, err)
	require.NoError(t, f.repo.SetSessionPrimary(t.Context(), "session-1"))
	require.NoError(t, f.repo.CreateTaskPlan(t.Context(), &models.TaskPlan{ID: "plan", TaskID: "task-1", Content: "Review the plan"}))
	_, err = f.repo.CreateTaskPlanComment(t.Context(), &models.TaskPlanComment{ID: "comment", TaskID: "task-1", PlanID: "plan", Body: "stored feedback", SelectedText: "Review", AnchorFrom: 0, AnchorTo: 6})
	require.NoError(t, err)
	admitted, err := f.svc.messageQueue.QueueMessageWithPlanComments(t.Context(), messagequeue.PlanCommentQueueRequest{
		ClientQueueID: "plan-comment-client", TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: identity.SessionIncarnationID,
		Content: "original plan comment", UserID: messagequeue.QueuedByUser, PlanCommentRefs: []models.TaskPlanCommentRef{{ID: "comment", Version: 1}}, RequirePrimarySession: true})
	require.NoError(t, err)
	comment := admitted.Message
	require.True(t, comment.IsDurablePlanComment())
	prepared := f.prepared(t, "prepared-token", "deleted", "full prepared result")
	live := f.result(t, "live")
	f.realComposer(t)
	_, err = f.svc.SendQueuedNowForSession(t.Context(), identity, QueueSendNowScopeAll, "")
	require.NoError(t, err)
	prompt := f.receivePrompt(t)
	reopened, queue := f.reopen(t)
	pending, err := queue.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	messages, err := reopened.ListMessages(t.Context(), "session-1")
	require.NoError(t, err)
	require.Len(t, messages, 1, "early creation and late update must share one row")
	require.Equal(t, queuedTranscriptMessageID(&pending[0].Claim.Dispatch, []string{comment.ID, prepared.ID}), messages[0].ID)
	snapshot, present, err := errands.DecodeSendNowDelivery(messages[0].Metadata)
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, prompt, snapshot.EffectivePrompt)
	require.Equal(t, live.ResultRevision, snapshot.Receipts[1].Claims[0].ResultRevision)
	for _, source := range pending[0].Claim.Sources {
		require.Equal(t, true, source.Metadata[metaKeyUserMessageRecorded])
	}
}

// The manager models runtime canonicalization and calls the real admission callback.
// All queue, claim, transcript and submission persistence remains production code.
type canonicalSendNowManager struct{ *mockAgentManager }

func (m *canonicalSendNowManager) PromptAgentWithAdmissionCallback(ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, before func(context.Context, string, []v1.MessageAttachment) error, accepted func()) (*executor.PromptResult, error) {
	finalPrompt := "runtime canonical prefix\n" + prompt + "\nruntime canonical suffix"
	return m.mockAgentManager.PromptAgentWithAdmissionCallback(ctx, executionID, finalPrompt, attachments, dispatchOnly, before, accepted)
}

func TestSendNowErrandPersistsCanonicalProviderPrompt(t *testing.T) {
	f := newSendNowErrandFixture(t)
	manager := &canonicalSendNowManager{mockAgentManager: f.manager}
	f.svc.agentManager = manager
	f.svc.executor = executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{})
	f.prepared(t, "prepared-token", "deleted", "full prepared notice")
	f.queue(t, "original ordinary input", messagequeue.QueuedByUser, nil)
	_, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
	require.NoError(t, err)
	prompt := f.receivePrompt(t)
	_, reopenedQueue := f.reopen(t)
	pending, err := reopenedQueue.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	snapshot, _, err := errands.DecodeSendNowDelivery(pending[0].Claim.Dispatch.Metadata)
	require.NoError(t, err)
	require.Equal(t, prompt, snapshot.EffectivePrompt, "durable claim must contain the exact canonical provider payload")
}

type outcomeSendNowManager struct {
	*mockAgentManager
	outcome string
}

func (m *outcomeSendNowManager) PromptAgentWithAdmissionCallback(ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, before func(context.Context, string, []v1.MessageAttachment) error, accepted func()) (*executor.PromptResult, error) {
	if before != nil {
		if err := before(ctx, prompt, attachments); err != nil {
			return nil, err
		}
	}
	switch m.outcome {
	case "refused":
		return nil, markPromptAdmissionRejected(errors.New("definite provider refusal"))
	case "uncertain":
		return nil, agentruntime.ErrUncertainPromptDelivery
	default:
		accepted()
		return nil, errors.New("provider reply lost after positive acceptance")
	}
}

func TestSendNowErrandClaimsSurviveRestartAndRefusal(t *testing.T) {
	for _, outcome := range []string{"refused", "accepted", "uncertain"} {
		t.Run(outcome, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			prepared := f.prepared(t, "prepared-token", "deleted", "full prepared notice")
			user := f.queue(t, "  original instruction\n ", messagequeue.QueuedByUser, nil)
			live := f.result(t, "live")
			f.realComposer(t)
			manager := &outcomeSendNowManager{mockAgentManager: f.manager, outcome: outcome}
			f.svc.agentManager = manager
			f.svc.executor = executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{})
			f.executeWithAutoRunPaused(t)
			reopened, queue := f.reopen(t)
			row, err := reopened.GetErrand(t.Context(), live.ID)
			require.NoError(t, err)
			status := queue.GetStatus(t.Context(), "session-1")
			pending, err := queue.ListPendingSendNowClaims(t.Context())
			require.NoError(t, err)
			if outcome == "refused" {
				require.Len(t, status.Entries, 2)
				require.Equal(t, []string{prepared.ID, user.ID}, []string{status.Entries[0].ID, status.Entries[1].ID})
				require.Equal(t, prepared.Content, status.Entries[0].Content)
				require.Equal(t, user.Content, status.Entries[1].Content)
				require.Empty(t, pending)
				require.Equal(t, models.ErrandResultPending, row.ResultState)
				return
			}
			require.Empty(t, status.Entries, "accepted or uncertain input cannot return to the queue")
			require.Equal(t, models.ErrandResultDelivered, row.ResultState)
			if outcome == "uncertain" {
				require.Len(t, pending, 1, "uncertainty must retain the durable owner")
				snapshot, _, err := errands.DecodeSendNowDelivery(pending[0].Claim.Dispatch.Metadata)
				require.NoError(t, err)
				require.Len(t, snapshot.Receipts, 2)
				require.Equal(t, live.ResultRevision, snapshot.Receipts[1].Claims[0].ResultRevision)
			}
		})
	}
}

func TestSendNowErrandPlanCommentRestoresExactReservationAfterReopen(t *testing.T) {
	f := newSendNowErrandFixture(t)
	identity, err := f.svc.parentQueueIdentity(t.Context(), "session-1")
	require.NoError(t, err)
	require.NoError(t, f.repo.SetSessionPrimary(t.Context(), "session-1"))
	require.NoError(t, f.repo.CreateTaskPlan(t.Context(), &models.TaskPlan{ID: "plan", TaskID: "task-1", Content: "Review the plan"}))
	_, err = f.repo.CreateTaskPlanComment(t.Context(), &models.TaskPlanComment{ID: "comment", TaskID: "task-1", PlanID: "plan", Body: "stored feedback", SelectedText: "Review", AnchorFrom: 0, AnchorTo: 6})
	require.NoError(t, err)
	admitted, err := f.svc.messageQueue.QueueMessageWithPlanComments(t.Context(), messagequeue.PlanCommentQueueRequest{
		ClientQueueID: "plan-comment-client", TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: identity.SessionIncarnationID,
		Content: "original plan comment", UserID: messagequeue.QueuedByUser, PlanCommentRefs: []models.TaskPlanCommentRef{{ID: "comment", Version: 1}}, RequirePrimarySession: true})
	require.NoError(t, err)
	prepared := f.prepared(t, "prepared-token", "deleted", "full prepared output")
	selection := f.svc.messageQueue.GetStatus(t.Context(), "session-1").Entries
	_, err = f.svc.messageQueue.ClaimSendNowForSession(t.Context(), identity, selection)
	require.NoError(t, err)
	_, queue := f.reopen(t)
	pending, err := queue.ListPendingSendNowClaims(t.Context())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.NoError(t, queue.RestoreSendNowClaim(t.Context(), &pending[0].Claim), "the exact pre-dispatch durable reservation must survive reopen")
	status := queue.GetStatus(t.Context(), "session-1")
	require.Len(t, status.Entries, 2)
	require.Equal(t, admitted.Message.ID, status.Entries[0].ID)
	require.Equal(t, admitted.Message.Content, status.Entries[0].Content)
	require.Equal(t, prepared.ID, status.Entries[1].ID)
	require.Equal(t, prepared.Content, status.Entries[1].Content)
}

func TestSendNowStaleNotificationWithOnlyLiveStatusStartsNoTurn(t *testing.T) {
	for _, ordinary := range []bool{false, true} {
		t.Run(fmt.Sprintf("ordinary=%t", ordinary), func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			f.manager.promptAgentFunc = nil
			require.NoError(t, f.repo.CreateErrand(t.Context(), &models.Errand{ID: "still-running", TaskID: "task-1", ParentSessionID: "session-1",
				Title: "still running", Objective: "check", AgentProfileID: "profile", FirstTurnID: "first", SubmissionID: "prompt:first",
				Lifecycle: models.ErrandLifecycleQueued, ResultState: models.ErrandResultNone}))
			f.realComposer(t)
			NewErrandParentDeliverer(f.svc).DeliverPending(t.Context(), "session-1")
			if ordinary {
				f.queue(t, "original ordinary input", messagequeue.QueuedByUser, nil)
			}
			_, err := f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
			require.NoError(t, err)
			waitSendNowErrandWorkers(t, f.svc)
			f.manager.mu.Lock()
			prompts := append([]string(nil), f.manager.capturedPrompts...)
			f.manager.mu.Unlock()
			turns, err := f.repo.ListTurnsBySession(t.Context(), "session-1")
			require.NoError(t, err)
			if ordinary {
				require.Len(t, prompts, 1)
				require.Contains(t, prompts[0], "original ordinary input")
				require.Contains(t, prompts[0], "still running")
				require.Len(t, turns, 1)
			} else {
				require.Empty(t, prompts, "a status-only block cannot revive a stale notification")
				require.Empty(t, turns)
			}
		})
	}
}

func TestSendNowErrandAttachmentOnlyInputPreservesEmptyPrompt(t *testing.T) {
	f := newSendNowErrandFixture(t)
	f.realComposer(t)
	attachments := make(chan []v1.MessageAttachment, 1)
	f.manager.promptAgentFunc = func(_ context.Context, _, prompt string, received []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		f.payload <- prompt
		attachments <- received
		<-f.release
		return &executor.PromptResult{}, nil
	}
	_, err := f.svc.messageQueue.QueueMessageWithMetadata(t.Context(), "session-1", "task-1", "", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{Type: "image", Name: "diagram.png", MimeType: "image/png", Data: "eA=="}}, nil)
	require.NoError(t, err)
	_, err = f.svc.SendQueuedNow(t.Context(), "session-1", QueueSendNowScopeAll, "")
	require.NoError(t, err)
	require.Empty(t, f.receivePrompt(t), "attachment-only input must retain its exact empty provider text")
	received := <-attachments
	require.Len(t, received, 1)
	require.Equal(t, "diagram.png", received[0].Name)
}

func TestSendNowErrandAmbiguousLegacySourcesRetained(t *testing.T) {
	for _, shape := range []string{"malformed_envelope", "scalar_without_envelope", "contradictory_token"} {
		t.Run(shape, func(t *testing.T) {
			f := newSendNowErrandFixture(t)
			f.manager.promptAgentFunc = nil
			row := f.result(t, "legacy-result")
			metadata := preparedSendNowMetadata("legacy-token", row.ID)
			metadata[errandNotificationKey] = true
			content := ""
			switch shape {
			case "malformed_envelope":
				metadata[errandDeliveryKey] = "invalid"
			case "scalar_without_envelope":
				delete(metadata, errandDeliveryKey)
			case "contradictory_token":
				metadata[ErrandDeliveryTokenMetadataKey] = "unrelated-token"
				content = "full prepared legacy block"
			}
			source := f.queue(t, content, messagequeue.QueuedByServer, metadata)
			claim := f.claimForExecution(t)
			f.svc.SetErrandsEnabled(func() bool { return true })
			f.svc.executeSendNowClaim(claim)
			f.manager.mu.Lock()
			prompts := append([]string(nil), f.manager.capturedPrompts...)
			f.manager.mu.Unlock()
			require.Empty(t, prompts, "ambiguous prepared source cannot enter provider I/O")
			_, queue := f.reopen(t)
			status := queue.GetStatus(t.Context(), "session-1")
			require.Len(t, status.Entries, 1, "ambiguous legacy source must remain recoverable")
			require.Equal(t, source.ID, status.Entries[0].ID)
			require.Equal(t, content, status.Entries[0].Content)
		})
	}
}
