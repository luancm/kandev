package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type moveTaskRequest struct {
	TaskID          string                     `json:"task_id"`
	WorkflowID      string                     `json:"workflow_id"`
	WorkflowStepID  string                     `json:"workflow_step_id"`
	Position        int                        `json:"position"`
	Prompt          string                     `json:"prompt"`
	SenderSessionID string                     `json:"sender_session_id"`
	EntryOptions    *workflowmove.EntryOptions `json:"entry_options,omitempty"`
}

// moveTaskMCPResponse keeps the historical nested task object while exposing
// the authoritative task DTO fields at the top level for agent compatibility.
// The source task is authoritative for deferred moves; the destination task
// is authoritative for committed moves.
type moveTaskMCPResponse struct {
	dto.TaskDTO
	Task         dto.TaskDTO                `json:"task"`
	MoveID       string                     `json:"move_id,omitempty"`
	Disposition  string                     `json:"disposition,omitempty"`
	EntryOptions *workflowmove.EntryOptions `json:"entry_options,omitempty"`
}

func (h *Handlers) handleMoveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req moveTaskRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}
	if req.WorkflowStepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_step_id is required", nil)
	}
	entryOptions, err := workflowmove.NormalizeEntryOptions(req.EntryOptions, req.Prompt)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entry_options.instructions conflicts with prompt", nil)
	}
	req.EntryOptions = entryOptions
	req.Prompt = ""

	// Prompt is OPTIONAL — config-mode/admin moves don't always have an agent
	// to hand off to. When supplied, it activates the deferred-move path that
	// hands the receiving agent a directive on its first turn at the new step.
	// When omitted, we just move the task and return.
	session, lookupErr := h.lookupSession(ctx, req.TaskID)
	if lookupErr != nil {
		// Backend lookup failure is an internal error, not validation — don't
		// collapse it into "you have no session" downstream.
		h.logger.Error("move_task: failed to look up primary session",
			zap.String("task_id", req.TaskID), zap.Error(lookupErr))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to look up task's primary session", nil)
	}

	// Active source session → deferred path. Running MoveTask immediately would
	// fail validateMoveSessions ("task has an active session (RUNNING)") and,
	// if it somehow succeeded, would race on_enter processing against the
	// agent's still-active turn. Defer until handleAgentReady fires
	// applyPendingMove on turn-end. Prompt is optional: omit it for simple
	// self-moves (e.g. Work → Done); include it for cross-agent hand-offs.
	if session != nil &&
		(session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting) {
		return h.deferMoveTask(ctx, msg, req, session)
	}

	// Idle path — apply immediately. If a prompt was supplied, queue it on the
	// session so the receiving agent's next turn picks it up; if not, just move.
	return h.applyMoveTaskImmediate(ctx, msg, req, session)
}

// deferMoveTask records a PendingMove for the agent's turn-end handler to
// apply. Optionally queues a hand-off prompt when the caller supplied one.
// Returns a synthetic moved-task DTO so the agent's tool call resolves
// successfully and ends the turn cleanly.
func (h *Handlers) deferMoveTask(
	ctx context.Context,
	msg *ws.Message,
	req moveTaskRequest,
	session *models.TaskSession,
) (*ws.Message, error) {
	if h.messageQueue == nil {
		h.logger.Error("move_task: message queue not configured; cannot defer move from active session",
			zap.String("task_id", req.TaskID), zap.String("session_id", session.ID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"move_task requires message queue support while the source session is active", nil)
	}

	// Validate the target step exists and belongs to the requested workflow
	// before committing the deferred move. Without this check a stale or
	// foreign step_id would be stored and silently fail at turn-end, leaving
	// the task orphaned on the board.
	var targetStep *wfmodels.WorkflowStep
	if h.workflowCtrl != nil {
		stepResp, err := h.workflowCtrl.GetStep(ctx, req.WorkflowStepID)
		if err != nil || stepResp == nil || stepResp.Step == nil {
			h.logger.Error("move_task: target step not found",
				zap.String("task_id", req.TaskID),
				zap.String("workflow_step_id", req.WorkflowStepID),
				zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
				"target workflow_step_id does not exist", nil)
		}
		if stepResp.Step.WorkflowID != req.WorkflowID {
			h.logger.Error("move_task: target step belongs to a different workflow",
				zap.String("task_id", req.TaskID),
				zap.String("workflow_step_id", req.WorkflowStepID),
				zap.String("step_workflow_id", stepResp.Step.WorkflowID),
				zap.String("requested_workflow_id", req.WorkflowID))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
				"target workflow_step_id does not belong to the requested workflow_id", nil)
		}
		targetStep = stepResp.Step
	}

	// Validate the target workflow lives in the same workspace as the task.
	// The immediate-apply path (task.Service.MoveTask) already enforces this
	// via validateTaskMove; the deferred path bypasses that service entirely
	// (see applyPendingMove's doc comment), so it must check independently or
	// a cross-workspace move_task_kandev call would silently succeed.
	if h.taskSvc != nil {
		task, err := h.taskSvc.GetTask(ctx, req.TaskID)
		if err != nil {
			h.logger.Error("move_task: failed to load task for workspace validation",
				zap.String("task_id", req.TaskID), zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to load task for move validation", nil)
		}
		if task.Metadata != nil {
			if _, pending := task.Metadata[models.MetaKeyWorkflowMovePending]; pending {
				return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict,
					"another workflow move is already pending for this task", nil)
			}
		}
		targetWorkflow, err := h.taskSvc.GetWorkflow(ctx, req.WorkflowID)
		if err != nil {
			h.logger.Error("move_task: target workflow not found",
				zap.String("task_id", req.TaskID),
				zap.String("workflow_id", req.WorkflowID),
				zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
				"target workflow_id does not exist", nil)
		}
		if targetWorkflow.WorkspaceID != task.WorkspaceID {
			h.logger.Error("move_task: target workflow is in a different workspace",
				zap.String("task_id", req.TaskID),
				zap.String("workflow_id", req.WorkflowID),
				zap.String("task_workspace_id", task.WorkspaceID),
				zap.String("target_workspace_id", targetWorkflow.WorkspaceID))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
				"target workflow is in a different workspace", nil)
		}
		change := workflowmove.MoveChangeStep
		if task.WorkflowStepID == req.WorkflowStepID {
			change = workflowmove.MoveChangePositionOnly
		}
		if err := workflowmove.ValidateEntryOptions(req.EntryOptions, change); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "entry_options require a workflow step change", nil)
		}
		if err := h.taskSvc.ValidateMoveEntryOptions(ctx, task, targetStep, req.EntryOptions); err != nil {
			return ws.NewError(msg.ID, msg.Action, classifyMoveTaskError(err), moveTaskErrorMessage(err), nil)
		}
	}

	// Every deferred move gets an ID so lifecycle events cannot replay a stale
	// request, even when the move carries no private entry options.
	moveID := uuid.NewString()
	if pendingReader, ok := h.messageQueue.(interface {
		GetPendingMove(context.Context, string) (*messagequeue.PendingMove, bool)
	}); ok {
		if _, exists := pendingReader.GetPendingMove(ctx, session.ID); exists {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, "another workflow move is already pending for this session", nil)
		}
	}
	pendingMove := &messagequeue.PendingMove{
		TaskID:          req.TaskID,
		WorkflowID:      req.WorkflowID,
		WorkflowStepID:  req.WorkflowStepID,
		Position:        req.Position,
		MoveID:          moveID,
		Actor:           string(wfmodels.StepTransitionActorAgent),
		SenderSessionID: req.SenderSessionID,
		EntryOptions:    req.EntryOptions,
	}
	h.messageQueue.SetPendingMove(ctx, session.ID, pendingMove)
	// The legacy MessageQueuer interface deliberately keeps SetPendingMove
	// fire-and-forget for compatibility with lightweight test adapters. The
	// production queue exposes a read-back API, so fail closed when persistence
	// did not retain the exact deferred move (including its move ID/options).
	if pendingReader, ok := h.messageQueue.(interface {
		GetPendingMove(context.Context, string) (*messagequeue.PendingMove, bool)
	}); ok {
		stored, exists := pendingReader.GetPendingMove(ctx, session.ID)
		if !exists || stored == nil || stored.TaskID != pendingMove.TaskID ||
			stored.WorkflowStepID != pendingMove.WorkflowStepID || stored.MoveID != pendingMove.MoveID {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to persist deferred workflow move", nil)
		}
	}
	// The task is intentionally still at its source step until the active turn
	// settles. Return that authoritative source task with an explicit deferred
	// disposition so the agent cannot mistake the accepted request for an
	// already-committed transition.
	task, err := h.taskSvc.GetTask(ctx, req.TaskID)
	if err != nil || task == nil {
		synthetic := h.synthesizeMovedTaskDTO(ctx, req.TaskID, req.WorkflowID, req.WorkflowStepID, req.Position)
		if taskDTO, ok := synthetic.(dto.TaskDTO); ok {
			return ws.NewResponse(msg.ID, msg.Action, moveTaskMCPResponse{
				TaskDTO:      taskDTO,
				Task:         taskDTO,
				MoveID:       moveID,
				Disposition:  string(workflowmove.DispositionDeferred),
				EntryOptions: req.EntryOptions,
			})
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]any{
			"id":            req.TaskID,
			"task":          synthetic,
			"move_id":       moveID,
			"disposition":   string(workflowmove.DispositionDeferred),
			"entry_options": req.EntryOptions,
		})
	}
	taskDTO := dto.FromTask(task)
	return ws.NewResponse(msg.ID, msg.Action, moveTaskMCPResponse{
		TaskDTO:      taskDTO,
		Task:         taskDTO,
		MoveID:       moveID,
		Disposition:  string(workflowmove.DispositionDeferred),
		EntryOptions: req.EntryOptions,
	})
}

// applyMoveTaskImmediate runs the move now, optionally queueing a hand-off
// prompt on the (idle) primary session beforehand. Used when the source
// session is idle, when there's no source session at all, or when no prompt
// was supplied (config-mode/admin moves).
func (h *Handlers) applyMoveTaskImmediate(
	ctx context.Context,
	msg *ws.Message,
	req moveTaskRequest,
	session *models.TaskSession,
) (*ws.Message, error) {
	// Attribution uses the CALLING session (req.SenderSessionID, injected
	// server-side by the MCP server from its own bound session — see
	// moveTaskHandler), never the target task's session captured above:
	// move_task_kandev routinely moves a task the caller doesn't run a
	// session on, so the target's session is not who caused this move.
	// SenderSessionID is empty for a config-mode/admin MCP server with no
	// bound session, which correctly falls back to ActorSystem.
	attribution := steptelemetry.Attribution{Trigger: steptelemetry.TriggerMCPMove, ActorKind: steptelemetry.ActorSystem}
	if req.SenderSessionID != "" {
		attribution.ActorKind = steptelemetry.ActorAgent
		attribution.ActorID = req.SenderSessionID
		attribution.SessionID = req.SenderSessionID
	}
	moveCtx := steptelemetry.WithAttribution(ctx, attribution)
	result, err := h.taskSvc.MoveTaskWithOptions(moveCtx, req.TaskID, req.WorkflowID, req.WorkflowStepID, req.Position,
		service.MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorAgent, EntryOptions: req.EntryOptions})
	if err != nil {
		h.logger.Error("failed to move task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, classifyMoveTaskError(err), moveTaskErrorMessage(err), nil)
	}
	taskDTO := dto.FromTask(result.Task)
	return ws.NewResponse(msg.ID, msg.Action, moveTaskMCPResponse{
		TaskDTO:      taskDTO,
		Task:         taskDTO,
		MoveID:       result.MoveID,
		Disposition:  string(workflowmove.DispositionCommitted),
		EntryOptions: result.EntryOptions,
	})
}

func classifyMoveTaskError(err error) string {
	if err == nil {
		return ws.ErrorCodeInternalError
	}
	if errors.Is(err, workflowmove.ErrMoveConflict) {
		return ws.ErrorCodeConflict
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "wip limit exceeded"),
		strings.Contains(msg, "active session"),
		strings.Contains(msg, "archived tasks cannot be moved"),
		strings.Contains(msg, "different workspace"),
		strings.Contains(msg, "does not belong to target workflow"):
		return ws.ErrorCodeConflict
	case strings.Contains(msg, "invalid"),
		strings.Contains(msg, "required"),
		strings.Contains(msg, "no session or auto-start"),
		strings.Contains(msg, "entry options require"),
		strings.Contains(msg, "profile unavailable"),
		strings.Contains(msg, "model unavailable"):
		return ws.ErrorCodeValidation
	default:
		return ws.ErrorCodeInternalError
	}
}

func moveTaskErrorMessage(err error) string {
	switch classifyMoveTaskError(err) {
	case ws.ErrorCodeConflict:
		return "Move task conflicts with the current task or workflow state"
	case ws.ErrorCodeValidation:
		return "Invalid move_task request"
	default:
		return "Failed to move task"
	}
}

// synthesizeMovedTaskDTO is retained as a best-effort fallback when the
// authoritative source task cannot be reloaded while building a deferred
// response. It never fabricates destination fields because the move has not
// committed yet.
func (h *Handlers) synthesizeMovedTaskDTO(ctx context.Context, taskID, workflowID, workflowStepID string, position int) any {
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil {
		h.logger.Warn("failed to load task for synthetic move response",
			zap.String("task_id", taskID),
			zap.Error(err))
		return map[string]any{
			"id": taskID,
		}
	}
	return dto.FromTask(task)
}

// lookupSession returns the task's primary session.
//   - (session, nil) — task has a primary session.
//   - (nil, nil)     — task has no primary session yet (legitimate "empty"
//     state — task was created but no agent has been launched). The
//     repository signals this with the taskrepo.ErrNoPrimarySession sentinel;
//     we treat it as a not-found rather than a failure so the caller can fall
//     through to the idle-move path instead of rejecting the request.
//   - (nil, err)     — real backend lookup failure (DB error, etc.). The
//     caller should map this to an internal error rather than collapsing it
//     into "no session" downstream.
func (h *Handlers) lookupSession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	session, err := h.taskSvc.GetPrimarySession(ctx, taskID)
	if err != nil {
		// Classify the repo's not-found signal via the typed sentinel rather
		// than substring-matching the formatted message, which is brittle.
		if errors.Is(err, taskrepo.ErrNoPrimarySession) {
			return nil, nil
		}
		return nil, err
	}
	return session, nil
}

// queueMoveTaskPrompt enqueues a user-supplied prompt on the task's primary session.
// Returns an error when the queue itself is missing or QueueMessage fails — the
// caller decides whether to fail the whole move (running-session deferred path)
// or proceed (idle path), since a queue failure makes the deferred contract
// impossible to honor.
func (h *Handlers) queueMoveTaskPrompt(ctx context.Context, taskID, sessionID, prompt string) error {
	return h.queueMoveTaskPromptWithMoveID(ctx, taskID, sessionID, prompt, "")
}

func (h *Handlers) queueMoveTaskPromptWithMoveID(ctx context.Context, taskID, sessionID, prompt, moveID string) error {
	if h.messageQueue == nil {
		return fmt.Errorf("message queue is unavailable")
	}
	if sessionID == "" {
		return fmt.Errorf("task has no primary session")
	}
	metadata := map[string]interface{}(nil)
	if moveID != "" {
		metadata = map[string]interface{}{messagequeue.MetadataDeferredMoveID: moveID}
	}
	if queueWithMetadata, ok := h.messageQueue.(messageMetadataQueuer); ok {
		if _, err := queueWithMetadata.QueueMessageWithMetadata(ctx, sessionID, taskID, prompt, "", messagequeue.QueuedByMoveTask, false, nil, metadata); err != nil {
			return fmt.Errorf("queue message: %w", err)
		}
		return nil
	}
	if _, err := h.messageQueue.QueueMessage(ctx, sessionID, taskID, prompt, "", messagequeue.QueuedByMoveTask, false, nil); err != nil {
		return fmt.Errorf("queue message: %w", err)
	}
	return nil
}

func (h *Handlers) handleDeleteTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	taskID, err := unmarshalStringField(msg.Payload, "task_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if taskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	if err := h.taskSvc.DeleteTask(ctx, taskID); err != nil {
		h.logger.Error("failed to delete task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) handleArchiveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	taskID, err := unmarshalStringField(msg.Payload, "task_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if taskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	callerTaskID, err := unmarshalStringField(msg.Payload, "caller_task_id")
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if err := h.validateAutomationArchiveTarget(ctx, callerTaskID, taskID); err != nil {
		h.logger.Warn("rejected archive target", zap.String("task_id", taskID), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}

	if err := h.taskSvc.ArchiveTask(ctx, taskID); err != nil {
		// Archiving is a goal-state operation: a task that is already archived
		// is in the requested state, so report success instead of an opaque
		// internal error. The flag lets the caller tell a no-op from a real
		// state change.
		if errors.Is(err, service.ErrTaskAlreadyArchived) {
			return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
				"success":          true,
				"already_archived": true,
			})
		}
		h.logger.Error("failed to archive task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to archive task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

func (h *Handlers) validateAutomationArchiveTarget(ctx context.Context, callerTaskID, targetTaskID string) error {
	if callerTaskID == "" {
		return nil
	}
	if h.taskSvc == nil {
		return errors.New("archive caller task is unavailable")
	}
	caller, err := h.taskSvc.GetTask(ctx, callerTaskID)
	if err != nil || caller == nil {
		if err != nil {
			return fmt.Errorf("archive caller task cannot be resolved: %w", err)
		}
		return errors.New("archive caller task cannot be resolved")
	}
	if caller.Origin != models.TaskOriginAutomationRun ||
		models.StringFromAny(caller.Metadata["trigger_type"]) != string(automation.TriggerTypeGitHubPRMerged) {
		return nil
	}
	expectedTarget := models.StringFromAny(caller.Metadata[models.MetaKeyAutomationTargetTaskID])
	if expectedTarget == "" || expectedTarget != targetTaskID {
		return errors.New("archive target is not bound to this automation run")
	}
	return nil
}

func (h *Handlers) handleUpdateTaskState(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.State == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "state is required", nil)
	}
	state := normalizeTaskState(req.State)
	if !isValidTaskState(state) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "invalid task state: "+req.State, nil)
	}

	task, err := h.taskSvc.UpdateTaskState(ctx, req.TaskID, state)
	if err != nil {
		h.logger.Error("failed to update task state", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task state", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}

func isValidTaskState(state v1.TaskState) bool {
	switch state {
	case v1.TaskStateTODO, v1.TaskStateCreated, v1.TaskStateScheduling,
		v1.TaskStateInProgress, v1.TaskStateReview, v1.TaskStateBlocked,
		v1.TaskStateWaitingForInput, v1.TaskStateCompleted,
		v1.TaskStateFailed, v1.TaskStateCancelled:
		return true
	default:
		return false
	}
}

// normalizeTaskState maps common agent-supplied aliases to canonical TaskState
// values. Agents often send lowercase or shorthand strings (e.g. "complete",
// "done") that are not valid v1.TaskState constants.
func normalizeTaskState(raw string) v1.TaskState {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return v1.TaskState("")
	}
	upper := strings.ToUpper(trimmed)
	switch upper {
	case "OPEN", "TODO":
		return v1.TaskStateTODO
	case "IN_PROGRESS", "INPROGRESS", "ACTIVE":
		return v1.TaskStateInProgress
	case "COMPLETE", "COMPLETED", "DONE":
		return v1.TaskStateCompleted
	case "BLOCKED":
		return v1.TaskStateBlocked
	case "CANCELLED", "CANCELED":
		return v1.TaskStateCancelled
	case "REVIEW":
		return v1.TaskStateReview
	case "FAILED":
		return v1.TaskStateFailed
	case "CREATED":
		return v1.TaskStateCreated
	case "SCHEDULING":
		return v1.TaskStateScheduling
	case "WAITING_FOR_INPUT", "WAITING":
		return v1.TaskStateWaitingForInput
	default:
		return v1.TaskState(trimmed)
	}
}
