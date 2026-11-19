package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const admittedToolSourceEffect = "agent_delivery.admitted_tool_source.v1"

type admittedToolSource struct {
	toolSource
	Admission       promptAdmission `json:"admission"`
	NativeSessionID string          `json:"native_session_id"`
	PayloadHash     string          `json:"payload_hash"`
}

func (r *Repository) storeAdmittedToolSourceTx(ctx context.Context, tx *sqlx.Tx, source admittedToolSource) error {
	raw, err := json.Marshal(source)
	if err != nil {
		return err
	}
	p := source.Admission
	key := fmt.Sprintf("%s:%s:%d", admittedToolSourceEffect, p.StreamID, source.Sequence)
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO agent_delivery_effects
 (effect_key,stream_id,sequence,effect_type,state,session_id,incarnation_id,harness_generation,submission_id,turn_id,outcome,created_at)
 VALUES(?,?,0,?,'pending',?,?,?,?,?,?,?) ON CONFLICT(effect_key) DO NOTHING`),
		key, p.StreamID, admittedToolSourceEffect, p.SessionID, p.IncarnationID,
		p.HarnessGeneration, p.SubmissionID, p.TurnID, string(raw), r.nowUTC())
	if err != nil {
		return err
	}
	var stored string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=?`), key).Scan(&stored); err != nil {
		return err
	}
	if stored != string(raw) {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}
