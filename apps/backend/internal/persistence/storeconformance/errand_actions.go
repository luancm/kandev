package storeconformance

import (
	"errors"
	"fmt"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

// errandAction exercises task_session_errands through the task repository's
// errand API: version-guarded updates, boolean columns, and duplicate and
// stale-version conflicts.
func errandAction() apiAction {
	action := apiAction{name: "task_session_errands", key: recordIdentifier}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		taskID, parentSessionID, err := ensureConformanceTaskAndSession(s, id)
		if err != nil {
			return nil, err
		}
		return createConformanceErrand(s, id, taskID, parentSessionID)
	}
	action.read = readConformanceErrand
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		return updateConformanceErrand(s, id, func(errand *taskmodels.Errand) {
			errand.Lifecycle = taskmodels.ErrandLifecycleLaunched
			errand.ResultState = taskmodels.ErrandResultPending
			errand.ResultText = "updated-" + id
		})
	}
	action.assertUpdated = func(before, after any, id string) error {
		previous, okBefore := before.(*taskmodels.Errand)
		current, okAfter := after.(*taskmodels.Errand)
		if !okBefore || !okAfter || previous == nil || current == nil {
			return fmt.Errorf("GetErrand returned %T and %T", before, after)
		}
		if current.ResultText != "updated-"+id || current.Version != previous.Version+1 {
			return fmt.Errorf("errand after update = (%q, version %d), want (%q, version %d)",
				current.ResultText, current.Version, "updated-"+id, previous.Version+1)
		}
		return nil
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := taskFactory(s)
		if err != nil {
			return err
		}
		return invokeError(store, "DeleteErrand", s.Context, id)
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, value bool) (any, error) {
		if err := updateConformanceErrand(s, id, func(errand *taskmodels.Errand) {
			errand.AwaitingResult = value
			errand.ResultTruncated = value
		}); err != nil {
			return nil, err
		}
		return readConformanceErrand(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		errand, ok := record.(*taskmodels.Errand)
		if !ok || errand == nil || errand.AwaitingResult != errand.ResultTruncated {
			return false, false
		}
		return errand.AwaitingResult, true
	}
	action.conflict = errandConflict
	return action
}

func createConformanceErrand(s testconformance.ScenarioContext, id, taskID, parentSessionID string) (*taskmodels.Errand, error) {
	store, err := taskFactory(s)
	if err != nil {
		return nil, err
	}
	errand := &taskmodels.Errand{
		ID: id, TaskID: taskID, ParentSessionID: parentSessionID,
		Title: "Conformance errand", Objective: "Conformance objective",
		MCPServerIDs: []string{"conformance-mcp"},
		Lifecycle:    taskmodels.ErrandLifecycleQueued, ResultState: taskmodels.ErrandResultNone,
	}
	if err := invokeError(store, "CreateErrand", s.Context, errand); err != nil {
		return nil, err
	}
	return errand, nil
}

func readConformanceErrand(s testconformance.ScenarioContext, id string) (any, error) {
	store, err := taskFactory(s)
	if err != nil {
		return nil, err
	}
	return invokeRecord(store, "GetErrand", s.Context, id)
}

func updateConformanceErrand(s testconformance.ScenarioContext, id string, mutate func(*taskmodels.Errand)) error {
	store, err := taskFactory(s)
	if err != nil {
		return err
	}
	value, err := invokeRecord(store, "GetErrand", s.Context, id)
	if err != nil {
		return err
	}
	errand, ok := value.(*taskmodels.Errand)
	if !ok || errand == nil {
		return fmt.Errorf("GetErrand returned %T", value)
	}
	mutate(errand)
	return invokeError(store, "UpdateErrand", s.Context, errand)
}

func errandConflict(s testconformance.ScenarioContext, id string, record any) error {
	created, ok := record.(*taskmodels.Errand)
	if !ok || created == nil {
		return fmt.Errorf("errand create returned %T", record)
	}
	if _, err := createConformanceErrand(s, id, created.TaskID, created.ParentSessionID); err == nil {
		return fmt.Errorf("duplicate CreateErrand succeeded")
	}
	store, err := taskFactory(s)
	if err != nil {
		return err
	}
	value, err := invokeRecord(store, "GetErrand", s.Context, id)
	if err != nil {
		return err
	}
	stale, ok := value.(*taskmodels.Errand)
	if !ok || stale == nil {
		return fmt.Errorf("GetErrand returned %T", value)
	}
	winner := *stale
	winner.ResultState = taskmodels.ErrandResultDiscarded
	if err := invokeError(store, "UpdateErrand", s.Context, &winner); err != nil {
		return fmt.Errorf("first versioned update: %w", err)
	}
	stale.ResultState = taskmodels.ErrandResultSuperseded
	if err := invokeError(store, "UpdateErrand", s.Context, stale); !errors.Is(err, taskmodels.ErrErrandVersionConflict) {
		return fmt.Errorf("stale versioned update = %v, want ErrErrandVersionConflict", err)
	}
	return nil
}
