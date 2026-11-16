package orchestrator

import (
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestComposeErrandTurnRetainsUnknownClaim(t *testing.T) {
	composer := &fakeErrandComposer{
		composition: errands.Composition{Token: "unknown-commit-token"},
		composeErr:  fmt.Errorf("%w (token unknown-commit-token)", models.ErrErrandClaimCommitUnknown),
	}
	svc := withErrandComposer(t, composer)
	prompt, token, err := svc.composeErrandTurn(t.Context(), "s1", "ordinary input", &promptTaskOptions{})
	require.ErrorIs(t, err, models.ErrErrandClaimCommitUnknown)
	require.Contains(t, err.Error(), "unknown-commit-token")
	require.Empty(t, prompt)
	require.Empty(t, token, "an unknown claim must not be passed to automatic release")
	require.Empty(t, composer.released)
}
