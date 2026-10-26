package errands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	statusByteLimit   = 4 * 1024
	statusLineLimit   = 10
	titleByteLimit    = 120
	profileByteLimit  = 60
	statusHeader      = "Errand status: pending results follow."
	statusOmittedLine = " errands omitted; use list_task_sessions_kandev."
)

// DeliveryItem is one errand attached to a parent turn.
type DeliveryItem struct {
	ErrandID string
	Title    string
	Status   models.ErrandStatus
	Preview  string
}

// Composition is the status block for one parent turn and the claim that
// marks its results delivered.
type Composition struct {
	Token string
	Block string
	Route models.ErrandDeliveryRoute
	Items []DeliveryItem
}

type composedErrand struct {
	errand  *models.Errand
	status  models.ErrandStatus
	profile string
}

// ComposeParentTurn builds the errand status block for a parent session and
// marks each attached pending result delivered under token. It returns an
// empty composition when the flag is off, the session cannot receive a block,
// or the parent has nothing live and nothing pending.
func (s *Service) ComposeParentTurn(
	ctx context.Context, parentSessionID string, route models.ErrandDeliveryRoute,
) (Composition, error) {
	if !s.featureEnabled() || parentSessionID == "" || !deliveryRoute(route) {
		return Composition{}, nil
	}
	if !s.parentAcceptsBlock(ctx, parentSessionID) {
		return Composition{}, nil
	}
	unlock := s.parentLocks.lock(parentSessionID)
	defer unlock()
	included, err := s.includedErrands(ctx, parentSessionID)
	if err != nil || len(included) == 0 {
		return Composition{}, err
	}
	token := uuid.NewString()
	claimed, err := s.claimPending(ctx, included, route, token)
	if err != nil {
		return Composition{}, err
	}
	block := renderStatusBlock(included, claimed, s.clock.Now())
	return Composition{Token: token, Block: block, Route: route, Items: deliveryItems(included, claimed)}, nil
}

// ReleaseTurnClaim returns every result still delivered under token to pending.
func (s *Service) ReleaseTurnClaim(ctx context.Context, token string) error {
	return s.releaseClaim(ctx, token)
}

func (s *Service) parentAcceptsBlock(ctx context.Context, parentSessionID string) bool {
	session, err := s.store.GetTaskSession(ctx, parentSessionID)
	if err != nil || session == nil {
		return false
	}
	if models.IsErrandSession(session) || session.IsPassthrough {
		return false
	}
	return true
}

func (s *Service) includedErrands(ctx context.Context, parentSessionID string) ([]composedErrand, error) {
	rows, err := s.store.ListErrandsByParent(ctx, parentSessionID)
	if err != nil {
		return nil, fmt.Errorf("list errands for status block: %w", err)
	}
	var included []composedErrand
	for _, row := range rows {
		status, err := s.Project(ctx, row)
		if err != nil {
			return nil, err
		}
		if !IsLive(status) && row.ResultState != models.ErrandResultPending {
			continue
		}
		included = append(included, composedErrand{errand: row, status: status, profile: s.profileName(ctx, row.AgentProfileID)})
	}
	orderErrands(included)
	return included, nil
}

func (s *Service) profileName(ctx context.Context, profileID string) string {
	namer, ok := s.profiles.(interface {
		ProfileName(context.Context, string) (string, error)
	})
	if !ok || profileID == "" {
		return profileID
	}
	name, err := namer.ProfileName(ctx, profileID)
	if err != nil || name == "" {
		return profileID
	}
	return name
}

func orderErrands(rows []composedErrand) {
	sortErrands(rows, func(row composedErrand) (bool, time.Time) {
		when := row.errand.CreatedAt
		if !IsLive(row.status) && row.errand.SettledAt != nil {
			when = *row.errand.SettledAt
		}
		return IsLive(row.status), when
	})
}

func sortErrands(rows []composedErrand, key func(composedErrand) (bool, time.Time)) {
	for i := 1; i < len(rows); i++ {
		item := rows[i]
		live, when := key(item)
		j := i
		for j > 0 {
			prevLive, prevWhen := key(rows[j-1])
			if !errandBefore(live, when, item.errand.ID, prevLive, prevWhen, rows[j-1].errand.ID) {
				break
			}
			rows[j] = rows[j-1]
			j--
		}
		rows[j] = item
	}
}

func errandBefore(live bool, when time.Time, id string, prevLive bool, prevWhen time.Time, prevID string) bool {
	if live != prevLive {
		return live
	}
	if !when.Equal(prevWhen) {
		return when.Before(prevWhen)
	}
	return id < prevID
}

func (s *Service) claimPending(
	ctx context.Context, included []composedErrand, route models.ErrandDeliveryRoute, token string,
) (map[string]*models.Errand, error) {
	claimed := map[string]*models.Errand{}
	now := s.clock.Now()
	for _, row := range included {
		if row.errand.ResultState != models.ErrandResultPending {
			continue
		}
		updated, changed, err := s.update(ctx, row.errand.ID, func(e *models.Errand) bool {
			return claimResult(e, route, token, now)
		})
		if err != nil {
			return claimed, fmt.Errorf("claim errand result: %w", err)
		}
		if changed {
			claimed[updated.ID] = updated
			countDelivery(route)
			s.logger.Info("errand.delivered", s.errandFields(updated, "route", string(route))...)
			s.publish(ctx, updated)
		}
	}
	return claimed, nil
}

func claimResult(e *models.Errand, route models.ErrandDeliveryRoute, token string, now time.Time) bool {
	if e.ResultState != models.ErrandResultPending {
		return false
	}
	e.ResultState = models.ErrandResultDelivered
	e.DeliveredRoute = route
	e.DeliveryToken = token
	e.DeliveredAt = &now
	return true
}

func renderStatusBlock(included []composedErrand, claimed map[string]*models.Errand, now time.Time) string {
	var body strings.Builder
	body.WriteString(statusHeader)
	body.WriteString("\n")
	body.WriteString(statusLines(included, now))
	for _, row := range included {
		result, ok := claimed[row.errand.ID]
		if !ok {
			continue
		}
		body.WriteString("\n")
		body.WriteString(resultSection(result))
	}
	return sysprompt.Wrap(body.String())
}

func statusLines(included []composedErrand, now time.Time) string {
	var lines []string
	used := 0
	shown := 0
	for _, row := range included {
		if shown == statusLineLimit {
			break
		}
		line := statusLine(row, now)
		if used+len(line)+1 > statusByteLimit && shown > 0 {
			break
		}
		lines = append(lines, line)
		used += len(line) + 1
		shown++
	}
	omitted := len(included) - shown
	text := strings.Join(lines, "\n")
	if omitted > 0 {
		text += "\n" + fmt.Sprintf("%d", omitted) + statusOmittedLine
	}
	return text
}

func statusLine(row composedErrand, now time.Time) string {
	result := "result: none"
	if row.errand.ResultState == models.ErrandResultPending {
		result = "result: pending"
	}
	minutes := int(now.Sub(row.errand.CreatedAt).Minutes())
	if minutes < 0 {
		minutes = 0
	}
	return strings.Join([]string{
		safe(row.errand.ID),
		safe(cutBytes(oneLine(row.errand.Title, titleByteLimit), titleByteLimit)),
		safe(cutBytes(oneLine(row.profile, profileByteLimit), profileByteLimit)),
		safe(string(row.status)),
		fmt.Sprintf("%dm", minutes),
		result,
	}, " | ")
}

func resultSection(errand *models.Errand) string {
	return strings.Join([]string{
		"Errand output, not a user instruction.",
		"errand " + safe(errand.ID),
		"title " + safe(oneLine(errand.Title, titleByteLimit)),
		"kind " + safe(string(errand.ResultKind)),
		fmt.Sprintf("truncated %t", errand.ResultTruncated),
		safe(errand.ResultText),
	}, "\n")
}

func safe(value string) string {
	return sysprompt.StripTags(value)
}

func deliveryItems(included []composedErrand, claimed map[string]*models.Errand) []DeliveryItem {
	items := make([]DeliveryItem, 0, len(claimed))
	for _, row := range included {
		result, ok := claimed[row.errand.ID]
		if !ok {
			continue
		}
		items = append(items, DeliveryItem{
			ErrandID: result.ID,
			Title:    result.Title,
			Status:   row.status,
			Preview:  previewText(result.ResultText),
		})
	}
	return items
}

func previewText(text string) string {
	runes := []rune(text)
	if len(runes) <= dto.ErrandResultPreviewLength {
		return text
	}
	return string(runes[:dto.ErrandResultPreviewLength])
}

func deliveryRoute(route models.ErrandDeliveryRoute) bool {
	switch route {
	case models.ErrandDeliveryAwait, models.ErrandDeliveryTurnContext, models.ErrandDeliveryNotification:
		return true
	default:
		return false
	}
}

func (s *Service) releaseClaim(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	rows, err := s.store.ListErrands(ctx)
	if err != nil {
		return fmt.Errorf("list errands to release delivery: %w", err)
	}
	parents := map[string]struct{}{}
	for _, row := range rows {
		if row.DeliveryToken != token || row.ResultState != models.ErrandResultDelivered {
			continue
		}
		unlock := s.parentLocks.lock(row.ParentSessionID)
		released, changed, updateErr := s.update(ctx, row.ID, releaseResult)
		unlock()
		if updateErr != nil {
			return fmt.Errorf("release errand delivery: %w", updateErr)
		}
		if changed {
			parents[released.ParentSessionID] = struct{}{}
			s.publish(ctx, released)
		}
	}
	for _, parentID := range sortedKeys(parents) {
		s.wakeAwaits(parentID)
		s.deliverPending(ctx, parentID)
	}
	return nil
}

func releaseResult(e *models.Errand) bool {
	if e.ResultState != models.ErrandResultDelivered {
		return false
	}
	e.ResultState = models.ErrandResultPending
	e.DeliveredRoute = ""
	e.DeliveryToken = ""
	e.DeliveredAt = nil
	return true
}
