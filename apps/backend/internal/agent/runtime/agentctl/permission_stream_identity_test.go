package client

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestPermissionStreamRejectsSameClientReconnect(t *testing.T) {
	var resolves atomic.Int32
	client, server := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		switch msg.Action {
		case "agent.permissions.list":
			response, _ := ws.NewResponse(msg.ID, msg.Action, streams.PermissionListResponse{Permissions: []streams.PendingAgentPermission{{RequestID: "request", PendingID: "pending"}}})
			return response
		case "agent.permissions.resolve":
			resolves.Add(1)
			response, _ := ws.NewResponse(msg.ID, msg.Action, streams.PermissionResolveResponse{RequestID: "request", PendingID: "pending"})
			return response
		default:
			return nil
		}
	})
	defer server.Close()
	defer client.Close()
	stream, err := client.CapturePermissionStream()
	require.NoError(t, err)
	listed, err := stream.ListPendingPermissions(context.Background())
	require.NoError(t, err)
	require.Len(t, listed, 1)
	client.streamWriteMu.Lock()
	locked := true
	defer func() {
		if locked {
			client.streamWriteMu.Unlock()
		}
	}()
	resolved := make(chan error, 1)
	go func() {
		_, resolveErr := stream.ResolvePermission(context.Background(), "request", "pending", "allow")
		resolved <- resolveErr
	}()
	waitPermissionRequestRegistered(t, client)
	require.NoError(t, client.StreamUpdates(context.Background(), func(AgentEvent) {}, nil, nil))
	client.streamWriteMu.Unlock()
	locked = false
	err = <-resolved
	var notSent *RequestNotSentError
	require.True(t, errors.As(err, &notSent))
	require.Zero(t, resolves.Load())
	client.pendingMu.Lock()
	remaining := len(client.pendingRequests)
	client.pendingMu.Unlock()
	require.Zero(t, remaining)
}

func waitPermissionRequestRegistered(t *testing.T, client *Client) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		client.pendingMu.Lock()
		registered := len(client.pendingRequests) != 0
		client.pendingMu.Unlock()
		if registered {
			break
		}
		select {
		case <-deadline:
			t.Fatal("request did not register before stream replacement")
		default:
			runtime.Gosched()
		}
	}
}

func TestPermissionStreamCancelledBeforeWriteIsNotSent(t *testing.T) {
	var calls atomic.Int32
	client, server := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		calls.Add(1)
		response, _ := ws.NewResponse(msg.ID, msg.Action, streams.PermissionResolveResponse{})
		return response
	})
	defer server.Close()
	defer client.Close()
	stream, err := client.CapturePermissionStream()
	require.NoError(t, err)
	client.streamWriteMu.Lock()
	locked := true
	defer func() {
		if locked {
			client.streamWriteMu.Unlock()
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	resolved := make(chan error, 1)
	go func() {
		_, resolveErr := stream.ResolvePermission(ctx, "request", "pending", "allow")
		resolved <- resolveErr
	}()
	waitPermissionRequestRegistered(t, client)
	cancel()
	client.streamWriteMu.Unlock()
	locked = false
	err = <-resolved
	var notSent *RequestNotSentError
	require.True(t, errors.As(err, &notSent))
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls.Load())
}

func TestPermissionStreamCancellationAfterWriteIsUncertain(t *testing.T) {
	received := make(chan struct{})
	release := make(chan struct{})
	client, server := newTestClientWithStream(t, func(msg ws.Message) *ws.Message {
		close(received)
		<-release
		return nil
	})
	defer server.Close()
	defer client.Close()
	stream, err := client.CapturePermissionStream()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, resolveErr := stream.ResolvePermission(ctx, "request", "pending", "allow")
		result <- resolveErr
	}()
	<-received
	cancel()
	err = <-result
	close(release)
	require.ErrorIs(t, err, context.Canceled)
	var notSent *RequestNotSentError
	require.False(t, errors.As(err, &notSent))
	client.pendingMu.Lock()
	remaining := len(client.pendingRequests)
	client.pendingMu.Unlock()
	require.Zero(t, remaining)
}
