package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type admissionWithoutAcceptanceManager struct{ *retentionPromptAgentManager }

func (m *admissionWithoutAcceptanceManager) PromptAgentWithAdmissionCallback(ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, before func(context.Context, string, []v1.MessageAttachment) error, _ func()) (*executor.PromptResult, error) {
	if err := before(ctx, prompt, attachments); err != nil {
		return nil, err
	}
	return m.PromptAgent(ctx, executionID, prompt, attachments, dispatchOnly)
}

func (m *admissionWithoutAcceptanceManager) PromptAgentWithAdmissionCallbackAndSubmissionID(ctx context.Context, executionID, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, before func(context.Context, string, []v1.MessageAttachment) error, accepted func(), _ string) (*executor.PromptResult, error) {
	return m.PromptAgentWithAdmissionCallback(ctx, executionID, prompt, attachments, dispatchOnly, before, accepted)
}

func TestErrandAdmissionMissingAcceptanceCannotReturnSuccess(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolLegacy, messagequeue.DeliveryProtocolV1} {
		t.Run(protocol, func(t *testing.T) {
			f := newAdmissionIntegrationFixture(t, protocol)
			manager := &admissionWithoutAcceptanceManager{retentionPromptAgentManager: f.manager}
			f.svc.agentManager = manager
			f.svc.executor = executor.NewExecutor(manager, f.repo, testLogger(), executor.ExecutorConfig{})
			require.Error(t, dispatchAdmissionFixture(t, f, f.options(protocol, true)), "missing acceptance must retain uncertain work")
			replay, err := f.repo.ReserveErrandParentAdmission(t.Context(), fixtureAdmission(f))
			require.NoError(t, err)
			require.Equal(t, models.ErrandAdmissionUnknown, replay.State)
		})
	}
}
