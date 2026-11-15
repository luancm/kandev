package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Historical sources are reconstructed only from intact, admitted receipts.
// Enumerate coordinates in bounded pages and decode one event body at a time.
func (r *Repository) legacyAdmittedToolSourcesTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent) ([]admittedToolSource, error) {
	var sources []admittedToolSource
	var after int64
	for {
		var sequences []int64
		err := tx.SelectContext(ctx, &sequences, tx.Rebind(`SELECT i.sequence FROM agent_delivery_inbox i
 WHERE i.stream_id=? AND i.session_id=? AND i.incarnation_id=? AND i.harness_generation=?
 AND i.event_type=? AND i.submission_id<>'' AND i.sequence>? AND i.sequence<?
 AND NOT EXISTS (SELECT 1 FROM agent_delivery_effects e
 WHERE e.effect_key=? || i.stream_id || ':' || CAST(i.sequence AS TEXT))
 ORDER BY i.sequence LIMIT 128`), event.StreamID, event.SessionID, event.IncarnationID,
			event.HarnessGeneration, streams.EventTypeToolCall, after, event.Sequence, admittedToolSourceEffect+":")
		if err != nil {
			return nil, err
		}
		for _, sequence := range sequences {
			source, err := r.legacyAdmittedToolSourceTx(ctx, tx, event.StreamID, sequence)
			if err != nil {
				return nil, err
			}
			sources = append(sources, source)
			after = sequence
		}
		if len(sequences) < 128 {
			return sources, nil
		}
	}
}

func (r *Repository) legacyAdmittedToolSourceTx(ctx context.Context, tx *sqlx.Tx, stream string, sequence int64) (admittedToolSource, error) {
	var empty admittedToolSource
	event, _, found, err := r.findAgentDeliveryEventTx(ctx, tx, stream, sequence)
	if err != nil {
		return empty, err
	}
	if !found || event.PayloadPrunedAt != nil || len(event.Payload) == 0 {
		return empty, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	valid, err := sameAgentDeliveryEvent(&event, &event)
	if err != nil {
		return empty, err
	}
	if !valid {
		return empty, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	turn, errand, err := r.agentDeliveryEventAdmissionTx(ctx, tx, &event)
	if err != nil {
		return empty, err
	}
	source, err := admittedToolSourceTx(ctx, tx, &event)
	if err != nil {
		return empty, err
	}
	if !errand || source.ToolID == "" || turn == "" || turn != source.TurnID {
		return empty, repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return source, nil
}
