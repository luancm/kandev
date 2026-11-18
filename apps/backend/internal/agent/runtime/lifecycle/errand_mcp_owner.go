package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
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

type errandCapabilityChecker interface {
	RequireCapability(context.Context, string) error
}

func requireErrandQuestionPolicy(ctx context.Context, control any, owner *agentctltypes.ErrandMCPOwner) error {
	if owner == nil {
		return nil
	}
	checker, ok := control.(errandCapabilityChecker)
	if !ok {
		return errors.New("agentctl errand question policy capability is unavailable")
	}
	return checker.RequireCapability(ctx, agentctltypes.ErrandMCPOwnerCapability)
}

func recordedErrandMCPOwner(record *models.ExecutorRunning) (*agentctltypes.ErrandMCPOwner, error) {
	if record == nil {
		return nil, errors.New("missing execution inventory")
	}
	rawOwner, present := record.Metadata[runExecutionOwnerMetadataKey]
	if !present {
		return nil, nil
	}
	raw, err := json.Marshal(rawOwner)
	if err != nil {
		return nil, err
	}
	var owner ExecutionOwner
	if err := json.Unmarshal(raw, &owner); err != nil {
		return nil, err
	}
	if owner.Kind == ExecutionOwnerRun && owner.ErrandID == "" && owner.Generation == 0 {
		return nil, nil
	}
	if owner.Kind != ExecutionOwnerErrand || !validRecordedErrandOwner(record, owner) {
		return nil, errors.New("invalid execution owner inventory")
	}
	return errandMCPOwner(owner), nil
}
