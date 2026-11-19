package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ResolveErrandPermissionOrigin reads immutable admission evidence for callbacks.
func (r *Repository) ResolveErrandPermissionOrigin(ctx context.Context, event *models.AgentDeliveryEvent) (*models.ErrandPermissionOrigin, error) {
	if event == nil || event.EventType != streams.EventTypePermissionRequest || event.SubmissionID == "" {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	errID, ok := errandDeliveryIdentity(event.SessionID)
	if !ok || errID == "" {
		return nil, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	turn, err := r.errandDeliveryEventAdmissionTx(ctx, tx, errID, event)
	if err != nil {
		return nil, err
	}
	var raw string
	if err = tx.GetContext(ctx, &raw, tx.Rebind(`SELECT outcome FROM agent_delivery_effects WHERE effect_key=? AND effect_type=?`), errandPromptAdmissionEffect+":"+event.SubmissionID, errandPromptAdmissionEffect); err != nil {
		return nil, err
	}
	var proof errandPromptProof
	if err = json.Unmarshal([]byte(raw), &proof); err != nil {
		return nil, err
	}
	return &models.ErrandPermissionOrigin{SubmissionID: proof.SubmissionID, PromptGeneration: proof.PromptGeneration, TurnID: turn}, nil
}
