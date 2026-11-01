package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/statussummary"
)

// TaskStatusSummaryErrandsReader supplies complete bounded task aggregates.
type TaskStatusSummaryErrandsReader func(context.Context, []string) (map[string]statussummary.ErrandsSummary, error)

// SetTaskStatusSummaryErrandsReader configures the flag-aware aggregate reader.
func (s *Service) SetTaskStatusSummaryErrandsReader(reader TaskStatusSummaryErrandsReader) {
	if s != nil {
		s.statusSummaryErrands = reader
	}
}

type errandsSummaryObservation struct {
	summary  *statussummary.ErrandsSummary
	observed bool
	err      error
}

func (s *Service) loadSummaryErrands(ctx context.Context, taskIDs []string) map[string]errandsSummaryObservation {
	observations := make(map[string]errandsSummaryObservation, len(taskIDs))
	if s.statusSummaryErrands == nil || len(taskIDs) == 0 {
		return observations
	}
	batch, err := s.statusSummaryErrands(ctx, taskIDs)
	for _, taskID := range taskIDs {
		value, found := batch[taskID]
		observation := errandsSummaryObservation{observed: true, err: err}
		if err == nil && !found {
			observation.err = fmt.Errorf("errands batch omitted task %q", taskID)
		}
		if err == nil && value.Live < 0 {
			observation.err = fmt.Errorf("errands batch returned negative live count for task %q", taskID)
		}
		if value.Live > 0 || value.Waiting {
			copy := value
			observation.summary = &copy
		}
		observations[taskID] = observation
	}
	return observations
}

func cloneErrandsForService(value *statussummary.ErrandsSummary) *statussummary.ErrandsSummary {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func errandsNeedReconcile(current *statussummary.ErrandsSummary, observation errandsSummaryObservation) bool {
	if !observation.observed {
		return false
	}
	desired := observation.summary
	if current == nil || desired == nil {
		return current != desired
	}
	return *current != *desired
}
