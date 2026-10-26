package dto

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func encodeToMap(t *testing.T, value interface{}) map[string]interface{} {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return payload
}

// @covers AC-TASKS-ERRANDS-001.1
func TestSessionRoleProjectsOnSessionDTOs(t *testing.T) {
	errand := &models.TaskSession{
		ID: "errand-1", TaskID: "task-1", State: models.TaskSessionStateRunning,
		Role: models.SessionRoleErrand, ParentSessionID: "parent-1",
	}
	for name, payload := range map[string]map[string]interface{}{
		"full":    encodeToMap(t, FromTaskSession(errand)),
		"summary": encodeToMap(t, FromTaskSessionSummary(errand)),
	} {
		if payload["role"] != "errand" || payload["parent_session_id"] != "parent-1" {
			t.Fatalf("%s DTO role fields = (%v, %v), want (errand, parent-1)", name, payload["role"], payload["parent_session_id"])
		}
	}

	ordinary := &models.TaskSession{ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateRunning}
	for name, payload := range map[string]map[string]interface{}{
		"full":    encodeToMap(t, FromTaskSession(ordinary)),
		"summary": encodeToMap(t, FromTaskSessionSummary(ordinary)),
	} {
		for _, key := range []string{"role", "parent_session_id"} {
			if _, ok := payload[key]; ok {
				t.Fatalf("%s DTO for an ordinary session contains %q", name, key)
			}
		}
	}
}

func TestErrandDTOProjectsPublicFieldsAndBoundedPreview(t *testing.T) {
	created := time.Date(2026, time.October, 26, 9, 0, 0, 0, time.UTC)
	settled := created.Add(5 * time.Minute)
	result := strings.Repeat("é", 300)
	errand := &models.Errand{
		ID: "errand-1", TaskID: "task-1", ParentSessionID: "parent-1",
		Title: "API audit", Objective: "Audit handlers", AgentProfileID: "profile-1",
		MCPServerIDs: []string{"mcp-1"}, FirstTurnID: "turn-1", SubmissionID: "submission-1",
		Lifecycle: models.ErrandLifecycleLaunched, LastOutcome: models.ErrandOutcomeDone,
		ResultText: result, ResultTruncated: true, ResultTurnID: "turn-1",
		ResultKind: models.ErrandResultOutput, ResultState: models.ErrandResultPending,
		DeliveryToken: "token-1", Version: 3,
		CreatedAt: created, UpdatedAt: settled, SettledAt: &settled,
	}

	got := FromErrand(errand, models.ErrandStatusDone)
	if got.ErrandID != "errand-1" || got.TaskID != "task-1" || got.ParentSessionID != "parent-1" ||
		got.Title != "API audit" || got.Objective != "Audit handlers" || got.AgentProfileID != "profile-1" ||
		got.Status != models.ErrandStatusDone || got.ResultState != models.ErrandResultPending ||
		!got.ResultTruncated || !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(settled) ||
		got.SettledAt == nil || !got.SettledAt.Equal(settled) {
		t.Fatalf("FromErrand = %+v", got)
	}
	if want := strings.Repeat("é", ErrandResultPreviewLength); got.ResultPreview != want {
		t.Fatalf("preview has %d runes, want the first %d", len([]rune(got.ResultPreview)), ErrandResultPreviewLength)
	}

	payload := encodeToMap(t, got)
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{
		"agent_profile_id", "created_at", "errand_id", "objective", "parent_session_id", "result_preview",
		"result_state", "result_truncated", "settled_at", "status", "task_id", "title", "updated_at",
	}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("ErrandDTO keys = %v, want %v; full result text and delivery identity stay private", keys, want)
	}

	errand.SettledAt = nil
	errand.ResultText = "short"
	if payload := encodeToMap(t, FromErrand(errand, models.ErrandStatusRunning)); payload["settled_at"] != nil ||
		payload["result_preview"] != "short" {
		t.Fatalf("unsettled DTO = %v, want explicit null settled_at and the whole short preview", payload)
	}
}
