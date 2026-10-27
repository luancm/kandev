package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// ErrandMCPService is the errands.Service surface the errand MCP actions use.
type ErrandMCPService interface {
	Start(ctx context.Context, req errands.StartRequest) (*errands.StartResult, error)
	Await(ctx context.Context, req errands.AwaitRequest) (errands.AwaitResult, error)
	ReleaseAwait(ctx context.Context, token string) error
	Cancel(ctx context.Context, req errands.CancelRequest) (*errands.Snapshot, error)
	ParentOutstanding(ctx context.Context, parentSessionID string) (bool, error)
	List(ctx context.Context, taskID string) ([]errands.Snapshot, error)
}

// SetErrands wires the errands service and its release toggle. While the
// toggle is off the start, await, and cancel actions fail with
// feature_disabled; guards on other actions still apply to existing errands.
func (h *Handlers) SetErrands(service ErrandMCPService, enabled bool) {
	h.errands = service
	h.errandsEnabled = enabled
}

func (h *Handlers) registerErrandHandlers(d *guardedMCPDispatcher) {
	d.RegisterFunc(ws.ActionMCPStartErrand, h.handleStartErrand)
	d.RegisterFunc(ws.ActionMCPAwaitErrands, h.handleAwaitErrands)
	d.RegisterFunc(ws.ActionMCPAwaitErrandsRelease, h.handleAwaitErrandsRelease)
	d.RegisterFunc(ws.ActionMCPCancelErrand, h.handleCancelErrand)
}

type startErrandRequest struct {
	TaskID          string `json:"task_id"`
	Title           string `json:"title"`
	Prompt          string `json:"prompt"`
	AgentProfile    string `json:"agent_profile"`
	SenderTaskID    string `json:"sender_task_id"`
	SenderSessionID string `json:"sender_session_id"`
}

type awaitErrandsRequest struct {
	ErrandIDs       []string `json:"errand_ids"`
	Mode            string   `json:"mode"`
	MaxWaitSeconds  int      `json:"max_wait_seconds"`
	AwaitToken      string   `json:"await_token"`
	SenderTaskID    string   `json:"sender_task_id"`
	SenderSessionID string   `json:"sender_session_id"`
}

type cancelErrandRequest struct {
	ErrandID        string `json:"errand_id"`
	SenderTaskID    string `json:"sender_task_id"`
	SenderSessionID string `json:"sender_session_id"`
}

// awaitErrandView is one errand in an await response. Result and Truncated
// appear only for an undelivered result, DeliveredVia only for a delivered one.
type awaitErrandView struct {
	ErrandID     string `json:"errand_id"`
	Status       string `json:"status"`
	Result       string `json:"result,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
	DeliveredVia string `json:"delivered_via,omitempty"`
}

type awaitErrandsResponse struct {
	Errands []awaitErrandView `json:"errands"`
	Ended   bool              `json:"ended,omitempty"`
}

// handleStartErrand starts an errand for a verified parent session. The
// result returns at once; the errand's answer comes back as a message.
func (h *Handlers) handleStartErrand(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req startErrandRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if refusal := h.errandsDisabled(msg); refusal != nil {
		return refusal, nil
	}
	parent := h.verifiedSession(ctx, req.SenderTaskID, req.SenderSessionID)
	if parent == nil || parent.IsPassthrough || models.IsErrandSession(parent) || isConfigSession(parent) {
		return errandRefusal(msg, errands.ReasonNotSupported, "start_errand_kandev needs an ordinary agent session on this task"), nil
	}
	if refusal := h.errandTaskRefusal(ctx, msg, parent.TaskID); refusal != nil {
		return refusal, nil
	}
	result, err := h.errands.Start(ctx, errands.StartRequest{
		TaskID:          parent.TaskID,
		ParentSessionID: parent.ID,
		Title:           req.Title,
		Objective:       req.Prompt,
		Profile:         req.AgentProfile,
	})
	if err != nil {
		return errandServiceError(msg, err), nil
	}
	return ws.NewResponse(msg.ID, msg.Action, startErrandView(result))
}

func startErrandView(result *errands.StartResult) map[string]interface{} {
	view := map[string]interface{}{
		stopTaskStatusKey:    string(result.Status),
		"agent_profile_name": result.ProfileName,
	}
	if result.Errand != nil {
		view[errandIDKey] = result.Errand.ID
		view["session_id"] = result.Errand.ID
		view["agent_profile_id"] = result.Errand.AgentProfileID
	}
	return view
}

// handleAwaitErrands is an optional in-turn join on the parent's errands.
func (h *Handlers) handleAwaitErrands(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req awaitErrandsRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if refusal := h.errandsDisabled(msg); refusal != nil {
		return refusal, nil
	}
	parent := h.verifiedSession(ctx, req.SenderTaskID, req.SenderSessionID)
	if parent == nil || models.IsErrandSession(parent) {
		return errandRefusal(msg, errands.ReasonNotPermitted, "await_errands_kandev needs a parent session"), nil
	}
	result, err := h.errands.Await(ctx, errands.AwaitRequest{
		ParentSessionID: parent.ID,
		ErrandIDs:       req.ErrandIDs,
		Mode:            req.Mode,
		MaxWaitSeconds:  req.MaxWaitSeconds,
		Token:           req.AwaitToken,
	})
	if err != nil {
		return errandServiceError(msg, err), nil
	}
	return ws.NewResponse(msg.ID, msg.Action, awaitErrandsView(result))
}

func awaitErrandsView(result errands.AwaitResult) awaitErrandsResponse {
	views := make([]awaitErrandView, 0, len(result.Errands))
	for _, item := range result.Errands {
		views = append(views, awaitErrandView{
			ErrandID:     item.ErrandID,
			Status:       item.Status,
			Result:       item.Result,
			Truncated:    item.Truncated,
			DeliveredVia: item.DeliveredVia,
		})
	}
	return awaitErrandsResponse{Errands: views, Ended: result.Ended}
}

// handleAwaitErrandsRelease returns the claim of an abandoned await so its
// results are delivered with the parent's next turn.
func (h *Handlers) handleAwaitErrandsRelease(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req awaitErrandsRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if refusal := h.errandsDisabled(msg); refusal != nil {
		return refusal, nil
	}
	parent := h.verifiedSession(ctx, req.SenderTaskID, req.SenderSessionID)
	if parent == nil || models.IsErrandSession(parent) {
		return errandRefusal(msg, errands.ReasonNotPermitted, "only a parent session can release an await"), nil
	}
	if err := h.errands.ReleaseAwait(ctx, req.AwaitToken); err != nil {
		return errandServiceError(msg, err), nil
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"released": true})
}

// handleCancelErrand cancels one errand of the verified parent.
func (h *Handlers) handleCancelErrand(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req cancelErrandRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if refusal := h.errandsDisabled(msg); refusal != nil {
		return refusal, nil
	}
	parent := h.verifiedSession(ctx, req.SenderTaskID, req.SenderSessionID)
	if parent == nil || models.IsErrandSession(parent) {
		return errandRefusal(msg, errands.ReasonNotPermitted, "cancel_errand_kandev needs a parent session"), nil
	}
	snapshot, err := h.errands.Cancel(ctx, errands.CancelRequest{
		ErrandID:        req.ErrandID,
		TaskID:          parent.TaskID,
		ParentSessionID: parent.ID,
		Source:          models.ErrandCancelSourceParent,
	})
	if err != nil {
		return errandServiceError(msg, err), nil
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		errandIDKey:       req.ErrandID,
		stopTaskStatusKey: string(snapshot.Status),
	})
}

// errandsDisabled refuses an errand action while the release toggle is off or
// no errands service is wired.
func (h *Handlers) errandsDisabled(msg *ws.Message) *ws.Message {
	if h.errands != nil && h.errandsEnabled {
		return nil
	}
	return errandRefusal(msg, errands.ReasonFeatureDisabled, "errands are disabled")
}

// verifiedSession returns the caller's session only when it belongs to the
// named task. The session ID is server-injected; the task check keeps a
// forged or stale ID from acting as another task's parent.
func (h *Handlers) verifiedSession(ctx context.Context, taskID, sessionID string) *models.TaskSession {
	if sessionID == "" || taskID == "" || h.taskSvc == nil {
		return nil
	}
	session, err := h.taskSvc.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID {
		return nil
	}
	return session
}

// errandTaskRefusal refuses a start on a task that cannot delegate errands:
// Office, ephemeral, coordinator, and automation tasks.
func (h *Handlers) errandTaskRefusal(ctx context.Context, msg *ws.Message, taskID string) *ws.Message {
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return errandRefusal(msg, errands.ReasonNotSupported, "errands are not available for this task")
	}
	if task.IsFromOffice || task.IsEphemeral || task.Origin == models.TaskOriginCoordinator ||
		models.IsAutomationTaskOrigin(task.Origin) {
		return errandRefusal(msg, errands.ReasonNotSupported, "errands are not available for this task")
	}
	return nil
}

// errandCallerRefusal refuses a mutation whose verified sender is an errand.
// Only the session row's role is read: a stale ID can at worst refuse, never
// grant, so the check needs no task match.
func (h *Handlers) errandCallerRefusal(ctx context.Context, msg *ws.Message, senderSessionID, message string) *ws.Message {
	if senderSessionID == "" || h.taskSvc == nil {
		return nil
	}
	session, err := h.taskSvc.GetTaskSession(ctx, senderSessionID)
	if err != nil || !models.IsErrandSession(session) {
		return nil
	}
	return errandRefusal(msg, errands.ReasonNotPermitted, message)
}

// errandMessageRefusal refuses a message to an errand from any sender other
// than its parent session.
func (h *Handlers) errandMessageRefusal(ctx context.Context, msg *ws.Message, senderSessionID, targetSessionID string) *ws.Message {
	if targetSessionID == "" || h.taskSvc == nil {
		return nil
	}
	target, err := h.taskSvc.GetTaskSession(ctx, targetSessionID)
	if err != nil || !models.IsErrandSession(target) || target.ParentSessionID == senderSessionID {
		return nil
	}
	return errandRefusal(msg, errands.ReasonSenderNotParent, "only the errand's parent session can message this errand")
}

// errandOutstandingRefusal refuses a parent's step completion while its
// errands owe an outcome. It runs before any signal is recorded.
func (h *Handlers) errandOutstandingRefusal(ctx context.Context, msg *ws.Message, parentSessionID string) *ws.Message {
	if h.errands == nil || parentSessionID == "" {
		return nil
	}
	outstanding, err := h.errands.ParentOutstanding(ctx, parentSessionID)
	if err != nil {
		h.logger.Warn("failed to read errand state for step completion",
			zap.String("session_id", parentSessionID), zap.Error(err))
		resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to read errand state", nil)
		return resp
	}
	if !outstanding {
		return nil
	}
	return errandRefusal(msg, errands.ReasonErrandsOutstanding,
		"wait for your errands to report before completing this step")
}

// errandIDKey is the payload key carrying an errand's identifier.
const errandIDKey = "errand_id"

// errandRefusal renders a typed errand failure with its reason in details.
func errandRefusal(msg *ws.Message, reason errands.Reason, message string) *ws.Message {
	resp, _ := ws.NewError(msg.ID, msg.Action, errands.ErrorCode(reason), message,
		map[string]interface{}{mcpReasonKey: string(reason)})
	return resp
}

func errandServiceError(msg *ws.Message, err error) *ws.Message {
	if resp, ok := errands.WSError(msg.ID, msg.Action, err); ok {
		return resp
	}
	resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "errand request failed: "+err.Error(), nil)
	return resp
}

// isConfigSession reports whether a session runs in configuration mode.
func isConfigSession(session *models.TaskSession) bool {
	if session == nil || session.Metadata == nil {
		return false
	}
	configMode, ok := session.Metadata["config_mode"].(bool)
	return ok && configMode
}
