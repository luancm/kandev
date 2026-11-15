package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestErrandBrowserDTOOmitsObjectiveAndResultBodies(t *testing.T) {
	row := &models.ErrandBrowserObservation{
		ID: "errand-1", TaskID: "task-1", ParentSessionID: "parent-1", Title: "Audit",
		Status: models.ErrandStatusDone, ResultState: models.ErrandResultDelivered,
		CreatedAt:       time.Date(2026, time.November, 13, 10, 0, 0, 0, time.UTC),
		ContextMetadata: map[string]interface{}{models.SessionMetaKeyContextWindow: map[string]interface{}{"size": float64(120000)}},
	}
	wire, err := json.Marshal(FromErrandBrowserObservation(row))
	if err != nil {
		t.Fatalf("marshal compact row: %v", err)
	}
	encoded := string(wire)
	for _, forbidden := range []string{"objective", "result_text", "private result body", "mcp_server_ids"} {
		if strings.Contains(encoded, forbidden) {
			t.Errorf("compact row contains forbidden field/content %q: %s", forbidden, encoded)
		}
	}
	for _, expected := range []string{"errand_id", "parent_session_id", "status", "result_state", "metadata", "tokens_in"} {
		if !strings.Contains(encoded, expected) {
			t.Errorf("compact row omits expected field %q: %s", expected, encoded)
		}
	}
}
