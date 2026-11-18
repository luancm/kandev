package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

type ownedErrandContinuationStore interface {
	ownedErrandLaunchStore
	PrepareErrandContinuation(context.Context, models.ErrandContinuation) (*models.PreparedErrandContinuation, error)
	DiscardUnallocatedErrandContinuation(context.Context, models.ErrandContinuation, uint64) error
}

// ContinueErrand starts the next explicit user instruction in the retained
// provider conversation after the preceding runtime has confirmed close.
func (a *ErrandAdapter) ContinueErrand(ctx context.Context, request errands.ContinuationRequest) (string, error) {
	if !request.Valid() {
		return "", errands.ErrInvalidRequest
	}
	if err := a.svc.authorizeTaskPrompt(ctx, request.TaskID); err != nil {
		return "", models.ErrErrandNotFound
	}
	store, ok := a.svc.repo.(ownedErrandContinuationStore)
	if !ok || a.runtime == nil {
		return "", errors.New("errand continuation is unavailable")
	}
	guard, err := a.svc.lockCancelInFlightGuardWithContext(ctx, "errand-launch:"+request.ErrandID)
	if err != nil {
		return "", err
	}
	defer guard.release()
	row, err := store.GetErrand(ctx, request.ErrandID)
	if err != nil || row == nil || row.TaskID != request.TaskID {
		return "", models.ErrErrandNotFound
	}
	decision := a.svc.sessionCeiling.admit(ctx, admissionRequest{
		taskID: request.TaskID, sessionID: models.ErrandExecutionIdentity(request.ErrandID),
		origin: launchOriginManual, seam: "errand_continue",
	})
	if !decision.admitted {
		return "", errands.ErrCeilingRefused
	}
	if a.svc.sessionCeiling != nil {
		defer a.svc.sessionCeiling.release(decision.reservationKey)
	}
	return a.continueOwnedErrand(ctx, store, row, request)
}

func (a *ErrandAdapter) continueOwnedErrand(ctx context.Context, store ownedErrandContinuationStore, row *models.Errand, request errands.ContinuationRequest) (string, error) {
	author, _ := authn.IdentityFromContext(ctx)
	identity := []byte(row.ID + "\x00" + request.ClientMessageID)
	turnID := uuid.NewSHA1(uuid.NameSpaceOID, identity).String()
	submissionID := "prompt:errand:" + uuid.NewSHA1(uuid.NameSpaceURL, identity).String()
	instruction := models.ErrandContinuation{
		TaskID: row.TaskID, ErrandID: row.ID, TurnID: turnID, SubmissionID: submissionID,
		Content: request.Content, AuthorID: author.UserID,
	}
	prepared, err := store.PrepareErrandContinuation(ctx, instruction)
	if errors.Is(err, models.ErrInvalidErrand) {
		return "", errands.ErrInvalidRequest
	}
	if err != nil {
		return "", fmt.Errorf("prepare errand continuation: %w", err)
	}
	if prepared.Accepted {
		return turnID, nil
	}
	if prepared.ExecutionID != "" {
		return "", db.ErrErrandSettlementPending
	}
	launch := errands.LaunchRequest{
		TaskID: row.TaskID, ErrandID: row.ID, ParentSessionID: row.ParentSessionID,
		Title: row.Title, Objective: request.Content, AgentProfileID: row.AgentProfileID,
		TurnID: turnID, SubmissionID: submissionID,
		MCPServerIDs: row.MCPServerIDs, MCPExcludedServerIDs: row.MCPExcludedServerIDs,
	}
	if err := a.launchOwnedErrandContinuation(ctx, store, launch, prepared); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if cleanupErr := store.DiscardUnallocatedErrandContinuation(cleanupCtx, instruction, prepared.Generation); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
		return "", fmt.Errorf("launch errand continuation: %w", err)
	}
	if a.ownedStatusChanged != nil {
		a.ownedStatusChanged(ctx, row.TaskID, row.ID)
	}
	return turnID, nil
}

func (a *ErrandAdapter) launchOwnedErrandContinuation(ctx context.Context, store ownedErrandContinuationStore, req errands.LaunchRequest, prepared *models.PreparedErrandContinuation) error {
	info, err := a.ownedErrandWorkspace(ctx, req)
	if err != nil {
		return err
	}
	attachment, err := a.loadOwnedErrandWorkspaceAttachment(ctx, info)
	if err != nil {
		return err
	}
	launch := ownedErrandLaunchRequest(req, info, attachment)
	launch.RequiredNativeConversationID = prepared.NativeID
	launch.ErrandPredecessor = &prepared.Predecessor
	return a.startOwnedErrandExecution(ctx, store, req, launch, attachment, prepared.Generation)
}
