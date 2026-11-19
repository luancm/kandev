package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type interruptionProjectionStore struct {
	AgentDeliveryRepository
	committed []models.ErrandInterruptedAdmission
	cursor    int64
}

func (*interruptionProjectionStore) ListOwnedErrandInterruptionAdmissions(context.Context) ([]models.ErrandInterruptedAdmission, error) {
	return nil, nil
}

func (s *interruptionProjectionStore) CommitOwnedErrandInterruption(_ context.Context, admission models.ErrandInterruptedAdmission, projected int64) (bool, error) {
	s.committed = append(s.committed, admission)
	s.cursor = projected
	return true, nil
}

func TestOwnedErrandInterruptionRequiresTerminatedWriterAndProjectedPrefix(t *testing.T) {
	admission := models.ErrandInterruptedAdmission{
		TaskID: "task", TurnID: "owned-turn", SubmissionID: "owned-prompt",
		Binding: models.ErrandHarnessBinding{ErrandID: "errand", Generation: 1, StreamID: "stream"},
	}
	for _, tc := range []struct {
		name       string
		terminated bool
		end        uint64
		state      journal.SubmissionState
		wantCommit bool
	}{
		{name: "live writer", end: 4, state: journal.SubmissionInterruptedUnknown},
		{name: "unprojected event", terminated: true, end: 3, state: journal.SubmissionInterruptedUnknown},
		{name: "unaccepted submission", terminated: true, end: 4, state: journal.SubmissionPrepared},
		{name: "interrupted", terminated: true, end: 4, state: journal.SubmissionInterruptedUnknown, wantCommit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &interruptionProjectionStore{}
			evidence := &journal.RetainedRecoveryResponse{
				ProcessTerminated: tc.terminated,
				Descriptor: journal.RecoveryDescriptor{
					Stream: &journal.Stream{HighWater: 4},
				},
				Submission: journal.Submission{State: tc.state},
			}
			err := settleRecoveredErrandInterruption(t.Context(), store, admission, evidence, tc.end)
			if !tc.wantCommit {
				require.Error(t, err)
				require.Empty(t, store.committed)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []models.ErrandInterruptedAdmission{admission}, store.committed)
			require.EqualValues(t, tc.end, store.cursor)
		})
	}
}
