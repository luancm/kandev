package sysprompt

import (
	"os"
	"path/filepath"
	"testing"
)

// TestErrandKandevContextMatchesGolden pins the read-only errand context. Set
// UPDATE_KANDEV_CONTEXT_GOLDEN=1 to rewrite the golden after an intentional
// change to the errand catalog text.
func TestErrandKandevContextMatchesGolden(t *testing.T) {
	got := FormatErrandKandevContext("task-1", "session-1")
	path := filepath.Join("testdata", "errand-session.golden")
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
		t.Fatalf("errand context drifted from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
