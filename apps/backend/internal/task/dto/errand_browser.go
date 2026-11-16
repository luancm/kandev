package dto

import (
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type ErrandBrowserScopeDTO struct {
	TaskID          string `json:"task_id"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	ChildSessionID  string `json:"child_session_id,omitempty"`
}

type ErrandBrowserCountsDTO struct {
	Live           int `json:"live"`
	Capacity       int `json:"capacity"`
	Queued         int `json:"queued"`
	Outstanding    int `json:"outstanding"`
	Finished       int `json:"finished"`
	PendingResults int `json:"pending_results"`
	Unread         int `json:"unread"`
}

type ErrandParentBrowserCountsDTO struct {
	ParentSessionID string                 `json:"parent_session_id"`
	Counts          ErrandBrowserCountsDTO `json:"counts"`
}

// ErrandBrowserRowDTO excludes objective and result bodies by construction.
type ErrandBrowserRowDTO struct {
	ErrandID        string                       `json:"errand_id"`
	TaskID          string                       `json:"task_id"`
	ParentSessionID string                       `json:"parent_session_id"`
	Title           string                       `json:"title"`
	AgentProfileID  string                       `json:"agent_profile_id"`
	Status          models.ErrandStatus          `json:"status"`
	AttentionReason models.ErrandAttentionReason `json:"attention_reason,omitempty"`
	ResultState     models.ErrandResultState     `json:"result_state"`
	DeliveryState   models.ErrandDeliveryState   `json:"delivery_state,omitempty"`
	ResultTruncated bool                         `json:"result_truncated"`
	CreatedAt       time.Time                    `json:"created_at"`
	UpdatedAt       time.Time                    `json:"updated_at"`
	SettledAt       *time.Time                   `json:"settled_at"`
	Version         int64                        `json:"version"`
	Live            bool                         `json:"live"`
	Outstanding     bool                         `json:"outstanding"`
	Unread          bool                         `json:"unread"`
	CanSendResult   bool                         `json:"can_send_result"`
	Metadata        map[string]interface{}       `json:"metadata,omitempty"`
	TokensIn        int64                        `json:"tokens_in"`
	TokensCachedIn  int64                        `json:"tokens_cached_in"`
	TokensOut       int64                        `json:"tokens_out"`
}

type ErrandBrowserSummaryDTO struct {
	Scope        ErrandBrowserScopeDTO          `json:"scope"`
	Errands      []ErrandBrowserRowDTO          `json:"errands"`
	Counts       ErrandBrowserCountsDTO         `json:"counts"`
	ParentCounts []ErrandParentBrowserCountsDTO `json:"parent_counts"`
}

type ErrandHistoryPageDTO struct {
	Scope         ErrandBrowserScopeDTO `json:"scope"`
	Errands       []ErrandBrowserRowDTO `json:"errands"`
	TotalFinished int                   `json:"total_finished"`
	HasMore       bool                  `json:"has_more"`
	NextCursor    *string               `json:"next_cursor"`
}

type ErrandBrowserParentDTO struct {
	Available         bool               `json:"available"`
	ID                string             `json:"id,omitempty"`
	Title             string             `json:"title,omitempty"`
	Role              models.SessionRole `json:"role,omitempty"`
	EnvironmentID     string             `json:"environment_id,omitempty"`
	TaskEnvironmentID string             `json:"task_environment_id,omitempty"`
}

type ErrandInspectionDTO struct {
	Scope              ErrandBrowserScopeDTO  `json:"scope"`
	Errand             ErrandBrowserRowDTO    `json:"errand"`
	ChildSessionExists bool                   `json:"child_session_exists"`
	Parent             ErrandBrowserParentDTO `json:"parent"`
}

func ErrandBrowserCounts(counts models.ErrandBrowserCounts) ErrandBrowserCountsDTO {
	return ErrandBrowserCountsDTO{
		Live: counts.Live, Capacity: counts.Capacity, Queued: counts.Queued,
		Outstanding: counts.Outstanding, Finished: counts.Finished,
		PendingResults: counts.PendingResults, Unread: counts.Unread,
	}
}

func FromErrandBrowserObservation(row *models.ErrandBrowserObservation) ErrandBrowserRowDTO {
	if row == nil {
		return ErrandBrowserRowDTO{}
	}
	return ErrandBrowserRowDTO{
		ErrandID: row.ID, TaskID: row.TaskID, ParentSessionID: row.ParentSessionID,
		Title: row.Title, AgentProfileID: row.AgentProfileID, Status: row.Status,
		AttentionReason: row.AttentionReason, ResultState: row.ResultState, DeliveryState: row.DeliveryState,
		ResultTruncated: row.ResultTruncated, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		SettledAt: row.SettledAt, Version: row.Version, Live: row.Live, Outstanding: row.Outstanding,
		Unread: row.Unread, CanSendResult: row.CanSendResult, Metadata: row.ContextMetadata,
		TokensIn: row.TokensIn, TokensCachedIn: row.TokensCachedIn, TokensOut: row.TokensOut,
	}
}

func FromErrandBrowserSummary(summary models.ErrandBrowserSummary) ErrandBrowserSummaryDTO {
	rows := make([]ErrandBrowserRowDTO, 0, len(summary.Errands))
	for _, row := range summary.Errands {
		rows = append(rows, FromErrandBrowserObservation(row))
	}
	parents := make([]ErrandParentBrowserCountsDTO, 0, len(summary.ParentCounts))
	for _, parent := range summary.ParentCounts {
		parents = append(parents, ErrandParentBrowserCountsDTO{
			ParentSessionID: parent.ParentSessionID, Counts: ErrandBrowserCounts(parent.Counts),
		})
	}
	return ErrandBrowserSummaryDTO{
		Scope: ErrandBrowserScopeDTO{TaskID: summary.TaskID}, Errands: rows,
		Counts: ErrandBrowserCounts(summary.Counts), ParentCounts: parents,
	}
}
