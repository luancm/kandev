package orchestrator

import (
	"context"
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

func TestComposeErrandTurnPrependsBlockForDirectPrompt(t *testing.T) {
	composer := &fakeErrandComposer{composition: errands.Composition{Token: "tok", Block: "BLOCK"}}
	svc := withErrandComposer(t, composer)
	options := &promptTaskOptions{queuedMessage: &messagequeue.QueuedMessage{}}

	prompt, token, err := svc.composeErrandTurn(context.Background(), "s1", "hello", options)

	require.NoError(t, err)
	require.Equal(t, "BLOCK\n\nhello", prompt)
	require.Equal(t, "tok", token)
	require.Equal(t, []models.ErrandDeliveryRoute{models.ErrandDeliveryTurnContext}, composer.routes)
	require.Equal(t, string(models.ErrandDeliveryTurnContext), options.queuedMessage.Metadata[errandDeliveryKey])
	require.Equal(t, "tok", options.queuedMessage.Metadata[ErrandDeliveryTokenMetadataKey])
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

func TestComposeErrandTurnNotificationCarriesBlockAsPrompt(t *testing.T) {
	composer := &fakeErrandComposer{composition: errands.Composition{Token: "tok", Block: "BLOCK"}}
	svc := withErrandComposer(t, composer)

	entry := &messagequeue.QueuedMessage{Metadata: map[string]interface{}{errandNotificationKey: true}}
	prompt, token, err := svc.composeErrandTurn(context.Background(), "s1", "",
		&promptTaskOptions{errandNotification: true, queuedMessage: entry})

	require.NoError(t, err)
	require.Equal(t, "BLOCK", prompt)
	require.Equal(t, "tok", token)
	require.Equal(t, "BLOCK", entry.Content, "the notification's transcript text is its status block")
	require.Equal(t, "tok", entry.Metadata[ErrandDeliveryTokenMetadataKey])
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
	require.NoError(t, deliverer.AdmitPrepared(ctx, "s1", "", map[string]interface{}{
		errandNotificationKey: true,
	}))
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "s1").Count)

	_, _, err := svc.composeErrandTurn(ctx, "s1", "user prompt", &promptTaskOptions{})

	require.NoError(t, err)
	require.Equal(t, 0, svc.messageQueue.GetStatus(ctx, "s1").Count,
		"a turn that takes the results must withdraw the waiting notification")
}
