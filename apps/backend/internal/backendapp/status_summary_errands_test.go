package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/statussummary"
)

type fakeErrandsSummaryReader struct {
	summary statussummary.ErrandsSummary
	err     error
	calls   int
}

func (r *fakeErrandsSummaryReader) ErrandsSummaryByTask(context.Context, string) (statussummary.ErrandsSummary, error) {
	r.calls++
	return r.summary, r.err
}

// @covers AC-TASKS-ERRANDS-010.5
func TestTaskErrandsSummaryLoaderReadsRowsOnlyWhenEnabled(t *testing.T) {
	reader := &fakeErrandsSummaryReader{summary: statussummary.ErrandsSummary{Live: 2, Waiting: true}}
	loader := taskErrandsSummaryLoader(reader, true)
	got, err := loader(context.Background(), "task-1")
	if err != nil || got != reader.summary {
		t.Fatalf("enabled loader = %+v, %v; want %+v", got, err, reader.summary)
	}

	disabled := &fakeErrandsSummaryReader{summary: statussummary.ErrandsSummary{Live: 2}}
	got, err = taskErrandsSummaryLoader(disabled, false)(context.Background(), "task-1")
	if err != nil || got != (statussummary.ErrandsSummary{}) {
		t.Fatalf("disabled loader = %+v, %v; want zero", got, err)
	}
	if disabled.calls != 0 {
		t.Fatalf("disabled loader read rows %d times", disabled.calls)
	}
}

func TestTaskErrandsSummaryLoaderPropagatesReadFailure(t *testing.T) {
	failure := errors.New("database unavailable")
	reader := &fakeErrandsSummaryReader{err: failure}
	if _, err := taskErrandsSummaryLoader(reader, true)(context.Background(), "task-1"); !errors.Is(err, failure) {
		t.Fatalf("loader error = %v, want %v", err, failure)
	}
}

func (r *fakeErrandsSummaryReader) BatchErrandsSummariesByTaskIDs(_ context.Context, ids []string) (map[string]statussummary.ErrandsSummary, error) {
	r.calls++
	result := make(map[string]statussummary.ErrandsSummary, len(ids))
	for _, id := range ids {
		result[id] = r.summary
	}
	return result, r.err
}

// @covers AC-TASKS-ERRANDS-010.5
func TestTaskErrandsSummaryBatchReaderFlagOffOmitsRetainedRows(t *testing.T) {
	reader := &fakeErrandsSummaryReader{summary: statussummary.ErrandsSummary{Live: 2, Waiting: true}}
	ids := []string{"task-1", "task-2"}
	for _, enabled := range []bool{true, false} {
		got, err := taskErrandsSummaryBatchReader(reader, enabled)(context.Background(), ids)
		if err != nil || len(got) != 2 {
			t.Fatalf("enabled=%v batch=%+v err=%v", enabled, got, err)
		}
		want := statussummary.ErrandsSummary{}
		if enabled {
			want = reader.summary
		}
		for _, id := range ids {
			if got[id] != want {
				t.Fatalf("enabled=%v task=%s got=%+v want=%+v", enabled, id, got[id], want)
			}
		}
		if reader.calls != 1 {
			t.Fatalf("enabled=%v repository reads=%d, want 1", enabled, reader.calls)
		}
	}
}

func TestTaskErrandsSummaryBatchReaderPropagatesReadFailure(t *testing.T) {
	failure := errors.New("database unavailable")
	reader := &fakeErrandsSummaryReader{err: failure}
	if _, err := taskErrandsSummaryBatchReader(reader, true)(context.Background(), []string{"task-1"}); !errors.Is(err, failure) {
		t.Fatalf("error=%v, want %v", err, failure)
	}
}
