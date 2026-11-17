package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type errandDurableAdmissionStore interface {
	LatestErrandHarnessSequence(context.Context, string, uint64) (uint64, error)
	AppendErrandHarnessBinding(context.Context, *models.ErrandHarnessBinding, uint64) error
	CompleteErrandHarnessNativeSession(context.Context, *models.ErrandHarnessBinding, string) error
	PrepareErrandDeliverySubmission(context.Context, *models.ErrandDeliveryAdmission) (bool, error)
}

func (sm *SessionManager) bindErrandDelivery(ctx context.Context, execution *AgentExecution) error {
	if execution == nil || execution.Owner.Kind != ExecutionOwnerErrand {
		return nil
	}
	if sm.streamManager == nil || execution.DeliveryHarnessGeneration == 0 || execution.DeliveryHarnessGeneration > math.MaxInt64 {
		return errors.New("errand durable delivery is unavailable")
	}
	store, ok := sm.streamManager.deliveryRepository().(errandDurableAdmissionStore)
	if !ok {
		return errors.New("errand durable admission is unavailable")
	}
	sequence, err := store.LatestErrandHarnessSequence(ctx, execution.Owner.ErrandID, execution.Owner.Generation)
	if err != nil {
		return err
	}
	binding := &models.ErrandHarnessBinding{ErrandID: execution.Owner.ErrandID, Generation: execution.Owner.Generation, ExecutionID: execution.ID, IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: int64(execution.DeliveryHarnessGeneration), StreamID: execution.DeliveryStreamID, NativeSessionID: execution.ACPSessionID}
	return store.AppendErrandHarnessBinding(ctx, binding, sequence)
}

func (sm *SessionManager) persistErrandDeliveryBeforeDispatch(ctx context.Context, execution *AgentExecution, prompt string, attachments []v1.MessageAttachment, submissionID string, promptGeneration uint64) (*models.AgentDeliverySubmission, error) {
	if execution.Owner.Kind != ExecutionOwnerErrand {
		return nil, nil
	}
	if sm.streamManager == nil || execution.DeliveryMode != DurableDeliveryV1 || execution.DeliveryHarnessGeneration == 0 || execution.DeliveryHarnessGeneration > math.MaxInt64 || submissionID == "" {
		return nil, errors.New("errand durable prompt admission is unavailable")
	}
	store, ok := sm.streamManager.deliveryRepository().(errandDurableAdmissionStore)
	if !ok {
		return nil, errors.New("errand durable prompt admission is unavailable")
	}
	payload, err := json.Marshal(struct {
		Text        string                 `json:"text"`
		Attachments []v1.MessageAttachment `json:"attachments,omitempty"`
	}{prompt, attachments})
	if err != nil {
		return nil, err
	}
	id := deliverySubmissionIdentityForPrompt(submissionID, "")
	submission := &models.AgentDeliverySubmission{ID: id, SessionID: models.ErrandExecutionIdentity(execution.Owner.ErrandID), IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: int64(execution.DeliveryHarnessGeneration), OwnerGeneration: int64(execution.DeliveryHarnessGeneration), DispatchAttemptID: execution.ID, Payload: payload, PayloadHash: journal.SubmissionHash(payload), State: models.DeliverySubmissionDispatching}
	admission := &models.ErrandDeliveryAdmission{ErrandID: execution.Owner.ErrandID, Generation: execution.Owner.Generation, ExecutionID: execution.ID, IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: int64(execution.DeliveryHarnessGeneration), StreamID: execution.DeliveryStreamID, NativeSessionID: execution.ACPSessionID, PromptGeneration: promptGeneration, TurnID: execution.promptTurnIDSnapshot(), Submission: submission}
	created, err := store.PrepareErrandDeliverySubmission(ctx, admission)
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, fmt.Errorf("errand prompt was already admitted")
	}
	return submission, nil
}

func errandDeliveryEventIsStale(ctx context.Context, execution *AgentExecution, event *models.AgentDeliveryEvent, repository AgentDeliveryRepository) (bool, error) {
	reader, ok := repository.(interface {
		ErrandHarnessBindingStale(context.Context, string, *models.ErrandHarnessBinding) (bool, error)
	})
	if !ok {
		return false, ErrDurableAdoptionIdentityMismatch
	}
	if execution.DeliveryMode != DurableDeliveryV1 || event.SessionID != models.ErrandExecutionIdentity(execution.Owner.ErrandID) || event.TaskID != execution.Owner.TaskID || event.ExecutionID != execution.ID || event.OwnerGeneration != event.HarnessGeneration || event.ExpectedStreamID != event.StreamID {
		return false, ErrDurableAdoptionIdentityMismatch
	}
	binding := &models.ErrandHarnessBinding{ErrandID: execution.Owner.ErrandID, Generation: execution.Owner.Generation, ExecutionID: event.ExecutionID, IncarnationID: event.IncarnationID, HarnessGeneration: event.HarnessGeneration, StreamID: event.StreamID}
	stale, err := reader.ErrandHarnessBindingStale(ctx, execution.Owner.TaskID, binding)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrDurableAdoptionIdentityMismatch, err)
	}
	return stale, nil
}

func (sm *SessionManager) completeErrandNativeSession(ctx context.Context, execution *AgentExecution) error {
	if execution == nil || execution.Owner.Kind != ExecutionOwnerErrand {
		return nil
	}
	if sm.streamManager == nil || execution.ACPSessionID == "" || execution.DeliveryHarnessGeneration == 0 || execution.DeliveryHarnessGeneration > math.MaxInt64 {
		return errors.New("errand native session identity unavailable")
	}
	store, ok := sm.streamManager.deliveryRepository().(errandDurableAdmissionStore)
	if !ok {
		return errors.New("errand durable admission is unavailable")
	}
	sequence, err := store.LatestErrandHarnessSequence(ctx, execution.Owner.ErrandID, execution.Owner.Generation)
	if err != nil {
		return err
	}
	binding := &models.ErrandHarnessBinding{ErrandID: execution.Owner.ErrandID, Generation: execution.Owner.Generation, Sequence: sequence, ExecutionID: execution.ID, IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: int64(execution.DeliveryHarnessGeneration), StreamID: execution.DeliveryStreamID}
	return store.CompleteErrandHarnessNativeSession(ctx, binding, execution.ACPSessionID)
}
