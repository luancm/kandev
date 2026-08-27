package messagequeue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

type pendingMoveAdmitter interface {
	InsertPendingMoveIfAbsent(context.Context, string, *PendingMove) (bool, error)
}

type exactPendingMoveDeleter interface {
	DeletePendingMoveIfExact(context.Context, string, string, string) (bool, error)
}

func TestRepositories_InsertPendingMoveIfAbsentAdmitsExactlyOneConcurrentMove(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			admitter, ok := repo.(pendingMoveAdmitter)
			if !ok {
				t.Fatal("repository does not implement atomic pending-move admission")
			}

			const contenders = 16
			start := make(chan struct{})
			var (
				wg       sync.WaitGroup
				admitted atomic.Int32
				errs     = make(chan error, contenders)
			)
			for i := 0; i < contenders; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					ok, err := admitter.InsertPendingMoveIfAbsent(context.Background(), "session-1", &PendingMove{
						MoveID:         fmt.Sprintf("move-%d", i),
						TaskID:         "task-1",
						WorkflowID:     "workflow-1",
						WorkflowStepID: fmt.Sprintf("step-%d", i),
					})
					if err != nil {
						errs <- err
						return
					}
					if ok {
						admitted.Add(1)
					}
				}(i)
			}
			close(start)
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Fatalf("InsertPendingMoveIfAbsent: %v", err)
			}
			if got := admitted.Load(); got != 1 {
				t.Fatalf("admitted = %d, want exactly 1", got)
			}

			stored, err := repo.GetPendingMove(context.Background(), "session-1")
			if err != nil {
				t.Fatalf("GetPendingMove: %v", err)
			}
			if stored == nil || stored.MoveID == "" {
				t.Fatalf("stored pending move = %#v, want the admitted contender", stored)
			}
		})
	}
}

func TestRepositories_DeletePendingMoveIfExactDoesNotDeleteReplacement(t *testing.T) {
	tests := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.new(t)
			deleter, ok := repo.(exactPendingMoveDeleter)
			if !ok {
				t.Fatal("repository does not implement exact pending-move deletion")
			}
			ctx := context.Background()
			move := &PendingMove{MoveID: "move-current", TaskID: "task-current", WorkflowID: "wf", WorkflowStepID: "target"}
			if err := repo.SetPendingMove(ctx, "session-1", move); err != nil {
				t.Fatalf("SetPendingMove: %v", err)
			}
			deleted, err := deleter.DeletePendingMoveIfExact(ctx, "session-1", "move-stale", "task-current")
			if err != nil || deleted {
				t.Fatalf("stale delete = (%v, %v), want (false, nil)", deleted, err)
			}
			stored, err := repo.GetPendingMove(ctx, "session-1")
			if err != nil || stored == nil || stored.MoveID != move.MoveID {
				t.Fatalf("replacement after stale delete = (%#v, %v)", stored, err)
			}
			deleted, err = deleter.DeletePendingMoveIfExact(ctx, "session-1", move.MoveID, move.TaskID)
			if err != nil || !deleted {
				t.Fatalf("exact delete = (%v, %v), want (true, nil)", deleted, err)
			}
		})
	}
}
