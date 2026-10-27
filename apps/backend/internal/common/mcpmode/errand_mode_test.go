package mcpmode

import "testing"

// TestErrand_IsAnInstanceMode proves the errand MCP mode is accepted by the
// agentctl instance API, so an errand launch is not rejected at start.
func TestErrand_IsAnInstanceMode(t *testing.T) {
	if Errand != "errand" {
		t.Fatalf("Errand = %q, want %q", Errand, "errand")
	}
	if !IsInstanceMode(Errand) {
		t.Fatalf("IsInstanceMode(%q) = false, want true", Errand)
	}
	found := false
	for _, mode := range InstanceModes() {
		if mode == Errand {
			found = true
		}
	}
	if !found {
		t.Fatalf("InstanceModes() = %#v, want it to include %q", InstanceModes(), Errand)
	}
}
