package runtime

import "github.com/kandev/kandev/internal/agent/runtime/lifecycle"

// SessionModelsSnapshot is the persisted provider-derived model state exposed
// through the runtime facade.
type SessionModelsSnapshot = lifecycle.SessionModelsSnapshot

// LoadSessionModelsSnapshot decodes typed and JSON-rehydrated model metadata.
func LoadSessionModelsSnapshot(raw any) (SessionModelsSnapshot, bool) {
	return lifecycle.LoadSessionModelsSnapshot(raw)
}
