package runtime

import (
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// GitOperationResult is the runtime seam's view of an agentctl Git operation.
// The alias keeps lifecycle implementations compatible without making
// higher-level callers import the low-level agentctl package directly.
type GitOperationResult = agentctlclient.GitOperationResult

// GitStatusResult is the runtime seam's view of an agentctl Git status.
type GitStatusResult = agentctlclient.GitStatusResult

// GitStatusData is the runtime seam's event view of a Git status update.
type GitStatusData = lifecycle.GitStatusData
