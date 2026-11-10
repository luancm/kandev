package process

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestLateTerminalKeepsItsSubmissionDuringSuccessorDispatch(t *testing.T) {
	manager := &Manager{deliveryActiveID: "successor"}
	manager.TrackDeliverySubmission("predecessor", 1)
	manager.TrackDeliverySubmission("successor", 2)
	id := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{
		Type: adapter.EventTypeComplete, PromptGeneration: 1,
	})
	if id != "predecessor" {
		t.Fatalf("late terminal submission = %q, want predecessor", id)
	}
	if manager.deliverySubmissionIDForEvent(adapter.AgentEvent{PromptGeneration: 2}) != "successor" {
		t.Fatal("late terminal removed successor association")
	}
}

// Probe retained output after A completed and B became the active dispatch.
func TestErrandRetainedOutputSubmission(t *testing.T) {
	manager := &Manager{deliveryActiveID: "successor"}
	manager.TrackDeliverySubmission("predecessor", 1)
	manager.TrackDeliverySubmission("successor", 2)
	if got := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{Type: adapter.EventTypeComplete, PromptGeneration: 1}); got != "predecessor" {
		t.Fatalf("terminal = %q", got)
	}
	if got := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 1}); got != "predecessor" {
		t.Fatalf("retained A output attributed to %q, want predecessor", got)
	}
}

func TestErrandRetainedOutputJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := journal.Open(journal.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{cfg: &config.InstanceConfig{DurableJournalPath: path, SessionID: "session", DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1}, logger: newTestLogger(t), deliveryJournal: j, deliveryActiveID: "successor"}
	t.Cleanup(func() {
		if err := manager.closeDeliveryJournal(); err != nil {
			t.Error(err)
		}
	})
	manager.TrackDeliverySubmission("predecessor", 1)
	manager.TrackDeliverySubmission("successor", 2)
	if got := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{Type: adapter.EventTypeComplete, PromptGeneration: 1}); got != "predecessor" {
		t.Fatalf("terminal = %q", got)
	}
	_, err = manager.persistDeliveryEvent(adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 1, Text: "late A output"})
	if err != nil {
		t.Fatal(err)
	}
	events, _, err := j.Replay(t.Context(), "stream", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	if events[0].SubmissionID != "predecessor" {
		t.Fatalf("journal persisted A output with submission %q, want predecessor", events[0].SubmissionID)
	}
}

func TestDeliveryUnknownGenerationDoesNotAdoptActiveSubmission(t *testing.T) {
	manager := &Manager{deliveryActiveID: "active"}
	for generation := uint64(1); generation <= 65; generation++ {
		manager.TrackDeliverySubmission("retained", generation)
		manager.deliverySubmissionIDForEvent(adapter.AgentEvent{Type: adapter.EventTypeComplete, PromptGeneration: generation})
	}
	for _, generation := range []uint64{1, 99} {
		if got := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{PromptGeneration: generation}); got != "" {
			t.Errorf("generation %d adopted %q", generation, got)
		}
	}
	if got := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{}); got != "active" {
		t.Errorf("generation-zero dispatch = %q", got)
	}
	manager.deliveryActiveID = ""
	if got := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{}); got != "" {
		t.Errorf("unowned async = %q", got)
	}
}

func TestDeliveryConflictingIdentityCannotEnterJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := journal.Open(journal.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{cfg: &config.InstanceConfig{DurableJournalPath: path, SessionID: "session", DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1}, logger: newTestLogger(t), deliveryJournal: j}
	t.Cleanup(func() {
		if err := manager.closeDeliveryJournal(); err != nil {
			t.Error(err)
		}
	})
	manager.TrackDeliverySubmission("original", 1)
	_, err = manager.persistDeliveryEvent(adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 1, DeliverySubmissionID: "conflict", Text: "work"})
	if err == nil {
		t.Fatal("conflicting identity committed")
	}
	events, _, err := j.Replay(t.Context(), "stream", 0, 10)
	if err != nil && !errors.Is(err, journal.ErrStreamNotFound) {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("conflicting events = %d", len(events))
	}
}

func TestDeliveryIdentityConflictDoesNotCancelSuccessor(t *testing.T) {
	manager := newAllToolManager(t, 4096)
	manager.lifetimeCtx = context.Background()
	cancelled := make(chan struct{}, 1)
	manager.adapter = &pressureCancelAdapter{stubAdapter: &stubAdapter{}, cancelled: cancelled}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := manager.WaitForAdmission(ctx); err != nil {
			t.Error(err)
		}
	})
	manager.TrackDeliverySubmission("predecessor", 1)
	manager.TrackDeliverySubmission("successor", 2)
	manager.deliveryActiveID = "successor"
	forwardAllToolUpdates(manager,
		adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 1, DeliverySubmissionID: "successor", Text: "conflict"},
		adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 2, Text: "valid successor"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := manager.WaitForAdmission(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
		t.Fatal("identity conflict cancelled successor provider")
	default:
	}
	if manager.DeliveryHealth().Failure != nil {
		t.Fatal("identity rejection poisoned storage health")
	}
	events, _, err := manager.deliveryJournal.Replay(t.Context(), "stream", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].SubmissionID != "successor" {
		t.Fatalf("successor delivery = %+v", events)
	}
}

func TestDeliveryIdentityConflictRejectsBatchAtomically(t *testing.T) {
	manager := newAllToolManager(t, 4096)
	manager.TrackDeliverySubmission("original", 1)
	manager.TrackDeliverySubmission("successor", 2)
	_, err := manager.persistDeliveryBatch(t.Context(), []adapter.AgentEvent{
		{Type: adapter.EventTypeMessageChunk, PromptGeneration: 2, Text: "valid"},
		{Type: adapter.EventTypeMessageChunk, PromptGeneration: 1, DeliverySubmissionID: "successor", Text: "conflict"},
	})
	if !errors.Is(err, errDeliveryEventIdentity) {
		t.Fatalf("batch error = %v", err)
	}
	events, _, err := manager.deliveryJournal.Replay(t.Context(), "stream", 0, 10)
	if err != nil && !errors.Is(err, journal.ErrStreamNotFound) {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatal("rejected batch partially committed")
	}
	if manager.DeliveryHealth().Failure != nil {
		t.Fatal("batch rejection poisoned storage health")
	}
	_, err = manager.persistDeliveryEvent(adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 2, Text: "valid retry"})
	if err != nil {
		t.Fatal(err)
	}
	events, _, err = manager.deliveryJournal.Replay(t.Context(), "stream", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].SubmissionID != "successor" {
		t.Fatalf("retry = %+v", events)
	}
}
