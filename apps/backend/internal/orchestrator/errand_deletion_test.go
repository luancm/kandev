package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/errands"
	"github.com/kandev/kandev/internal/task/models"
)

// deletingErrandController adds the deletion and sweep methods to the
// settlement fake.
type deletingErrandController struct {
	settlingErrandController
	preserved []string
	preserveE error
	sweeps    int
	sweepErr  error
}

func (c *deletingErrandController) PreserveCancellationNotice(_ context.Context, errandID string) error {
	c.preserved = append(c.preserved, errandID)
	return c.preserveE
}

func (c *deletingErrandController) SweepDelivery(context.Context) error {
	c.sweeps++
	return c.sweepErr
}

// @covers AC-TASKS-ERRANDS-007.3
func TestPreserveErrandNoticeOnDeleteKeepsRowWhenAdmissionRefused(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &deletingErrandController{preserveE: errors.New("queue full")}
	svc.SetErrandController(controller)

	err := svc.preserveErrandNoticeOnDelete(context.Background(), "errand-1")
	require.Error(t, err, "a refused notice must stop the delete so the notice is not lost")
	require.Equal(t, []string{"errand-1"}, controller.preserved)
}

func TestPreserveErrandNoticeOnDeleteIgnoresSessionThatIsNotAnErrand(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &deletingErrandController{preserveE: models.ErrErrandNotFound}
	svc.SetErrandController(controller)

	require.NoError(t, svc.preserveErrandNoticeOnDelete(context.Background(), "session-1"))

	svc.SetErrandController(nil)
	require.NoError(t, svc.preserveErrandNoticeOnDelete(context.Background(), "session-1"))
}

func TestPreserveErrandNoticeOnDeleteFallsBackToCancelWithoutNotifier(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &recordingErrandController{}
	svc.SetErrandController(controller)

	require.NoError(t, svc.preserveErrandNoticeOnDelete(context.Background(), "session-1"))
	require.Equal(t, []string{"cancel-on-stop:session-1"}, controller.seen())
}

// @covers AC-TASKS-ERRANDS-008.2
func TestSweepErrandDeliveryRunsSweepAndToleratesFailure(t *testing.T) {
	svc := &Service{logger: testLogger()}
	controller := &deletingErrandController{sweepErr: errors.New("list failed")}
	svc.SetErrandController(controller)

	require.NotPanics(t, func() { svc.sweepErrandDelivery(context.Background()) })
	require.Equal(t, 1, controller.sweeps)

	svc.SetErrandController(nil)
	require.NotPanics(t, func() { svc.sweepErrandDelivery(context.Background()) })
}

type deletionNoticeAdmission struct {
	content       []string
	metadata      []map[string]interface{}
	pendingParent []string
}

func (d *deletionNoticeAdmission) NotificationBlocked(context.Context, string) bool { return false }

func (d *deletionNoticeAdmission) DeliverPending(_ context.Context, parentID string) {
	d.pendingParent = append(d.pendingParent, parentID)
}

func (d *deletionNoticeAdmission) AdmitPrepared(_ context.Context, _, content string, metadata map[string]interface{}) error {
	d.content = append(d.content, content)
	d.metadata = append(d.metadata, metadata)
	return nil
}

type deletionNoticeLauncher struct{}

func (deletionNoticeLauncher) LaunchErrand(context.Context, errands.LaunchRequest) error { return nil }

type deletionNoticeStopper struct{}

func (deletionNoticeStopper) StopErrandSession(context.Context, string, string) error { return nil }

type deletionNoticeProfiles struct{}

func (deletionNoticeProfiles) ResolveProfile(context.Context, string, string, string) (errands.ResolvedProfile, error) {
	return errands.ResolvedProfile{}, nil
}

func (deletionNoticeProfiles) CompatibleProfiles(context.Context, string) ([]errands.ProfileInfo, error) {
	return nil, nil
}

type deletionNoticeMCP struct{}

func (deletionNoticeMCP) SessionServerIDs(context.Context, string, string) ([]string, error) {
	return nil, nil
}

type deletionNoticeEvents struct{}

func (deletionNoticeEvents) PublishErrandUpdated(context.Context, *models.Errand, models.ErrandStatus) {
}

// @covers AC-TASKS-ERRANDS-003.5 AC-TASKS-ERRANDS-012.2
func TestDeleteErrandSessionAdmitsCancellationNoticeOnlyWhenEnabled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
	}{
		{name: "disabled retained session", enabled: false},
		{name: "enabled deletion", enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCancelTurnFixture(t)
			errID := "errand-delete"
			require.NoError(t, f.repo.CreateErrand(t.Context(), &models.Errand{
				ID: errID, TaskID: "t1", ParentSessionID: "s1", Title: "Review the change",
				Objective: "Review the change", AgentProfileID: "profile-reviewer", FirstTurnID: "turn-first",
				SubmissionID: "submission-first", Lifecycle: models.ErrandLifecycleLaunched,
				AwaitingResult: true, ResultState: models.ErrandResultNone,
			}))
			require.NoError(t, f.repo.CreateTaskSession(t.Context(), &models.TaskSession{
				ID: errID, TaskID: "t1", Role: models.SessionRoleErrand, ParentSessionID: "s1",
				State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(),
			}))

			admission := &deletionNoticeAdmission{}
			errService, err := errands.New(errands.Dependencies{
				Store: f.repo, Launcher: deletionNoticeLauncher{}, Stopper: deletionNoticeStopper{},
				Profiles: deletionNoticeProfiles{}, MCP: deletionNoticeMCP{}, Events: deletionNoticeEvents{},
				Deliverer: admission,
			})
			require.NoError(t, err)
			errService.SetEnabled(func() bool { return tc.enabled })
			f.svc.SetErrandController(errService)

			require.NoError(t, f.svc.DeleteSession(t.Context(), errID))
			_, sessionErr := f.repo.GetTaskSession(t.Context(), errID)
			require.ErrorIs(t, sessionErr, models.ErrTaskSessionNotFound, "the retained errand session is deleted")
			_, errandErr := f.repo.GetErrand(t.Context(), errID)
			require.ErrorIs(t, errandErr, models.ErrErrandNotFound, "session deletion removes its bookkeeping row")

			if !tc.enabled {
				require.Empty(t, admission.content, "flag-off deletion must not admit cancellation output")
				require.Empty(t, admission.pendingParent, "flag-off deletion must not queue a parent notification")
				return
			}
			require.Len(t, admission.content, 1, "enabled deletion preserves one cancellation notice")
			require.Empty(t, admission.pendingParent, "deletion uses its prepared bounded notice")
			require.True(t, utf8.ValidString(admission.content[0]))
			require.LessOrEqual(t, len(admission.content[0]), 16*1024, "the prepared notice stays within the result byte bound")
			require.True(t, strings.Contains(admission.content[0], "The errand was cancelled."))
			require.Len(t, admission.metadata, 1)
			envelope, ok := admission.metadata[0][errands.DeliveryEnvelopeMetadataKey].(errands.DeliveryEnvelope)
			require.True(t, ok, "deletion notice carries its immutable delivery snapshot")
			require.Equal(t, models.ErrandDeliveryNotification, envelope.Route)
			require.Len(t, envelope.Items, 1)
			require.Equal(t, errID, envelope.Items[0].ErrandID)
			require.Equal(t, models.ErrandStatusCancelled, envelope.Items[0].Status)
			require.Equal(t, "The errand was cancelled.", envelope.Items[0].Preview)
			require.LessOrEqual(t, utf8.RuneCountInString(envelope.Items[0].Preview), 280)
		})
	}
}
