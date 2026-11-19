package orchestrator

import "github.com/kandev/kandev/internal/task/models"

// ParentAwaitEnder ends every in-flight await of a parent session without
// cancelling its errands.
type ParentAwaitEnder interface {
	EndParentAwaits(parentSessionID string)
}

// SetParentAwaitEnder wires the registry that ends a cancelled parent turn's
// awaits. Nil disables the hook.
func (s *Service) SetParentAwaitEnder(ender ParentAwaitEnder) {
	s.parentAwaits = ender
}

// endParentAwaits ends the awaits of a parent whose turn was cancelled. It
// cancels no errand.
func (s *Service) endParentAwaits(sessionID string) {
	if s.parentAwaits != nil {
		s.parentAwaits.EndParentAwaits(sessionID)
	}
}

// cascadeErrandsOfParent cancels the errands of a session that stopped,
// failed, or was deleted, off the calling transition.
func (s *Service) cascadeErrandsOfParent(sessionID string) {
	if s.errands != nil {
		s.errands.CascadeParent(sessionID)
	}
}

// onSessionTerminalForErrands cascades a terminal parent to its errands.
func (s *Service) onSessionTerminalForErrands(sessionID string, state models.TaskSessionState) {
	if isTerminalSessionState(state) {
		s.cascadeErrandsOfParent(sessionID)
	}
}
