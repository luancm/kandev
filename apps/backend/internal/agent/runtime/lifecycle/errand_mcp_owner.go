package lifecycle

import (
	"encoding/json"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
)

func errandMCPOwner(owner ExecutionOwner) *agentctltypes.ErrandMCPOwner {
	if owner.Kind != ExecutionOwnerErrand {
		return nil
	}
	return &agentctltypes.ErrandMCPOwner{TaskID: owner.TaskID, ErrandID: owner.ErrandID, Generation: owner.Generation}
}

func matchErrandMCPOwner(expected, actual *agentctltypes.ErrandMCPOwner) bool {
	if expected == nil {
		return actual == nil
	}
	return actual != nil && *expected == *actual
}

func matchesRecordedErrandMCPOwner(record *models.ExecutorRunning, actual *agentctltypes.ErrandMCPOwner) bool {
	if record == nil {
		return false
	}
	rawOwner, present := record.Metadata[runExecutionOwnerMetadataKey]
	if !present {
		return actual == nil
	}
	raw, err := json.Marshal(rawOwner)
	if err != nil {
		return false
	}
	var owner ExecutionOwner
	if json.Unmarshal(raw, &owner) != nil {
		return false
	}
	if owner.Kind != ExecutionOwnerErrand {
		return owner.Kind == ExecutionOwnerRun && owner.ErrandID == "" && owner.Generation == 0 && actual == nil
	}
	if !validRecordedErrandOwner(record, owner) {
		return false
	}
	return matchErrandMCPOwner(errandMCPOwner(owner), actual)
}

func validRecordedErrandOwner(record *models.ExecutorRunning, owner ExecutionOwner) bool {
	return owner.ErrandID != "" && owner.Generation > 0 && owner.TaskID != "" && owner.WorkspaceID != "" &&
		owner.SessionID == "" && owner.RunID == "" && owner.RunSessionID == "" && owner.Attempt == 0 &&
		record.TaskID == "" && record.SessionID == models.ErrandExecutionIdentity(owner.ErrandID)
}
