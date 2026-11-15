package orchestrator

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
)

// errandWaitingTimeout bounds the errand-state read made while a session is
// serialized, so a slow store cannot stall a session payload.
const errandWaitingTimeout = 2 * time.Second

// errandStateReporter is the errand-state part of the errands service. It is
// type-asserted from the controller, which keeps its own method set fixed.
type errandStateReporter interface {
	ParentErrandState(ctx context.Context, parentSessionID string) (hasErrands, outstanding bool, err error)
}

// ParentErrandState reports a parent's errand state for serialization. A
// failed read is logged and reported as no errand rows, so the field is omitted
// rather than guessed. It satisfies dto.ErrandWaitingProvider.
func (s *Service) ParentErrandState(sessionID string) (bool, bool) {
	if s.errands == nil || sessionID == "" {
		return false, false
	}
	reporter, ok := s.errands.(errandStateReporter)
	if !ok {
		return false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), errandWaitingTimeout)
	defer cancel()
	hasErrands, outstanding, err := reporter.ParentErrandState(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to read errand state of parent session",
			zap.String("session_id", sessionID), zap.Error(err))
		return false, false
	}
	return hasErrands, outstanding
}

// addWaitingOnErrandsField adds waiting_on_errands to a session event payload
// when the session owns an errand row. Ordinary sessions keep their payloads.
func (s *Service) addWaitingOnErrandsField(payload map[string]interface{}, session *models.TaskSession) {
	if session == nil {
		return
	}
	if waiting := dto.ErrandWaitingPayload(session.ID, session.State, s); waiting != nil {
		payload["waiting_on_errands"] = *waiting
	}
}
