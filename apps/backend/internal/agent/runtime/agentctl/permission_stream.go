package client

import (
	"context"
	"time"

	"github.com/gorilla/websocket"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type permissionRequestSender func(context.Context, string, interface{}) (*ws.Message, error)

// PermissionStream binds operations to one accepted agent stream connection.
type PermissionStream struct {
	client *Client
	conn   *websocket.Conn
}

// CapturePermissionStream fails if no current agent stream can be addressed.
func (c *Client) CapturePermissionStream() (*PermissionStream, error) {
	if c == nil || !c.RuntimeCurrent() {
		return nil, &RequestNotSentError{Err: ErrAgentStreamNotConnected}
	}
	c.mu.RLock()
	conn := c.agentStreamConn
	c.mu.RUnlock()
	if conn == nil {
		return nil, &RequestNotSentError{Err: ErrAgentStreamNotConnected}
	}
	return &PermissionStream{client: c, conn: conn}, nil
}

func (s *PermissionStream) send(ctx context.Context, action string, payload interface{}) (*ws.Message, error) {
	if s == nil || s.client == nil || s.conn == nil || !s.client.RuntimeCurrent() {
		return nil, &RequestNotSentError{Err: ErrAgentStreamNotConnected}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.client.sendStreamRequestOnConnection(ctx, s.conn, action, payload, true)
}

func (s *PermissionStream) ListPendingPermissions(ctx context.Context) ([]PendingAgentPermission, error) {
	return listPendingPermissions(ctx, s.send)
}

func (s *PermissionStream) ResolvePermission(ctx context.Context, requestID, pendingID, optionID string) (*PermissionResolveResponse, error) {
	return resolvePermission(ctx, s.send, requestID, pendingID, optionID)
}

func (s *PermissionStream) CancelPermission(ctx context.Context, requestID, pendingID string) (*PermissionCancelResponse, error) {
	return cancelPermission(ctx, s.send, requestID, pendingID)
}
