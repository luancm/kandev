package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

// ReadErrandParentAdmissionEvidence requires acceptance for the exact owned turn and submission.
func (s *Service) ReadErrandParentAdmissionEvidence(ctx context.Context, a models.ErrandParentAdmission) (models.ErrandAdmissionState, error) {
	if store, ok := s.repo.(interface {
		ReadOwnedErrandContinuationAcceptance(context.Context, models.ErrandParentAdmission) (models.ErrandAdmissionState, bool, error)
	}); ok {
		if state, owned, err := store.ReadOwnedErrandContinuationAcceptance(ctx, a); owned || err != nil {
			return state, err
		}
	}
	return models.ErrandAdmissionUnknown, nil
}

func (s *Service) reconcileErrandParentAdmissions(ctx context.Context) error {
	controller, ok := s.errands.(interface {
		ReconcileParentAdmissions(context.Context, string, errands.ParentAdmissionEvidenceReader) error
	})
	if !ok {
		return nil
	}
	if err := controller.ReconcileParentAdmissions(ctx, "", s); err != nil {
		return fmt.Errorf("recover errand parent admissions: %w", err)
	}
	return nil
}
