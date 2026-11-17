package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// LatestErrandPendingPermissions returns the newest current interactive permission per Errand.
func (r *Repository) LatestErrandPendingPermissions(ctx context.Context, errandIDs []string) (map[string]*models.Message, error) {
	result := make(map[string]*models.Message)
	for start := 0; start < len(errandIDs); start += 400 {
		end := min(start+400, len(errandIDs))
		if err := r.loadErrandPendingPermissions(ctx, errandIDs[start:end], result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *Repository) loadErrandPendingPermissions(ctx context.Context, ids []string, result map[string]*models.Message) error {
	typ := dialect.JSONExtract(r.ro.DriverName(), "m.payload", "type")
	status := dialect.JSONExtractPath(r.ro.DriverName(), "m.payload", "metadata", "status")
	presentation := dialect.JSONExtractPath(r.ro.DriverName(), "m.payload", "metadata", models.PermissionPresentationMetadataKey)
	query := fmt.Sprintf(`WITH ranked AS (
 SELECT m.errand_id,m.id,e.task_id,m.turn_id,m.payload,m.created_at,m.updated_at,
 COALESCE(%s,'') AS permission_status,COALESCE(%s,'') AS presentation,
 ROW_NUMBER() OVER (PARTITION BY m.errand_id ORDER BY m.created_at DESC,m.id DESC) AS rn
 FROM errand_messages m
 JOIN task_session_errands e ON e.id=m.errand_id
 JOIN errand_execution_generations g ON g.errand_id=m.errand_id AND g.generation=(SELECT MAX(g2.generation) FROM errand_execution_generations g2 WHERE g2.errand_id=g.errand_id)
 JOIN agent_delivery_effects effect ON effect.effect_key=('agent_delivery.errand_permission_owner.v1:' || m.id)
  AND effect.effect_type='agent_delivery.errand_permission_owner.v1' AND effect.session_id=('errand:' || m.errand_id) AND effect.turn_id=m.turn_id AND effect.submission_id=g.active_submission_id
 JOIN errand_harness_bindings b ON b.errand_id=g.errand_id AND b.owner_generation=g.generation AND b.execution_id=g.execution_id
  AND b.stream_id=effect.stream_id AND b.incarnation_id=effect.incarnation_id AND b.harness_generation=effect.harness_generation
 WHERE m.errand_id IN (?) AND e.lifecycle<>'cancelled' AND m.turn_id=g.active_turn_id AND g.active_submission_id<>'' AND %s='permission_request'
 ) SELECT errand_id,id,task_id,turn_id,payload,created_at,updated_at FROM ranked
 WHERE rn=1 AND permission_status IN ('','pending') AND presentation NOT IN ('automatic','unavailable')`, status, presentation, typ)
	query, args, err := sqlx.In(query, ids)
	if err != nil {
		return err
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var errandID, id, taskID, turnID, payload string
		var created, updated time.Time
		if err = rows.Scan(&errandID, &id, &taskID, &turnID, &payload, &created, &updated); err != nil {
			return err
		}
		var message models.Message
		decoder := json.NewDecoder(bytes.NewBufferString(payload))
		decoder.UseNumber()
		if err = decoder.Decode(&message); err != nil {
			return err
		}
		message.ID, message.TaskID, message.TurnID, message.TaskSessionID = id, taskID, turnID, ""
		message.CreatedAt, message.UpdatedAt = created, updated
		result[errandID] = &message
	}
	return rows.Err()
}
