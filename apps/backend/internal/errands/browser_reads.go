package errands

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	ErrandHistoryPageSize = 25
	errHistoryOrder       = "created_at_desc_id_asc"
)

// ErrBrowserReadUnavailable indicates that a Store has not opted into the
// additive browser read capability.
var ErrBrowserReadUnavailable = errors.New("errand browser reads are unavailable")

type errandHistoryCursorWire struct {
	Version         int    `json:"v"`
	TaskID          string `json:"task_id"`
	ParentSessionID string `json:"parent_session_id"`
	Order           string `json:"order"`
	CreatedAt       string `json:"created_at"`
	ID              string `json:"id"`
}

// BrowserSummary returns compact outstanding rows and database-owned totals.
// It is read-only and intentionally independent of the feature-enabled gate.
func (s *Service) BrowserSummary(ctx context.Context, taskID string) (models.ErrandBrowserSummary, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return models.ErrandBrowserSummary{}, ErrInvalidRequest
	}
	reader, ok := s.store.(BrowserReadStore)
	if !ok {
		return models.ErrandBrowserSummary{}, ErrBrowserReadUnavailable
	}
	summary, err := reader.GetErrandBrowserSummary(ctx, taskID)
	if err != nil {
		return models.ErrandBrowserSummary{}, fmt.Errorf("read errand browser summary: %w", err)
	}
	summary.Counts.Capacity = MaxLiveErrands
	for i := range summary.ParentCounts {
		summary.ParentCounts[i].Counts.Capacity = MaxLiveErrands
	}
	return summary, nil
}

// HistoryPage returns one explicitly requested, bounded finished-history page.
func (s *Service) HistoryPage(ctx context.Context, request models.ErrandHistoryPageRequest, cursorToken string) (models.ErrandHistoryPage, string, error) {
	request.TaskID = strings.TrimSpace(request.TaskID)
	request.ParentSessionID = strings.TrimSpace(request.ParentSessionID)
	if request.TaskID == "" || request.ParentSessionID == "" || request.Limit < 0 || request.Limit > ErrandHistoryPageSize {
		return models.ErrandHistoryPage{}, "", ErrInvalidRequest
	}
	if request.Limit == 0 {
		request.Limit = ErrandHistoryPageSize
	}
	cursor, err := decodeErrandHistoryCursor(cursorToken, request.TaskID, request.ParentSessionID)
	if err != nil {
		return models.ErrandHistoryPage{}, "", ErrInvalidRequest
	}
	request.Cursor = cursor
	reader, ok := s.store.(BrowserReadStore)
	if !ok {
		return models.ErrandHistoryPage{}, "", ErrBrowserReadUnavailable
	}
	page, err := reader.GetErrandHistoryPage(ctx, request)
	if err != nil {
		return models.ErrandHistoryPage{}, "", fmt.Errorf("read errand history page: %w", err)
	}
	if page.NextCursor == nil {
		return page, "", nil
	}
	if page.NextCursor.TaskID != request.TaskID || page.NextCursor.ParentSessionID != request.ParentSessionID {
		return models.ErrandHistoryPage{}, "", errors.New("repository returned an out-of-scope errand history cursor")
	}
	next, err := encodeErrandHistoryCursor(*page.NextCursor)
	if err != nil {
		return models.ErrandHistoryPage{}, "", fmt.Errorf("encode errand history cursor: %w", err)
	}
	return page, next, nil
}

// Inspect verifies one child and returns only its compact identity projection.
func (s *Service) Inspect(ctx context.Context, taskID, childSessionID string) (models.ErrandBrowserInspection, error) {
	taskID = strings.TrimSpace(taskID)
	childSessionID = strings.TrimSpace(childSessionID)
	if taskID == "" || childSessionID == "" {
		return models.ErrandBrowserInspection{}, ErrInvalidRequest
	}
	reader, ok := s.store.(BrowserReadStore)
	if !ok {
		return models.ErrandBrowserInspection{}, ErrBrowserReadUnavailable
	}
	inspection, err := reader.GetErrandInspection(ctx, taskID, childSessionID)
	if err != nil {
		return models.ErrandBrowserInspection{}, fmt.Errorf("inspect errand: %w", err)
	}
	return inspection, nil
}

func decodeErrandHistoryCursor(token, taskID, parentID string) (*models.ErrandHistoryCursor, error) {
	if token == "" {
		return nil, nil
	}
	encoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var wire errandHistoryCursorWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid errand history cursor payload")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, wire.CreatedAt)
	if err != nil || wire.Version != 1 || wire.TaskID != taskID || wire.ParentSessionID != parentID ||
		wire.Order != errHistoryOrder || wire.ID == "" {
		return nil, errors.New("errand history cursor scope or version mismatch")
	}
	return &models.ErrandHistoryCursor{
		Version: wire.Version, TaskID: wire.TaskID, ParentSessionID: wire.ParentSessionID,
		Order: wire.Order, CreatedAt: createdAt, ID: wire.ID,
	}, nil
}

func encodeErrandHistoryCursor(cursor models.ErrandHistoryCursor) (string, error) {
	if cursor.Version != 1 || cursor.TaskID == "" || cursor.ParentSessionID == "" || cursor.ID == "" ||
		cursor.Order != errHistoryOrder {
		return "", errors.New("invalid errand history position")
	}
	encoded, err := json.Marshal(errandHistoryCursorWire{
		Version: cursor.Version, TaskID: cursor.TaskID, ParentSessionID: cursor.ParentSessionID,
		Order: cursor.Order, CreatedAt: cursor.CreatedAt.UTC().Format(time.RFC3339Nano), ID: cursor.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}
