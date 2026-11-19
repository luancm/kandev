package mcpconfig

import (
	"fmt"
	"slices"
)

// ErrandMCPContribution identifies the selection captured for an Errand.
type ErrandMCPContribution struct {
	ErrandID    string
	WorkspaceID string
	TaskID      string
	Snapshot    SelectionSnapshot
}

// ApplyErrandMCPSelection contributes the saved selection before reading the
// catalog. Exclusions also apply to inherited repository, profile and task IDs.
func ApplyErrandMCPSelection(rc ResolutionContext, inputs *ResolutionInputs) error {
	errand := rc.Errand
	if errand == nil {
		return nil
	}
	if errand.ErrandID == "" || errand.WorkspaceID == "" || errand.TaskID == "" ||
		errand.WorkspaceID != rc.WorkspaceID || errand.TaskID != rc.TaskID || rc.SessionID != "" {
		return fmt.Errorf("%w: errand MCP selection owner mismatch", ErrMCPInvalidSelection)
	}
	addOrigins(inputs.Origins, errand.Snapshot.DefinitionIDs, SelectionOrigin{
		Scope: SelectionScope("errand"), WorkspaceID: rc.WorkspaceID, OwnerID: errand.ErrandID,
	})
	inputs.ExcludedDefinitionIDs = slices.Clone(errand.Snapshot.ExcludedDefinitionIDs)
	return nil
}
