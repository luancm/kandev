package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-004.11 AC-TASKS-ERRANDS-012.2
func TestErrandDispatchSelectionRetainsBypassedSourcesBeforeReservation(t *testing.T) {
	for _, factory := range []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{"memory", func(*testing.T) Repository { return NewMemoryRepository() }}, {"sqlite", newTestSQLiteRepo}, {"postgres", newTestPostgresRepo},
	} {
		for _, retained := range []bool{false, true} {
			name := factory.name + "/ordinary"
			if retained {
				name = factory.name + "/retained"
			}
			t.Run(name, func(t *testing.T) {
				repo := factory.new(t)
				identity := QueueSessionIdentity{TaskID: "task-1", SessionID: "session-1", SessionIncarnationID: "inc-1"}
				seedQueueSessionIdentity(t, repo, identity)
				service := NewService(repo, 3, logger.Default())
				service.SetAutoMergeEnabled(false)
				held := insertTestEntry(t, repo, identity.SessionID, identity.TaskID, "full retained notice", QueuedByServer, nil, map[string]interface{}{"held": true})
				ordinary := insertTestEntry(t, repo, identity.SessionID, identity.TaskID, "ordinary first", QueuedByUser, nil, nil)
				second := insertTestEntry(t, repo, identity.SessionID, identity.TaskID, "ordinary second", QueuedByUser, nil, nil)
				service.SetDispatchSourcePolicy(func(_ context.Context, entry *QueuedMessage) DispatchSourceDecision {
					return DispatchSourceDecision{Allowed: entry.Metadata["held"] != true}
				})
				require.NoError(t, service.SetAutoRunForSession(t.Context(), identity, false))
				_, exists, autoRun, err := service.ReserveQueuedWithAutoRunForSession(t.Context(), identity)
				require.NoError(t, err)
				require.False(t, exists)
				require.False(t, autoRun)
				require.NoError(t, service.SetAutoRunForSession(t.Context(), identity, true))
				var chosen *QueuedMessage
				if retained {
					chosen, exists, autoRun, err = service.ReserveQueuedForDeliveryWithAutoRunForSession(t.Context(), identity)
				} else {
					chosen, exists, autoRun, err = service.ReserveQueuedWithAutoRunForSession(t.Context(), identity)
				}
				require.NoError(t, err)
				require.True(t, exists)
				require.True(t, autoRun)
				require.Equal(t, ordinary.ID, chosen.ID)
				status := service.GetStatus(t.Context(), identity.SessionID)
				require.Len(t, status.Entries, 2)
				require.Equal(t, held.ID, status.Entries[0].ID)
				require.Equal(t, held.Content, status.Entries[0].Content)
				require.False(t, status.Entries[0].IsReservedInFlight())
				require.Equal(t, second.ID, status.Entries[1].ID)
			})
		}
	}
}

// @covers AC-TASKS-ERRANDS-004.9 AC-TASKS-ERRANDS-012.2
func TestErrandDispatchSelectionRechecksExactSourceInsideReservation(t *testing.T) {
	for _, factory := range []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{"memory", func(*testing.T) Repository { return NewMemoryRepository() }}, {"sqlite", newTestSQLiteRepo}, {"postgres", newTestPostgresRepo},
	} {
		for _, owner := range []string{QueuedByUser} {
			t.Run(factory.name+"/"+owner, func(t *testing.T) {
				repo := factory.new(t)
				service := NewService(repo, 10, logger.Default())
				source := insertTestEntry(t, repo, "session-1", "task-1", "validated content", owner, nil, nil)
				next := insertTestEntry(t, repo, "session-1", "task-1", "ordinary input", QueuedByUser, nil, nil)
				mutated := false
				service.SetDispatchSourcePolicy(func(ctx context.Context, entry *QueuedMessage) DispatchSourceDecision {
					if entry.ID == source.ID && !mutated {
						mutated = true
						require.NoError(t, repo.UpdateContent(ctx, "session-1", source.ID, "changed after validation", nil, owner))
					}
					return DispatchSourceDecision{Allowed: true}
				})
				_, exists, autoRun := service.ReserveQueuedWithAutoRun(t.Context(), "session-1")
				require.False(t, exists, "changed selection must be revalidated before advancing FIFO")
				require.True(t, autoRun)
				entries, err := repo.ListBySession(t.Context(), "session-1")
				require.NoError(t, err)
				require.Len(t, entries, 2)
				require.Equal(t, source.ID, entries[0].ID)
				require.Equal(t, "changed after validation", entries[0].Content)
				require.Equal(t, next.ID, entries[1].ID)
				chosen, exists, _ := service.ReserveQueuedWithAutoRun(t.Context(), "session-1")
				require.True(t, exists)
				require.Equal(t, source.ID, chosen.ID, "revalidation preserves ordinary FIFO")
				require.Equal(t, "changed after validation", chosen.Content)
			})
		}
	}
}
