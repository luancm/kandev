package orchestrator

import "testing"

func TestAppendWorkflowMoveInstructionsAddsOneShotBlock(t *testing.T) {
	got := appendWorkflowMoveInstructions("Review the task.", "  Create the PR ready for review.  ")
	want := "Review the task.\n\n## One-time workflow move instructions\n\nCreate the PR ready for review.\n\n<!-- /one-time-workflow-move-instructions -->"
	if got != want {
		t.Fatalf("appendWorkflowMoveInstructions() = %q, want %q", got, want)
	}
}

func TestAppendWorkflowMoveInstructionsLeavesPromptUnchangedWhenEmpty(t *testing.T) {
	if got := appendWorkflowMoveInstructions("Review the task.", "   "); got != "Review the task." {
		t.Fatalf("appendWorkflowMoveInstructions() = %q, want unchanged prompt", got)
	}
}
