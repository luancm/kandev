package errands

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
)

func (s *memoryStore) ListErrandResultClaims(_ context.Context, _, _ string, token string) ([]models.ErrandResultClaim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]models.ErrandResultClaim(nil), s.claims[token]...), nil
}

func (s *memoryStore) ReserveErrandParentAdmission(_ context.Context, a models.ErrandParentAdmission) (models.ErrandAdmissionReservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state := s.admissions[a]; state != "" {
		return models.ErrandAdmissionReservation{State: state}, nil
	}
	s.admissions[a] = models.ErrandAdmissionReserved
	return models.ErrandAdmissionReservation{Created: true, State: models.ErrandAdmissionReserved}, nil
}

func (s *memoryStore) DecideErrandParentAdmission(_ context.Context, a models.ErrandParentAdmission, state models.ErrandAdmissionState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.admissions[a] == models.ErrandAdmissionAccepted || s.admissions[a] == models.ErrandAdmissionRefused {
		return nil
	}
	s.admissions[a] = state
	if state == models.ErrandAdmissionAccepted {
		row := s.errands[a.ErrandID]
		row.ParentAdmissionGeneration++
		row.AwaitingResult = true
		if row.ResultState == models.ErrandResultPending {
			row.ResultState = models.ErrandResultSuperseded
		}
		row.Version++
	}
	return nil
}

func (s *memoryStore) ListUnresolvedErrandParentAdmissions(_ context.Context, id string) ([]models.ErrandParentAdmission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []models.ErrandParentAdmission
	for a, state := range s.admissions {
		if (id == "" || id == a.ErrandID) && (state == models.ErrandAdmissionReserved || state == models.ErrandAdmissionUnknown) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *memoryStore) ReleaseErrandResultClaims(_ context.Context, taskID, parentID string, claims []models.ErrandResultClaim) ([]*models.Errand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*models.Errand
	for _, claim := range claims {
		row := s.errands[claim.ErrandID]
		if row == nil || row.TaskID != taskID || row.ParentSessionID != parentID || row.ResultRevision != claim.ResultRevision || row.ParentAdmissionGeneration != claim.ParentAdmissionGeneration || row.DeliveryToken != claim.Token || row.DeliveredRoute != claim.Route {
			continue
		}
		if releaseResult(row) {
			row.Version++
			copy := *row
			out = append(out, &copy)
		}
	}
	return out, nil
}

func (s *memoryStore) AbortAwait(_ context.Context, taskID, parentID, token string) ([]*models.Errand, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aborted[token] = true
	var out []*models.Errand
	for _, row := range s.errands {
		if row.TaskID == taskID && row.ParentSessionID == parentID && row.DeliveryToken == token && row.DeliveredRoute == models.ErrandDeliveryAwait && releaseResult(row) {
			row.Version++
			copy := *row
			out = append(out, &copy)
		}
	}
	return out, nil
}
