package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type fakeErrandService struct {
	snapshots  []errands.Snapshot
	errand     *models.Errand
	listed     []string
	cancelled  []errands.CancelRequest
	cancelAll  []string
	cancelErr  error
	cancelAllN int
	cancelAllE error
}

func (f *fakeErrandService) List(_ context.Context, taskID string) ([]errands.Snapshot, error) {
	f.listed = append(f.listed, taskID)
	return f.snapshots, nil
}

func (f *fakeErrandService) Lookup(_ context.Context, id string) (*models.Errand, error) {
	if f.errand == nil || f.errand.ID != id {
		return nil, errors.New("errand_not_found: " + id)
	}
	return f.errand, nil
}

func (f *fakeErrandService) Cancel(_ context.Context, req errands.CancelRequest) (*errands.Snapshot, error) {
	f.cancelled = append(f.cancelled, req)
	if f.cancelErr != nil {
		return nil, f.cancelErr
	}
	return &errands.Snapshot{Errand: f.errand, Status: models.ErrandStatusCancelled}, nil
}

func (f *fakeErrandService) CancelAll(_ context.Context, taskID string) (int, error) {
	f.cancelAll = append(f.cancelAll, taskID)
	return f.cancelAllN, f.cancelAllE
}

func errandMessage(t *testing.T, action string, payload interface{}) *ws.Message {
	t.Helper()
	msg, err := ws.NewRequest("req-1", action, payload)
	require.NoError(t, err)
	return msg
}

func errandHandlers(t *testing.T, svc ErrandService, enabled bool) *TaskHandlers {
	t.Helper()
	h := newForeignSessionHandlers(t)
	h.SetErrands(svc, enabled)
	return h
}

func decodeError(t *testing.T, response *ws.Message) ws.ErrorPayload {
	t.Helper()
	require.Equal(t, ws.MessageTypeError, response.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	return payload
}

func sampleErrand() *models.Errand {
	now := time.Date(2026, time.October, 26, 9, 0, 0, 0, time.UTC)
	return &models.Errand{
		ID: "errand-1", TaskID: "task-b", ParentSessionID: "parent-1", Title: "Audit", Objective: "audit",
		Lifecycle: models.ErrandLifecycleLaunched, LastOutcome: models.ErrandOutcomeDone,
		LatestSettledTurnID: "turn-2", ResultTurnID: "turn-1", ResultState: models.ErrandResultDelivered,
		CreatedAt: now, UpdatedAt: now,
	}
}

func errandCallerAs(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, Role: authn.RoleMember})
}

// @covers AC-TASKS-ERRANDS-012.2
func TestErrandRequestsFailWithFeatureDisabledWhileTheFlagIsOff(t *testing.T) {
	svc := &fakeErrandService{errand: sampleErrand()}
	h := errandHandlers(t, svc, false)
	for _, tc := range []struct {
		action  string
		payload map[string]string
		handler func(context.Context, *ws.Message) (*ws.Message, error)
	}{
		{ws.ActionErrandList, map[string]string{"task_id": "task-b"}, h.wsListErrands},
		{ws.ActionErrandCancel, map[string]string{"session_id": "errand-1"}, h.wsCancelErrand},
		{ws.ActionErrandCancelAll, map[string]string{"task_id": "task-b"}, h.wsCancelAllErrands},
	} {
		t.Run(tc.action, func(t *testing.T) {
			response, err := tc.handler(context.Background(), errandMessage(t, tc.action, tc.payload))
			require.NoError(t, err)
			payload := decodeError(t, response)
			require.Equal(t, ws.ErrorCodeNotFound, payload.Code)
			require.Equal(t, "feature_disabled", payload.Details["reason"])
		})
	}
	require.Empty(t, svc.listed)
	require.Empty(t, svc.cancelled)
	require.Empty(t, svc.cancelAll)
}

func TestErrandListReturnsTheTasksErrandsWithCanSendResult(t *testing.T) {
	svc := &fakeErrandService{snapshots: []errands.Snapshot{{Errand: sampleErrand(), Status: models.ErrandStatusDone}}}
	h := errandHandlers(t, svc, true)

	response, err := h.wsListErrands(errandCallerAs("user-b"), errandMessage(t, ws.ActionErrandList, map[string]string{"task_id": "task-b"}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var body struct {
		Errands []map[string]interface{} `json:"errands"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &body))
	require.Len(t, body.Errands, 1)
	require.Equal(t, "errand-1", body.Errands[0]["errand_id"])
	require.Equal(t, "done", body.Errands[0]["status"])
	require.Equal(t, true, body.Errands[0]["can_send_result"])
	require.Equal(t, []string{"task-b"}, svc.listed)
}

func TestErrandListReturnsAnEmptyArrayNotNull(t *testing.T) {
	h := errandHandlers(t, &fakeErrandService{}, true)
	response, err := h.wsListErrands(errandCallerAs("user-b"), errandMessage(t, ws.ActionErrandList, map[string]string{"task_id": "task-b"}))
	require.NoError(t, err)
	require.JSONEq(t, `{"errands":[]}`, string(response.Payload))
}

// A caller who cannot reach the task learns nothing and changes nothing.
func TestErrandRequestsDenyAForeignTaskBeforeTouchingErrands(t *testing.T) {
	svc := &fakeErrandService{errand: sampleErrand()}
	h := errandHandlers(t, svc, true)
	foreign := errandCallerAs("user-a")

	response, err := h.wsListErrands(foreign, errandMessage(t, ws.ActionErrandList, map[string]string{"task_id": "task-b"}))
	require.NoError(t, err)
	require.Equal(t, ws.ErrorCodeNotFound, decodeError(t, response).Code)

	response, err = h.wsCancelErrand(foreign, errandMessage(t, ws.ActionErrandCancel, map[string]string{"session_id": "errand-1"}))
	require.NoError(t, err)
	payload := decodeError(t, response)
	require.Equal(t, ws.ErrorCodeNotFound, payload.Code)
	require.Equal(t, "errand_not_found", payload.Details["reason"], "a foreign errand looks like a missing one")

	response, err = h.wsCancelAllErrands(foreign, errandMessage(t, ws.ActionErrandCancelAll, map[string]string{"task_id": "task-b"}))
	require.NoError(t, err)
	require.Equal(t, ws.ErrorCodeNotFound, decodeError(t, response).Code)

	require.Empty(t, svc.listed)
	require.Empty(t, svc.cancelled, "an unauthorized cancel must never reach the service")
	require.Empty(t, svc.cancelAll)
}

// @covers AC-TASKS-ERRANDS-007.1
func TestErrandCancelScopesTheCancelToTheErrandsOwnVerifiedTask(t *testing.T) {
	svc := &fakeErrandService{errand: sampleErrand()}
	h := errandHandlers(t, svc, true)

	response, err := h.wsCancelErrand(errandCallerAs("user-b"), errandMessage(t, ws.ActionErrandCancel,
		map[string]string{"session_id": "errand-1", "task_id": "someone-elses-task"}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, []errands.CancelRequest{{
		ErrandID: "errand-1", TaskID: "task-b", Source: models.ErrandCancelSourceUser,
	}}, svc.cancelled, "the task comes from the errand row, never from a payload field")
	var body struct {
		Errand map[string]interface{} `json:"errand"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &body))
	require.Equal(t, "cancelled", body.Errand["status"])
}

func TestErrandCancelMapsServiceFailuresToTypedErrors(t *testing.T) {
	svc := &fakeErrandService{errand: sampleErrand(), cancelErr: errors.New("stop failed")}
	h := errandHandlers(t, svc, true)
	response, err := h.wsCancelErrand(errandCallerAs("user-b"), errandMessage(t, ws.ActionErrandCancel, map[string]string{"session_id": "errand-1"}))
	require.NoError(t, err)
	payload := decodeError(t, response)
	require.Equal(t, ws.ErrorCodeInternalError, payload.Code)
	require.NotContains(t, payload.Message, "stop failed", "internal errors do not echo internals")

	response, err = h.wsCancelErrand(errandCallerAs("user-b"), errandMessage(t, ws.ActionErrandCancel, map[string]string{"session_id": "ghost"}))
	require.NoError(t, err)
	require.Equal(t, ws.ErrorCodeInternalError, decodeError(t, response).Code)
}

// @covers AC-TASKS-ERRANDS-007.6
func TestErrandCancelAllReportsTheCancelledCount(t *testing.T) {
	svc := &fakeErrandService{cancelAllN: 3}
	h := errandHandlers(t, svc, true)
	response, err := h.wsCancelAllErrands(errandCallerAs("user-b"), errandMessage(t, ws.ActionErrandCancelAll, map[string]string{"task_id": "task-b"}))
	require.NoError(t, err)
	require.JSONEq(t, `{"cancelled":3}`, string(response.Payload))
	require.Equal(t, []string{"task-b"}, svc.cancelAll)

	svc.cancelAllN = 0
	response, err = h.wsCancelAllErrands(errandCallerAs("user-b"), errandMessage(t, ws.ActionErrandCancelAll, map[string]string{"task_id": "task-b"}))
	require.NoError(t, err)
	require.JSONEq(t, `{"cancelled":0}`, string(response.Payload), "no live errand succeeds without change")
}

func TestErrandCancelAllSurfacesPartialStopFailureWithTheCount(t *testing.T) {
	svc := &fakeErrandService{cancelAllN: 2, cancelAllE: errors.New("one runtime would not stop")}
	h := errandHandlers(t, svc, true)
	response, err := h.wsCancelAllErrands(errandCallerAs("user-b"), errandMessage(t, ws.ActionErrandCancelAll, map[string]string{"task_id": "task-b"}))
	require.NoError(t, err)
	payload := decodeError(t, response)
	require.Equal(t, ws.ErrorCodeInternalError, payload.Code)
	require.Equal(t, float64(2), payload.Details["cancelled"])
}

func TestErrandRequestsRejectBadPayloads(t *testing.T) {
	h := errandHandlers(t, &fakeErrandService{}, true)
	for _, tc := range []struct {
		name    string
		handler func(context.Context, *ws.Message) (*ws.Message, error)
		payload string
		code    string
	}{
		{"list undecodable", h.wsListErrands, `"nope"`, ws.ErrorCodeBadRequest},
		{"list blank task", h.wsListErrands, `{"task_id":" "}`, ws.ErrorCodeValidation},
		{"cancel undecodable", h.wsCancelErrand, `[1]`, ws.ErrorCodeBadRequest},
		{"cancel blank session", h.wsCancelErrand, `{}`, ws.ErrorCodeValidation},
		{"cancel all undecodable", h.wsCancelAllErrands, `7`, ws.ErrorCodeBadRequest},
		{"cancel all blank task", h.wsCancelAllErrands, `{"task_id":""}`, ws.ErrorCodeValidation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := &ws.Message{ID: "req-1", Type: ws.MessageTypeRequest, Action: "errand.test", Payload: json.RawMessage(tc.payload)}
			response, err := tc.handler(context.Background(), msg)
			require.NoError(t, err)
			require.Equal(t, tc.code, decodeError(t, response).Code)
		})
	}
}

func TestErrandActionsAreRegisteredOnTheDispatcher(t *testing.T) {
	h := errandHandlers(t, &fakeErrandService{}, true)
	dispatcher := ws.NewDispatcher()
	h.registerWS(dispatcher)
	for _, action := range []string{ws.ActionErrandList, ws.ActionErrandCancel, ws.ActionErrandCancelAll} {
		require.True(t, dispatcher.HasHandler(action), action)
	}
	require.False(t, dispatcher.HasHandler(ws.ActionErrandSendResult), "send_result belongs to result delivery")
}
