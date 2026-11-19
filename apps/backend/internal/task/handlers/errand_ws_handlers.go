package handlers

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// ErrandService is the errands.Service surface the WebSocket requests use.
type ErrandService interface {
	List(ctx context.Context, taskID string) ([]errands.Snapshot, error)
	BrowserSummary(ctx context.Context, taskID string) (models.ErrandBrowserSummary, error)
	HistoryPage(ctx context.Context, request models.ErrandHistoryPageRequest, cursor string) (models.ErrandHistoryPage, string, error)
	Inspect(ctx context.Context, taskID, childSessionID string) (models.ErrandBrowserInspection, error)
	ReadErrandMessages(ctx context.Context, taskID, errandID, beforeMessageID string, limit int) ([]*models.Message, bool, error)
	MarkErrandRead(ctx context.Context, taskID, errandID, messageID string) error
	Lookup(ctx context.Context, errandID string) (*models.Errand, error)
	Cancel(ctx context.Context, req errands.CancelRequest) (*errands.Snapshot, error)
	CancelAll(ctx context.Context, taskID string) (int, error)
	SendResult(ctx context.Context, errandID string) (*errands.Snapshot, error)
}

// SetErrands wires the errands service and its release toggle. While the
// toggle is off every errand request fails with feature_disabled.
func (h *TaskHandlers) SetErrands(service ErrandService, enabled bool) {
	h.errands = service
	h.errandsEnabled = enabled
}

// SetErrandPermissionResponder wires action approval for owned executions.
func (h *TaskHandlers) SetErrandPermissionResponder(respond func(context.Context, errands.PermissionResponseRequest) error) {
	h.respondToErrandPermission = respond
}

func (h *TaskHandlers) SetErrandContinuation(continuation func(context.Context, errands.ContinuationRequest) (string, error)) {
	h.continueErrand = continuation
}

func (h *TaskHandlers) registerErrandWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionErrandList, h.wsListErrands)
	dispatcher.RegisterFunc(ws.ActionErrandBrowserSummary, h.wsErrandBrowserSummary)
	dispatcher.RegisterFunc(ws.ActionErrandHistoryPage, h.wsErrandHistoryPage)
	dispatcher.RegisterFunc(ws.ActionErrandInspect, h.wsErrandInspect)
	dispatcher.RegisterFunc(ws.ActionErrandMessages, h.wsErrandMessages)
	dispatcher.RegisterFunc(ws.ActionErrandMarkRead, h.wsMarkErrandRead)
	dispatcher.RegisterFunc(ws.ActionErrandContinue, h.wsContinueErrand)
	dispatcher.RegisterFunc(ws.ActionErrandPermissionRespond, h.wsRespondToErrandPermission)
	dispatcher.RegisterFunc(ws.ActionErrandCancel, h.wsCancelErrand)
	dispatcher.RegisterFunc(ws.ActionErrandCancelAll, h.wsCancelAllErrands)
	dispatcher.RegisterFunc(ws.ActionErrandSendResult, h.wsSendErrandResult)
}

func (h *TaskHandlers) wsContinueErrand(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.errandGuard(msg); refused != nil {
		return refused, nil
	}
	var request errands.ContinuationRequest
	if err := msg.ParsePayload(&request); err != nil || !request.Valid() || len(request.Content) > maxMessageContentBytes {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid errand continuation", nil)
	}
	if err := h.authorizeErrandTask(ctx, request.TaskID, authz.ScopeSessionPrompt); err != nil {
		return h.errandFailure(msg, models.ErrErrandNotFound)
	}
	if h.continueErrand == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnavailable, "errand continuation is unavailable", nil)
	}
	turnID, err := h.continueErrand(ctx, request)
	if err != nil {
		if _, classified := errands.ReasonOf(err); !classified && !errors.Is(err, errands.ErrInvalidRequest) && !errors.Is(err, models.ErrErrandNotFound) {
			h.logger.Warn("errand continuation failed", zap.String("errand_id", request.ErrandID), zap.Error(err))
		}
		return h.errandFailure(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, struct {
		TurnID string `json:"turn_id"`
	}{TurnID: turnID})
}

func (h *TaskHandlers) wsRespondToErrandPermission(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.errandGuard(msg); refused != nil {
		return refused, nil
	}
	var request errands.PermissionResponseRequest
	if err := msg.ParsePayload(&request); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid permission response payload", nil)
	}
	if !request.Valid() {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "Invalid permission response identity", nil)
	}
	if err := h.authorizeErrandTask(ctx, request.TaskID, authz.ScopeSessionControl); err != nil {
		return h.errandFailure(msg, models.ErrErrandNotFound)
	}
	if h.respondToErrandPermission == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnavailable, "errand permissions are unavailable", nil)
	}
	if err := h.respondToErrandPermission(ctx, request); err != nil {
		return h.errandPermissionFailure(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, struct {
		Success bool `json:"success"`
	}{Success: true})
}

func (h *TaskHandlers) errandPermissionFailure(msg *ws.Message, err error) (*ws.Message, error) {
	for _, unavailable := range []error{orchestrator.ErrPermissionNotFound, orchestrator.ErrPermissionStale, orchestrator.ErrPermissionAlreadyResolved} {
		if errors.Is(err, unavailable) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, unavailable.Error(), nil)
		}
	}
	if errors.Is(err, orchestrator.ErrPermissionResolutionInProgress) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, orchestrator.ErrPermissionResolutionInProgress.Error(), nil)
	}
	return h.errandFailure(msg, err)
}

type errandListResponse struct {
	Errands []dto.ErrandDTO `json:"errands"`
}

type errandCancelResponse struct {
	Errand dto.ErrandDTO `json:"errand"`
}

type errandCancelAllResponse struct {
	Cancelled int `json:"cancelled"`
}

const (
	errandTaskNotFound    = "task not found"
	errandDetailReason    = "reason"
	errandDetailCancelled = "cancelled"
)

// errandGuard refuses a request while the feature is off or unwired.
func (h *TaskHandlers) errandGuard(msg *ws.Message) *ws.Message {
	if !h.errandsEnabled {
		return errandError(msg, errands.ReasonFeatureDisabled, "errands are disabled")
	}
	if h.errands == nil {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "errands are unavailable", nil)
		return response
	}
	return nil
}

// browserErrandGuard wires retained inspection without applying the mutation gate.
func (h *TaskHandlers) browserErrandGuard(msg *ws.Message) *ws.Message {
	if h.errands == nil {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "errands are unavailable", nil)
		return response
	}
	return nil
}

func errandError(msg *ws.Message, reason errands.Reason, message string) *ws.Message {
	response, _ := ws.NewError(msg.ID, msg.Action, errands.ErrorCode(reason), message,
		map[string]interface{}{errandDetailReason: string(reason)})
	return response
}

// authorizeErrandTask checks the caller's reach into a task. A denial and a
// missing task are indistinguishable to the caller.
func (h *TaskHandlers) authorizeErrandTask(ctx context.Context, taskID string, scope authz.Scope) error {
	return h.service.AuthorizeTaskScope(ctx, taskID, scope)
}

func (h *TaskHandlers) wsListErrands(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.errandGuard(msg); refused != nil {
		return refused, nil
	}
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if err := h.authorizeErrandTask(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, errandTaskNotFound, nil)
	}
	snapshots, err := h.errands.List(ctx, taskID)
	if err != nil {
		return h.errandFailure(msg, err)
	}
	items := make([]dto.ErrandDTO, 0, len(snapshots))
	for _, snapshot := range snapshots {
		items = append(items, dto.FromErrand(snapshot.Errand, snapshot.Status, snapshot.AttentionReason))
	}
	return ws.NewResponse(msg.ID, msg.Action, errandListResponse{Errands: items})
}

func (h *TaskHandlers) wsErrandBrowserSummary(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.browserErrandGuard(msg); refused != nil {
		return refused, nil
	}
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if err := h.authorizeErrandTask(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, errandTaskNotFound, nil)
	}
	summary, err := h.errands.BrowserSummary(ctx, taskID)
	if err != nil {
		return h.errandFailure(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromErrandBrowserSummary(summary))
}

func (h *TaskHandlers) wsErrandHistoryPage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.browserErrandGuard(msg); refused != nil {
		return refused, nil
	}
	var req struct {
		TaskID          string `json:"task_id"`
		ParentSessionID string `json:"parent_session_id"`
		Limit           int    `json:"limit"`
		Cursor          string `json:"cursor"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	req.TaskID, req.ParentSessionID = strings.TrimSpace(req.TaskID), strings.TrimSpace(req.ParentSessionID)
	if req.TaskID == "" || req.ParentSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id and parent_session_id are required", nil)
	}
	if err := h.authorizeErrandTask(ctx, req.TaskID, authz.ScopeWorkspaceRead); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, errandTaskNotFound, nil)
	}
	page, nextCursor, err := h.errands.HistoryPage(ctx, models.ErrandHistoryPageRequest{
		TaskID: req.TaskID, ParentSessionID: req.ParentSessionID, Limit: req.Limit,
	}, req.Cursor)
	if err != nil {
		return h.errandFailure(msg, err)
	}
	rows := make([]dto.ErrandBrowserRowDTO, 0, len(page.Errands))
	for _, row := range page.Errands {
		rows = append(rows, dto.FromErrandBrowserObservation(row))
	}
	var next *string
	if nextCursor != "" {
		next = &nextCursor
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.ErrandHistoryPageDTO{
		Scope:   dto.ErrandBrowserScopeDTO{TaskID: req.TaskID, ParentSessionID: req.ParentSessionID},
		Errands: rows, TotalFinished: page.TotalFinished, HasMore: page.HasMore, NextCursor: next,
	})
}

func (h *TaskHandlers) wsErrandInspect(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.browserErrandGuard(msg); refused != nil {
		return refused, nil
	}
	var req struct {
		TaskID         string `json:"task_id"`
		ChildSessionID string `json:"child_session_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	req.TaskID, req.ChildSessionID = strings.TrimSpace(req.TaskID), strings.TrimSpace(req.ChildSessionID)
	if req.TaskID == "" || req.ChildSessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id and child_session_id are required", nil)
	}
	if err := h.authorizeErrandTask(ctx, req.TaskID, authz.ScopeWorkspaceRead); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, errandTaskNotFound, nil)
	}
	inspection, err := h.errands.Inspect(ctx, req.TaskID, req.ChildSessionID)
	if err != nil {
		return h.errandFailure(msg, err)
	}
	parent := dto.ErrandBrowserParentDTO{
		Available: inspection.Parent.Available, ID: inspection.Parent.ID, Title: inspection.Parent.Title,
		Role: inspection.Parent.Role, EnvironmentID: inspection.Parent.EnvironmentID,
		TaskEnvironmentID: inspection.Parent.TaskEnvironmentID,
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.ErrandInspectionDTO{
		Scope:            dto.ErrandBrowserScopeDTO{TaskID: req.TaskID, ChildSessionID: req.ChildSessionID},
		Errand:           dto.FromErrandBrowserObservation(inspection.Errand),
		OwnsConversation: inspection.OwnsConversation,
		Parent:           parent,
	})
}

// wsCancelErrand stops one errand for the user. The request names the errand
// by its session ID; the task is read from the errand itself and authorized
// before anything changes, and the errand must belong to that task.
func (h *TaskHandlers) wsCancelErrand(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.errandGuard(msg); refused != nil {
		return refused, nil
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	errandID := strings.TrimSpace(req.SessionID)
	if errandID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	errand, err := h.errands.Lookup(ctx, errandID)
	if err != nil {
		return h.errandFailure(msg, err)
	}
	if err := h.authorizeErrandTask(ctx, errand.TaskID, authz.ScopeSessionControl); err != nil {
		return errandError(msg, errands.ReasonNotFound, "errand not found"), nil
	}
	snapshot, err := h.errands.Cancel(ctx, errands.CancelRequest{
		ErrandID: errand.ID, TaskID: errand.TaskID, Source: models.ErrandCancelSourceUser,
	})
	if err != nil {
		return h.errandFailure(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, errandCancelResponse{
		Errand: dto.FromErrand(snapshot.Errand, snapshot.Status, snapshot.AttentionReason),
	})
}

// wsSendErrandResult records the latest settled turn of an errand as its
// result, when the send-result rule allows it.
func (h *TaskHandlers) wsSendErrandResult(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.errandGuard(msg); refused != nil {
		return refused, nil
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	errandID := strings.TrimSpace(req.SessionID)
	if errandID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	errand, err := h.errands.Lookup(ctx, errandID)
	if err != nil {
		return h.errandFailure(msg, err)
	}
	if err := h.authorizeErrandTask(ctx, errand.TaskID, authz.ScopeSessionControl); err != nil {
		return errandError(msg, errands.ReasonNotFound, "errand not found"), nil
	}
	snapshot, err := h.errands.SendResult(ctx, errand.ID)
	if err != nil {
		return h.errandFailure(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, errandCancelResponse{
		Errand: dto.FromErrand(snapshot.Errand, snapshot.Status, snapshot.AttentionReason),
	})
}

func (h *TaskHandlers) wsCancelAllErrands(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if refused := h.errandGuard(msg); refused != nil {
		return refused, nil
	}
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if err := h.authorizeErrandTask(ctx, taskID, authz.ScopeSessionControl); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, errandTaskNotFound, nil)
	}
	cancelled, err := h.errands.CancelAll(ctx, taskID)
	if err != nil {
		h.logger.Warn("stop all errands left some runtimes running",
			zap.String("task_id", taskID), zap.Int("cancelled", cancelled), zap.Error(err))
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"some errands could not be stopped", map[string]interface{}{errandDetailCancelled: cancelled})
		return response, nil
	}
	return ws.NewResponse(msg.ID, msg.Action, errandCancelAllResponse{Cancelled: cancelled})
}

// errandFailure renders typed errand failures with their reason and details;
// anything else is an internal error without internals.
func (h *TaskHandlers) errandFailure(msg *ws.Message, err error) (*ws.Message, error) {
	if response, handled := errands.WSError(msg.ID, msg.Action, err); handled {
		return response, nil
	}
	if errors.Is(err, models.ErrErrandNotFound) {
		return errandError(msg, errands.ReasonNotFound, "errand not found"), nil
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "errand request failed", nil)
}
