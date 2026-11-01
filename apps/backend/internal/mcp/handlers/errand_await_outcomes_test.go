package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestAwaitRequiresAuthenticatedLaunchedParentOwner(t *testing.T) {
	for _, field := range []string{"missing", "task", "parent", "incarnation", "generation", "submission-owner"} {
		t.Run(field, func(t *testing.T) {
			h, fake, task, parent, _ := newErrandMCPFixture(t, true)
			owner := streams.MCPExecutionContext{TaskID: task.ID, SessionID: parent.ID, ParentIncarnationID: "launched", HarnessGeneration: 2}
			switch field {
			case "task":
				owner.TaskID = "foreign"
			case "parent":
				owner.SessionID = "foreign"
			case "incarnation":
				owner.ParentIncarnationID = ""
			case "generation":
				owner.HarnessGeneration = 0
			}
			ctx := streams.WithMCPExecutionContext(t.Context(), owner)
			if field == "missing" {
				ctx = t.Context()
			}
			if field == "submission-owner" {
				ctx = mcporigin.WithAwaitInvocation(ctx, models.ErrandAwaitIdentity{TaskID: task.ID, ParentSessionID: parent.ID, Token: "token", ParentIncarnationID: "foreign", HarnessGeneration: 2})
			}
			msg := makeWSMessage(t, ws.ActionMCPAwaitErrands, errandSenderArgs(task, parent, map[string]interface{}{"errand_ids": []string{"child"}, "mode": "all", "await_token": "token", "parent_incarnation_id": "forged", "harness_generation": 99}))
			response, err := h.handleAwaitErrands(ctx, msg)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)
			require.Empty(t, fake.awaitReq.ParentSessionID)
		})
	}
}

type awaitOfferFailure struct {
	*fakeErrandService
	cancel context.CancelFunc
}

func (s *awaitOfferFailure) OfferAwait(context.Context, models.ErrandAwaitIdentity, models.ErrandAwaitDelivery) error {
	if s.cancel != nil {
		s.cancel()
		return nil
	}
	return errors.New("offer commit outcome unknown")
}
func TestAwaitOfferFailureOrCancellationSelectsError(t *testing.T) {
	for _, cancelDuringOffer := range []bool{false, true} {
		t.Run(map[bool]string{false: "offer-failed", true: "cancelled"}[cancelDuringOffer], func(t *testing.T) {
			h, fake, task, parent, _ := newErrandMCPFixture(t, true)
			ctx, cancel := context.WithCancel(awaitTestContext(task.ID, parent.ID))
			defer cancel()
			service := &awaitOfferFailure{fakeErrandService: fake}
			if cancelDuringOffer {
				service.cancel = cancel
			}
			h.errands = service
			msg := makeWSMessage(t, ws.ActionMCPAwaitErrands, errandSenderArgs(task, parent, map[string]interface{}{"errand_ids": []string{"child"}, "mode": "all", "await_token": "token"}))
			response, err := h.handleAwaitErrands(ctx, msg)
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)
			require.Equal(t, 1, fake.releaseCalls)
			require.Equal(t, "token", fake.releaseToken)
		})
	}
}
func TestAwaitAbortRetainedOwnerSurvivesDisabledAndNewGeneration(t *testing.T) {
	h, fake, task, parent, _ := newErrandMCPFixture(t, false)
	original := models.ErrandAwaitIdentity{TaskID: task.ID, ParentSessionID: parent.ID, Token: "token", ParentIncarnationID: "prior", HarnessGeneration: 1, SubmissionID: "original"}
	ctx := mcporigin.WithTrustedInternalCall(mcporigin.WithAwaitInvocation(awaitTestContext(task.ID, parent.ID), original))
	msg := makeWSMessage(t, ws.ActionMCPAwaitErrandsRelease, errandSenderArgs(task, parent, map[string]interface{}{"await_token": "token"}))
	response, err := h.handleAwaitErrandsRelease(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, 1, fake.releaseCalls)
	require.Equal(t, original, fake.abortOwner)
}
