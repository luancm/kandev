package errands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	// SendNowDeliveryMetadataKey carries internal ownership, independently of the public display envelope.
	SendNowDeliveryMetadataKey = "errand_send_now"
	// SendNowErrandProvenance is stamped only on trusted server-owned errand sources.
	SendNowErrandProvenance = "errands"
)

// ErrInvalidSendNowDelivery blocks recovery from incomplete or contradictory ownership evidence.
var ErrInvalidSendNowDelivery = errors.New("invalid send-now errand delivery")

// SendNowSourceKind separates ordinary input from server-only components.
type SendNowSourceKind string

const (
	SendNowSourceOrdinary     SendNowSourceKind = "ordinary"
	SendNowSourceNotification SendNowSourceKind = "notification"
	SendNowSourcePrepared     SendNowSourceKind = "prepared"
)

// SendNowDelivery is the internal immutable dispatch snapshot stored in claim and transcript metadata.
// Its target identity applies to every source. QueueGeneration is the claim's session generation;
// each source separately retains its original task queue generation. Zero is a valid generation.
type SendNowDelivery struct {
	Version              int                      `json:"version"`
	TaskID               string                   `json:"task_id"`
	SessionID            string                   `json:"session_id"`
	SessionIncarnationID string                   `json:"session_incarnation_id"`
	OperationGeneration  int64                    `json:"operation_generation"`
	QueueGeneration      int64                    `json:"queue_generation"`
	SourceIDs            []string                 `json:"source_ids"`
	Sources              []SendNowDeliverySource  `json:"sources"`
	Receipts             []SendNowDeliveryReceipt `json:"receipts"`
	EffectivePrompt      string                   `json:"effective_prompt"`
}

// SendNowDeliverySource retains source order and provenance without folding prepared blocks into ordinary input.
// The durable queue claim also retains the complete original source, including attachments and mode semantics.
type SendNowDeliverySource struct {
	ID              string            `json:"id"`
	QueueGeneration int64             `json:"queue_generation"`
	Kind            SendNowSourceKind `json:"kind"`
	QueuedBy        string            `json:"queued_by"`
	Provenance      string            `json:"provenance,omitempty"`
	Content         string            `json:"content"`
	PreparedBlock   string            `json:"prepared_block,omitempty"`
}

// SendNowDeliveryReceipt owns exact result claims and their immutable display snapshot.
// SourceID binds a row-independent prepared notice; empty identifies results composed at dispatch.
type SendNowDeliveryReceipt struct {
	SourceID string                     `json:"source_id,omitempty"`
	Envelope DeliveryEnvelope           `json:"envelope"`
	Claims   []models.ErrandResultClaim `json:"claims"`
}

// DecodeSendNowDelivery distinguishes absent legacy data from invalid structured data.
// Invalid data returns present=true and no partial snapshot; callers must not fall back to a scalar token.
// Only trusted metadata is read. User content never establishes source provenance.
func DecodeSendNowDelivery(metadata map[string]interface{}) (SendNowDelivery, bool, error) {
	value, present := metadata[SendNowDeliveryMetadataKey]
	if !present {
		return SendNowDelivery{}, false, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return SendNowDelivery{}, true, invalidSendNowDelivery("metadata encoding")
	}
	var snapshot SendNowDelivery
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return SendNowDelivery{}, true, invalidSendNowDelivery("metadata shape")
	}
	if err := requireSendNowGenerations(encoded); err != nil {
		return SendNowDelivery{}, true, err
	}
	if err := snapshot.validate(); err != nil {
		return SendNowDelivery{}, true, err
	}
	if scalar, exists := metadata[DeliveryTokenMetadataKey]; exists {
		token, ok := scalar.(string)
		if !ok || (token != "" && !snapshot.HasToken(token)) {
			return SendNowDelivery{}, true, invalidSendNowDelivery("scalar token contradicts receipts")
		}
	}
	return snapshot, true, nil
}

// HasToken checks every receipt of a successfully decoded snapshot.
func (d SendNowDelivery) HasToken(token string) bool {
	for _, receipt := range d.Receipts {
		if token != "" && receipt.Envelope.Token == token {
			return true
		}
	}
	return false
}

func (d SendNowDelivery) validate() error {
	if !d.validIdentity() {
		return invalidSendNowDelivery("dispatch identity")
	}
	sources, err := d.validateSources()
	if err != nil {
		return err
	}
	tokens := make(map[string]bool, len(d.Receipts))
	prepared := make(map[string]bool)
	claims := make(map[models.ErrandResultClaim]bool)
	for _, receipt := range d.Receipts {
		if tokens[receipt.Envelope.Token] {
			return invalidSendNowDelivery("duplicate receipt token")
		}
		tokens[receipt.Envelope.Token] = true
		if receipt.SourceID != "" {
			if sources[receipt.SourceID].Kind != SendNowSourcePrepared || prepared[receipt.SourceID] ||
				receipt.Envelope.Route != models.ErrandDeliveryNotification {
				return invalidSendNowDelivery("prepared receipt source")
			}
			prepared[receipt.SourceID] = true
		}
		if err := validateSendNowReceipt(receipt, claims); err != nil {
			return err
		}
	}
	for id, source := range sources {
		if source.Kind == SendNowSourcePrepared && !prepared[id] {
			return invalidSendNowDelivery("missing prepared receipt")
		}
	}
	return nil
}

func (d SendNowDelivery) validIdentity() bool {
	return d.Version == 1 && d.TaskID != "" && d.SessionID != "" && d.SessionIncarnationID != "" &&
		d.QueueGeneration >= 0 && d.OperationGeneration == d.QueueGeneration
}

func (d SendNowDelivery) validateSources() (map[string]SendNowDeliverySource, error) {
	if len(d.Sources) == 0 || len(d.Sources) != len(d.SourceIDs) {
		return nil, invalidSendNowDelivery("source set")
	}
	sources := make(map[string]SendNowDeliverySource, len(d.Sources))
	hasOrdinary := false
	for i, source := range d.Sources {
		if source.ID == "" || source.ID != d.SourceIDs[i] || source.QueueGeneration < 0 {
			return nil, invalidSendNowDelivery("source identity")
		}
		if _, exists := sources[source.ID]; exists {
			return nil, invalidSendNowDelivery("duplicate source")
		}
		if err := validateSendNowSource(source); err != nil {
			return nil, err
		}
		if source.Kind == SendNowSourcePrepared && !strings.Contains(d.EffectivePrompt, source.PreparedBlock) {
			return nil, invalidSendNowDelivery("prepared block absent from dispatch")
		}
		hasOrdinary = hasOrdinary || source.Kind == SendNowSourceOrdinary
		sources[source.ID] = source
	}
	if !hasOrdinary && strings.TrimSpace(d.EffectivePrompt) == "" {
		return nil, invalidSendNowDelivery("empty dispatch without ordinary input")
	}
	return sources, nil
}

func validateSendNowSource(source SendNowDeliverySource) error {
	if source.QueuedBy == "" {
		return invalidSendNowDelivery("source owner")
	}
	if source.Kind == SendNowSourceOrdinary {
		if source.Provenance != "" || source.PreparedBlock != "" {
			return invalidSendNowDelivery("ordinary source provenance")
		}
		return nil
	}
	if source.QueuedBy != "server" || source.Provenance != SendNowErrandProvenance || source.Content != "" {
		return invalidSendNowDelivery("server source provenance")
	}
	switch source.Kind {
	case SendNowSourceNotification:
		if source.PreparedBlock != "" {
			return invalidSendNowDelivery("notification has prepared content")
		}
	case SendNowSourcePrepared:
		if strings.TrimSpace(source.PreparedBlock) == "" {
			return invalidSendNowDelivery("missing prepared block")
		}
	default:
		return invalidSendNowDelivery("source kind")
	}
	return nil
}

func validateSendNowReceipt(receipt SendNowDeliveryReceipt, claimed map[models.ErrandResultClaim]bool) error {
	envelope := receipt.Envelope
	if envelope.Token == "" || (envelope.Route != models.ErrandDeliveryNotification && envelope.Route != models.ErrandDeliveryTurnContext) ||
		len(envelope.Items) == 0 || len(receipt.Claims) == 0 {
		return invalidSendNowDelivery("receipt envelope")
	}
	items := make(map[string]models.ErrandStatus, len(envelope.Items))
	for _, item := range envelope.Items {
		if item.ErrandID == "" || items[item.ErrandID] != "" || !validSendNowStatus(item.Status) {
			return invalidSendNowDelivery("receipt items")
		}
		items[item.ErrandID] = item.Status
	}
	for _, claim := range receipt.Claims {
		if !validSendNowClaim(claim, envelope, items[claim.ErrandID]) {
			return invalidSendNowDelivery("result claim identity")
		}
		identity := claim
		identity.Token, identity.Route = "", ""
		if claimed[identity] {
			return invalidSendNowDelivery("duplicate result claim")
		}
		claimed[identity] = true
	}
	return nil
}

func validSendNowClaim(claim models.ErrandResultClaim, envelope DeliveryEnvelope, status models.ErrandStatus) bool {
	return claim.ErrandID != "" && claim.ResultRevision != "" && claim.ParentAdmissionGeneration >= 0 &&
		claim.Token == envelope.Token && claim.Route == envelope.Route && terminalSendNowStatus(status)
}

func validSendNowStatus(status models.ErrandStatus) bool {
	return terminalSendNowStatus(status) || status == models.ErrandStatusQueued ||
		status == models.ErrandStatusRunning || status == models.ErrandStatusNeedsAttention
}

func terminalSendNowStatus(status models.ErrandStatus) bool {
	return status == models.ErrandStatusDone || status == models.ErrandStatusFailed || status == models.ErrandStatusCancelled
}

// Zero generations and empty ordinary prompts are valid; omitted or null fields are missing evidence.
func requireSendNowGenerations(encoded []byte) error {
	var fields struct {
		EffectivePrompt *string `json:"effective_prompt"`
		Operation       *int64  `json:"operation_generation"`
		Queue           *int64  `json:"queue_generation"`
		Sources         []struct {
			Queue *int64 `json:"queue_generation"`
		} `json:"sources"`
		Receipts []struct {
			Claims []struct {
				Generation *int64 `json:"ParentAdmissionGeneration"`
			} `json:"claims"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(encoded, &fields); err != nil || fields.Operation == nil || fields.Queue == nil {
		return invalidSendNowDelivery("missing dispatch generation")
	}
	if fields.EffectivePrompt == nil {
		return invalidSendNowDelivery("missing effective prompt")
	}
	for _, source := range fields.Sources {
		if source.Queue == nil {
			return invalidSendNowDelivery("missing source generation")
		}
	}
	for _, receipt := range fields.Receipts {
		for _, claim := range receipt.Claims {
			if claim.Generation == nil {
				return invalidSendNowDelivery("missing admission generation")
			}
		}
	}
	return nil
}

func invalidSendNowDelivery(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidSendNowDelivery, reason)
}
