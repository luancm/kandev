package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// Read from the latest binding of the current owner. An unbound successor has
// no reading, and a late observation only changes its own binding.
const errandContextMetadataColumn = `(SELECT b.context_metadata FROM errand_harness_bindings b
	WHERE b.errand_id=e.id AND b.owner_generation=(
		SELECT MAX(generation) FROM errand_execution_generations WHERE errand_id=e.id
	)
	ORDER BY b.binding_sequence DESC LIMIT 1)`

func decodeErrandContextMetadata(raw string) (map[string]any, error) {
	if raw == "" {
		return nil, nil
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, fmt.Errorf("decode stored errand context: %w", err)
	}
	return models.ProjectSessionContextMetadata(metadata), nil
}

func persistErrandContextWindowTx(ctx context.Context, tx *sqlx.Tx, errandID string, event *models.AgentDeliveryEvent) error {
	var sample streams.AgentEvent
	if err := json.Unmarshal(event.Payload, &sample); err != nil {
		return fmt.Errorf("decode errand context window: %w", err)
	}
	metadata := map[string]any{models.SessionMetaKeyContextWindow: nil}
	if size, used := sample.ContextWindowSize, sample.ContextWindowUsed; size > 0 && used >= 0 && used <= size {
		metadata[models.SessionMetaKeyContextWindow] = map[string]any{
			"size": size, "used": used, "remaining": size - used, "efficiency": float64(used) * 100 / float64(size),
			"source": "acp", "timestamp": event.ReceivedAt.UTC().Format(time.RFC3339Nano),
		}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_harness_bindings SET context_metadata=?
		WHERE errand_id=? AND stream_id=? AND incarnation_id=? AND harness_generation=?
		  AND (native_session_id='' OR native_session_id=?)`),
		string(raw), errandID, event.StreamID, event.IncarnationID, event.HarnessGeneration, sample.SessionID)
	return err
}
