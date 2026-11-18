package instance

import (
	"context"
	"errors"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

var ErrErrandInstanceConflict = errors.New("errand instance owner or startup state is unresolved")

// StopErrandInstance stops the captured owner and confirms that neither a
// published nor a provisional instance occupies the ID.
func (m *Manager) StopErrandInstance(ctx context.Context, id string, owner agentctltypes.ErrandMCPOwner) error {
	m.mu.RLock()
	inst, provisional := m.instances[id], m.provisional[id]
	m.mu.RUnlock()
	if provisional != nil {
		return ErrErrandInstanceConflict
	}
	if inst == nil {
		return nil
	}
	if inst.ErrandMCPOwner == nil || *inst.ErrandMCPOwner != owner {
		return ErrErrandInstanceConflict
	}
	if err := m.stopInstance(ctx, id, inst); err != nil {
		return err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.instances[id] != nil || m.provisional[id] != nil {
		return ErrErrandInstanceConflict
	}
	return nil
}
