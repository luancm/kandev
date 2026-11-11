package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/stretchr/testify/require"
)

func TestMessageAdapterExposesErrandContinuationAdmission(t *testing.T) {
	_, ok := any(&orchestratorWrapper{svc: &orchestrator.Service{}}).(interface {
		ValidateErrandContinuation(context.Context, string) error
	})
	require.True(t, ok, "browser admission must reach the real orchestrator check")
}
