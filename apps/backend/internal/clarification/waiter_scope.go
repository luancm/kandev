package clarification

import "errors"

type WaiterScopeKind string

const (
	WaiterScopeSession WaiterScopeKind = "session"
	WaiterScopeErrand  WaiterScopeKind = "errand"
)

// WaiterScope is host-supplied in-memory waiter ownership, not durable authority.
type WaiterScope struct {
	Kind      WaiterScopeKind
	SessionID string
	ErrandID  string
	// Generation is the Errand execution-owner generation.
	Generation uint64
}

var ErrWaiterScope = errors.New("invalid clarification waiter scope")
var ErrWaiterScopeCollision = errors.New("clarification pending ID belongs to another waiter scope")
var ErrWaiterScopeUnknownDelivery = errors.New("clarification delivery receipt has no waiter scope")

func requestWaiterScope(req *Request) (WaiterScope, error) {
	if req == nil {
		return WaiterScope{}, ErrWaiterScope
	}
	scope := req.WaiterScope
	if scope == (WaiterScope{}) {
		scope = WaiterScope{Kind: WaiterScopeSession, SessionID: req.SessionID}
	}
	switch scope.Kind {
	case WaiterScopeSession:
		if scope.SessionID == "" || scope.SessionID != req.SessionID || scope.ErrandID != "" || scope.Generation != 0 {
			return WaiterScope{}, ErrWaiterScope
		}
	case WaiterScopeErrand:
		if scope.ErrandID == "" || scope.Generation == 0 || scope.SessionID != "" || req.SessionID != "" {
			return WaiterScope{}, ErrWaiterScope
		}
	default:
		return WaiterScope{}, ErrWaiterScope
	}
	return scope, nil
}
