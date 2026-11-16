package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// errandsSummaryReader reads the bounded errand projection of one task.
type errandsSummaryReader interface {
	ErrandsSummaryByTask(ctx context.Context, taskID string) (statussummary.ErrandsSummary, error)
}

// taskErrandsSummaryLoader feeds the task status projection. With the errands
// flag off it reports nothing, even rows left by an earlier boot, which keeps
// the projection aligned with the parent's waiting_on_errands gate.
func taskErrandsSummaryLoader(reader errandsSummaryReader, enabled bool) statussummary.ErrandsLoader {
	return func(ctx context.Context, taskID string) (statussummary.ErrandsSummary, error) {
		if !enabled || reader == nil {
			return statussummary.ErrandsSummary{}, nil
		}
		return reader.ErrandsSummaryByTask(ctx, taskID)
	}
}

// errandsSummaryBatchReader loads only bounded aggregates for requested tasks.
type errandsSummaryBatchReader interface {
	BatchErrandsSummariesByTaskIDs(context.Context, []string) (map[string]statussummary.ErrandsSummary, error)
}

func taskErrandsSummaryBatchReader(reader errandsSummaryBatchReader, enabled bool) service.TaskStatusSummaryErrandsReader {
	return func(ctx context.Context, ids []string) (map[string]statussummary.ErrandsSummary, error) {
		if enabled && reader != nil {
			return reader.BatchErrandsSummariesByTaskIDs(ctx, ids)
		}
		zeros := make(map[string]statussummary.ErrandsSummary, len(ids))
		for _, id := range ids {
			zeros[id] = statussummary.ErrandsSummary{}
		}
		return zeros, nil
	}
}
