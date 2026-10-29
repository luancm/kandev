package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

type errandScenarioRequest struct {
	Profile string   `json:"profile"`
	Prompts []string `json:"prompts"`
}
type errandScenarioCall func(context.Context, string, map[string]any) (string, error)

func runErrandsScenario(ctx context.Context, call errandScenarioCall, request errandScenarioRequest, await bool) error {
	ids := make([]string, 0, len(request.Prompts))
	for index, prompt := range request.Prompts {
		args := map[string]any{"title": fmt.Sprintf("Errand %d", index+1), "prompt": prompt}
		if request.Profile != "" {
			args["agent_profile"] = request.Profile
		}
		result, err := call(ctx, "start_errand_kandev", args)
		if err != nil {
			return err
		}
		var started struct {
			ErrandID string `json:"errand_id"`
		}
		if err := json.Unmarshal([]byte(result), &started); err != nil {
			return fmt.Errorf("decode errand start: %w", err)
		}
		if started.ErrandID == "" {
			return fmt.Errorf("errand start returned no identity: %s", result)
		}
		ids = append(ids, started.ErrandID)
	}
	if await && len(ids) > 0 {
		_, err := call(ctx, "await_errands_kandev", map[string]any{"errand_ids": ids, "mode": "all", "max_wait_seconds": 600})
		return err
	}
	return nil
}

func emitErrandsScenario(e *emitter, request errandScenarioRequest, await bool) {
	if len(request.Prompts) == 0 {
		request.Prompts = []string{`e2e:message("Errand result 1")`, `e2e:message("Errand result 2")`}
	}
	call := func(ctx context.Context, tool string, args map[string]any) (string, error) {
		id := nextToolID()
		e.startTool(id, tool, acp.ToolKindOther, args)
		result, err := e.callMCPToolCtx(ctx, "kandev", tool, args)
		if err != nil {
			e.completeTool(id, map[string]any{"error": err.Error()})
			return "", err
		}
		e.completeTool(id, map[string]any{"result": result})
		return result, nil
	}
	if err := runErrandsScenario(e.ctx, call, request, await); err != nil {
		e.text("Errands scenario error: " + err.Error())
		return
	}
	e.text("Errands parent returned")
}

func scenarioErrands(e *emitter)      { emitErrandsScenario(e, errandScenarioRequest{}, false) }
func scenarioErrandsAwait(e *emitter) { emitErrandsScenario(e, errandScenarioRequest{}, true) }

// handleErrandsScenario keeps fixture arguments and status echo local to Errands.
func handleErrandsScenario(e *emitter, fullPrompt, command string) bool {
	if command == "/e2e:errands-status" {
		e.text(fmt.Sprintf("ERRAND_STATUS_BLOCKS=%d", strings.Count(fullPrompt, "Errand status: pending results follow.")))
		return true
	}
	name, argument, hasArgument := strings.Cut(command, " ")
	if !hasArgument || (name != "/e2e:errands" && name != "/e2e:errands-await") {
		return false
	}
	var request errandScenarioRequest
	if err := json.Unmarshal([]byte(strings.TrimSpace(argument)), &request); err != nil {
		e.text("Errands scenario arguments: " + err.Error())
		return true
	}
	emitErrandsScenario(e, request, name == "/e2e:errands-await")
	return true
}
