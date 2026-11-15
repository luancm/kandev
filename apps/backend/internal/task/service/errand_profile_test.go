package service

import (
	"context"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

func TestValidateAgentProfileForTaskExecutorChecksProfileAndTaskExecutor(t *testing.T) {
	fixture := newWorkflowChangeFixture(t)
	ctx := context.Background()

	if err := fixture.svc.ValidateAgentProfileForTaskExecutor(ctx, fixture.task.ID, "replacement-profile"); err != nil {
		t.Fatalf("a compatible profile was refused: %v", err)
	}
	if len(fixture.validator.profiles) != 1 || fixture.validator.profiles[0] != "replacement-profile" {
		t.Fatalf("validated profiles = %v, want the requested profile on the task executor", fixture.validator.profiles)
	}

	fixture.validator.executorID = "some-other-executor"
	if err := fixture.svc.ValidateAgentProfileForTaskExecutor(ctx, fixture.task.ID, "replacement-profile"); err == nil {
		t.Fatal("a profile the task's executor cannot run was accepted")
	}
}

func TestValidateAgentProfileForTaskExecutorRefusesForeignDisabledAndMissingProfiles(t *testing.T) {
	fixture := newWorkflowChangeFixture(t)
	ctx := context.Background()
	fixture.svc.agentProfiles = workflowAgentOverrideProfilesStub{profiles: map[string]*settingsmodels.AgentProfile{
		"foreign":  {ID: "foreign", Enabled: true, WorkspaceID: "another-workspace"},
		"disabled": {ID: "disabled", Enabled: false, WorkspaceID: "workspace-change"},
		"global":   {ID: "global", Enabled: true},
	}}
	for _, profileID := range []string{"foreign", "disabled", "missing"} {
		if err := fixture.svc.ValidateAgentProfileForTaskExecutor(ctx, fixture.task.ID, profileID); err == nil {
			t.Fatalf("profile %q was accepted", profileID)
		}
	}
	if err := fixture.svc.ValidateAgentProfileForTaskExecutor(ctx, fixture.task.ID, "global"); err != nil {
		t.Fatalf("a global profile was refused: %v", err)
	}
	if err := fixture.svc.ValidateAgentProfileForTaskExecutor(ctx, "no-such-task", "global"); err == nil {
		t.Fatal("a missing task was accepted")
	}
}
