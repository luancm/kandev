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
