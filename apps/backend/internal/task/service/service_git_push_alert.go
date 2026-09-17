package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// GitPushFailure describes one failed push at the operation boundary.
type GitPushFailure struct {
	TaskRepositoryID string
	Diagnostic       string
	Remote           string
	Branch           string
	OccurredAt       time.Time
}

// GitPushSuccess identifies an affirmative push result for one task
// repository. Destination fields are diagnostic context; repository identity
// is the authoritative scope.
type GitPushSuccess struct {
	TaskRepositoryID string
	Remote           string
	Branch           string
	OccurredAt       time.Time
}

// GitPushStatusObservation is the validated upstream-relative status evidence
// used for terminal or agent repairs.
type GitPushStatusObservation struct {
	TaskRepositoryID  string
	Branch            string
	RemoteBranch      string
	HeadCommit        string
	RemoteHeadCommit  string
	RemoteAhead       int
	RemoteBehind      int
	RemoteAheadKnown  bool
	RemoteBehindKnown bool
	ObservedAt        time.Time
}

// RecordGitPushFailure replaces the session's current push alert. Repeated
// failures for the same repository update one presentation message and advance
// the projection revision instead of creating an attempt history.
func (s *Service) RecordGitPushFailure(ctx context.Context, sessionID string, failure GitPushFailure) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(failure.TaskRepositoryID) == "" {
		return nil
	}
	taskRepositoryID := strings.TrimSpace(failure.TaskRepositoryID)
	if failure.OccurredAt.IsZero() {
		failure.OccurredAt = time.Now().UTC()
	}
	s.gitPushAlertMu.Lock()
	defer s.gitPushAlertMu.Unlock()
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	current, hasCurrent, err := s.loadGitPushAlert(ctx, session, taskRepositoryID)
	if err != nil || supersededGitPushFailure(current, hasCurrent, failure) {
		return err
	}
	next := failedGitPushAlertState(current, failure)
	return s.persistGitPushFailure(ctx, session, current, next, hasCurrent)
}

// RecordGitPushSuccess clears the active alert only for the same canonical
// task repository and a success observed after the active failure.
func (s *Service) RecordGitPushSuccess(ctx context.Context, sessionID string, success GitPushSuccess) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(success.TaskRepositoryID) == "" {
		return nil
	}
	if success.OccurredAt.IsZero() {
		success.OccurredAt = time.Now().UTC()
	}
	s.gitPushAlertMu.Lock()
	defer s.gitPushAlertMu.Unlock()
	return s.recoverGitPushAlert(ctx, sessionID, gitPushRecovery{
		taskRepositoryID: strings.TrimSpace(success.TaskRepositoryID),
		observedAt:       success.OccurredAt,
		resolution:       "push_success",
		matches: func(alert models.GitPushAlertState) bool {
			return pushSuccessMatchesAlert(alert, success)
		},
	})
}

// ReconcileGitPushStatus clears the active alert when a newer observation proves
// that the repository's local branch and configured upstream are synchronized.
func (s *Service) ReconcileGitPushStatus(ctx context.Context, sessionID string, observation GitPushStatusObservation) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(observation.TaskRepositoryID) == "" || !synchronizedGitPushStatus(observation) {
		return nil
	}
	s.gitPushAlertMu.Lock()
	defer s.gitPushAlertMu.Unlock()
	return s.recoverGitPushAlert(ctx, sessionID, gitPushRecovery{
		taskRepositoryID: strings.TrimSpace(observation.TaskRepositoryID),
		observedAt:       observation.ObservedAt,
		resolution:       "git_status",
		matches: func(alert models.GitPushAlertState) bool {
			return statusMatchesAlert(alert, observation)
		},
	})
}

type gitPushRecovery struct {
	taskRepositoryID string
	observedAt       time.Time
	resolution       string
	matches          func(models.GitPushAlertState) bool
}

func (s *Service) recoverGitPushAlert(ctx context.Context, sessionID string, recovery gitPushRecovery) error {
	session, err := s.sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	current, ok, err := s.loadGitPushAlert(ctx, session, recovery.taskRepositoryID)
	if err != nil {
		return err
	}
	if !ok {
		return s.storeGitPushAlertState(ctx, sessionID, inactiveGitPushAlertForRepository(recovery.taskRepositoryID, recovery.observedAt))
	}
	if !current.Active {
		return s.recoverInactiveGitPushAlert(ctx, sessionID, current, recovery)
	}
	if current.TaskRepositoryID != recovery.taskRepositoryID ||
		(!current.OccurredAt.IsZero() && !recovery.observedAt.After(current.OccurredAt)) ||
		!recovery.matches(current) {
		return nil
	}
	next := clearGitPushAlert(current, recovery.observedAt)
	if err := s.resolveGitPushAlertMessage(ctx, current.MessageID, recovery.resolution, next.Revision, recovery.observedAt); err != nil {
		return err
	}
	return s.storeGitPushAlertState(ctx, sessionID, next)
}

func (s *Service) recoverInactiveGitPushAlert(ctx context.Context, sessionID string, current models.GitPushAlertState, recovery gitPushRecovery) error {
	if current.TaskRepositoryID != "" && current.TaskRepositoryID != recovery.taskRepositoryID {
		return nil
	}
	next := advanceInactiveGitPushAlert(current, recovery.taskRepositoryID, recovery.observedAt)
	if current.TaskRepositoryID != recovery.taskRepositoryID {
		return s.storeGitPushAlertState(ctx, sessionID, next)
	}
	if next.OccurredAt.Equal(current.OccurredAt) {
		return nil
	}
	if err := s.resolveGitPushAlertMessage(ctx, current.MessageID, recovery.resolution, next.Revision, next.OccurredAt); err != nil {
		return err
	}
	return s.storeGitPushAlertState(ctx, sessionID, next)
}

func (s *Service) loadGitPushAlert(ctx context.Context, session *models.TaskSession, taskRepositoryID string) (models.GitPushAlertState, bool, error) {
	current, ok := models.LoadGitPushAlert(session.Metadata)
	if ok {
		return current, true, nil
	}
	return s.importLegacyGitPushAlert(ctx, session, taskRepositoryID)
}

func supersededGitPushFailure(current models.GitPushAlertState, hasCurrent bool, failure GitPushFailure) bool {
	if !hasCurrent || current.OccurredAt.IsZero() || (!current.Active && current.TaskRepositoryID != strings.TrimSpace(failure.TaskRepositoryID)) {
		return false
	}
	if failure.OccurredAt.Before(current.OccurredAt) {
		return true
	}
	return failure.OccurredAt.Equal(current.OccurredAt) && current.Active &&
		current.TaskRepositoryID == strings.TrimSpace(failure.TaskRepositoryID) && current.Diagnostic == failure.Diagnostic &&
		current.Remote == strings.TrimSpace(failure.Remote) && current.Branch == strings.TrimSpace(failure.Branch)
}

func failedGitPushAlertState(current models.GitPushAlertState, failure GitPushFailure) models.GitPushAlertState {
	current.Active = true
	current.Revision++
	current.TaskRepositoryID = strings.TrimSpace(failure.TaskRepositoryID)
	current.Diagnostic = failure.Diagnostic
	current.OccurredAt = failure.OccurredAt.UTC()
	current.Remote = strings.TrimSpace(failure.Remote)
	current.Branch = strings.TrimSpace(failure.Branch)
	current.StampValue = fmt.Sprintf("%d", current.Revision)
	return current
}

func (s *Service) persistGitPushFailure(ctx context.Context, session *models.TaskSession, current, next models.GitPushAlertState, hasCurrent bool) error {
	if hasCurrent && current.Active && current.MessageID != "" {
		messageID, err := s.updateGitPushAlertMessage(ctx, next, session.TaskID, session.ID)
		if err != nil {
			return err
		}
		next.MessageID = messageID
	} else {
		message, err := s.createGitPushAlertMessage(ctx, next, session.TaskID, session.ID)
		if err != nil {
			return err
		}
		next.MessageID = message.ID
	}
	return s.storeGitPushAlertState(ctx, session.ID, next)
}

func (s *Service) storeGitPushAlertState(ctx context.Context, sessionID string, state models.GitPushAlertState) error {
	return s.sessions.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyGitPushAlert, state)
}

func clearGitPushAlert(current models.GitPushAlertState, at time.Time) models.GitPushAlertState {
	current.Active = false
	current.Revision++
	current.OccurredAt = at.UTC()
	current.StampValue = fmt.Sprintf("%d", current.Revision)
	return current
}

func advanceInactiveGitPushAlert(current models.GitPushAlertState, taskRepositoryID string, at time.Time) models.GitPushAlertState {
	if at.IsZero() || (!current.OccurredAt.IsZero() && !at.After(current.OccurredAt)) {
		return current
	}
	if current.TaskRepositoryID == "" {
		current.TaskRepositoryID = strings.TrimSpace(taskRepositoryID)
	}
	current.Active = false
	current.OccurredAt = at.UTC()
	return current
}

func inactiveGitPushAlert(at time.Time) models.GitPushAlertState {
	return models.GitPushAlertState{Revision: 1, OccurredAt: at.UTC(), StampValue: "1"}
}

func inactiveGitPushAlertForRepository(taskRepositoryID string, at time.Time) models.GitPushAlertState {
	state := inactiveGitPushAlert(at)
	state.TaskRepositoryID = strings.TrimSpace(taskRepositoryID)
	return state
}

func synchronizedGitPushStatus(observation GitPushStatusObservation) bool {
	if observation.ObservedAt.IsZero() || strings.TrimSpace(observation.Branch) == "" ||
		strings.TrimSpace(observation.RemoteBranch) == "" || strings.TrimSpace(observation.HeadCommit) == "" ||
		strings.TrimSpace(observation.RemoteHeadCommit) == "" || observation.HeadCommit != observation.RemoteHeadCommit ||
		!observation.RemoteAheadKnown || !observation.RemoteBehindKnown || observation.RemoteAhead != 0 || observation.RemoteBehind != 0 {
		return false
	}
	remote, remoteBranch := splitGitRemoteBranch(observation.RemoteBranch)
	return remote != "" && strings.EqualFold(strings.TrimSpace(observation.Branch), remoteBranch)
}

func statusMatchesAlert(alert models.GitPushAlertState, observation GitPushStatusObservation) bool {
	if alert.Branch != "" && !strings.EqualFold(alert.Branch, observation.Branch) {
		return false
	}
	if alert.Remote != "" {
		remote, _ := splitGitRemoteBranch(observation.RemoteBranch)
		return strings.EqualFold(alert.Remote, remote)
	}
	return true
}

func pushSuccessMatchesAlert(alert models.GitPushAlertState, success GitPushSuccess) bool {
	if alert.Branch != "" && success.Branch != "" && !strings.EqualFold(alert.Branch, success.Branch) {
		return false
	}
	if alert.Remote != "" && success.Remote != "" && !strings.EqualFold(alert.Remote, success.Remote) {
		return false
	}
	return true
}

func splitGitRemoteBranch(remoteBranch string) (string, string) {
	remoteBranch = strings.TrimPrefix(strings.TrimSpace(remoteBranch), "refs/remotes/")
	if slash := strings.IndexByte(remoteBranch, '/'); slash >= 0 {
		return remoteBranch[:slash], remoteBranch[slash+1:]
	}
	return "", remoteBranch
}

func isLegacyGitPushError(message *models.Message) bool {
	if message == nil || message.Metadata == nil {
		return false
	}
	failed, _ := message.Metadata["git_operation_error"].(bool)
	operation, _ := message.Metadata["operation"].(string)
	resolved, _ := message.Metadata["git_operation_resolved"].(bool)
	active, hasActiveMarker := message.Metadata["git_push_alert_active"].(bool)
	return failed && strings.EqualFold(strings.TrimSpace(operation), "push") && !resolved && (!hasActiveMarker || active)
}

func (s *Service) importLegacyGitPushAlert(
	ctx context.Context,
	session *models.TaskSession,
	taskRepositoryID string,
) (models.GitPushAlertState, bool, error) {
	if s.taskRepos == nil {
		return models.GitPushAlertState{}, false, nil
	}
	repositories, err := s.taskRepos.ListTaskRepositories(ctx, session.TaskID)
	if err != nil {
		return models.GitPushAlertState{}, false, err
	}
	if !isOnlyTaskRepository(repositories, taskRepositoryID) {
		return models.GitPushAlertState{}, false, nil
	}
	messages, err := s.messages.ListMessages(ctx, session.ID)
	if err != nil {
		return models.GitPushAlertState{}, false, err
	}
	legacy, latest := legacyGitPushAlerts(messages)
	if latest == nil {
		return models.GitPushAlertState{}, false, nil
	}
	if err := s.resolveSupersededLegacyGitPushAlerts(ctx, legacy, latest); err != nil {
		return models.GitPushAlertState{}, false, err
	}
	state := importedGitPushAlertState(latest, taskRepositoryID)
	messageID, err := s.updateGitPushAlertMessage(ctx, state, session.TaskID, session.ID)
	if err != nil {
		return models.GitPushAlertState{}, false, err
	}
	state.MessageID = messageID
	if err := s.storeGitPushAlertState(ctx, session.ID, state); err != nil {
		return models.GitPushAlertState{}, false, err
	}
	return state, true, nil
}

func isOnlyTaskRepository(repositories []*models.TaskRepository, taskRepositoryID string) bool {
	return len(repositories) == 1 && repositories[0] != nil && repositories[0].ID == taskRepositoryID
}

func legacyGitPushAlerts(messages []*models.Message) ([]*models.Message, *models.Message) {
	legacy := make([]*models.Message, 0)
	var latest *models.Message
	for _, message := range messages {
		if !isLegacyGitPushError(message) {
			continue
		}
		legacy = append(legacy, message)
		if latest == nil || message.CreatedAt.After(latest.CreatedAt) ||
			(message.CreatedAt.Equal(latest.CreatedAt) && message.ID > latest.ID) {
			latest = message
		}
	}
	return legacy, latest
}

func (s *Service) resolveSupersededLegacyGitPushAlerts(ctx context.Context, legacy []*models.Message, latest *models.Message) error {
	for _, message := range legacy {
		if message.ID == latest.ID {
			continue
		}
		if err := s.resolveLegacyGitPushAlertMessage(ctx, message.ID, "legacy_superseded", latest.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func importedGitPushAlertState(latest *models.Message, taskRepositoryID string) models.GitPushAlertState {
	diagnostic, _ := latest.Metadata["error_output"].(string)
	if diagnostic == "" {
		diagnostic = latest.Content
	}
	return models.GitPushAlertState{
		Active:           true,
		Revision:         1,
		TaskRepositoryID: taskRepositoryID,
		MessageID:        latest.ID,
		Diagnostic:       diagnostic,
		OccurredAt:       latest.CreatedAt,
		StampValue:       "1",
	}
}

func (s *Service) createGitPushAlertMessage(ctx context.Context, state models.GitPushAlertState, taskID, sessionID string) (*models.Message, error) {
	return s.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		Content:       "Git push failed",
		AuthorType:    "agent",
		Type:          string(models.MessageTypeError),
		Metadata:      s.gitPushAlertMetadata(ctx, state, taskID, sessionID),
	})
}

func (s *Service) updateGitPushAlertMessage(ctx context.Context, state models.GitPushAlertState, taskID, sessionID string) (string, error) {
	message, err := s.messages.GetMessage(ctx, state.MessageID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if message == nil {
		created, createErr := s.createGitPushAlertMessage(ctx, state, taskID, sessionID)
		if createErr != nil {
			return "", createErr
		}
		return created.ID, nil
	}
	message.Content = "Git push failed"
	message.Metadata = s.gitPushAlertMetadata(ctx, state, taskID, sessionID)
	if err := s.UpdateMessage(ctx, message); err != nil {
		return "", err
	}
	return message.ID, nil
}

func (s *Service) gitPushAlertMetadata(ctx context.Context, state models.GitPushAlertState, taskID, sessionID string) map[string]interface{} {
	diagnostic := state.Diagnostic
	fixPrompt := "The git push command failed with the following error:\n\n```\n" + diagnostic + "\n```\n\nPlease fix the issues reported above."
	return map[string]interface{}{
		"git_operation_error":          true,
		"operation":                    "push",
		"error_output":                 diagnostic,
		"session_id":                   sessionID,
		"task_id":                      taskID,
		"variant":                      "error",
		"git_push_alert_active":        true,
		"git_push_alert_revision":      state.Revision,
		"git_push_alert_repository_id": state.TaskRepositoryID,
		"git_push_alert_legacy_single_repository": s.hasSingleTaskRepository(ctx, taskID, state.TaskRepositoryID),
		"actions": []map[string]interface{}{{
			"type": "ws_request", "label": "Fix", "icon": "sparkles",
			"tooltip": "Ask the agent to fix the git error", "test_id": "git-fix-button",
			"params": map[string]interface{}{
				"method":  "message.add",
				"payload": map[string]interface{}{"task_id": taskID, "session_id": sessionID, "content": fixPrompt},
			},
		}},
	}
}

func (s *Service) hasSingleTaskRepository(ctx context.Context, taskID, taskRepositoryID string) bool {
	if s.taskRepos == nil || strings.TrimSpace(taskID) == "" || strings.TrimSpace(taskRepositoryID) == "" {
		return false
	}
	repositories, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	return err == nil && len(repositories) == 1 && repositories[0] != nil && repositories[0].ID == taskRepositoryID
}

func (s *Service) resolveLegacyGitPushAlertMessage(ctx context.Context, messageID, resolution string, resolvedAt time.Time) error {
	if messageID == "" {
		return nil
	}
	message, err := s.messages.GetMessage(ctx, messageID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if message == nil {
		return nil
	}
	metadata := make(map[string]interface{}, len(message.Metadata)+3)
	for key, value := range message.Metadata {
		metadata[key] = value
	}
	metadata["git_operation_resolved"] = true
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	metadata["git_operation_resolved_at"] = resolvedAt.UTC().Format(time.RFC3339Nano)
	metadata["git_operation_resolution_source"] = resolution
	message.Metadata = metadata
	return s.UpdateMessage(ctx, message)
}

func (s *Service) resolveGitPushAlertMessage(
	ctx context.Context,
	messageID, resolution string,
	revision uint64,
	resolvedAt time.Time,
) error {
	if messageID == "" {
		return nil
	}
	message, err := s.messages.GetMessage(ctx, messageID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if message == nil {
		return nil
	}
	metadata := make(map[string]interface{}, len(message.Metadata)+5)
	for key, value := range message.Metadata {
		metadata[key] = value
	}
	metadata["git_operation_resolved"] = true
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	metadata["git_operation_resolved_at"] = resolvedAt.UTC().Format(time.RFC3339Nano)
	metadata["git_operation_resolution_source"] = resolution
	metadata["git_push_alert_active"] = false
	metadata["git_push_alert_revision"] = revision
	taskID, _ := metadata["task_id"].(string)
	taskRepositoryID, _ := metadata["git_push_alert_repository_id"].(string)
	metadata["git_push_alert_legacy_single_repository"] = s.hasSingleTaskRepository(ctx, taskID, taskRepositoryID)
	message.Metadata = metadata
	return s.UpdateMessage(ctx, message)
}
