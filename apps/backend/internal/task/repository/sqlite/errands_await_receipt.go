package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const (
	errandAwaitOfferedState   = "offered"
	errandAwaitReceivedState  = "received"
	errandAwaitAbortedState   = "aborted"
	errandAwaitUncertainState = "uncertain"
)

// This event field is produced only by native adapters. Plugin ingress builds
// its own typed event and cannot copy arbitrary incoming JSON into it.
type awaitObservation struct {
	Type             string                      `json:"type"`
	ToolName         string                      `json:"tool_name"`
	ToolCallID       string                      `json:"tool_call_id"`
	ParentToolCallID string                      `json:"parent_tool_call_id"`
	ToolStatus       string                      `json:"tool_status"`
	Receipt          *streams.ErrandAwaitReceipt `json:"errand_await_receipt"`
}

func readAwaitObservation(event *models.AgentDeliveryEvent) *awaitObservation {
	var field struct {
		Receipt json.RawMessage `json:"errand_await_receipt"`
	}
	if json.Unmarshal(event.Payload, &field) != nil || len(field.Receipt) == 0 || string(field.Receipt) == jsonNull {
		return nil
	}
	var observation awaitObservation
	if json.Unmarshal(event.Payload, &observation) != nil {
		return &awaitObservation{}
	}
	return &observation
}

func (r *Repository) receiveErrandAwaitReceiptTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent) error {
	observed := readAwaitObservation(event)
	if observed == nil {
		return nil
	}
	if !hasValidAgentDeliveryEventOwnerIdentity(event) || event.EventType != streams.EventTypeToolUpdate || observed.Type != event.EventType || observed.ToolName != streams.ErrandAwaitToolName || observed.ToolCallID == "" || len(observed.ToolCallID) > 4096 || observed.ParentToolCallID != "" || (observed.ToolStatus != shellToolStatusComplete && observed.ToolStatus != shellToolStatusCompleted) || !observed.Receipt.Valid() {
		r.diagnoseAwaitReceipt(event, "unsupported or incomplete observation")
		return nil
	}
	return r.reduceErrandAwaitReceiptTx(ctx, tx, event, observed)
}

func (r *Repository) reduceErrandAwaitReceiptTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, observed *awaitObservation) error {
	var task, parent, incarnation, state string
	var generation int64
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id,parent_session_id,parent_incarnation_id,harness_generation,state FROM errand_await_attempt WHERE token=?`), observed.Receipt.Token).
		Scan(&task, &parent, &incarnation, &generation, &state)
	if errors.Is(err, sql.ErrNoRows) {
		r.diagnoseAwaitReceipt(event, "unknown attempt")
		return nil
	}
	if err != nil {
		return err
	}
	if task != event.TaskID || parent != event.SessionID || incarnation != event.IncarnationID || generation != event.HarnessGeneration {
		r.diagnoseAwaitReceipt(event, "attempt owner mismatch")
		return nil
	}
	items := make([]models.ErrandAwaitDeliveryItem, 0, len(observed.Receipt.Items))
	for _, item := range observed.Receipt.Items {
		items = append(items, models.ErrandAwaitDeliveryItem{ErrandID: item.ErrandID, ResultRevision: item.ResultRevision})
	}
	if err := matchErrandAwaitItems(ctx, tx, observed.Receipt.Token, items); err != nil {
		if !errors.Is(err, models.ErrErrandVersionConflict) {
			return err
		}
		r.diagnoseAwaitReceipt(event, "attempt item set mismatch")
		return nil
	}
	if !awaitReceiptCanSettle(state) {
		r.diagnoseAwaitReceipt(event, "attempt was not offered")
		return nil
	}
	if state == errandAwaitReceivedState {
		return nil
	}
	if state == errandAwaitAbortedState {
		r.diagnoseAwaitReceipt(event, "positive receipt contradicts terminal abort")
	} else {
		state = errandAwaitReceivedState
	}
	// Preserve the first proof even for an aborted contradiction. Current result
	// rows are deliberately untouched: their revision or admission may be newer.
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE errand_await_attempt SET state=?,receipt_stream_id=?,receipt_sequence=?,receipt_tool_call_id=?,receipt_payload_hash=?,receipt_hash_version=?,received_at=?,updated_at=? WHERE token=? AND receipt_sequence=0`),
		state, event.StreamID, event.Sequence, observed.ToolCallID, event.ContentHash, event.HashVersion, event.ReceivedAt, event.ReceivedAt, observed.Receipt.Token)
	return err
}

func (r *Repository) diagnoseAwaitReceipt(event *models.AgentDeliveryEvent, reason string) {
	if r.log != nil {
		r.log.Warn("errand await receipt ignored", zap.String("reason", reason), zap.String("stream_id", event.StreamID), zap.Int64("sequence", event.Sequence))
	}
}

func awaitReceiptCanSettle(state string) bool {
	switch state {
	case errandAwaitOfferedState, errandAwaitUncertainState, errandAwaitReceivedState, errandAwaitAbortedState:
		return true
	default:
		return false
	}
}
