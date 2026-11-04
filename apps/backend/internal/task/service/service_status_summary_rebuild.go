package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// TaskStatusSummaryPRReader supplies already-persisted provider state for the
// one-time repair of a task whose summary row predates the projector.
type TaskStatusSummaryPRReader interface {
	ListTaskStatusSummaryPullRequests(
		context.Context,
		[]string,
	) (map[string][]statussummary.PullRequestInput, error)
}

// TaskStatusSummaryLaunchQueueReader supplies the task-owned launch queue
// projection and may refresh shared admission observations once for a batch.
// The task service does not depend on the orchestrator's controller type.
type TaskStatusSummaryLaunchQueueReader func(
	context.Context,
	[]*models.Task,
) map[string]*statussummary.LaunchQueueSummary

type runningSessionObservation struct {
	observed bool
	running  bool
}

type completionGateSummaryObservation struct {
	summary  *statussummary.CompletionGateSummary
	observed bool
	err      error
}

func runningSessionSummary(sessions []*models.TaskSession, observed bool) runningSessionObservation {
	result := runningSessionObservation{observed: observed}
	for _, session := range sessions {
		if observed && session != nil && session.State == models.TaskSessionStateRunning {
			result.running = true
			break
		}
	}
	return result
}

// SetTaskStatusSummaryPRReader wires the optional provider-backed PR reader.
// The task service keeps the interface narrow so it does not depend on the
// GitHub package.
func (s *Service) SetTaskStatusSummaryPRReader(reader TaskStatusSummaryPRReader) {
	if s != nil {
		s.statusSummaryPRs = reader
	}
}

// SetTaskStatusSummaryLaunchQueueReader wires the optional live admission
// observation used when boot and task-list summaries are rebuilt.
func (s *Service) SetTaskStatusSummaryLaunchQueueReader(reader TaskStatusSummaryLaunchQueueReader) {
	if s != nil {
		s.statusSummaryLaunchQueue = reader
	}
}

func isRequestCancellationError(ctx context.Context, err error) bool {
	return errors.Is(ctx.Err(), context.Canceled) && errors.Is(err, context.Canceled)
}

// ReconcileTaskStatusSummaries repairs stale status projections in existing rows
// and builds absent rows. A non-nil sessionsByTask map is a complete batch
// observation; a missing task key means that task has no sessions. All durable
// inputs are batch-loaded by the caller or optional readers, so startup does
// not scan every historical task. A present nil result is an explicit cache
// invalidation; an absent key remains an ordinary partial-response omission.
func (s *Service) ReconcileTaskStatusSummaries(
	ctx context.Context,
	tasks []*models.Task,
	sessionsByTask map[string][]*models.TaskSession,
	pendingBySession map[string]models.TaskPendingAction,
	summaries map[string]*statussummary.TaskStatusSummary,
) (map[string]*statussummary.TaskStatusSummary, error) {
	if summaries == nil {
		summaries = make(map[string]*statussummary.TaskStatusSummary)
	}
	if s == nil || s.statusSummaries == nil || len(tasks) == 0 {
		return summaries, nil
	}
	if err := ctx.Err(); err != nil {
		return summaries, err
	}
	ids := taskIDs(tasks)
	errandsByTask := s.loadSummaryErrands(ctx, ids)
	activityByTask, activityObserved := s.loadSummaryActivity(ctx, ids)
	if err := ctx.Err(); err != nil {
		return summaries, err
	}
	launchQueueByTask := s.launchQueueSummaries(ctx, tasks)
	if err := ctx.Err(); err != nil {
		return summaries, err
	}
	completionGates, completionGatesLoaded := s.loadCompletionGateSummaryObservations(ctx, ids)
	if err := ctx.Err(); err != nil {
		return summaries, err
	}
	var unread summaryUnreadObservation
	if sessionsByTask != nil {
		unread = s.loadSummaryUnread(ctx, flattenTaskSessions(tasks, sessionsByTask))
	}
	failedTaskIDs, reconcileErr := s.reconcileExistingSummaries(
		ctx, tasks, sessionsByTask, sessionsByTask != nil, pendingBySession, summaries, activityByTask,
		activityObserved, launchQueueByTask, completionGates, completionGatesLoaded, errandsByTask, unread,
	)
	if err := ctx.Err(); err != nil {
		return summaries, err
	}
	rebuildTasks := tasks
	if len(failedTaskIDs) > 0 {
		rebuildTasks = make([]*models.Task, 0, len(tasks)-len(failedTaskIDs))
		for _, task := range tasks {
			if task == nil {
				continue
			}
			if _, failed := failedTaskIDs[task.ID]; !failed {
				rebuildTasks = append(rebuildTasks, task)
			}
		}
	}
	var rebuildErr error
	summaries, rebuildErr = s.rebuildMissingSummaries(
		ctx, rebuildTasks, sessionsByTask, pendingBySession, summaries, activityByTask,
		activityObserved, launchQueueByTask, completionGates, completionGatesLoaded, errandsByTask, unread,
	)
	if err := ctx.Err(); err != nil {
		return summaries, err
	}
	return summaries, errors.Join(reconcileErr, rebuildErr)
}

func (s *Service) reconcileExistingSummaries(
	ctx context.Context,
	tasks []*models.Task,
	sessionsByTask map[string][]*models.TaskSession,
	sessionsObserved bool,
	pendingBySession map[string]models.TaskPendingAction,
	summaries map[string]*statussummary.TaskStatusSummary,
	activityByTask map[string]time.Time,
	activityObserved bool,
	launchQueueByTask map[string]*statussummary.LaunchQueueSummary,
	completionGates map[string]completionGateSummaryObservation,
	completionGatesLoaded bool,
	errandsByTask map[string]errandsSummaryObservation,
	unread summaryUnreadObservation,
) (map[string]struct{}, error) {
	var reconcileErr error
	failedTaskIDs := make(map[string]struct{})
	for _, task := range tasks {
		if ctx.Err() != nil {
			return failedTaskIDs, reconcileErr
		}
		if task == nil || task.ID == "" {
			continue
		}
		if observation := errandsByTask[task.ID]; observation.err != nil {
			failedTaskIDs[task.ID] = struct{}{}
			s.logSummaryRepairFailure(ctx, task.ID, "errands", observation.err)
			reconcileErr = errors.Join(reconcileErr, fmt.Errorf("load task %s errands: %w", task.ID, observation.err))
			continue
		}
		if summaries[task.ID] == nil {
			continue
		}
		sessions := sessionsByTask[task.ID]
		runningSessions := runningSessionSummary(sessions, sessionsObserved)
		action := pendingActionForTask(sessions, pendingBySession)
		var reconciled *statussummary.TaskStatusSummary
		var err error
		observation := completionGates[task.ID]
		unreadValue := unread.forTask(sessions)
		if completionGatesLoaded {
			if _, ok := completionGates[task.ID]; !ok {
				observation.err = fmt.Errorf("completion-gate batch omitted task %q", task.ID)
			}
		} else {
			observation.summary, observation.observed, observation.err = s.completionGateSummary(ctx, task.ID)
		}
		errandsObservation := errandsByTask[task.ID]
		reconciled, err = s.reconcileExistingSummaryWithObservations(ctx, task, summaries[task.ID], action, activityByTask[task.ID], activityObserved, runningSessions, observation, &errandsObservation, unreadValue, launchQueueByTask[task.ID])
		errandsByTask[task.ID] = errandsObservation
		if err != nil {
			if errandsObservation.err != nil && reconciled != nil {
				summaries[task.ID] = reconciled
			} else {
				// A present nil entry is an explicit invalidation. DTO assembly
				// distinguishes it from an ordinarily absent partial projection so
				// clients can clear a known-stale cached summary.
				summaries[task.ID] = nil
			}
			failedTaskIDs[task.ID] = struct{}{}
			s.logSummaryRepairFailure(ctx, task.ID, "reconcile", err)
			reconcileErr = errors.Join(
				reconcileErr,
				fmt.Errorf("reconcile task %s status summary: %w", task.ID, err),
			)
			if ctx.Err() != nil {
				return failedTaskIDs, reconcileErr
			}
			continue
		}
		if reconciled == nil {
			delete(summaries, task.ID)
			continue
		}
		summaries[task.ID] = reconciled
	}
	return failedTaskIDs, reconcileErr
}

func (s *Service) rebuildMissingSummaries(
	ctx context.Context,
	tasks []*models.Task,
	sessionsByTask map[string][]*models.TaskSession,
	pendingBySession map[string]models.TaskPendingAction,
	summaries map[string]*statussummary.TaskStatusSummary,
	activityByTask map[string]time.Time,
	activityObserved bool,
	launchQueueByTask map[string]*statussummary.LaunchQueueSummary,
	completionGates map[string]completionGateSummaryObservation,
	completionGatesLoaded bool,
	errandsByTask map[string]errandsSummaryObservation,
	unread summaryUnreadObservation,
) (map[string]*statussummary.TaskStatusSummary, error) {
	if ctx.Err() != nil {
		return summaries, nil
	}
	missing := missingSummaryTasks(tasks, summaries)
	if len(missing) == 0 {
		return summaries, nil
	}
	prByTask, prObserved := s.loadSummaryPRs(ctx, taskIDs(missing))
	if ctx.Err() != nil {
		return summaries, nil
	}
	environmentIDsByTask, environmentIDs := s.taskEnvironmentIDsForTasks(ctx, missing, sessionsByTask)
	if ctx.Err() != nil {
		return summaries, nil
	}
	gitByEnvironment, gitObserved := s.loadSummaryGit(ctx, environmentIDs)
	if ctx.Err() != nil {
		return summaries, nil
	}
	queuedByTask := s.loadQueuedSummaryCounts(ctx, taskIDs(missing))
	if ctx.Err() != nil {
		return summaries, nil
	}
	activityAtByTask := activityByTask
	now := time.Now().UTC()
	var rebuildErr error
	for _, task := range missing {
		if ctx.Err() != nil {
			return summaries, rebuildErr
		}
		activityAt := activityAtByTask[task.ID]
		var completionGate *statussummary.CompletionGateSummary
		var completionGateObserved bool
		var gateErr error
		if completionGatesLoaded {
			observation, ok := completionGates[task.ID]
			if !ok {
				gateErr = fmt.Errorf("completion-gate batch omitted task %q", task.ID)
			} else {
				completionGate, completionGateObserved, gateErr = observation.summary, observation.observed, observation.err
			}
		} else {
			completionGate, completionGateObserved, gateErr = s.completionGateSummary(ctx, task.ID)
		}
		if gateErr != nil {
			s.logSummaryRepairFailure(ctx, task.ID, "completion gate", gateErr)
			if ctx.Err() != nil {
				return summaries, rebuildErr
			}
			continue
		}
		if ctx.Err() != nil {
			return summaries, rebuildErr
		}
		errandsObservation := errandsByTask[task.ID]
		input := s.rebuildInput(
			taskLaunchErrorSummary(task),
			taskTurnErrorSummary(task),
			true,
			sessionsByTask[task.ID],
			environmentIDsByTask[task.ID],
			pendingBySession,
			gitByEnvironment,
			gitObserved,
			prByTask[task.ID],
			prObserved,
			queuedByTask[task.ID],
			launchQueueByTask[task.ID],
			completionGate,
			completionGateObserved,
			errandsObservation,
			activityAt,
			activityObserved,
			sessionsByTask != nil,
			now,
		)
		unread.applyToRebuild(&input)
		if err := s.rebuildMissingSummary(ctx, task, summaries, input); err != nil {
			rebuildErr = errors.Join(rebuildErr, fmt.Errorf("reconcile missing task %s status summary: %w", task.ID, err))
		}
	}
	return summaries, rebuildErr
}

func (s *Service) loadQueuedSummaryCounts(ctx context.Context, taskIDs []string) map[string]int {
	if ctx.Err() != nil {
		return nil
	}
	queuedByTask, err := s.CountPendingQueuedByTaskIDs(ctx, taskIDs)
	if err == nil {
		if ctx.Err() != nil {
			return nil
		}
		return queuedByTask
	}
	if s.logger != nil && !isRequestCancellationError(ctx, err) {
		s.logger.Warn("failed to load queued prompt counts for status summary repair", zap.Error(err))
	}
	return map[string]int{}
}

func (s *Service) loadSummaryActivity(ctx context.Context, taskIDs []string) (map[string]time.Time, bool) {
	if ctx.Err() != nil || s.taskActivity == nil || len(taskIDs) == 0 {
		return nil, false
	}
	activityByTask, err := s.taskActivity.LoadTaskLastActivity(ctx, taskIDs)
	if err != nil {
		if s.logger != nil && !isRequestCancellationError(ctx, err) {
			s.logger.Warn("failed to load task activity for status summary repair", zap.Error(err))
		}
		return nil, false
	}
	if ctx.Err() != nil {
		return nil, false
	}
	return activityByTask, true
}

func (s *Service) launchQueueSummaries(
	ctx context.Context,
	tasks []*models.Task,
) map[string]*statussummary.LaunchQueueSummary {
	if ctx.Err() != nil {
		return nil
	}
	if s.statusSummaryLaunchQueue != nil {
		if queues := s.statusSummaryLaunchQueue(ctx, tasks); queues != nil {
			return queues
		}
	}
	queues := make(map[string]*statussummary.LaunchQueueSummary, len(tasks))
	for _, task := range tasks {
		if task == nil || task.ID == "" {
			continue
		}
		queues[task.ID] = statussummary.LaunchQueueSummaryFromTask(task)
	}
	return queues
}

func cloneLaunchQueueForService(queue *statussummary.LaunchQueueSummary) *statussummary.LaunchQueueSummary {
	if queue == nil {
		return nil
	}
	copy := *queue
	if queue.Capacity != nil {
		capacity := *queue.Capacity
		copy.Capacity = &capacity
	}
	return &copy
}

func statussummaryLaunchQueueEqual(left, right *statussummary.LaunchQueueSummary) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.SessionID != right.SessionID || left.AgentProfileID != right.AgentProfileID ||
		left.WorkflowStepID != right.WorkflowStepID || !left.QueuedAt.Equal(right.QueuedAt) ||
		left.Reason != right.Reason || left.Retrying != right.Retrying {
		return false
	}
	if left.Capacity == nil || right.Capacity == nil {
		return left.Capacity == right.Capacity
	}
	// The sample time is an in-memory freshness overlay. Persist only changes
	// to the queue's identity or capacity values.
	return left.Capacity.InUse == right.Capacity.InUse &&
		left.Capacity.Limit == right.Capacity.Limit
}

func (s *Service) completionGateSummary(
	ctx context.Context,
	taskID string,
) (*statussummary.CompletionGateSummary, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if observations, loaded := s.loadCompletionGateSummaryObservations(ctx, []string{taskID}); loaded {
		observation, ok := observations[taskID]
		if !ok {
			return nil, true, fmt.Errorf("completion-gate batch omitted task %q", taskID)
		}
		return observation.summary, observation.observed, observation.err
	}
	reader, ok := s.tasks.(repository.TaskCompletionGateRepository)
	if !ok {
		return nil, false, nil
	}
	snapshot, err := reader.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return nil, false, nil
		}
		return nil, true, err
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return statussummary.CompletionGateSummaryFromSnapshot(snapshot), true, nil
}

func (s *Service) loadCompletionGateSummaryObservations(
	ctx context.Context,
	taskIDs []string,
) (map[string]completionGateSummaryObservation, bool) {
	reader, ok := s.tasks.(repository.TaskCompletionGateSummaryReader)
	if !ok {
		return nil, false
	}
	observations := make(map[string]completionGateSummaryObservation, len(taskIDs))
	if len(taskIDs) == 0 {
		return observations, true
	}
	batch, err := reader.GetTaskCompletionGateSummaries(ctx, taskIDs)
	if err != nil {
		wrapped := fmt.Errorf("load completion-gate summaries: %w", err)
		for _, taskID := range taskIDs {
			observations[taskID] = completionGateSummaryObservation{observed: true, err: wrapped}
		}
		return observations, true
	}
	missing := make(map[string]struct{}, len(batch.MissingTaskIDs))
	for _, taskID := range batch.MissingTaskIDs {
		missing[taskID] = struct{}{}
	}
	for _, taskID := range taskIDs {
		if _, isMissing := missing[taskID]; isMissing {
			observations[taskID] = completionGateSummaryObservation{}
			continue
		}
		gate, exists := batch.ByTaskID[taskID]
		if !exists {
			observations[taskID] = completionGateSummaryObservation{
				observed: true,
				err:      fmt.Errorf("completion-gate batch omitted task %q without marking it missing", taskID),
			}
			continue
		}
		observations[taskID] = completionGateSummaryObservation{
			summary:  completionGateSummaryFromObservation(gate),
			observed: true,
		}
	}
	return observations, true
}

func completionGateSummaryFromObservation(
	observation *models.TaskCompletionGateSummaryObservation,
) *statussummary.CompletionGateSummary {
	if observation == nil {
		return nil
	}
	return &statussummary.CompletionGateSummary{
		Revision: observation.Revision, CriteriaCount: observation.CriteriaCount,
		VerifiedCount: observation.VerifiedCount, BlockerCount: observation.BlockerCount,
		Blocked: observation.Blocked,
	}
}

func (s *Service) rebuildMissingSummary(
	ctx context.Context,
	task *models.Task,
	summaries map[string]*statussummary.TaskStatusSummary,
	input statussummary.RebuildInput,
) error {
	if ctx.Err() != nil || task == nil || task.ID == "" {
		return nil
	}
	next := statussummary.BuildFromAuthoritative(input)
	next.Revision = 1
	next.UpdatedAt = input.Now
	if err := next.Validate(); err != nil {
		s.logSummaryRepairFailure(ctx, task.ID, "validate", err)
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}
	accepted, err := s.statusSummaries.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID:      task.ID,
		WorkspaceID: task.WorkspaceID,
		Summary:     next,
	})
	if err != nil {
		s.logSummaryRepairFailure(ctx, task.ID, "persist", err)
		return nil
	}
	if accepted {
		summaries[task.ID] = &next
		s.publishReconciledSummary(ctx, task, next)
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}
	reloaded, err := s.reloadSummaryReconcileState(ctx, task.ID, true)
	if reloaded.summaryObserved && reloaded.current != nil {
		summaries[task.ID] = reloaded.current
	}
	if err != nil {
		s.logSummaryRepairFailure(ctx, task.ID, "reload", err)
		return err
	}
	if !reloaded.summaryObserved || reloaded.current == nil {
		if reloaded.errands.err != nil {
			s.logSummaryRepairFailure(ctx, task.ID, "errands", reloaded.errands.err)
			return fmt.Errorf("load errands after compare-and-set rejection: %w", reloaded.errands.err)
		}
		if _, _, err := completionGateObservationValue(ctx, reloaded.completionGate); err != nil {
			s.logSummaryRepairFailure(ctx, task.ID, "completion gate", err)
			return fmt.Errorf("reload completion gate after compare-and-set rejection: %w", err)
		}
		return nil
	}
	if reloaded.errands.err != nil {
		s.logSummaryRepairFailure(ctx, task.ID, "errands", reloaded.errands.err)
		return fmt.Errorf("load errands after compare-and-set rejection: %w", reloaded.errands.err)
	}
	launchQueue, _ := summaryReconcileLaunchQueue(task, reloaded.current, []*statussummary.LaunchQueueSummary{reloaded.launchQueue})
	var activityAt time.Time
	activityObserved := input.LastActivityAt != nil
	if input.LastActivityAt != nil {
		activityAt = *input.LastActivityAt
	}
	errands := reloaded.errands
	unread, err := s.reloadTaskUnread(ctx, task.ID, summaryUnreadValue{observed: input.UnreadObserved})
	if err != nil {
		s.logSummaryRepairFailure(ctx, task.ID, "reload unread", err)
		return fmt.Errorf("reload unread state: %w", err)
	}
	reconciled, err := s.reconcileExistingSummaryWithObservations(
		ctx, task, reloaded.current, reloaded.pendingAction, activityAt, activityObserved,
		reloaded.runningSessions,
		reloaded.completionGate,
		&errands, unread, launchQueue,
	)
	if err != nil {
		if reconciled != nil {
			summaries[task.ID] = reconciled
		}
		s.logSummaryRepairFailure(ctx, task.ID, "reconcile compare-and-set winner", err)
		return err
	}
	if reconciled == nil {
		delete(summaries, task.ID)
		return nil
	}
	summaries[task.ID] = reconciled
	return nil
}

const maxSummaryReconcileAttempts = 3
