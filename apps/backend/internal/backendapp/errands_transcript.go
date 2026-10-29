package backendapp

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// errandTranscriptRepo is the transcript read the errands service needs. The
// task repository satisfies it.
type errandTranscriptRepo interface {
	ListTurnsBySession(ctx context.Context, sessionID string) ([]*models.Turn, error)
	ListMessagesByTurnID(ctx context.Context, turnID string) ([]*models.Message, error)
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

func (t errandTranscript) ReadTurn(ctx context.Context, sessionID, turnID string) (errands.TurnView, error) {
	turns, err := t.repo.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return errands.TurnView{}, err
	}
	for _, turn := range turns {
		if turn.ID == turnID {
			return t.view(ctx, turn)
		}
	}
	return errands.TurnView{}, nil
}

func (t errandTranscript) LatestTurn(ctx context.Context, sessionID string) (string, errands.TurnView, error) {
	turns, err := t.repo.ListTurnsBySession(ctx, sessionID)
	if err != nil || len(turns) == 0 {
		return "", errands.TurnView{}, err
	}
	latest := turns[len(turns)-1]
	view, err := t.view(ctx, latest)
	return latest.ID, view, err
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
		if message == nil || message.Metadata[orchestrator.ErrandDeliveryTokenMetadataKey] != token {
			continue
		}
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
		if submission == nil || (submission.State != models.DeliverySubmissionFailed &&
			submission.State != models.DeliverySubmissionCancelled) {
			return true, nil
		}
	}
	return false, nil
}

func (t errandTranscript) view(ctx context.Context, turn *models.Turn) (errands.TurnView, error) {
	messages, err := t.repo.ListMessagesByTurnID(ctx, turn.ID)
	if err != nil {
		return errands.TurnView{}, err
	}
	view := errands.TurnView{Completed: turn.CompletedAt != nil}
	for _, message := range messages {
		if message.AuthorType == models.MessageAuthorUser && view.EarliestUserSender == "" {
			view.EarliestUserSender, _ = message.Metadata["sender_session_id"].(string)
		}
		if message.AuthorType == models.MessageAuthorAgent && (message.Type == "" || message.Type == models.MessageTypeMessage) {
			view.LastAgentText = message.Content
		}
	}
	return view, nil
}
