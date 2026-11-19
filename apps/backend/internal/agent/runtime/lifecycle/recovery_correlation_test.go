package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCorrelateRecoveryInstancesSingleInstanceReTracksRegardlessOfIDMatch(t *testing.T) {
	records := []*models.ExecutorRunning{
		{SessionID: "session-1", AgentExecutionID: "some-other-id"},
	}
	instance := &agentctl.InstanceInfo{ID: "instance-1", SessionID: "session-1"}

	result := CorrelateRecoveryInstances(records, []*agentctl.InstanceInfo{instance})

	if got := result.Winners["session-1"]; got != instance {
		t.Fatalf("Winners[session-1] = %v, want the sole live instance despite ID mismatch", got)
	}
	if len(result.ToStop) != 0 {
		t.Fatalf("ToStop = %v, want empty", result.ToStop)
	}
}

func TestCorrelateRecoveryInstancesRequiresExactErrandOwner(t *testing.T) {
	owner := ExecutionOwner{Kind: ExecutionOwnerErrand, WorkspaceID: "workspace", TaskID: "task", ErrandID: "errand", Generation: 2}
	sessionID := models.ErrandExecutionIdentity(owner.ErrandID)
	record := &models.ExecutorRunning{
		SessionID: sessionID, AgentExecutionID: "expected",
		Metadata: map[string]interface{}{runExecutionOwnerMetadataKey: owner},
	}
	exact := &agentctl.InstanceInfo{ID: "expected", SessionID: sessionID, ErrandMCPOwner: errandMCPOwner(owner)}
	wrongID := *exact
	wrongID.ID = "other"
	wrongGeneration := *exact
	wrongGeneration.ErrandMCPOwner = errandMCPOwner(owner)
	wrongGeneration.ErrandMCPOwner.Generation++
	unowned := *exact
	unowned.ErrandMCPOwner = nil
	taskScoped := *exact
	taskScoped.TaskID = owner.TaskID

	for _, test := range []struct {
		name      string
		records   []*models.ExecutorRunning
		instances []*agentctl.InstanceInfo
		winner    *agentctl.InstanceInfo
	}{
		{"exact", []*models.ExecutorRunning{record}, []*agentctl.InstanceInfo{exact, &wrongGeneration, &wrongID}, exact},
		{"wrong execution", []*models.ExecutorRunning{record}, []*agentctl.InstanceInfo{&wrongID}, nil},
		{"wrong generation", []*models.ExecutorRunning{record}, []*agentctl.InstanceInfo{&wrongGeneration}, nil},
		{"missing owner", []*models.ExecutorRunning{record}, []*agentctl.InstanceInfo{&unowned}, nil},
		{"task session identity", []*models.ExecutorRunning{record}, []*agentctl.InstanceInfo{&taskScoped}, nil},
		{"duplicate exact", []*models.ExecutorRunning{record}, []*agentctl.InstanceInfo{exact, exact}, nil},
		{"owned orphan", nil, []*agentctl.InstanceInfo{exact}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := CorrelateRecoveryInstances(test.records, test.instances)
			if got := result.Winners[sessionID]; got != test.winner {
				t.Errorf("winner = %v, want %v", got, test.winner)
			}
			if len(result.ToStop) != 0 {
				t.Errorf("ordinary startup stop selected an Errand candidate: %v", result.ToStop)
			}
		})
	}

	result := CorrelateRecoveryInstances(
		[]*models.ExecutorRunning{{SessionID: sessionID, TaskID: "ordinary-task"}},
		[]*agentctl.InstanceInfo{&unowned, exact},
	)
	if result.Winners[sessionID] != &unowned || len(result.ToStop) != 0 {
		t.Errorf("owned peer influenced ordinary startup correlation: %+v", result)
	}
}

func TestCorrelateRecoveryInstancesOrphanInstanceIsStopped(t *testing.T) {
	instance := &agentctl.InstanceInfo{ID: "instance-1", SessionID: "session-no-record"}

	result := CorrelateRecoveryInstances(nil, []*agentctl.InstanceInfo{instance})

	if len(result.Winners) != 0 {
		t.Fatalf("Winners = %v, want empty", result.Winners)
	}
	if len(result.ToStop) != 1 || result.ToStop[0] != instance {
		t.Fatalf("ToStop = %v, want [instance]", result.ToStop)
	}
}

func TestCorrelateRecoveryInstancesRecordWithNoInstanceIsLeftAlone(t *testing.T) {
	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "instance-1"}}

	result := CorrelateRecoveryInstances(records, nil)

	if len(result.Winners) != 0 {
		t.Fatalf("Winners = %v, want empty", result.Winners)
	}
	if len(result.ToStop) != 0 {
		t.Fatalf("ToStop = %v, want empty (left to the existing stale-execution repair path)", result.ToStop)
	}
}

// TestCorrelateRecoveryInstancesDuplicateTiebreakExactMatchWins pins
// AC-EXECUTORS-SURVIVAL-002.10: with two live instances for one session, the
// one whose ID matches the record's agent execution identifier wins and the
// other is stopped.
func TestCorrelateRecoveryInstancesDuplicateTiebreakExactMatchWins(t *testing.T) {
	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "winner"}}
	winner := &agentctl.InstanceInfo{ID: "winner", SessionID: "session-1"}
	loser := &agentctl.InstanceInfo{ID: "loser", SessionID: "session-1"}

	result := CorrelateRecoveryInstances(records, []*agentctl.InstanceInfo{loser, winner})

	if got := result.Winners["session-1"]; got != winner {
		t.Fatalf("Winners[session-1] = %v, want winner", got)
	}
	if len(result.ToStop) != 1 || result.ToStop[0] != loser {
		t.Fatalf("ToStop = %v, want [loser]", result.ToStop)
	}
}

// TestCorrelateRecoveryInstancesDuplicateNoMatchStopsAll pins the "no match"
// branch of AC-EXECUTORS-SURVIVAL-002.10: neither instance's ID equals the
// record's agent execution identifier, so none is re-tracked and both stop.
func TestCorrelateRecoveryInstancesDuplicateNoMatchStopsAll(t *testing.T) {
	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "neither"}}
	instanceA := &agentctl.InstanceInfo{ID: "a", SessionID: "session-1"}
	instanceB := &agentctl.InstanceInfo{ID: "b", SessionID: "session-1"}

	result := CorrelateRecoveryInstances(records, []*agentctl.InstanceInfo{instanceA, instanceB})

	if len(result.Winners) != 0 {
		t.Fatalf("Winners = %v, want empty", result.Winners)
	}
	if len(result.ToStop) != 2 {
		t.Fatalf("ToStop = %v, want both instances stopped", result.ToStop)
	}
}

// TestCorrelateRecoveryInstancesDuplicateAmbiguousMatchStopsAll pins the
// "more than one match" branch of AC-EXECUTORS-SURVIVAL-002.10: attributing
// the transcript to the wrong instance is worse than a cold resume, so an
// ambiguous match re-tracks nothing.
func TestCorrelateRecoveryInstancesDuplicateAmbiguousMatchStopsAll(t *testing.T) {
	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "dup"}}
	instanceA := &agentctl.InstanceInfo{ID: "dup", SessionID: "session-1"}
	instanceB := &agentctl.InstanceInfo{ID: "dup", SessionID: "session-1"}

	result := CorrelateRecoveryInstances(records, []*agentctl.InstanceInfo{instanceA, instanceB})

	if len(result.Winners) != 0 {
		t.Fatalf("Winners = %v, want empty (ambiguous match)", result.Winners)
	}
	if len(result.ToStop) != 2 {
		t.Fatalf("ToStop = %v, want both instances stopped", result.ToStop)
	}
}

// TestCorrelateRecoveryInstancesMultipleSessionsAreIndependent pins
// AC-EXECUTORS-SURVIVAL-002.11: session-1's single-instance re-track and
// session-2's ambiguous-duplicate stop-all are decided independently, and
// neither's outcome depends on the other's.
func TestCorrelateRecoveryInstancesMultipleSessionsAreIndependent(t *testing.T) {
	records := []*models.ExecutorRunning{
		{SessionID: "session-1", AgentExecutionID: "s1-instance"},
		{SessionID: "session-2", AgentExecutionID: "neither-s2-candidate"},
	}
	s1Instance := &agentctl.InstanceInfo{ID: "s1-instance", SessionID: "session-1"}
	s2A := &agentctl.InstanceInfo{ID: "s2-a", SessionID: "session-2"}
	s2B := &agentctl.InstanceInfo{ID: "s2-b", SessionID: "session-2"}

	result := CorrelateRecoveryInstances(records, []*agentctl.InstanceInfo{s1Instance, s2A, s2B})

	if got := result.Winners["session-1"]; got != s1Instance {
		t.Fatalf("Winners[session-1] = %v, want s1Instance", got)
	}
	if _, ok := result.Winners["session-2"]; ok {
		t.Fatalf("Winners[session-2] should not exist: neither of session-2's two instances matched")
	}
	if len(result.ToStop) != 2 {
		t.Fatalf("ToStop = %v, want both session-2 instances stopped, session-1 untouched", result.ToStop)
	}
}

func TestCorrelateRecoveryInstancesEmptyInputsReturnEmptyResult(t *testing.T) {
	result := CorrelateRecoveryInstances(nil, nil)
	if len(result.Winners) != 0 || len(result.ToStop) != 0 {
		t.Fatalf("CorrelateRecoveryInstances(nil, nil) = %+v, want empty result", result)
	}
}
