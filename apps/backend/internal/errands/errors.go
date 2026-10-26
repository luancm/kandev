package errands

import (
	"errors"
	"fmt"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// Reason is the stable `details.reason` of a typed errand failure.
type Reason string

const (
	ReasonFeatureDisabled     Reason = "feature_disabled"
	ReasonNotSupported        Reason = "errand_not_supported"
	ReasonNotPermitted        Reason = "errand_not_permitted"
	ReasonSenderNotParent     Reason = "errand_sender_not_parent"
	ReasonProfileNotFound     Reason = "errand_profile_not_found"
	ReasonProfileAmbiguous    Reason = "errand_profile_ambiguous"
	ReasonProfileIncompatible Reason = "errand_profile_incompatible"
	ReasonNotFound            Reason = "errand_not_found"
	ReasonLimitReached        Reason = "errand_limit_reached"
)

// MaxLiveErrands is the number of live errands one task may hold.
const MaxLiveErrands = 5

// MaxAvailableProfiles bounds the profile names a profile failure lists.
const MaxAvailableProfiles = 20

// ErrInvalidRequest wraps a request whose arguments are unusable. It maps to
// a validation failure without a reason.
var ErrInvalidRequest = errors.New("invalid errand request")

// ErrCeilingRefused is returned by a Launcher when the session ceiling refused
// the launch. The errand stays queued until capacity frees.
var ErrCeilingRefused = errors.New("errands: session ceiling refused the launch")

// Error is a typed errand failure that carries its machine-readable reason
// and bounded details for the transport layer.
type Error struct {
	Reason  Reason
	Details map[string]interface{}
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Reason, e.Message)
}

func newError(reason Reason, message string, details map[string]interface{}) *Error {
	return &Error{Reason: reason, Message: message, Details: details}
}

// ReasonOf returns the typed reason of err, if it carries one.
func ReasonOf(err error) (Reason, bool) {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Reason, true
	}
	return "", false
}

// ErrorCode maps a reason to its WebSocket error code.
func ErrorCode(reason Reason) string {
	switch reason {
	case ReasonFeatureDisabled, ReasonNotFound:
		return ws.ErrorCodeNotFound
	case ReasonNotSupported, ReasonNotPermitted, ReasonSenderNotParent:
		return ws.ErrorCodeForbidden
	case ReasonProfileNotFound, ReasonProfileAmbiguous, ReasonProfileIncompatible:
		return ws.ErrorCodeValidation
	case ReasonLimitReached:
		return ws.ErrorCodeConflict
	default:
		return ws.ErrorCodeInternalError
	}
}

// WSError renders err as a WebSocket error response when it is a typed errand
// failure or an invalid request, and reports whether it did.
func WSError(msgID, action string, err error) (*ws.Message, bool) {
	var typed *Error
	if errors.As(err, &typed) {
		details := map[string]interface{}{"reason": string(typed.Reason)}
		for key, value := range typed.Details {
			details[key] = value
		}
		msg, marshalErr := ws.NewError(msgID, action, ErrorCode(typed.Reason), typed.Message, details)
		return msg, marshalErr == nil
	}
	if errors.Is(err, ErrInvalidRequest) {
		msg, marshalErr := ws.NewError(msgID, action, ws.ErrorCodeValidation, err.Error(), nil)
		return msg, marshalErr == nil
	}
	return nil, false
}
