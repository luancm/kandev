package errands

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// ParentPromptSenderMetadataKey is written only after MCP sender verification.
// Queue dispatch also requires agent provenance before reading it.
const ParentPromptSenderMetadataKey = "errand_parent_sender_session_id"

type parentPromptContextKey struct{}

// ParentPromptOrigin carries authenticated server provenance across direct,
// created-session and fallback dispatch. SubmissionID is allocated at intake.
type ParentPromptOrigin struct {
	SenderSessionID string
	SubmissionID    string
}

// WithParentPromptOrigin binds a verified sender; it must never be derived from
// formatted prompt text, user metadata or telemetry attribution.
func WithParentPromptOrigin(ctx context.Context, origin ParentPromptOrigin) context.Context {
	return context.WithValue(ctx, parentPromptContextKey{}, origin)
}

// ParentPromptOriginFromContext returns the server-owned direct prompt origin.
func ParentPromptOriginFromContext(ctx context.Context) ParentPromptOrigin {
	origin, _ := ctx.Value(parentPromptContextKey{}).(ParentPromptOrigin)
	return origin
}

// ParentAdmissionEvidenceReader resolves only the exact retained identity.
type ParentAdmissionEvidenceReader interface {
	ReadErrandParentAdmissionEvidence(context.Context, models.ErrandParentAdmission) (models.ErrandAdmissionState, error)
}

// ReserveParentAdmission commits a fence for a real parent-origin dispatch.
func (s *Service) ReserveParentAdmission(ctx context.Context, admission models.ErrandParentAdmission) (models.ErrandAdmissionReservation, error) {
	row, err := s.store.GetErrand(ctx, admission.ErrandID)
	if errors.Is(err, models.ErrErrandNotFound) {
		return models.ErrandAdmissionReservation{}, nil
	}
	if err != nil {
		return models.ErrandAdmissionReservation{}, err
	}
	if admission.SenderSessionID != row.ParentSessionID {
		return models.ErrandAdmissionReservation{}, nil
	}
	if admission.TaskID != row.TaskID || admission.ParentSessionID != row.ParentSessionID {
		return models.ErrandAdmissionReservation{}, models.ErrInvalidErrand
	}
	return s.store.ReserveErrandParentAdmission(ctx, admission)
}

// DecideParentAdmission consumes positive admission evidence and wakes any
// deferred exact releases resolved by that same transaction.
func (s *Service) DecideParentAdmission(ctx context.Context, admission models.ErrandParentAdmission, state models.ErrandAdmissionState) error {
	if err := s.store.DecideErrandParentAdmission(ctx, admission, state); err != nil {
		return err
	}
	row, err := s.store.GetErrand(ctx, admission.ErrandID)
	if err != nil {
		return err
	}
	s.afterResultWrite(ctx, row)
	return nil
}

// ReconcileParentAdmissions never dispatches. Missing evidence retains unknown
// ownership, including legacy prompts whose process died before a decision.
func (s *Service) ReconcileParentAdmissions(ctx context.Context, errandID string, reader ParentAdmissionEvidenceReader) error {
	admissions, err := s.store.ListUnresolvedErrandParentAdmissions(ctx, errandID)
	if err != nil {
		return fmt.Errorf("list errand admission reservations: %w", err)
	}
	for _, admission := range admissions {
		state, err := reader.ReadErrandParentAdmissionEvidence(ctx, admission)
		if err != nil {
			return err
		}
		if err := s.DecideParentAdmission(ctx, admission, state); err != nil {
			return err
		}
	}
	return nil
}
