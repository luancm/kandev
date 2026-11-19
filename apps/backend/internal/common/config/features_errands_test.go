package config

import "testing"

// @covers AC-TASKS-ERRANDS-012.1
func TestFeaturesErrandsDefaultsOffAndEnablesFromEnv(t *testing.T) {
	unsetEnv(t, "KANDEV_FEATURES_ERRANDS")
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "")
	t.Setenv("KANDEV_DEBUG_PPROF_ENABLED", "")
	t.Setenv("KANDEV_E2E_MOCK", "")

	cfg, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Features.Errands {
		t.Fatal("Features.Errands = true, want false without an explicit opt-in")
	}

	t.Setenv("KANDEV_FEATURES_ERRANDS", "true")
	cfg, err = LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if !cfg.Features.Errands {
		t.Fatal("Features.Errands = false, want true when KANDEV_FEATURES_ERRANDS=true")
	}
}
