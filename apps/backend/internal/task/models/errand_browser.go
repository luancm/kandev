package models

import "time"

// ErrandBrowserObservation is the compact, read-only browser projection. It
// never carries an objective, result body, MCP snapshot, or transcript data.
type ErrandBrowserObservation struct {
	ID              string
	TaskID          string
	ParentSessionID string
	Title           string
	AgentProfileID  string
	Status          ErrandStatus
	AttentionReason ErrandAttentionReason
	ResultState     ErrandResultState
	DeliveryState   ErrandDeliveryState
	ResultTruncated bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	SettledAt       *time.Time
	Version         int64
	Live            bool
	Outstanding     bool
	Unread          bool
	CanSendResult   bool
	ContextMetadata map[string]interface{}
	TokensIn        int64
	TokensCachedIn  int64
	TokensOut       int64
}

// ErrandBrowserCounts are authoritative database totals, not page counts.
type ErrandBrowserCounts struct {
	Live           int
	Capacity       int
	Queued         int
	Outstanding    int
	Finished       int
	PendingResults int
	Unread         int
}

// ErrandParentBrowserCounts contains task-scoped totals for one parent.
type ErrandParentBrowserCounts struct {
	ParentSessionID string
	Counts          ErrandBrowserCounts
}

// ErrandBrowserSummary is the initial compact task inspection projection.
type ErrandBrowserSummary struct {
	TaskID       string
	Errands      []*ErrandBrowserObservation
	Counts       ErrandBrowserCounts
	ParentCounts []ErrandParentBrowserCounts
}

// ErrandHistoryCursor is an opaque-on-the-wire keyset position. The service
// validates version and scope before the repository uses its ordering tuple.
type ErrandHistoryCursor struct {
	Version         int
	TaskID          string
	ParentSessionID string
	Order           string
	CreatedAt       time.Time
	ID              string
}

// ErrandHistoryPageRequest scopes a bounded history read to one parent.
type ErrandHistoryPageRequest struct {
	TaskID          string
	ParentSessionID string
	Limit           int
	Cursor          *ErrandHistoryCursor
}

// ErrandHistoryPage contains at most Limit rows, excluding its lookahead.
type ErrandHistoryPage struct {
	Errands       []*ErrandBrowserObservation
	TotalFinished int
	HasMore       bool
	NextCursor    *ErrandHistoryCursor
}

// ErrandBrowserParentIdentity is the minimum hosting identity needed by a
// targeted child inspector. Unavailable means the retained child has no
// current parent row.
type ErrandBrowserParentIdentity struct {
	Available         bool
	ID                string
	Title             string
	Role              SessionRole
	EnvironmentID     string
	TaskEnvironmentID string
}

// ErrandBrowserInspection verifies one retained child without fetching a
// transcript or enumerating its siblings.
type ErrandBrowserInspection struct {
	Errand             *ErrandBrowserObservation
	ChildSessionExists bool
	Parent             ErrandBrowserParentIdentity
}
