package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// ErrandResultPreviewLength is the number of characters of result text that
// leave the backend outside the parent's MCP surface.
const ErrandResultPreviewLength = 280

// ErrandDTO is the browser projection of one errand. Full result text and
// delivery identity are deliberately absent.
type ErrandDTO struct {
	ErrandID        string                   `json:"errand_id"`
	TaskID          string                   `json:"task_id"`
	ParentSessionID string                   `json:"parent_session_id"`
	Title           string                   `json:"title"`
	Objective       string                   `json:"objective"`
	AgentProfileID  string                   `json:"agent_profile_id"`
	Status          models.ErrandStatus      `json:"status"`
	ResultState     models.ErrandResultState `json:"result_state"`
	ResultPreview   string                   `json:"result_preview"`
	ResultTruncated bool                     `json:"result_truncated"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
	SettledAt       *time.Time               `json:"settled_at"`
	// CanSendResult is computed by the server so every client applies the
	// same eligibility rule for Send result to parent.
	CanSendResult bool `json:"can_send_result"`
}

// FromErrand projects an errand row with its already projected status.
func FromErrand(errand *models.Errand, status models.ErrandStatus) ErrandDTO {
	return ErrandDTO{
		ErrandID:        errand.ID,
		TaskID:          errand.TaskID,
		ParentSessionID: errand.ParentSessionID,
		Title:           errand.Title,
		Objective:       errand.Objective,
		AgentProfileID:  errand.AgentProfileID,
		Status:          status,
		ResultState:     errand.ResultState,
		ResultPreview:   errandResultPreview(errand.ResultText),
		ResultTruncated: errand.ResultTruncated,
		CreatedAt:       errand.CreatedAt,
		UpdatedAt:       errand.UpdatedAt,
		SettledAt:       errand.SettledAt,
		CanSendResult:   errand.CanSendResult(status),
	}
}

func errandResultPreview(text string) string {
	characters := 0
	for index := range text {
		if characters == ErrandResultPreviewLength {
			return text[:index]
		}
		characters++
	}
	return text
}

// ErrandUpdatedEvent is the errand.updated payload: the errand DTO plus the
// workspace that routes it to the right clients.
type ErrandUpdatedEvent struct {
	ErrandDTO
	WorkspaceID string `json:"workspace_id"`
}
