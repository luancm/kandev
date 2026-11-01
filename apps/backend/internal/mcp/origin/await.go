package origin

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type awaitInvocationKey struct{}

const awaitInvocationMetadata = "kandev_await_invocation"

func WithAwaitInvocation(ctx context.Context, id models.ErrandAwaitIdentity) context.Context {
	return context.WithValue(ctx, awaitInvocationKey{}, id)
}
func AwaitInvocation(ctx context.Context) (models.ErrandAwaitIdentity, bool) {
	id, ok := ctx.Value(awaitInvocationKey{}).(models.ErrandAwaitIdentity)
	return id, ok
}

// AttachAwaitInvocation is used only by the host-owned channel bridge.
func AttachAwaitInvocation(ctx context.Context, msg *ws.Message) {
	if id, ok := AwaitInvocation(ctx); ok {
		data, _ := json.Marshal(id)
		msg.EnsureMetadata()[awaitInvocationMetadata] = string(data)
	}
}
func AwaitInvocationFromMessage(msg *ws.Message) (models.ErrandAwaitIdentity, bool) {
	var id models.ErrandAwaitIdentity
	raw := msg.Metadata[awaitInvocationMetadata]
	ok := raw != "" && len(raw) <= 2048 && json.Unmarshal([]byte(raw), &id) == nil
	return id, ok
}
