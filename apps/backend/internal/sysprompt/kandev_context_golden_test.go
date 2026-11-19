package sysprompt

import (
	"os"
	"path/filepath"
	"testing"
)

// kandevContextGoldenCases pin the task-mode context for every capability mix
// that exists without features.errands. With the flag off, each render must
// stay byte-identical to the golden file.
var kandevContextGoldenCases = map[string]KandevContextOptions{
	"task-default": {
		IncludeCoordinatorTaskControls: true,
		IncludeUserQuestionTool:        true,
	},
	"task-step-complete-title-canvas": {
		RequiresCompletionSignal: true,
		IncludeTaskTitleTool:     true,
		IncludeCanvasGuidance:    true,
		IncludeUserQuestionTool:  true,
	},
	"task-autopilot-parent-question": {
		Autopilot:                      true,
		IncludeParentQuestionTool:      true,
		IncludeCoordinatorTaskControls: true,
	},
	"task-config-mode": {
		IncludeUserQuestionTool: true,
	},
}

// TestKandevContextMatchesGoldenWhenErrandsOff proves the flag-off task-mode
// context is byte-identical to the text that shipped before the errand
// placeholders existed. Set UPDATE_KANDEV_CONTEXT_GOLDEN=1 to rewrite the
// golden files after an intentional change to flag-off text.
func TestKandevContextMatchesGoldenWhenErrandsOff(t *testing.T) {
	for name, options := range kandevContextGoldenCases {
		t.Run(name, func(t *testing.T) {
			got := FormatKandevContextWithOptions("task-1", "session-1", options)
			path := filepath.Join("testdata", name+".golden")
			if os.Getenv("UPDATE_KANDEV_CONTEXT_GOLDEN") == "1" {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v", path, err)
			}
			if got != string(want) {
				t.Fatalf("flag-off context drifted from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
			}
		})
	}
}

// TestInjectedKandevContextMatchesGoldenWhenErrandsOff pins the first-turn
// injection path, which wraps the same render in <kandev-system> tags.
func TestInjectedKandevContextMatchesGoldenWhenErrandsOff(t *testing.T) {
	got := InjectKandevContextWithOptions("task-1", "session-1", "hello", KandevContextOptions{
		IncludeCoordinatorTaskControls: true,
		IncludeUserQuestionTool:        true,
	})
	path := filepath.Join("testdata", "task-injected.golden")
	if os.Getenv("UPDATE_KANDEV_CONTEXT_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if got != string(want) {
		t.Fatalf("injected flag-off context drifted from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
