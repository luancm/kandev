package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrandScenarioAwaitsRealStartReturnedIDs(t *testing.T) {
	ctx := t.Context()
	var calls []string
	call := func(gotCtx context.Context, tool string, args map[string]any) (string, error) {
		require.Equal(t, ctx, gotCtx)
		calls = append(calls, tool)
		if tool == "await_errands_kandev" {
			require.Equal(t, []string{"actual-1", "actual-2"}, args["errand_ids"])
			return `{"statuses":[]}`, nil
		}
		require.Equal(t, "chosen-profile", args["agent_profile"])
		require.Equal(t, fmt.Sprintf("objective-%d", len(calls)), args["prompt"])
		return fmt.Sprintf(`{"errand_id":"actual-%d","session_id":"actual-%d"}`, len(calls), len(calls)), nil
	}
	require.NoError(t, runErrandsScenario(ctx, call, errandScenarioRequest{Profile: "chosen-profile", Prompts: []string{"objective-1", "objective-2"}}, true))
	require.Equal(t, []string{"start_errand_kandev", "start_errand_kandev", "await_errands_kandev"}, calls)
}

func TestErrandScenarioRejectsMissingReturnedIdentity(t *testing.T) {
	call := func(context.Context, string, map[string]any) (string, error) { return `{"status":"failed"}`, nil }
	require.Error(t, runErrandsScenario(t.Context(), call, errandScenarioRequest{Prompts: []string{"objective"}}, false))
}
