package sqlite

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDeliveryRetentionOpaquePayloadAdmission(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		bound, errand, accept bool
	}{
		{name: "ordinary unbound", accept: true}, {name: "ordinary bound", bound: true}, {name: "errand", errand: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			seedAgentDeliveryTestSession(t, repo, "opaque-session", "opaque-incarnation")
			if tc.errand {
				_, err := repo.db.Exec(`UPDATE task_sessions SET role='errand' WHERE id='opaque-session'`)
				require.NoError(t, err)
			}
			if tc.bound {
				_, err := repo.db.Exec(`INSERT INTO agent_delivery_effects (effect_key,stream_id,sequence,effect_type,state,outcome,created_at) VALUES (?, 'opaque-stream',0,?,'pending','{}',CURRENT_TIMESTAMP)`, promptAdmissionEffect+":opaque-submission", promptAdmissionEffect)
				require.NoError(t, err)
			}
			event := &models.AgentDeliveryEvent{SessionID: "opaque-session", IncarnationID: "opaque-incarnation", HarnessGeneration: 1, StreamID: "opaque-stream", Sequence: 1, SubmissionID: "opaque-submission", EventType: "adapter_event", Payload: []byte(`{"broken":`)}
			inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
			if tc.accept {
				require.NoError(t, err)
				require.True(t, inserted)
			} else {
				require.Error(t, err)
				require.False(t, inserted)
			}
		})
	}
}
