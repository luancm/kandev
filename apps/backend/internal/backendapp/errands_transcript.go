package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// errandTranscriptRepo is the transcript read the errands service needs. The
// task repository satisfies it.
type errandTranscriptRepo interface {
	GetErrandTurn(context.Context, string, string) (*models.Turn, error)
	GetErrandLastAssistantMessage(context.Context, string, string) (*models.Message, error)
	ListMessages(ctx context.Context, sessionID string) ([]*models.Message, error)
	GetAgentDeliverySubmission(ctx context.Context, id string) (*models.AgentDeliverySubmission, error)
}

// errandTranscript serves errand turn evidence and delivery-token checks from
// stored turns and messages. It satisfies errands.TurnReader and
// errands.DeliveryLog.
type errandTranscript struct {
	repo errandTranscriptRepo
}

var (
	_ errands.TurnReader  = errandTranscript{}
	_ errands.DeliveryLog = errandTranscript{}
)

func (t errandTranscript) ReadTurn(ctx context.Context, errandID, turnID string) (errands.TurnView, error) {
	turn, err := t.repo.GetErrandTurn(ctx, errandID, turnID)
	if err != nil {
		return errands.TurnView{}, err
	}
	view := errands.TurnView{Completed: turn.CompletedAt != nil}
	message, err := t.repo.GetErrandLastAssistantMessage(ctx, errandID, turn.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errands.TurnView{}, err
	}
	if message != nil {
		view.LastAgentText = message.Content
	}
	return view, nil
}

func (t errandTranscript) HasDeliveryToken(ctx context.Context, parentSessionID, token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	messages, err := t.repo.ListMessages(ctx, parentSessionID)
	if err != nil {
		return false, err
	}
	for _, message := range messages {
		if message == nil {
			continue
		}
		matched, snapshot, err := errandTranscriptMessageToken(message, parentSessionID, token)
		if err != nil {
			return false, err
		}
		if !matched {
			continue
		}
		retained, err := t.deliveryRetained(ctx, message, snapshot)
		if err != nil || retained {
			return retained, err
		}
	}
	return false, nil
}

func errandTranscriptMessageToken(message *models.Message, parentSessionID, token string) (bool, errands.SendNowDelivery, error) {
	snapshot, present, err := errands.DecodeSendNowDelivery(message.Metadata)
	if err != nil {
		return false, errands.SendNowDelivery{}, err
	}
	if !present {
		return message.Metadata[orchestrator.ErrandDeliveryTokenMetadataKey] == token, snapshot, nil
	}
	if snapshot.TaskID != message.TaskID || snapshot.SessionID != parentSessionID || snapshot.SessionID != message.TaskSessionID {
		return false, errands.SendNowDelivery{}, fmt.Errorf("%w: transcript identity", errands.ErrInvalidSendNowDelivery)
	}
	return snapshot.HasToken(token), snapshot, nil
}

func (t errandTranscript) deliveryRetained(ctx context.Context, message *models.Message, snapshot errands.SendNowDelivery) (bool, error) {
	submissionID, _ := message.Metadata[orchestrator.ErrandDeliverySubmissionMetadataKey].(string)
	if submissionID == "" {
		return true, nil
	}
	submission, err := t.repo.GetAgentDeliverySubmission(ctx, submissionID)
	if errors.Is(err, repository.ErrAgentDeliverySubmissionNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if submission == nil {
		return true, nil
	}
	if snapshot.SessionID != "" && (snapshot.SessionID != submission.SessionID || snapshot.SessionIncarnationID != submission.IncarnationID) {
		return false, fmt.Errorf("%w: submission identity", errands.ErrInvalidSendNowDelivery)
	}
	return submission.State != models.DeliverySubmissionFailed && submission.State != models.DeliverySubmissionCancelled, nil
}
