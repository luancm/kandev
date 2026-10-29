package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// fakeErrandComposer records composition and release calls. It satisfies
// ErrandController with no-op lifecycle hooks.
type fakeErrandComposer struct {
	composition errands.Composition
	composeErr  error
	routes      []models.ErrandDeliveryRoute
	released    []string
}

func (f *fakeErrandComposer) DrainQueued(context.Context)                       {}
func (f *fakeErrandComposer) CascadeParent(string) bool                         { return false }
func (f *fakeErrandComposer) CancelOnSessionStop(context.Context, string) error { return nil }
func (f *fakeErrandComposer) CascadeTask(string) bool                           { return false }

func (f *fakeErrandComposer) ComposeParentTurn(
	_ context.Context, _ string, route models.ErrandDeliveryRoute,
) (errands.Composition, error) {
	f.routes = append(f.routes, route)
	return f.composition, f.composeErr
}

func (f *fakeErrandComposer) ReleaseTurnClaim(_ context.Context, token string) error {
	f.released = append(f.released, token)
	return nil
}

func withErrandComposer(t *testing.T, composer *fakeErrandComposer) *Service {
	t.Helper()
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.SetErrandController(composer)
	return svc
}

// @covers AC-TASKS-ERRANDS-004.10
func TestComposeErrandTurnPrependsBlockForDirectPrompt(t *testing.T) {
	composer := &fakeErrandComposer{composition: errands.Composition{
		Token: "tok", Block: "BLOCK", Items: []errands.DeliveryItem{{
			ErrandID: "errand-1", Title: "Review API", Status: models.ErrandStatusDone, Preview: "Reviewed the API.",
		}},
	}}
	svc := withErrandComposer(t, composer)
	message := &models.Message{
		ID: "message-1", AuthorType: models.MessageAuthorUser, Content: "hello",
		Metadata: map[string]interface{}{"existing": "keep"},
	}
	messages := &transientRetryMessageServiceStub{messages: []*models.Message{message}}
	svc.SetTransientRetryMessageService(messages)
	options := &promptTaskOptions{deliverySubmissionID: "message-1"}

	prompt, token, err := svc.composeErrandTurn(context.Background(), "s1", "hello", options)

	require.NoError(t, err)
	require.Equal(t, "BLOCK\n\nhello", prompt)
	require.Equal(t, "tok", token)
	require.Equal(t, []models.ErrandDeliveryRoute{models.ErrandDeliveryTurnContext}, composer.routes)
	require.Nil(t, options.queuedMessage, "direct prompts do not have a queued entry")
	require.Equal(t, "hello", message.Content, "delivery metadata must preserve the user's visible text")
	require.Equal(t, "keep", message.Metadata["existing"])
	require.Equal(t, "tok", message.Metadata[ErrandDeliveryTokenMetadataKey])
	require.Equal(t, "prompt:message-1", message.Metadata["errand_delivery_submission_id"])
	wire, marshalErr := json.Marshal(message.Metadata[errandDeliveryKey])
	require.NoError(t, marshalErr)
	require.JSONEq(t, `{"token":"tok","route":"turn_context","items":[{"errand_id":"errand-1","title":"Review API","status":"done","preview":"Reviewed the API."}]}`, string(wire))
}

func TestComposeErrandTurnNotificationWithoutResultsConsumesEntry(t *testing.T) {
	composer := &fakeErrandComposer{}
	svc := withErrandComposer(t, composer)

	_, token, err := svc.composeErrandTurn(context.Background(), "s1", "",
		&promptTaskOptions{errandNotification: true})

	require.ErrorIs(t, err, errErrandNotificationEmpty)
	require.Empty(t, token)
	require.Equal(t, []models.ErrandDeliveryRoute{models.ErrandDeliveryNotification}, composer.routes)
}

func TestComposeErrandTurnConsumedNotificationIgnoresWorkflowPrompt(t *testing.T) {
	composer := &fakeErrandComposer{}
	svc := withErrandComposer(t, composer)
	entry := &messagequeue.QueuedMessage{Metadata: map[string]interface{}{errandNotificationKey: true}}

	_, _, err := svc.composeErrandTurn(context.Background(), "s1", "workflow instructions",
		&promptTaskOptions{errandNotification: true, queuedMessage: entry})

	require.ErrorIs(t, err, errErrandNotificationEmpty)
}

func TestComposeErrandTurnNotificationCarriesBlockAsPrompt(t *testing.T) {
	composer := &fakeErrandComposer{composition: errands.Composition{
		Token: "tok", Block: "BLOCK", Items: []errands.DeliveryItem{{ErrandID: "errand-1", Title: "Review API", Status: models.ErrandStatusDone}},
	}}
	svc := withErrandComposer(t, composer)

	entry := &messagequeue.QueuedMessage{Metadata: map[string]interface{}{
		errandNotificationKey: true,
		errandDeliveryKey: map[string]interface{}{
			"token": "deletion-token", "route": "notification",
			"items": []interface{}{map[string]interface{}{
				"errand_id": "deleted-errand", "title": "Cancelled work", "status": "cancelled", "preview": "Cancelled.",
			}},
		},
	}}
	prompt, token, err := svc.composeErrandTurn(context.Background(), "s1", "",
		&promptTaskOptions{errandNotification: true, queuedMessage: entry})

	require.NoError(t, err)
	require.Equal(t, "BLOCK", prompt)
	require.Equal(t, "tok", token)
	require.Equal(t, "BLOCK", entry.Content, "the notification's transcript text is its status block")
	require.Equal(t, "tok", entry.Metadata[ErrandDeliveryTokenMetadataKey])
	wire, marshalErr := json.Marshal(entry.Metadata[errandDeliveryKey])
	require.NoError(t, marshalErr)
	require.JSONEq(t, `{"token":"tok","route":"notification","items":[{"errand_id":"deleted-errand","title":"Cancelled work","status":"cancelled","preview":"Cancelled."},{"errand_id":"errand-1","title":"Review API","status":"done","preview":""}]}`, string(wire))
	require.Equal(t, []models.ErrandDeliveryRoute{models.ErrandDeliveryNotification}, composer.routes)
}

func TestComposeErrandTurnFailsOpenOnCompositionError(t *testing.T) {
	composer := &fakeErrandComposer{composeErr: errors.New("store unavailable")}
	svc := withErrandComposer(t, composer)

	prompt, token, err := svc.composeErrandTurn(context.Background(), "s1", "hello", &promptTaskOptions{})

	require.NoError(t, err, "a composition failure must not gate a parent prompt")
	require.Equal(t, "hello", prompt)
	require.Empty(t, token)
}

func TestComposeErrandTurnWithoutPersistableDirectMessageReleasesClaim(t *testing.T) {
	composer := &fakeErrandComposer{composition: errands.Composition{Token: "tok", Block: "BLOCK"}}
	svc := withErrandComposer(t, composer)

	prompt, token, err := svc.composeErrandTurn(context.Background(), "s1", "hello", &promptTaskOptions{})

	require.NoError(t, err)
	require.Equal(t, "hello", prompt)
	require.Empty(t, token)
	require.Equal(t, []string{"tok"}, composer.released)
}

func TestComposeErrandTurnFindsDirectMessageWithoutSubmissionID(t *testing.T) {
	composer := &fakeErrandComposer{composition: errands.Composition{Token: "tok", Block: "BLOCK"}}
	svc := withErrandComposer(t, composer)
	message := &models.Message{
		ID: "mcp-message", TurnID: "turn-1", AuthorType: models.MessageAuthorUser, Content: "hello",
	}
	svc.activeTurns.Store("s1", "turn-1")
	svc.SetTransientRetryMessageService(&transientRetryMessageServiceStub{messages: []*models.Message{message}})

	prompt, token, err := svc.composeErrandTurn(context.Background(), "s1", "hello", &promptTaskOptions{
		directMessageContent: "hello", deliverySubmissionID: "internal:submission-1",
	})

	require.NoError(t, err)
	require.Equal(t, "BLOCK\n\nhello", prompt)
	require.Equal(t, "tok", token)
	require.Equal(t, "prompt:internal:submission-1", message.Metadata["errand_delivery_submission_id"])
	require.Equal(t, "tok", message.Metadata[ErrandDeliveryTokenMetadataKey])
}

func TestComposeErrandTurnWithoutErrandServiceLeavesPromptUnchanged(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())

	prompt, token, err := svc.composeErrandTurn(context.Background(), "s1", "hello", &promptTaskOptions{})

	require.NoError(t, err)
	require.Equal(t, "hello", prompt)
	require.Empty(t, token)
}

func TestReleaseErrandClaimOnlyWhenPromptNotAccepted(t *testing.T) {
	composer := &fakeErrandComposer{}
	svc := withErrandComposer(t, composer)
	ctx := context.Background()

	svc.releaseErrandClaimUnlessAccepted(ctx, "rejected", false)
	svc.releaseErrandClaimUnlessAccepted(ctx, "accepted", true)
	svc.releaseErrandClaimUnlessAccepted(ctx, "", false)

	require.Equal(t, []string{"rejected"}, composer.released)
}

func TestComposeErrandTurnWithdrawsWaitingNotification(t *testing.T) {
	ctx := context.Background()
	composer := &fakeErrandComposer{composition: errands.Composition{Token: "tok", Block: "BLOCK"}}
	svc, deliverer := newErrandDelivererFixture(t)
	svc.SetErrandController(composer)
	require.NoError(t, svc.messageQueue.SetAutoRun(ctx, "s1", false))
	svc.SetTransientRetryMessageService(&transientRetryMessageServiceStub{messages: []*models.Message{{
		ID: "message-withdraw", AuthorType: models.MessageAuthorUser, Content: "user prompt",
	}}})
	require.NoError(t, deliverer.AdmitPrepared(ctx, "s1", "", map[string]interface{}{
		errandNotificationKey: true,
	}))
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "s1").Count)

	_, _, err := svc.composeErrandTurn(ctx, "s1", "user prompt", &promptTaskOptions{deliverySubmissionID: "message-withdraw"})

	require.NoError(t, err)
	require.Equal(t, 0, svc.messageQueue.GetStatus(ctx, "s1").Count,
		"a turn that takes the results must withdraw the waiting notification")
}
