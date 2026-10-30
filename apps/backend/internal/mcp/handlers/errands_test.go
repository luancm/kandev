package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// fakeErrandService records the errand calls the MCP handlers make.
type fakeErrandService struct {
	startReq       errands.StartRequest
	startCalls     int
	startErr       error
	awaitReq       errands.AwaitRequest
	awaitResult    errands.AwaitResult
	awaitErr       error
	releaseToken   string
	releaseCalls   int
	cancelReq      errands.CancelRequest
	cancelStatus   models.ErrandStatus
	outstanding    bool
	outstandingErr error
	listed         []errands.Snapshot
	listErr        error
}

func (f *fakeErrandService) Start(_ context.Context, req errands.StartRequest) (*errands.StartResult, error) {
	f.startCalls++
	f.startReq = req
	if f.startErr != nil {
		return nil, f.startErr
	}
	return &errands.StartResult{
		Snapshot: errands.Snapshot{
			Errand: &models.Errand{ID: "errand-1", AgentProfileID: "profile-1"},
			Status: models.ErrandStatusQueued,
		},
		ProfileName: "Claude",
	}, nil
}

func (f *fakeErrandService) Await(_ context.Context, req errands.AwaitRequest) (errands.AwaitResult, error) {
	f.awaitReq = req
	return f.awaitResult, f.awaitErr
}

func (f *fakeErrandService) ReleaseAwait(_ context.Context, token string) error {
	f.releaseCalls++
	f.releaseToken = token
	return nil
}

func (f *fakeErrandService) Cancel(_ context.Context, req errands.CancelRequest) (*errands.Snapshot, error) {
	f.cancelReq = req
	return &errands.Snapshot{Status: f.cancelStatus}, nil
}

func (f *fakeErrandService) ParentOutstanding(_ context.Context, _ string) (bool, error) {
	return f.outstanding, f.outstandingErr
}

func (f *fakeErrandService) List(_ context.Context, _ string) ([]errands.Snapshot, error) {
	return f.listed, f.listErr
}

// newErrandMCPFixture seeds a task with a primary session and returns the
// handlers wired to a fake errand service with the given release toggle.
func newErrandMCPFixture(t *testing.T, enabled bool) (*Handlers, *fakeErrandService, *models.Task, *models.TaskSession, seedRepo) {
	t.Helper()
	svc, repo := newTestTaskService(t)
	task, primary, _ := seedTaskWithSessions(t, svc, repo, 0)
	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	fake := &fakeErrandService{}
	h.SetErrands(fake, enabled)
	return h, fake, task, primary, repo
}

// seedErrandSession inserts an errand session under the parent session.
func seedErrandSession(t *testing.T, repo seedRepo, task *models.Task, parent *models.TaskSession, id string) *models.TaskSession {
	t.Helper()
	session := &models.TaskSession{
		ID:              id,
		TaskID:          task.ID,
		AgentProfileID:  "profile-1",
		Role:            models.SessionRoleErrand,
		ParentSessionID: parent.ID,
		State:           models.TaskSessionStateRunning,
		StartedAt:       time.Now(),
	}
	require.NoError(t, repo.CreateTaskSession(context.Background(), session))
	return session
}

func errandSenderArgs(task *models.Task, session *models.TaskSession, extra map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"sender_task_id":    task.ID,
		"sender_session_id": session.ID,
	}
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

func errandErrorReason(t *testing.T, resp *ws.Message) string {
	t.Helper()
	require.Equal(t, ws.MessageTypeError, resp.Type)
	var body struct {
		Details map[string]interface{} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	reason, _ := body.Details["reason"].(string)
	return reason
}

func TestStartErrandRefusesWhenFeatureDisabled(t *testing.T) {
	h, fake, task, primary, _ := newErrandMCPFixture(t, false)
	msg := makeWSMessage(t, ws.ActionMCPStartErrand, errandSenderArgs(task, primary, map[string]interface{}{
		"task_id": task.ID, "title": "Survey", "prompt": "Look around",
	}))

	resp, err := h.handleStartErrand(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
	assert.Equal(t, string(errands.ReasonFeatureDisabled), errandErrorReason(t, resp))
	assert.Equal(t, 0, fake.startCalls)
}

func TestStartErrandRefusesWhenNoErrandsService(t *testing.T) {
	h, _, task, primary, _ := newErrandMCPFixture(t, true)
	h.errands = nil
	msg := makeWSMessage(t, ws.ActionMCPStartErrand, errandSenderArgs(task, primary, map[string]interface{}{
		"task_id": task.ID, "title": "Survey", "prompt": "Look around",
	}))

	resp, err := h.handleStartErrand(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, string(errands.ReasonFeatureDisabled), errandErrorReason(t, resp))
}

func TestStartErrandForwardsVerifiedParentSession(t *testing.T) {
	h, fake, task, primary, _ := newErrandMCPFixture(t, true)
	msg := makeWSMessage(t, ws.ActionMCPStartErrand, errandSenderArgs(task, primary, map[string]interface{}{
		"task_id": task.ID, "title": "Survey", "prompt": "Look around", "agent_profile": "Claude",
	}))

	resp, err := h.handleStartErrand(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.Equal(t, errands.StartRequest{
		TaskID:          task.ID,
		ParentSessionID: primary.ID,
		Title:           "Survey",
		Objective:       "Look around",
		Profile:         "Claude",
	}, fake.startReq)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	assert.Equal(t, "errand-1", body["errand_id"])
	assert.Equal(t, "errand-1", body["session_id"])
	assert.Equal(t, "queued", body["status"])
	assert.Equal(t, "Claude", body["agent_profile_name"])
}

func TestStartErrandRefusesErrandOrUnverifiedSender(t *testing.T) {
	h, fake, task, primary, repo := newErrandMCPFixture(t, true)
	errandSession := seedErrandSession(t, repo, task, primary, "errand-sess-1")
	args := map[string]interface{}{"task_id": task.ID, "title": "Survey", "prompt": "Look around"}

	resp, err := h.handleStartErrand(context.Background(), makeWSMessage(t, ws.ActionMCPStartErrand,
		errandSenderArgs(task, errandSession, args)))
	require.NoError(t, err)
	assert.Equal(t, string(errands.ReasonNotSupported), errandErrorReason(t, resp))

	forged := errandSenderArgs(task, primary, args)
	forged["sender_task_id"] = "some-other-task"
	resp, err = h.handleStartErrand(context.Background(), makeWSMessage(t, ws.ActionMCPStartErrand, forged))
	require.NoError(t, err)
	assert.Equal(t, string(errands.ReasonNotSupported), errandErrorReason(t, resp))
	assert.Equal(t, 0, fake.startCalls)
}

func TestAwaitErrandsForwardsParentAndReturnsView(t *testing.T) {
	h, fake, task, primary, _ := newErrandMCPFixture(t, true)
	fake.awaitResult = errands.AwaitResult{
		Token: "token-1", DurationMS: 12000, EndReason: "all_ready",
		Errands: []errands.AwaitErrand{
			{ErrandID: "errand-1", Status: "done", Result: "found it"},
			{ErrandID: "errand-2", Status: "done", DeliveredVia: "turn"},
		},
	}
	msg := makeWSMessage(t, ws.ActionMCPAwaitErrands, errandSenderArgs(task, primary, map[string]interface{}{
		"errand_ids": []string{"errand-1", "errand-2"}, "mode": "all", "max_wait_seconds": 30,
	}))

	resp, err := h.handleAwaitErrands(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.Equal(t, primary.ID, fake.awaitReq.ParentSessionID)
	assert.Equal(t, []string{"errand-1", "errand-2"}, fake.awaitReq.ErrandIDs)
	assert.Equal(t, "all", fake.awaitReq.Mode)
	assert.Equal(t, 30, fake.awaitReq.MaxWaitSeconds)

	var body struct {
		Errands    []map[string]interface{} `json:"errands"`
		DurationMS int64                    `json:"duration_ms"`
		EndReason  string                   `json:"end_reason"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.Equal(t, int64(12000), body.DurationMS)
	require.Equal(t, "all_ready", body.EndReason)
	require.Len(t, body.Errands, 2)
	assert.Equal(t, "found it", body.Errands[0]["result"])
	assert.NotContains(t, body.Errands[0], "delivered_via")
	assert.Equal(t, "turn", body.Errands[1]["delivered_via"])
	assert.NotContains(t, body.Errands[1], "result")
}

func TestAwaitErrandsRefusesErrandSender(t *testing.T) {
	h, fake, task, primary, repo := newErrandMCPFixture(t, true)
	errandSession := seedErrandSession(t, repo, task, primary, "errand-sess-1")
	msg := makeWSMessage(t, ws.ActionMCPAwaitErrands, errandSenderArgs(task, errandSession, map[string]interface{}{
		"errand_ids": []string{"errand-1"}, "mode": "any",
	}))

	resp, err := h.handleAwaitErrands(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, string(errands.ReasonNotPermitted), errandErrorReason(t, resp))
	assert.Equal(t, "", fake.awaitReq.ParentSessionID, "the service must not be called")
}

func TestAwaitErrandsReleaseReturnsClaimWithToken(t *testing.T) {
	h, fake, task, primary, _ := newErrandMCPFixture(t, true)
	msg := makeWSMessage(t, ws.ActionMCPAwaitErrandsRelease, errandSenderArgs(task, primary, map[string]interface{}{
		"await_token": "token-1",
	}))

	resp, err := h.handleAwaitErrandsRelease(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.Equal(t, 1, fake.releaseCalls)
	assert.Equal(t, "token-1", fake.releaseToken)
}

func TestCancelErrandUsesParentSourceAndReturnsStatus(t *testing.T) {
	h, fake, task, primary, _ := newErrandMCPFixture(t, true)
	fake.cancelStatus = models.ErrandStatusCancelled
	msg := makeWSMessage(t, ws.ActionMCPCancelErrand, errandSenderArgs(task, primary, map[string]interface{}{
		"errand_id": "errand-1",
	}))

	resp, err := h.handleCancelErrand(context.Background(), msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	assert.Equal(t, errands.CancelRequest{
		ErrandID:        "errand-1",
		TaskID:          task.ID,
		ParentSessionID: primary.ID,
		Source:          models.ErrandCancelSourceParent,
	}, fake.cancelReq)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	assert.Equal(t, "errand-1", body["errand_id"])
	assert.Equal(t, "cancelled", body["status"])
}

func TestCancelErrandRefusesErrandSenderAndDisabledToggle(t *testing.T) {
	h, fake, task, primary, repo := newErrandMCPFixture(t, true)
	errandSession := seedErrandSession(t, repo, task, primary, "errand-sess-1")
	msg := makeWSMessage(t, ws.ActionMCPCancelErrand, errandSenderArgs(task, errandSession, map[string]interface{}{
		"errand_id": "errand-1",
	}))
	resp, err := h.handleCancelErrand(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, string(errands.ReasonNotPermitted), errandErrorReason(t, resp))

	h.errandsEnabled = false
	msg = makeWSMessage(t, ws.ActionMCPCancelErrand, errandSenderArgs(task, primary, map[string]interface{}{
		"errand_id": "errand-1",
	}))
	resp, err = h.handleCancelErrand(context.Background(), msg)
	require.NoError(t, err)
	assert.Equal(t, string(errands.ReasonFeatureDisabled), errandErrorReason(t, resp))
	assert.Equal(t, models.ErrandCancelSource(""), fake.cancelReq.Source)
}

func TestErrandCallerRefusalOnlyForErrandSessions(t *testing.T) {
	h, _, task, primary, repo := newErrandMCPFixture(t, true)
	errandSession := seedErrandSession(t, repo, task, primary, "errand-sess-1")
	ctx := context.Background()
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{})

	refusal := h.errandCallerRefusal(ctx, msg, errandSession.ID, "errand sessions cannot move tasks")
	require.NotNil(t, refusal)
	assert.Equal(t, string(errands.ReasonNotPermitted), errandErrorReason(t, refusal))

	assert.Nil(t, h.errandCallerRefusal(ctx, msg, primary.ID, "unused"))
	assert.Nil(t, h.errandCallerRefusal(ctx, msg, "", "unused"))
	assert.Nil(t, h.errandCallerRefusal(ctx, msg, "missing-session", "unused"))
}

func TestErrandMessageRefusalRequiresParentSender(t *testing.T) {
	h, _, task, primary, repo := newErrandMCPFixture(t, true)
	errandSession := seedErrandSession(t, repo, task, primary, "errand-sess-1")
	ctx := context.Background()
	msg := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{})

	assert.Nil(t, h.errandMessageRefusal(ctx, msg, primary.ID, errandSession.ID), "the parent may message its errand")
	assert.Nil(t, h.errandMessageRefusal(ctx, msg, primary.ID, primary.ID), "an ordinary target is not gated")

	refusal := h.errandMessageRefusal(ctx, msg, "other-session", errandSession.ID)
	require.NotNil(t, refusal)
	assert.Equal(t, string(errands.ReasonSenderNotParent), errandErrorReason(t, refusal))
}

func TestErrandOutstandingRefusalBlocksStepCompletion(t *testing.T) {
	h, fake, _, primary, _ := newErrandMCPFixture(t, true)
	ctx := context.Background()
	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{})

	fake.outstanding = true
	refusal := h.errandOutstandingRefusal(ctx, msg, primary.ID)
	require.NotNil(t, refusal)
	assertWSError(t, refusal, ws.ErrorCodeConflict)
	assert.Equal(t, string(errands.ReasonErrandsOutstanding), errandErrorReason(t, refusal))

	fake.outstanding = false
	assert.Nil(t, h.errandOutstandingRefusal(ctx, msg, primary.ID))

	fake.outstandingErr = errors.New("store down")
	refusal = h.errandOutstandingRefusal(ctx, msg, primary.ID)
	require.NotNil(t, refusal, "a failed read must fail closed")
	assertWSError(t, refusal, ws.ErrorCodeInternalError)
}

func TestListTaskSessionsReportsErrandRoleAndStatus(t *testing.T) {
	h, fake, task, primary, repo := newErrandMCPFixture(t, true)
	errandSession := seedErrandSession(t, repo, task, primary, "errand-sess-1")
	fake.listed = []errands.Snapshot{{
		Errand: &models.Errand{ID: errandSession.ID},
		Status: models.ErrandStatusRunning,
	}}

	resp, err := h.handleListTaskSessions(context.Background(), makeWSMessage(t, ws.ActionMCPListTaskSessions,
		map[string]interface{}{"task_id": task.ID}))
	require.NoError(t, err)
	entries := sessionEntries(t, listSessionsPayload(t, resp))

	byID := map[string]map[string]interface{}{}
	for _, entry := range entries {
		byID[entry["session_id"].(string)] = entry
	}
	errandEntry := byID[errandSession.ID]
	assert.Equal(t, "errand", errandEntry["role"])
	assert.Equal(t, primary.ID, errandEntry["parent_session_id"])
	assert.Equal(t, "running", errandEntry["errand_status"])
	assert.NotContains(t, byID[primary.ID], "role")
	assert.NotContains(t, byID[primary.ID], "errand_status")
}

func TestListTaskSessionsOmitsErrandStatusWhenReadFails(t *testing.T) {
	h, fake, task, primary, repo := newErrandMCPFixture(t, true)
	errandSession := seedErrandSession(t, repo, task, primary, "errand-sess-1")
	fake.listErr = errors.New("store down")

	resp, err := h.handleListTaskSessions(context.Background(), makeWSMessage(t, ws.ActionMCPListTaskSessions,
		map[string]interface{}{"task_id": task.ID}))
	require.NoError(t, err)
	entries := sessionEntries(t, listSessionsPayload(t, resp))
	require.Len(t, entries, 2)
	for _, entry := range entries {
		if entry["session_id"] == errandSession.ID {
			assert.NotContains(t, entry, "errand_status")
			assert.Equal(t, "errand", entry["role"])
		}
	}
}

func TestMessageTaskGuardsErrandSenderAndNonParentSender(t *testing.T) {
	svc, repo := newTestTaskService(t)
	task, primary, _ := seedTaskWithSessions(t, svc, repo, 0)
	h, _ := newMessageTaskHandler(t, svc, repo)
	ctx := context.Background()
	errandSession := seedErrandSession(t, repo, task, primary, "errand-sess-msg")
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-sibling", TaskID: task.ID, AgentProfileID: "profile-1",
		State: models.TaskSessionStateRunning, StartedAt: time.Now(),
	}))

	fromErrand := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": task.ID, "session_id": primary.ID, "prompt": "hello",
		"sender_task_id": task.ID, "sender_session_id": errandSession.ID,
	})
	resp, err := h.handleMessageTask(ctx, fromErrand)
	require.NoError(t, err)
	assert.Equal(t, string(errands.ReasonNotPermitted), errandErrorReason(t, resp))

	toErrand := makeWSMessage(t, ws.ActionMCPMessageTask, map[string]interface{}{
		"task_id": task.ID, "session_id": errandSession.ID, "prompt": "hello",
		"sender_task_id": task.ID, "sender_session_id": "sess-sibling",
	})
	resp, err = h.handleMessageTask(ctx, toErrand)
	require.NoError(t, err)
	assert.Equal(t, string(errands.ReasonSenderNotParent), errandErrorReason(t, resp))
}

func TestWrapMessageForErrandTargetStatesResultContract(t *testing.T) {
	task := &models.Task{ID: "task-1", Title: "Parent task"}
	errandSession := &models.TaskSession{ID: "errand-sess-1", Role: models.SessionRoleErrand}

	wrapped, meta := wrapMessageForTarget("Check the logs", task, errandSession, "parent-sess", "", true)
	assert.Contains(t, wrapped, "Your result is your final message in this turn")
	assert.NotContains(t, wrapped, "To reply")
	assert.Contains(t, wrapped, "Check the logs")
	assert.NotNil(t, meta)

	ordinary := &models.TaskSession{ID: "sess-2"}
	wrapped, _ = wrapMessageForTarget("Check the logs", task, ordinary, "parent-sess", "", true)
	assert.Contains(t, wrapped, "To reply")
	assert.NotContains(t, wrapped, "Your result is your final message")
}
