package runtimeflags

import (
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/profiles"
)

// @covers AC-TASKS-ERRANDS-012.1
func TestErrandsFlagIsOffInEveryProfileAndRequiresRestart(t *testing.T) {
	const key = "features.errands"
	def, ok := DefinitionByKey(key)
	if !ok {
		t.Fatalf("%s definition missing", key)
	}
	if def.EnvVar != "KANDEV_FEATURES_ERRANDS" || def.Kind != KindFeature {
		t.Fatalf("errands identity changed: %+v", def)
	}
	if !def.RestartRequired || !def.Mutable || def.Stability != StabilityExperimental || def.RiskDescription == "" {
		t.Fatalf("errands must stay a restart-required experimental override: %+v", def)
	}
	for _, profile := range []struct{ name, dev, e2e string }{
		{name: "prod", dev: "false", e2e: "false"},
		{name: "dev", dev: "true", e2e: "false"},
		{name: "e2e", dev: "false", e2e: "true"},
	} {
		t.Run(profile.name, func(t *testing.T) {
			t.Setenv("KANDEV_DEBUG_DEV_MODE", profile.dev)
			t.Setenv("KANDEV_E2E_MOCK", profile.e2e)
			defaults, err := profiles.FeatureFlagDefaults()
			if err != nil {
				t.Fatal(err)
			}
			if got := defaults["errands"]; got != "false" {
				t.Fatalf("errands defaults to %q in %s, want false", got, profile.name)
			}
		})
	}
	cfg := &config.Config{}
	for _, enabled := range []bool{true, false} {
		ApplyStatesToConfig(cfg, []RuntimeFlagState{{Key: key, EffectiveValue: enabled}})
		if cfg.Features.Errands != enabled || ValuesFromConfig(cfg)[key] != enabled {
			t.Fatalf("errands typed override did not round trip %v", enabled)
		}
	}
}
