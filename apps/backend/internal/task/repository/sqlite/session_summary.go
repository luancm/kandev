package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) BatchGetTaskSessionSummaryObservations(
	ctx context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSessionSummaryObservation, error) {
	result := make(map[string][]*models.TaskSessionSummaryObservation, len(taskIDs))
	ids := uniqueSessionSummaryTaskIDs(taskIDs)
	for _, taskID := range ids {
		result[taskID] = []*models.TaskSessionSummaryObservation{}
	}
	for _, chunk := range chunkIDs(ids, sqliteMaxHostParams) {
		placeholders, args := buildInPlaceholders(chunk)
		query := `SELECT ` + taskSessionSummarySelectCols(r.ro.DriverName()) + ` ` + taskSessionFromClause +
			` LEFT JOIN executors e ON e.id = ts.executor_id` +
			` WHERE ts.task_id IN (` + placeholders + `) ORDER BY ts.task_id, ts.started_at DESC`
		rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
		if err != nil {
			return nil, fmt.Errorf("query task session summaries: %w", err)
		}
		for rows.Next() {
			observation, err := scanTaskSessionSummaryObservation(rows)
			if err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan task session summary: %w", err)
			}
			result[observation.TaskID] = append(result[observation.TaskID], observation)
		}
		rowsErr := rows.Err()
		closeErr := rows.Close()
		if rowsErr != nil {
			return nil, fmt.Errorf("iterate task session summaries: %w", rowsErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close task session summaries: %w", closeErr)
		}
	}
	return result, nil
}

func (r *Repository) ListTaskSessionSummaryObservations(
	ctx context.Context,
	taskID string,
) ([]*models.TaskSessionSummaryObservation, error) {
	if taskID == "" {
		return []*models.TaskSessionSummaryObservation{}, nil
	}
	byTask, err := r.BatchGetTaskSessionSummaryObservations(ctx, []string{taskID})
	if err != nil {
		return nil, err
	}
	observations := byTask[taskID]
	if len(observations) == 0 {
		return observations, nil
	}
	sessions := make([]*models.TaskSession, len(observations))
	for index, observation := range observations {
		sessions[index] = observation.ToTaskSession()
	}
	loaded, err := r.loadWorktreesBatch(ctx, sessions)
	if err != nil {
		return nil, err
	}
	for index := range observations {
		observations[index].Worktrees = loaded[index].Worktrees
	}
	return observations, nil
}

func uniqueSessionSummaryTaskIDs(taskIDs []string) []string {
	seen := make(map[string]struct{}, len(taskIDs))
	ids := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if _, exists := seen[taskID]; exists {
			continue
		}
		seen[taskID] = struct{}{}
		ids = append(ids, taskID)
	}
	return ids
}

func taskSessionSummarySelectCols(driver string) string {
	metadata := "CASE WHEN ts.metadata IS NULL OR ts.metadata = 'null' OR ts.metadata = '' THEN '{}' ELSE ts.metadata END"
	if dialect.IsPostgres(driver) {
		metadata = "CASE WHEN ts.metadata IS NULL OR ts.metadata = 'null' OR ts.metadata = '' THEN '{}'::jsonb ELSE ts.metadata::jsonb END"
	}
	metadata = "(" + metadata + ")"
	modelID, modelName := sessionSummaryModelColumns(driver, metadata)
	columns := []string{
		"ts.id", "ts.task_id", "ts.queue_incarnation_id",
		"COALESCE(er.agent_execution_id, '')", "COALESCE(er.container_id, '')",
		"ts.agent_profile_id", "ts.execution_profile_id", "ts.route_generation", "ts.route_state", "ts.route_reason",
		"ts.executor_id", "ts.executor_profile_id", "ts.environment_id",
		"ts.repository_id", "ts.base_branch", "ts.base_commit_sha",
		"COALESCE(NULLIF(te.workspace_path, ''), ts.workspace_path)",
		dialect.JSONExtract(driver, "ts.agent_profile_snapshot", "name"),
		"e.type AS executor_type", "e.name AS executor_name",
		dialect.JSONExtract(driver, "ts.repository_snapshot", "path"),
		"ts.state", "ts.error_message", dialect.JSONExtract(driver, metadata, models.SessionMetaKeyLastAgentError), modelID, modelName,
	}
	columns = append(columns, sessionSummaryContextWindowColumns(driver, metadata)...)
	columns = append(columns,
		"ts.started_at", "ts.completed_at", "ts.updated_at", "ts.is_primary", "ts.review_status",
		"ts.is_passthrough", "ts.task_environment_id", "ts.name", "ts.last_read_message_id",
		"ts.role", "ts.parent_session_id",
	)
	return strings.Join(columns, ", ")
}

func sessionSummaryContextWindowColumns(driver, metadata string) []string {
	contextWindowPath := []string{models.SessionMetaKeyContextWindow}
	columns := []string{dialect.JSONType(driver, metadata, contextWindowPath...)}
	for _, field := range []string{"size", "used", "remaining", "efficiency"} {
		path := append(append([]string(nil), contextWindowPath...), field)
		columns = append(columns, sessionSummaryJSONScalar(driver, metadata, true, path...))
	}
	for _, field := range []string{"source", "timestamp"} {
		path := append(append([]string(nil), contextWindowPath...), field)
		columns = append(columns, sessionSummaryJSONScalar(driver, metadata, false, path...))
	}
	columns = append(columns, sessionSummaryJSONScalar(driver, metadata, true, models.SessionMetaKeyContextCompactionCount))
	return columns
}

func sessionSummaryJSONScalar(driver, metadata string, number bool, path ...string) string {
	var typeExpression string
	if number {
		typeExpression = dialect.JSONTypeIsNumber(driver, metadata, path...)
	} else {
		typeExpression = dialect.JSONTypeIsStringPath(driver, metadata, path...)
	}
	return "CASE WHEN " + typeExpression + " THEN " + dialect.JSONExtractPath(driver, metadata, path...) + " ELSE NULL END"
}

type sessionSummaryContextWindowProjection struct {
	typeName   string
	size       sql.NullFloat64
	used       sql.NullFloat64
	remaining  sql.NullFloat64
	efficiency sql.NullFloat64
	source     sql.NullString
	timestamp  sql.NullString
}

func (projection sessionSummaryContextWindowProjection) metadataValue() interface{} {
	if projection.typeName != "object" {
		return nil
	}
	window := make(map[string]interface{})
	if projection.size.Valid {
		window["size"] = projection.size.Float64
	}
	if projection.used.Valid {
		window["used"] = projection.used.Float64
	}
	if projection.remaining.Valid {
		window["remaining"] = projection.remaining.Float64
	}
	if projection.efficiency.Valid {
		window["efficiency"] = projection.efficiency.Float64
	}
	if projection.source.Valid {
		window["source"] = projection.source.String
	}
	if projection.timestamp.Valid {
		window["timestamp"] = projection.timestamp.String
	}
	return window
}

func scanTaskSessionSummaryObservation(rows *sql.Rows) (*models.TaskSessionSummaryObservation, error) {
	var observation models.TaskSessionSummaryObservation
	var (
		queueIncarnationID sql.NullString
		agentProfileID     sql.NullString
		executionProfileID sql.NullString
		routeState         sql.NullString
		routeReason        sql.NullString
		executorID         sql.NullString
		executorProfileID  sql.NullString
		environmentID      sql.NullString
		repositoryID       sql.NullString
		baseBranch         sql.NullString
		baseCommitSHA      sql.NullString
		workspacePath      sql.NullString
		agentProfileName   sql.NullString
		executorType       sql.NullString
		executorName       sql.NullString
		repositoryPath     sql.NullString
		state              sql.NullString
		errorMessage       sql.NullString
		modelID            sql.NullString
		modelName          sql.NullString
		lastAgentErrorJSON sql.NullString
		contextWindowType  sql.NullString
		contextWindowSize  sql.NullFloat64
		contextWindowUsed  sql.NullFloat64
		contextRemaining   sql.NullFloat64
		contextEfficiency  sql.NullFloat64
		contextSource      sql.NullString
		contextTimestamp   sql.NullString
		compactionCount    sql.NullFloat64
		completedAt        sql.NullTime
		isPrimary          int
		reviewStatus       sql.NullString
		isPassthrough      int
		taskEnvironmentID  sql.NullString
		name               sql.NullString
		lastReadMessageID  sql.NullString
	)
	if err := rows.Scan(
		&observation.ID, &observation.TaskID, &queueIncarnationID,
		&observation.AgentExecutionID, &observation.ContainerID,
		&agentProfileID, &executionProfileID, &observation.RouteGeneration,
		&routeState, &routeReason, &executorID, &executorProfileID,
		&environmentID, &repositoryID, &baseBranch, &baseCommitSHA, &workspacePath,
		&agentProfileName, &executorType, &executorName, &repositoryPath,
		&state, &errorMessage, &lastAgentErrorJSON, &modelID, &modelName,
		&contextWindowType, &contextWindowSize, &contextWindowUsed, &contextRemaining,
		&contextEfficiency, &contextSource, &contextTimestamp, &compactionCount,
		&observation.StartedAt, &completedAt, &observation.UpdatedAt, &isPrimary,
		&reviewStatus, &isPassthrough, &taskEnvironmentID, &name, &lastReadMessageID,
		&observation.Role, &observation.ParentSessionID,
	); err != nil {
		return nil, err
	}
	observation.QueueIncarnationID = sessionSummaryNullableString(queueIncarnationID)
	observation.AgentProfileID = sessionSummaryNullableString(agentProfileID)
	observation.ExecutionProfileID = sessionSummaryNullableString(executionProfileID)
	observation.RouteState = sessionSummaryNullableString(routeState)
	observation.RouteReason = sessionSummaryNullableString(routeReason)
	observation.ExecutorID = sessionSummaryNullableString(executorID)
	observation.ExecutorProfileID = sessionSummaryNullableString(executorProfileID)
	observation.EnvironmentID = sessionSummaryNullableString(environmentID)
	observation.RepositoryID = sessionSummaryNullableString(repositoryID)
	observation.BaseBranch = sessionSummaryNullableString(baseBranch)
	observation.BaseCommitSHA = sessionSummaryNullableString(baseCommitSHA)
	observation.WorkspacePath = sessionSummaryNullableString(workspacePath)
	observation.AgentProfileName = sessionSummaryNullableString(agentProfileName)
	observation.ExecutorType = sessionSummaryNullableString(executorType)
	observation.ExecutorName = sessionSummaryNullableString(executorName)
	observation.RepositoryPath = sessionSummaryNullableString(repositoryPath)
	observation.State = models.TaskSessionState(sessionSummaryNullableString(state))
	observation.ErrorMessage = sessionSummaryNullableString(errorMessage)
	observation.IsPrimary = isPrimary != 0
	observation.IsPassthrough = isPassthrough != 0
	observation.ReviewStatus = models.ReviewStatus(sessionSummaryNullableString(reviewStatus))
	observation.TaskEnvironmentID = sessionSummaryNullableString(taskEnvironmentID)
	observation.Name = sessionSummaryNullableString(name)
	observation.LastReadMessageID = sessionSummaryNullableString(lastReadMessageID)
	if completedAt.Valid {
		observation.CompletedAt = &completedAt.Time
	}
	contextMetadata := make(map[string]interface{})
	if contextWindowType.Valid {
		contextMetadata[models.SessionMetaKeyContextWindow] = sessionSummaryContextWindowProjection{
			typeName: contextWindowType.String, size: contextWindowSize, used: contextWindowUsed,
			remaining: contextRemaining, efficiency: contextEfficiency, source: contextSource, timestamp: contextTimestamp,
		}.metadataValue()
	}
	if compactionCount.Valid {
		contextMetadata[models.SessionMetaKeyContextCompactionCount] = compactionCount.Float64
	}
	metadata, err := sessionSummaryMetadata(lastAgentErrorJSON, modelID.String, modelName.String)
	if err != nil {
		return nil, err
	}
	for key, value := range models.ProjectSessionContextMetadata(contextMetadata) {
		metadata[key] = value
	}
	observation.Metadata = metadata
	return &observation, nil
}

func sessionSummaryMetadata(lastAgentErrorJSON sql.NullString, modelID, modelName string) (map[string]interface{}, error) {
	metadata := map[string]interface{}{models.SessionMetaKeyLastAgentError: nil}
	if lastAgentErrorJSON.Valid && lastAgentErrorJSON.String != "" && lastAgentErrorJSON.String != "null" {
		var lastAgentError interface{}
		if err := json.Unmarshal([]byte(lastAgentErrorJSON.String), &lastAgentError); err != nil {
			return nil, fmt.Errorf("decode last agent error projection: %w", err)
		}
		metadata[models.SessionMetaKeyLastAgentError] = lastAgentError
	}
	if label := models.SessionModelLabelMetadata(modelID, modelName); label != nil {
		metadata[models.SessionMetaKeyACPModelState] = label
	}
	return metadata, nil
}

func sessionSummaryNullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
