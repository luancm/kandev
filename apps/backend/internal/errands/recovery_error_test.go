package errands

import (
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/db"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRecoveryRefusalPreservesOperationalFailures(t *testing.T) {
	readFailure := errors.New("evidence read failed")
	for _, err := range []error{readFailure, errors.Join(db.ErrErrandSettlementPending, readFailure)} {
		_, ok := WSError("request", "message.add", err)
		require.False(t, ok, "operational failure must not become recovery uncertainty")
	}
	_, ok := WSError("request", "message.add", fmt.Errorf("admission: %w", db.ErrErrandSettlementPending))
	require.True(t, ok)
}
