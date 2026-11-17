package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestErrandPermissionDelivery(t *testing.T) {
	exerciseErrandPermissionDelivery(t, newRepoForSessionTests(t), false)
}

func TestPostgresErrandPermissionDelivery(t *testing.T) {
	exerciseErrandPermissionDelivery(t, newPostgresErrandRepo(t), true)
}

func exerciseErrandPermissionDelivery(t *testing.T, repo *Repository, concurrent bool) {
	t.Helper()
	ctx := context.Background()
	task := newErrandTask(t, repo, "permission-delivery")
	mustCreateErrand(t, repo, sampleErrand("permission-errand", task, "parent", errandScenarioBase))
	_, err := repo.ReserveErrandExecutionGeneration(ctx, "permission-errand", 0)
	require.NoError(t, err)
	bound, err := repo.BindErrandExecution(ctx, "permission-errand", 1, "execution")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &models.ErrandHarnessBinding{ErrandID: "permission-errand", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "permission-stream"}, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, "permission-errand", &models.Turn{ID: "turn", TaskID: task}))
	sub := &models.AgentDeliverySubmission{ID: "submission", Payload: []byte("prompt"), PayloadHash: journal.SubmissionHash([]byte("prompt"))}
	_, err = repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "permission-errand", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "permission-stream", PromptGeneration: 1, TurnID: "turn", Submission: sub})
	require.NoError(t, err)
	agent := streams.AgentEvent{Type: streams.EventTypePermissionRequest, RequestID: "request", PendingID: "provider-pending", ToolCallID: "tool", PermissionTitle: "Allow this?", PermissionHumanInput: true, PermissionOptions: []streams.PermissionOption{{OptionID: "allow", Name: "Allow", Kind: streams.PermissionOptionKindAllowOnce}}, TurnID: "turn", PromptGeneration: 1}
	payload, err := json.Marshal(agent)
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{SessionID: "errand:permission-errand", TaskID: task, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, OwnerGeneration: 1, ExpectedStreamID: "permission-stream", StreamID: "permission-stream", Sequence: 1, SubmissionID: "submission", EventType: agent.Type, Payload: payload}
	wrong := *event
	wrong.ExecutionID = "foreign"
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &wrong, 1)
	require.Error(t, err)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	id := errandPermissionMessageID("permission-errand", "permission-stream", "submission", "turn", "request")
	require.NoError(t, repo.CreateErrandMessage(ctx, "permission-errand", &models.Message{ID: id, TaskID: task, TurnID: "turn", AuthorType: models.MessageAuthorUser, Content: "untouched"}))
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.Error(t, err)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, "permission-stream")
	require.NoError(t, err)
	require.Zero(t, cursor.ProjectedSequence)
	var effects int
	require.NoError(t, repo.db.GetContext(ctx, &effects, repo.db.Rebind(`SELECT COUNT(*) FROM agent_delivery_effects WHERE effect_key=?`), "agent_delivery.errand_permission_owner.v1:"+id))
	require.Zero(t, effects)
	foreign, err := repo.GetErrandMessage(ctx, "permission-errand", id)
	require.NoError(t, err)
	require.Equal(t, "untouched", foreign.Content)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`DELETE FROM errand_messages WHERE errand_id=? AND id=?`), "permission-errand", id)
	require.NoError(t, err)
	projected, err := repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	require.True(t, projected)
	again, err := repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	require.False(t, again)
	messages, _, err := repo.ListErrandMessages(ctx, "permission-errand", "", 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, models.MessageTypePermissionRequest, messages[0].Type)
	require.Equal(t, "request", messages[0].Metadata["request_id"])
	require.Equal(t, "provider-pending", messages[0].Metadata["pending_id"])
	require.Equal(t, true, messages[0].Metadata["permission_human_input"])
	require.True(t, messages[0].RequestsInput)
	require.Empty(t, messages[0].TaskSessionID)
	descriptor, err := repo.GetErrandExecutionDescriptor(ctx, "permission-errand")
	require.NoError(t, err)
	require.Equal(t, "submission", descriptor.ActiveSubmissionID)
	require.Equal(t, "turn", descriptor.ActiveTurnID)
	require.EqualValues(t, 1, descriptor.PromptGeneration)
	pending, err := repo.LatestErrandPendingPermissions(ctx, []string{"permission-errand"})
	require.NoError(t, err)
	require.Equal(t, "request", pending["permission-errand"].Metadata["request_id"])
	resolver, ok := any(repo).(interface {
		ClaimErrandPermissionResolution(context.Context, models.ErrandExecutionDescriptor, models.PermissionResolutionAudit, bool) (*models.PermissionResolutionClaimResult, error)
		FinalizeErrandPermissionResolution(context.Context, models.ErrandExecutionDescriptor, models.ErrandPermissionFinalization) (*models.PermissionResolutionFinalizeResult, error)
	})
	require.True(t, ok, "Errand permission resolution must be available")
	audit := models.PermissionResolutionAudit{ClaimID: "claim-1", RequestID: "request", PendingID: "provider-pending", OptionID: "allow", SelectedAt: errandScenarioBase}
	competing := audit
	competing.ClaimID = "claim-2"
	if concurrent {
		repo.db.SetMaxOpenConns(2)
		type claimResult struct {
			audit  models.PermissionResolutionAudit
			result *models.PermissionResolutionClaimResult
			err    error
		}
		results := make(chan claimResult, 2)
		start := make(chan struct{})
		for _, candidate := range []models.PermissionResolutionAudit{audit, competing} {
			go func(candidate models.PermissionResolutionAudit) {
				<-start
				result, claimErr := resolver.ClaimErrandPermissionResolution(ctx, *descriptor, candidate, false)
				results <- claimResult{candidate, result, claimErr}
			}(candidate)
		}
		close(start)
		seen := map[models.PermissionResolutionClaimOutcome]int{}
		for range 2 {
			result := <-results
			require.NoError(t, result.err)
			seen[result.result.Outcome]++
			if result.result.Outcome == models.PermissionClaimed {
				audit = result.audit
			}
		}
		require.Equal(t, 1, seen[models.PermissionClaimed])
		require.Equal(t, 1, seen[models.PermissionClaimInProgress])
	} else {
		claimed, claimErr := resolver.ClaimErrandPermissionResolution(ctx, *descriptor, audit, false)
		require.NoError(t, claimErr)
		require.Equal(t, models.PermissionClaimed, claimed.Outcome)
		claimed, claimErr = resolver.ClaimErrandPermissionResolution(ctx, *descriptor, competing, false)
		require.NoError(t, claimErr)
		require.Equal(t, models.PermissionClaimInProgress, claimed.Outcome)
	}
	terminalAgent := streams.AgentEvent{Type: streams.EventTypeComplete, TurnID: "turn", PromptGeneration: 1}
	terminalPayload, err := json.Marshal(terminalAgent)
	require.NoError(t, err)
	terminal := *event
	terminal.Sequence, terminal.EventType, terminal.Terminal, terminal.Payload = 2, terminalAgent.Type, true, terminalPayload
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &terminal, 2)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, &terminal, nil)
	require.NoError(t, err)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, "permission-stream", 2, models.DeliverySubmissionCompleted, time.Time{})
	require.NoError(t, err)
	require.True(t, settled)
	nextPrompt := &models.AgentDeliverySubmission{ID: "submission-next", Payload: []byte("next prompt"), PayloadHash: journal.SubmissionHash([]byte("next prompt"))}
	_, err = repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "permission-errand", Generation: 1, ExecutionID: "execution", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "permission-stream", PromptGeneration: 2, TurnID: "turn", Submission: nextPrompt})
	require.NoError(t, err)
	agent.PromptGeneration = 2
	nextPayload, err := json.Marshal(agent)
	require.NoError(t, err)
	nextEvent := *event
	nextEvent.Sequence, nextEvent.SubmissionID, nextEvent.Payload = 3, nextPrompt.ID, nextPayload
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, &nextEvent, 3)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, &nextEvent, nil)
	require.NoError(t, err)
	pending, err = repo.LatestErrandPendingPermissions(ctx, []string{"permission-errand"})
	require.NoError(t, err)
	require.Equal(t, "submission-next", pending["permission-errand"].AuthorID)
	_, err = resolver.ClaimErrandPermissionResolution(ctx, *descriptor, competing, false)
	require.Error(t, err, "old prompt cannot claim after a same-execution successor")
	_, err = repo.ReserveErrandExecutionGeneration(ctx, "permission-errand", 1)
	require.NoError(t, err)
	pending, err = repo.LatestErrandPendingPermissions(ctx, []string{"permission-errand"})
	require.NoError(t, err)
	require.Empty(t, pending)
	_, err = resolver.ClaimErrandPermissionResolution(ctx, *descriptor, competing, false)
	require.Error(t, err)
	bound, err = repo.BindErrandExecution(ctx, "permission-errand", 2, "execution-2")
	require.NoError(t, err)
	require.True(t, bound)
	require.NoError(t, repo.AppendErrandHarnessBinding(ctx, &models.ErrandHarnessBinding{ErrandID: "permission-errand", Generation: 2, ExecutionID: "execution-2", IncarnationID: "incarnation-2", HarnessGeneration: 1, StreamID: "permission-stream-2"}, 0))
	require.NoError(t, repo.CreateErrandTurn(ctx, "permission-errand", &models.Turn{ID: "turn-2", TaskID: task}))
	sub2 := &models.AgentDeliverySubmission{ID: "submission-2", Payload: []byte("next"), PayloadHash: journal.SubmissionHash([]byte("next"))}
	_, err = repo.PrepareErrandDeliverySubmission(ctx, &models.ErrandDeliveryAdmission{ErrandID: "permission-errand", Generation: 2, ExecutionID: "execution-2", IncarnationID: "incarnation-2", HarnessGeneration: 1, StreamID: "permission-stream-2", PromptGeneration: 1, TurnID: "turn-2", Submission: sub2})
	require.NoError(t, err)
	agent.TurnID, agent.PromptGeneration = "turn-2", 1
	payload, err = json.Marshal(agent)
	require.NoError(t, err)
	successor := &models.AgentDeliveryEvent{SessionID: "errand:permission-errand", TaskID: task, ExecutionID: "execution-2", IncarnationID: "incarnation-2", HarnessGeneration: 1, OwnerGeneration: 1, ExpectedStreamID: "permission-stream-2", StreamID: "permission-stream-2", Sequence: 1, SubmissionID: "submission-2", EventType: agent.Type, Payload: payload}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, successor, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, successor, nil)
	require.NoError(t, err)
	pending, err = repo.LatestErrandPendingPermissions(ctx, []string{"permission-errand"})
	require.NoError(t, err)
	require.Equal(t, "submission-2", pending["permission-errand"].AuthorID)
	agent.RequestID, agent.PendingID = "automatic-request", "automatic-pending"
	agent.PermissionHumanInput = false
	agent.AutoApprovedOptionID, agent.AutoApprovalPending = "allow", true
	successor.Sequence = 2
	successor.Payload, err = json.Marshal(agent)
	require.NoError(t, err)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, successor, 2)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, successor, nil)
	require.NoError(t, err)
	automaticID := errandPermissionMessageID("permission-errand", "permission-stream-2", "submission-2", "turn-2", "automatic-request")
	automatic, err := repo.GetErrandMessage(ctx, "permission-errand", automaticID)
	require.NoError(t, err)
	require.Equal(t, "allow", automatic.Metadata["auto_approved_option_id"])
	require.Equal(t, true, automatic.Metadata["auto_approval_pending"])
	pending, err = repo.LatestErrandPendingPermissions(ctx, []string{"permission-errand"})
	require.NoError(t, err)
	require.Empty(t, pending)
	agent.RequestID, agent.PendingID = "human-request", "human-pending"
	agent.PermissionHumanInput = true
	successor.Sequence = 3
	successor.Payload, err = json.Marshal(agent)
	require.NoError(t, err)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, successor, 3)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, successor, nil)
	require.NoError(t, err)
	pending, err = repo.LatestErrandPendingPermissions(ctx, []string{"permission-errand"})
	require.NoError(t, err)
	require.Equal(t, "human-request", pending["permission-errand"].Metadata["request_id"])
	currentOwner, err := repo.GetErrandExecutionDescriptor(ctx, "permission-errand")
	require.NoError(t, err)
	autoAudit := models.PermissionResolutionAudit{ClaimID: "auto-claim", RequestID: "human-request", PendingID: "human-pending"}
	autoClaim, err := resolver.ClaimErrandPermissionResolution(ctx, *currentOwner, autoAudit, true)
	require.NoError(t, err)
	require.NotEqual(t, models.PermissionClaimed, autoClaim.Outcome)
	olderAudit := models.PermissionResolutionAudit{ClaimID: "older-current-claim", RequestID: "request", PendingID: "provider-pending"}
	olderClaim, err := resolver.ClaimErrandPermissionResolution(ctx, *currentOwner, olderAudit, false)
	require.NoError(t, err)
	require.Equal(t, models.PermissionClaimed, olderClaim.Outcome)
	finalizedAt := errandScenarioBase.Add(time.Second)
	finalized, err := resolver.FinalizeErrandPermissionResolution(ctx, *descriptor, models.ErrandPermissionFinalization{RequestID: audit.RequestID, PendingID: audit.PendingID, ClaimID: audit.ClaimID, Result: models.PermissionResolutionAccepted, Status: models.PermissionStatusApproved, FinalizedAt: finalizedAt})
	require.NoError(t, err)
	require.Equal(t, models.PermissionFinalized, finalized.Outcome)
	pending, err = repo.LatestErrandPendingPermissions(ctx, []string{"permission-errand"})
	require.NoError(t, err)
	require.Equal(t, "submission-2", pending["permission-errand"].AuthorID)
	malformed := pending["permission-errand"]
	malformed.Metadata["permission_resolution"] = map[string]interface{}{"result": "indeterminate"}
	require.NoError(t, repo.UpdateErrandMessage(ctx, "permission-errand", malformed))
	humanAudit := models.PermissionResolutionAudit{ClaimID: "human-claim", RequestID: "human-request", PendingID: "human-pending"}
	_, err = resolver.ClaimErrandPermissionResolution(ctx, *currentOwner, humanAudit, false)
	require.Error(t, err, "an existing malformed resolution must not be reopened")
	agent.RequestID, agent.PendingID = "approved-request", "approved-pending"
	agent.PermissionHumanInput, agent.AutoApprovalPending = false, false
	successor.Sequence = 4
	successor.Payload, err = json.Marshal(agent)
	require.NoError(t, err)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, successor, 4)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, successor, nil)
	require.NoError(t, err)
	approvedID := errandPermissionMessageID("permission-errand", "permission-stream-2", "submission-2", "turn-2", "approved-request")
	approved, err := repo.GetErrandMessage(ctx, "permission-errand", approvedID)
	require.NoError(t, err)
	require.Equal(t, "approved", approved.Metadata["status"])
	require.False(t, approved.RequestsInput)
	agent.RequestID, agent.PendingID = "uncertain-request", "uncertain-pending"
	agent.AutoApprovedOptionID = ""
	successor.Sequence = 5
	successor.Payload, err = json.Marshal(agent)
	require.NoError(t, err)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, successor, 5)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, successor, nil)
	require.NoError(t, err)
	uncertainAudit := models.PermissionResolutionAudit{ClaimID: "uncertain-claim", RequestID: "uncertain-request", PendingID: "uncertain-pending", ActorKind: models.PermissionActorBrowser, OptionID: "allow"}
	uncertainClaim, err := resolver.ClaimErrandPermissionResolution(ctx, *currentOwner, uncertainAudit, false)
	require.NoError(t, err)
	require.Equal(t, models.PermissionClaimed, uncertainClaim.Outcome)
	uncertain, err := resolver.FinalizeErrandPermissionResolution(ctx, *currentOwner, models.ErrandPermissionFinalization{ClaimID: uncertainAudit.ClaimID, RequestID: uncertainAudit.RequestID, PendingID: uncertainAudit.PendingID, Result: models.PermissionResolutionIndeterminate, FinalizedAt: finalizedAt})
	require.NoError(t, err)
	require.Equal(t, models.PermissionFinalized, uncertain.Outcome)
	storedAudit, ok := permissionAuditFromMetadata(uncertain.Message.Metadata)
	require.True(t, ok)
	require.Equal(t, models.PermissionResolutionIndeterminate, storedAudit.Result)
	require.Equal(t, models.PermissionActorBrowser, storedAudit.ActorKind)
	require.Equal(t, "allow", storedAudit.OptionID)
	retry, err := resolver.ClaimErrandPermissionResolution(ctx, *currentOwner, uncertainAudit, false)
	require.NoError(t, err)
	require.Equal(t, models.PermissionClaimAlreadyFinal, retry.Outcome)
	agent.RequestID, agent.PendingID = "cancel-request", "cancel-pending"
	agent.AutoApprovedOptionID = ""
	successor.Sequence = 6
	successor.Payload, err = json.Marshal(agent)
	require.NoError(t, err)
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, successor, 6)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, successor, nil)
	require.NoError(t, err)
	pending, err = repo.LatestErrandPendingPermissions(ctx, []string{"permission-errand"})
	require.NoError(t, err)
	require.Equal(t, "cancel-request", pending["permission-errand"].Metadata["request_id"])
	ids := make([]string, 401)
	for i := range ids {
		ids[i] = fmt.Sprintf("missing-%d", i)
	}
	ids = append(ids, "permission-errand")
	pending, err = repo.LatestErrandPendingPermissions(ctx, ids)
	require.NoError(t, err)
	require.Equal(t, "cancel-request", pending["permission-errand"].Metadata["request_id"])
	cancelled, err := repo.GetErrand(ctx, "permission-errand")
	require.NoError(t, err)
	cancelled.Lifecycle, cancelled.CancelSource = models.ErrandLifecycleCancelled, models.ErrandCancelSourceUser
	require.NoError(t, repo.UpdateErrand(ctx, cancelled))
	stillActive, err := repo.GetErrandExecutionDescriptor(ctx, "permission-errand")
	require.NoError(t, err)
	require.Equal(t, "submission-2", stillActive.ActiveSubmissionID)
	pending, err = repo.LatestErrandPendingPermissions(ctx, []string{"permission-errand"})
	require.NoError(t, err)
	require.Empty(t, pending)
	_, err = resolver.ClaimErrandPermissionResolution(ctx, *currentOwner, models.PermissionResolutionAudit{ClaimID: "cancel-claim", RequestID: "cancel-request", PendingID: "cancel-pending"}, false)
	require.Error(t, err)
	finished, err := resolver.FinalizeErrandPermissionResolution(ctx, *currentOwner, models.ErrandPermissionFinalization{ClaimID: olderAudit.ClaimID, RequestID: olderAudit.RequestID, PendingID: olderAudit.PendingID, Result: models.PermissionResolutionAccepted, Status: models.PermissionStatusApproved, FinalizedAt: finalizedAt})
	require.NoError(t, err)
	require.Equal(t, models.PermissionFinalized, finished.Outcome)
}
