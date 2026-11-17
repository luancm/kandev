package requiredstores

import (
	"slices"
	"testing"
)

func TestTaskCatalogRequiresErrandTranscript(t *testing.T) {
	for _, descriptor := range Catalog() {
		if descriptor.ID != "task" {
			continue
		}
		for _, table := range []string{"errand_turns", "errand_messages", "errand_execution_generations", "errand_harness_bindings"} {
			if !slices.Contains(descriptor.RequiredTables, table) {
				t.Errorf("task readiness must require %s", table)
			}
		}
		return
	}
	t.Fatal("task descriptor missing")
}
