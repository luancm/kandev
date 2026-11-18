package clarification

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateRequestDoesNotJoinDifferentSessionsByPendingID(t *testing.T) {
	store := NewStore(time.Minute)
	_, created, err := store.CreateRequest(&Request{PendingID: "same", SessionID: "session-a"})
	require.NoError(t, err)
	require.True(t, created)
	_, created, err = store.CreateRequest(&Request{PendingID: "same", SessionID: "session-b"})
	require.ErrorIs(t, err, ErrWaiterSessionCollision)
	require.False(t, created)
	_, created, missed, wait, err := store.CreateRetryRequestWithWaiter(&Request{PendingID: "same", SessionID: "session-b"})
	require.ErrorIs(t, err, ErrWaiterSessionCollision)
	require.False(t, created)
	require.False(t, missed)
	require.Nil(t, wait)
	_, _, err = store.CreateRequest(&Request{})
	require.ErrorIs(t, err, ErrWaiterSession)
}

func TestWaiterSessionSnapshotIsolatesDedupAndCancellation(t *testing.T) {
	store := NewStore(time.Minute)
	questions := []Question{{Prompt: "Question?"}}
	first := &Request{SessionID: "a", Questions: questions}
	id, created, err := store.CreateRequest(first)
	require.NoError(t, err)
	require.True(t, created)
	same, created, err := store.CreateRequest(&Request{SessionID: "a", Questions: questions})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, id, same)
	other, created, err := store.CreateRequest(&Request{SessionID: "b", Questions: questions})
	require.NoError(t, err)
	require.True(t, created)
	require.NotEqual(t, id, other)
	first.SessionID = "b"
	require.Equal(t, []string{id}, store.CancelSession("a"))
	require.True(t, store.HasWaiter(other))
	require.Equal(t, []string{other}, store.CancelSession("b"))
}

func TestLateCancelledResponseDoesNotCancelForeignReplacement(t *testing.T) {
	store := NewStore(time.Second)
	_, created, err := store.CreateRequest(&Request{PendingID: "same", SessionID: "a"})
	require.NoError(t, err)
	require.True(t, created)
	loaded, release := make(chan struct{}), make(chan struct{})
	store.onRespondLoaded = func(string) { close(loaded); <-release }
	responded := make(chan error, 1)
	go func() {
		responded <- store.RespondWithDeliveryConfirmation(context.Background(), "same", &Response{}, func() error { return nil })
	}()
	<-loaded
	require.Equal(t, []string{"same"}, store.CancelSession("a"))
	_, created, err = store.CreateRequest(&Request{PendingID: "same", SessionID: "b"})
	require.NoError(t, err)
	require.True(t, created)
	close(release)
	require.ErrorIs(t, <-responded, ErrNotFound)
	store.onRespondLoaded = nil
	require.True(t, store.HasWaiter("same"))
	_, created, missed, wait, err := store.CreateRetryRequestWithWaiter(&Request{PendingID: "same", SessionID: "b"})
	require.NoError(t, err)
	require.False(t, created)
	require.False(t, missed)
	require.NotNil(t, wait)
	delivered := make(chan error, 1)
	go func() { _, err := wait(context.Background(), nil); delivered <- err }()
	require.NoError(t, store.RespondWithDeliveryConfirmation(context.Background(), "same", &Response{}, func() error { return nil }))
	require.NoError(t, <-delivered)
	_, created, missed, wait, err = store.CreateRetryRequestWithWaiter(&Request{PendingID: "same", SessionID: "a"})
	require.NoError(t, err)
	require.False(t, created)
	require.True(t, missed)
	require.Nil(t, wait)
}

func TestClearingSessionDeliveryMissPreservesOtherSessionReceipt(t *testing.T) {
	store := NewStore(time.Second)
	for _, session := range []string{"a", "b"} {
		_, created, err := store.CreateRequest(&Request{PendingID: "same", SessionID: session})
		require.NoError(t, err)
		require.True(t, created)
		loaded, release := make(chan struct{}), make(chan struct{})
		store.onRespondLoaded = func(string) { close(loaded); <-release }
		done := make(chan error, 1)
		go func() {
			done <- store.RespondWithDeliveryConfirmation(context.Background(), "same", &Response{}, func() error { return nil })
		}()
		<-loaded
		require.True(t, store.CancelRequest("same"))
		close(release)
		require.ErrorIs(t, <-done, ErrNotFound)
		store.onRespondLoaded = nil
	}
	store.ClearSessionDeliveryMiss("same", "b")
	_, created, missed, _, err := store.CreateRetryRequestWithWaiter(&Request{PendingID: "same", SessionID: "a"})
	require.NoError(t, err)
	require.False(t, created)
	require.True(t, missed)
	_, created, missed, _, err = store.CreateRetryRequestWithWaiter(&Request{PendingID: "same", SessionID: "b"})
	require.NoError(t, err)
	require.True(t, created)
	require.False(t, missed)
}

func TestVerifiedSessionRestoreClearsOwnerlessReceipt(t *testing.T) {
	store := NewStore(time.Second)
	require.ErrorIs(t, store.RespondWithDeliveryConfirmation(context.Background(), "pending", &Response{}, func() error { return nil }), ErrNotFound)
	store.ClearSessionDeliveryMiss("pending", "session")
	_, created, missed, _, err := store.CreateRetryRequestWithWaiter(&Request{PendingID: "pending", SessionID: "session"})
	require.NoError(t, err)
	require.True(t, created)
	require.False(t, missed)
}
