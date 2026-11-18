package lifecycle

import "context"

func launchJournalOwnerID(req *LaunchRequest) string {
	if req.Owner.Kind == ExecutionOwnerErrand {
		return "errand-" + req.Owner.ErrandID
	}
	if req.SessionID != "" {
		return req.SessionID
	}
	return req.TaskEnvironmentID
}

func (m *Manager) admitOwnerBeforeRegistration(ctx context.Context, execution *AgentExecution) error {
	if execution == nil || execution.Owner.Kind == ExecutionOwnerErrand {
		return nil
	}
	owner := execution.Owner
	if owner.Kind == ExecutionOwnerTask || owner.Kind == "" {
		owner = ExecutionOwner{
			Kind: executionOwnerKind(execution), WorkspaceID: execution.WorkspaceID,
			TaskID: execution.TaskID, SessionID: execution.SessionID, RunID: execution.RunID,
			RunSessionID: execution.RunSessionID, Attempt: execution.RunAttempt,
			AgentProfileID: execution.OfficeAgentProfileID,
		}
	}
	return m.admitExecutionOwner(ctx, &LaunchRequest{Owner: owner, OwnerAdmission: execution.OwnerAdmission})
}

// An allocated Errand registers its exact inventory before checking whether
// it can start. Cancellation can then recover an unconfirmed rollback.
func (m *Manager) admitRegisteredErrand(ctx context.Context, execution *AgentExecution) error {
	if execution.Owner.Kind != ExecutionOwnerErrand {
		return nil
	}
	return m.admitExecutionOwner(ctx, &LaunchRequest{Owner: execution.Owner, OwnerAdmission: execution.OwnerAdmission})
}
