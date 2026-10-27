package mcp

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/google/uuid"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

// errandAwaitKeepAliveInterval paces progress notifications while an await is
// blocked. It stays below the agent client's idle timeout, as the
// ask_user_question keepalive does. Tests shorten it.
var errandAwaitKeepAliveInterval = 20 * time.Second

// errandAwaitReleaseTimeout bounds the release sent after an abandoned await.
const errandAwaitReleaseTimeout = 5 * time.Second

const (
	errandAwaitDefaultWaitSeconds = 600
	errandAwaitMaxWaitSeconds     = 600
	errandAwaitMaxIDs             = 5
	errandAwaitModeAll            = "all"
	errandAwaitModeAny            = "any"
	errandIDArg                   = "errand_id"
	errandIDsArg                  = "errand_ids"
	errandModeArg                 = "mode"
	errandMaxWaitArg              = "max_wait_seconds"
	errandAwaitTokenArg           = "await_token"
	errandAgentProfileArg         = "agent_profile"
	errandSenderSessionArg        = "sender_session_id"
	errandSenderTaskArg           = "sender_task_id"
)

// Tool names of the errand catalog. Named once so the allowlist below stays
// readable and the repeated literals stay out of goconst's reach.
const (
	toolListWorkspaces      = "list_workspaces_kandev"
	toolListWorkflows       = "list_workflows_kandev"
	toolListWorkflowSteps   = "list_workflow_steps_kandev"
	toolListTasks           = "list_tasks_kandev"
	toolListRelatedTasks    = "list_related_tasks_kandev"
	toolListTaskSessions    = "list_task_sessions_kandev"
	toolGetTaskConversation = "get_task_conversation_kandev"
	toolGetTaskPlan         = "get_task_plan_kandev"
)

// errandCatalogTools is the fixed read-only Kandev catalog of an errand session
// (AC-TASKS-ERRANDS-009.2). Nothing outside this list is visible to an errand,
// whatever providers or capabilities its parent inherited.
var errandCatalogTools = []string{
	toolListWorkspaces,
	toolListWorkflows,
	toolListWorkflowSteps,
	toolListTasks,
	toolListRelatedTasks,
	toolListTaskSessions,
	toolGetTaskConversation,
	toolGetTaskPlan,
}

// registerErrandCatalogTools composes the errand catalog from the existing read
// registrations and then removes every tool outside the allowlist. Plugin tools
// are never added to this mode (see registerTools).
func (s *Server) registerErrandCatalogTools() {
	s.registerKanbanTools()
	s.registerRelatedTasksTool()
	s.registerPlanTools()
	s.keepOnlyTools(errandCatalogTools)
}

// keepOnlyTools deletes every registered tool whose name is not in allowed.
func (s *Server) keepOnlyTools(allowed []string) {
	keep := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		keep[name] = struct{}{}
	}
	var drop []string
	for name := range s.mcpServer.ListTools() {
		if _, ok := keep[name]; !ok {
			drop = append(drop, name)
		}
	}
	s.mcpServer.DeleteTools(drop...)
}

// registerErrandParentTools adds the three errand tools for a parent session
// that holds CapabilityErrands. Sender identity is always filled from this
// server's own session, never from tool arguments.
func (s *Server) registerErrandParentTools() {
	s.mcpServer.AddTool(
		mcp.NewTool("start_errand_kandev",
			mcp.WithDescription("Start an errand: a short, bounded piece of work run by a separate agent session on this task. The call returns at once with the errand ID and status. Each errand's result is delivered to you as a message in this conversation when it finishes, so end your turn after starting errands and continue when results arrive. Use await_errands_kandev only to wait for results within this same turn."),
			mcp.WithString("title", mcp.Required(), mcp.Description("Short human-readable name for the errand")),
			mcp.WithString("prompt", mcp.Required(), mcp.Description("Complete instructions for the errand agent. It sees only this prompt and the task it works on.")),
			mcp.WithString(errandAgentProfileArg, mcp.Description("Optional agent profile ID or exact name. Omit to use the default errand profile.")),
		),
		s.wrapHandler("start_errand_kandev", s.startErrandHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("await_errands_kandev",
			mcp.WithDescription("Optionally wait in this turn for errands to reach a result. mode \"all\" waits until every listed errand has a result; mode \"any\" returns when one does. Results already delivered to you as messages are reported with delivered_via. Returns the pending status for errands still running when max_wait_seconds elapses."),
			mcp.WithArray(errandIDsArg, mcp.Required(), mcp.Items(map[string]any{typeKey: stringType}), mcp.MinItems(1), mcp.MaxItems(errandAwaitMaxIDs), mcp.Description("Errand IDs to wait for, 1 to 5")),
			mcp.WithString(errandModeArg, mcp.Required(), mcp.Enum(errandAwaitModeAll, errandAwaitModeAny), mcp.Description("all: wait for every errand. any: return when one errand has a result.")),
			mcp.WithNumber(errandMaxWaitArg, mcp.Min(0), mcp.Max(errandAwaitMaxWaitSeconds), mcp.Description("Seconds to wait, 0 to 600. Defaults to 600.")),
		),
		s.wrapHandler("await_errands_kandev", s.awaitErrandsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewTool("cancel_errand_kandev",
			mcp.WithDescription("Cancel an errand you started. A running errand is stopped; a finished errand is left unchanged."),
			mcp.WithString(errandIDArg, mcp.Required(), mcp.Description("ID of the errand to cancel")),
		),
		s.wrapHandler("cancel_errand_kandev", s.cancelErrandHandler()),
	)
}

func (s *Server) startErrandHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title, err := req.RequireString("title")
		if err != nil || title == "" {
			return mcp.NewToolResultError("title is required"), nil
		}
		prompt, err := req.RequireString(promptArg)
		if err != nil || prompt == "" {
			return mcp.NewToolResultError("prompt is required"), nil
		}
		payload := s.errandSenderPayload()
		payload["task_id"] = s.taskID
		payload["title"] = title
		payload[promptArg] = prompt
		copyOptionalStringArg(payload, req, errandAgentProfileArg)
		return s.errandBackendResult(ctx, ws.ActionMCPStartErrand, payload)
	}
}

func (s *Server) cancelErrandHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		errandID, err := req.RequireString(errandIDArg)
		if err != nil || errandID == "" {
			return mcp.NewToolResultError("errand_id is required"), nil
		}
		payload := s.errandSenderPayload()
		payload[errandIDArg] = errandID
		return s.errandBackendResult(ctx, ws.ActionMCPCancelErrand, payload)
	}
}

func (s *Server) awaitErrandsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ids, mode, wait, errResult := parseAwaitErrandsArgs(req)
		if errResult != nil {
			return errResult, nil
		}
		// The token names this call's claim on the errand results. Releasing it
		// on an abandoned call lets a later await deliver them.
		token := uuid.NewString()
		payload := s.errandSenderPayload()
		payload[errandIDsArg] = ids
		payload[errandModeArg] = mode
		payload[errandMaxWaitArg] = wait
		payload[errandAwaitTokenArg] = token

		stop := make(chan struct{})
		defer close(stop)
		go emitKeepAlivePings(ctx, stop, errandAwaitKeepAliveInterval,
			toolProgressKeepAlive(ctx, req, "await_errands_kandev", "Waiting for errands in Kandev"))

		result, err := s.errandBackendCall(ctx, ws.ActionMCPAwaitErrands, payload)
		if ctx.Err() != nil {
			s.releaseErrandAwait(token)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return errandResultText(result), nil
	}
}

// parseAwaitErrandsArgs validates the await arguments before any backend
// call. The backend repeats the checks; these keep malformed calls off it.
func parseAwaitErrandsArgs(req mcp.CallToolRequest) ([]string, string, float64, *mcp.CallToolResult) {
	ids := req.GetStringSlice(errandIDsArg, nil)
	if len(ids) == 0 || len(ids) > errandAwaitMaxIDs {
		return nil, "", 0, mcp.NewToolResultError("errand_ids must list 1 to 5 errand IDs")
	}
	for _, id := range ids {
		if id == "" {
			return nil, "", 0, mcp.NewToolResultError("errand_ids must not contain empty IDs")
		}
	}
	mode := req.GetString(errandModeArg, "")
	if mode != errandAwaitModeAll && mode != errandAwaitModeAny {
		return nil, "", 0, mcp.NewToolResultError("mode must be \"all\" or \"any\"")
	}
	wait := req.GetFloat(errandMaxWaitArg, errandAwaitDefaultWaitSeconds)
	if math.IsNaN(wait) || wait != math.Trunc(wait) || wait < 0 || wait > errandAwaitMaxWaitSeconds {
		return nil, "", 0, mcp.NewToolResultError("max_wait_seconds must be a whole number from 0 to 600")
	}
	return ids, mode, wait, nil
}

// errandSenderPayload carries the caller's own task and session. The backend
// authorizes against these fields, so they are never read from arguments.
func (s *Server) errandSenderPayload() map[string]interface{} {
	return map[string]interface{}{
		errandSenderSessionArg: s.sessionID,
		errandSenderTaskArg:    s.taskID,
	}
}

// releaseErrandAwait tells the backend an await was abandoned. It runs on a
// fresh context because the call's own context has already ended.
func (s *Server) releaseErrandAwait(token string) {
	ctx, cancel := context.WithTimeout(context.Background(), errandAwaitReleaseTimeout)
	defer cancel()
	payload := s.errandSenderPayload()
	payload[errandAwaitTokenArg] = token
	var ignored map[string]interface{}
	if err := s.backend.RequestPayload(ctx, ws.ActionMCPAwaitErrandsRelease, payload, &ignored); err != nil {
		s.logger.Warn("failed to release abandoned errand await", zap.Error(err))
	}
}

func (s *Server) errandBackendResult(ctx context.Context, action string, payload map[string]interface{}) (*mcp.CallToolResult, error) {
	result, err := s.errandBackendCall(ctx, action, payload)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return errandResultText(result), nil
}

func (s *Server) errandBackendCall(ctx context.Context, action string, payload map[string]interface{}) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := s.backend.RequestPayload(ctx, action, payload, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func errandResultText(result map[string]interface{}) *mcp.CallToolResult {
	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data))
}

// toolProgressKeepAlive builds the callback that streams a progress notification
// on an in-flight tool call. The progress token mirrors the client's token when
// present. It is a no-op when no MCP server is bound to ctx.
func toolProgressKeepAlive(ctx context.Context, req mcp.CallToolRequest, tool, message string) func() {
	srv := server.ServerFromContext(ctx)
	if srv == nil {
		return func() {}
	}
	var token mcp.ProgressToken = tool
	if req.Params.Meta != nil && req.Params.Meta.ProgressToken != nil {
		token = req.Params.Meta.ProgressToken
	}
	var progress float64
	return func() {
		progress++
		_ = srv.SendNotificationToClient(ctx, "notifications/progress", map[string]any{
			"progressToken": token,
			"progress":      progress,
			messageArg:      message,
		})
	}
}
