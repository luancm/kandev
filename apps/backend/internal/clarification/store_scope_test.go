package clarification

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCreateRequestDoesNotJoinDifferentSessionsByPendingID(t *testing.T) {
	store := NewStore(time.Minute)
	first, created, err := store.CreateRequest(&Request{PendingID: "same", SessionID: "session-a"})
	if err != nil || !created || first != "same" {
		t.Fatal("first request was not created")
	}
	_, created, err = store.CreateRequest(&Request{PendingID: "same", SessionID: "session-b"})
	if err != ErrWaiterScopeCollision || created {
		t.Fatalf("different session collision: created=%v err=%v", created, err)
	}
}

func TestWaiterScopesIsolateDeduplicationAndCancellation(t *testing.T) {
	store := NewStore(time.Minute)
	questions := []Question{{Prompt: "Question?"}}
	one := &Request{WaiterScope: WaiterScope{Kind: WaiterScopeErrand, ErrandID: "child", Generation: 1}, Questions: questions}
	id, created, err := store.CreateRequest(one)
	require.NoError(t, err)
	require.True(t, created)
	same, created, err := store.CreateRequest(&Request{WaiterScope: one.WaiterScope, Questions: questions})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, id, same)
	other, created, err := store.CreateRequest(&Request{WaiterScope: WaiterScope{Kind: WaiterScopeErrand, ErrandID: "child", Generation: 2}, Questions: questions})
	require.NoError(t, err)
	require.True(t, created)
	require.NotEqual(t, id, other)
	session, created, err := store.CreateRequest(&Request{SessionID: "child", Questions: questions})
	require.NoError(t, err)
	require.True(t, created)
	require.NotEqual(t, id, session)
	one.WaiterScope.Generation = 2
	one.SessionID = "child"
	require.Empty(t, store.CancelSession(""))
	require.ElementsMatch(t, []string{id}, store.CancelErrand("child", 1))
	require.True(t, store.HasWaiter(other))
	require.True(t, store.HasWaiter(session))
	require.ElementsMatch(t, []string{session}, store.CancelSession("child"))
	require.True(t, store.HasWaiter(other))
	require.ElementsMatch(t, []string{other}, store.CancelErrand("child", 2))
}

func TestWaiterScopeRejectsInvalidAndCrossOwnerExactRetries(t *testing.T) {
	for _, errandFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(errandFirst), func(t *testing.T) {
			store := NewStore(time.Minute)
			session := &Request{PendingID: "same", SessionID: "session"}
			errand := &Request{PendingID: "same", WaiterScope: WaiterScope{Kind: WaiterScopeErrand, ErrandID: "child", Generation: 1}}
			first, second := session, errand
			if errandFirst {
				first, second = errand, session
			}
			_, created, err := store.CreateRequest(first)
			require.NoError(t, err)
			require.True(t, created)
			_, created, err = store.CreateRequest(second)
			require.ErrorIs(t, err, ErrWaiterScopeCollision)
			require.False(t, created)
			_, created, missed, wait, err := store.CreateRetryRequestWithWaiter(second)
			require.ErrorIs(t, err, ErrWaiterScopeCollision)
			require.False(t, created)
			require.False(t, missed)
			require.Nil(t, wait)
			_, created, missed, wait, err = store.CreateRetryRequestWithWaiter(first)
			require.NoError(t, err)
			require.False(t, created)
			require.False(t, missed)
			require.NotNil(t, wait)
		})
	}
	for _, req := range []*Request{{}, {WaiterScope: WaiterScope{Kind: WaiterScopeErrand, ErrandID: "child"}}, {SessionID: "session", WaiterScope: WaiterScope{Kind: WaiterScopeErrand, ErrandID: "child", Generation: 1}}, {SessionID: "session", WaiterScope: WaiterScope{Kind: WaiterScopeSession, SessionID: "other"}}} {
		_, _, err := NewStore(time.Minute).CreateRequest(req)
		require.ErrorIs(t, err, ErrWaiterScope)
	}
}

func TestErrandRetryRejectsOwnerlessDeliveryReceipt(t *testing.T) {
	store := NewStore(time.Minute)
	err := store.RespondWithDeliveryConfirmation(context.Background(), "pending", &Response{}, func() error { return nil })
	require.ErrorIs(t, err, ErrNotFound)
	_, created, missed, wait, err := store.CreateRetryRequestWithWaiter(&Request{PendingID: "pending", WaiterScope: WaiterScope{Kind: WaiterScopeErrand, ErrandID: "child", Generation: 1}})
	require.ErrorIs(t, err, ErrWaiterScopeUnknownDelivery)
	require.False(t, created)
	require.False(t, missed)
	require.Nil(t, wait)
}

func TestErrandCancelPreservesInFlightDeliveryConfirmation(t *testing.T) {
	store := NewStore(time.Second)
	id, _, err := store.CreateRequest(&Request{WaiterScope: WaiterScope{Kind: WaiterScopeErrand, ErrandID: "child", Generation: 1}})
	require.NoError(t, err)
	started, release := make(chan struct{}), make(chan struct{})
	waiter := make(chan error, 1)
	responder := make(chan error, 1)
	go func() { _, err := store.WaitForResponse(context.Background(), id); waiter <- err }()
	go func() {
		responder <- store.RespondWithDeliveryConfirmation(context.Background(), id, &Response{}, func() error { close(started); <-release; return nil })
	}()
	<-started
	require.Empty(t, store.CancelErrand("child", 1))
	require.True(t, store.HasWaiter(id))
	close(release)
	require.NoError(t, <-waiter)
	require.NoError(t, <-responder)
	require.False(t, store.HasWaiter(id))
}

func TestErrandCancelConcurrentWithResponse(t *testing.T) {
	store := NewStore(time.Second)
	id, _, err := store.CreateRequest(&Request{WaiterScope: WaiterScope{Kind: WaiterScopeErrand, ErrandID: "child", Generation: 1}})
	require.NoError(t, err)
	entered, release := make(chan struct{}), make(chan struct{})
	store.onCancelSessionEntered = func(string) { close(entered); <-release }
	cancelled := make(chan []string, 1)
	go func() { cancelled <- store.CancelErrand("child", 1) }()
	<-entered
	require.NoError(t, store.Respond(id, &Response{}))
	close(release)
	require.Equal(t, []string{id}, <-cancelled)
	pending, ok := store.GetRequest(id)
	require.False(t, ok)
	require.Nil(t, pending)
}

func TestWaiterScopeIsNeverDecodedFromRequestJSON(t *testing.T) {
	var req Request
	require.NoError(t, json.Unmarshal([]byte(`{"session_id":"session","WaiterScope":{"Kind":"errand","ErrandID":"child","Generation":1}}`), &req))
	require.Equal(t, WaiterScope{}, req.WaiterScope)
	scope, err := requestWaiterScope(&req)
	require.NoError(t, err)
	require.Equal(t, WaiterScope{Kind: WaiterScopeSession, SessionID: "session"}, scope)
}

func TestLateCancelledResponseDoesNotCancelForeignReplacement(t *testing.T) {
	store := NewStore(time.Second)
	a := WaiterScope{Kind: WaiterScopeErrand, ErrandID: "a", Generation: 1}
	b := WaiterScope{Kind: WaiterScopeErrand, ErrandID: "b", Generation: 1}
	_, created, err := store.CreateRequest(&Request{PendingID: "same", WaiterScope: a})
	require.NoError(t, err)
	require.True(t, created)
	loaded, release := make(chan struct{}), make(chan struct{})
	store.onRespondLoaded = func(string) { close(loaded); <-release }
	responded := make(chan error, 1)
	go func() {
		responded <- store.RespondWithDeliveryConfirmation(context.Background(), "same", &Response{}, func() error { return nil })
	}()
	<-loaded
	require.Equal(t, []string{"same"}, store.CancelErrand("a", 1))
	_, created, err = store.CreateRequest(&Request{PendingID: "same", WaiterScope: b})
	require.NoError(t, err)
	require.True(t, created)
	close(release)
	require.ErrorIs(t, <-responded, ErrNotFound)
	store.onRespondLoaded = nil
	require.True(t, store.HasWaiter("same"))
	_, created, missed, wait, err := store.CreateRetryRequestWithWaiter(&Request{PendingID: "same", WaiterScope: b})
	require.NoError(t, err)
	require.False(t, created)
	require.False(t, missed)
	require.NotNil(t, wait)
	delivered := make(chan error, 1)
	go func() { _, err := wait(context.Background(), nil); delivered <- err }()
	require.NoError(t, store.RespondWithDeliveryConfirmation(context.Background(), "same", &Response{}, func() error { return nil }))
	require.NoError(t, <-delivered)
	_, created, missed, wait, err = store.CreateRetryRequestWithWaiter(&Request{PendingID: "same", WaiterScope: a})
	require.NoError(t, err)
	require.False(t, created)
	require.True(t, missed)
	require.Nil(t, wait)
}

func TestClearingSessionDeliveryMissPreservesForeignErrandReceipt(t *testing.T) {
	store := NewStore(time.Second)
	scopes := []WaiterScope{{Kind: WaiterScopeErrand, ErrandID: "child", Generation: 1}, {Kind: WaiterScopeSession, SessionID: "session"}}
	for _, scope := range scopes {
		req := &Request{PendingID: "same", WaiterScope: scope, SessionID: scope.SessionID}
		_, created, err := store.CreateRequest(req)
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
	store.ClearSessionDeliveryMiss("same", "session")
	_, created, missed, _, err := store.CreateRetryRequestWithWaiter(&Request{PendingID: "same", WaiterScope: scopes[0]})
	require.NoError(t, err)
	require.False(t, created)
	require.True(t, missed)
	_, created, missed, _, err = store.CreateRetryRequestWithWaiter(&Request{PendingID: "same", SessionID: "session"})
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
