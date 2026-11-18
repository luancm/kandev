package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

func TestRecoveredErrandSubmissionRequiresExactAdmittedTurn(t *testing.T) {
	const session = "errand:owned"
	execution := &AgentExecution{ID: "execution", DeliveryDescriptor: &agentctl.DeliveryStatus{
		SessionID: session, IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream",
		Stream: &journal.Stream{StreamID: "stream", HighWater: 8, Acknowledged: 4},
	}}
	descriptor := &models.ErrandExecutionDescriptor{ActiveSubmissionID: "prompt:original", ActiveTurnID: "turn", PromptGeneration: 3}
	stored := &models.AgentDeliverySubmission{
		ID: descriptor.ActiveSubmissionID, SessionID: session, State: models.DeliverySubmissionAccepted,
		IncarnationID: "incarnation", DispatchAttemptID: execution.ID, HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: "hash",
	}
	accepted := journal.Submission{
		ID: stored.ID, SessionID: session, IncarnationID: stored.IncarnationID, HarnessGeneration: 2,
		StreamID: "stream", Hash: stored.PayloadHash, State: journal.SubmissionAccepted,
	}
	completed := accepted
	completed.State = journal.SubmissionCompleted
	completed.TerminalEventRetained = true
	completed.TerminalSequence = 8
	acknowledged := completed
	acknowledged.TerminalSequence = 4
	outsideSnapshot := completed
	outsideSnapshot.TerminalSequence = 9
	foreign := accepted
	foreign.Hash = "different"
	unconfirmed := accepted
	unconfirmed.State = journal.SubmissionDispatching
	awaitingReceipt := *stored
	awaitingReceipt.State = models.DeliverySubmissionDispatching

	for _, tc := range []struct {
		name   string
		peer   journal.Submission
		stored *models.AgentDeliverySubmission
		want   bool
	}{
		{"accepted", accepted, stored, true},
		{"acknowledged live dispatch", unconfirmed, stored, true},
		{"retained terminal", completed, stored, true},
		{"terminal already acknowledged", acknowledged, stored, false},
		{"terminal outside captured prefix", outsideSnapshot, stored, false},
		{"hash mismatch", foreign, stored, false},
		{"unconfirmed dispatch", unconfirmed, &awaitingReceipt, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesRecoveredErrandSubmission(execution, descriptor, tc.stored, &tc.peer); got != tc.want {
				t.Errorf("matches recovered submission = %t, want %t", got, tc.want)
			}
		})
	}
}
