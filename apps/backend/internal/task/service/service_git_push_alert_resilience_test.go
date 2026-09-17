package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

var errGitPushAlertWrite = errors.New("injected alert projection write failure")

type failingGitPushAlertSessionStore struct {
	repository.SessionRepository
	failNext bool
}

func (r *failingGitPushAlertSessionStore) SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error {
	if key == models.SessionMetaKeyGitPushAlert && r.failNext {
		r.failNext = false
		return errGitPushAlertWrite
	}
	return r.SessionRepository.SetSessionMetadataKey(ctx, sessionID, key, value)
}

func TestService_GitPushRecoveryAfterProjectionWriteFailure(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	if err := svc.RecordGitPushSuccess(ctx, "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID, OccurredAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	svc.sessions = &failingGitPushAlertSessionStore{SessionRepository: svc.sessions, failNext: true}
	err := svc.RecordGitPushFailure(ctx, "sess-msg", GitPushFailure{
		TaskRepositoryID: gitPushAlertTaskRepositoryID,
		Diagnostic:       "remote rejected push",
		OccurredAt:       at.Add(time.Second),
	})
	if !errors.Is(err, errGitPushAlertWrite) {
		t.Fatalf("record failure returned %v, want injected persistence error", err)
	}
	if err := svc.RecordGitPushSuccess(ctx, "sess-msg", GitPushSuccess{
		TaskRepositoryID: gitPushAlertTaskRepositoryID, OccurredAt: at.Add(2 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	if state := readGitPushAlertState(t, repo); state.Active {
		t.Fatalf("successful recovery left active state: %+v", state)
	}
	messages, err := repo.ListMessages(ctx, "sess-msg")
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if message.Metadata["git_push_alert_active"] == true {
			t.Errorf("successful recovery left active Fix card %s after projection write failure", message.ID)
		}
	}
}

func TestService_GitPushConcurrentTransitionsPreserveNewestEvent(t *testing.T) {
	for _, lastIsSuccess := range []bool{false, true} {
		name := "latest failure"
		if lastIsSuccess {
			name = "latest success"
		}
		t.Run(name, func(t *testing.T) {
			svc, repo := newGitPushAlertTestService(t)
			ctx := context.Background()
			at := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
			start := make(chan struct{})
			results := make(chan error, 16)
			for i := range 16 {
				go func() {
					<-start
					occurredAt := at.Add(time.Duration(i) * time.Second)
					if (i%2 == 1) == lastIsSuccess {
						results <- svc.RecordGitPushSuccess(ctx, "sess-msg", GitPushSuccess{
							TaskRepositoryID: gitPushAlertTaskRepositoryID, OccurredAt: occurredAt,
						})
						return
					}
					results <- svc.RecordGitPushFailure(ctx, "sess-msg", GitPushFailure{
						TaskRepositoryID: gitPushAlertTaskRepositoryID, Diagnostic: "rejected", OccurredAt: occurredAt,
					})
				}()
			}
			close(start)
			for range 16 {
				if err := <-results; err != nil {
					t.Error(err)
				}
			}
			state := readGitPushAlertState(t, repo)
			if state.Active == lastIsSuccess || !state.OccurredAt.Equal(at.Add(15*time.Second)) {
				t.Fatalf("concurrent transitions lost newest event: %+v", state)
			}
			messages, err := repo.ListMessages(ctx, "sess-msg")
			if err != nil {
				t.Fatal(err)
			}
			active := 0
			for _, message := range messages {
				if message.Metadata["git_push_alert_active"] == true {
					active++
				}
			}
			want := 1
			if lastIsSuccess {
				want = 0
			}
			if active != want {
				t.Fatalf("active Fix cards = %d, want %d", active, want)
			}
		})
	}
}

func TestService_GitPushSuccessClearsAfterDestinationCorrection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remote string
		branch string
	}{
		{"corrected remote", "backup", "main"},
		{"renamed branch", "origin", "repaired-branch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newGitPushAlertTestService(t)
			ctx := context.Background()
			at := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
			recordGitPushFailure(t, svc, at, gitPushAlertTaskRepositoryID, "remote rejected push")
			if err := svc.RecordGitPushSuccess(ctx, "sess-msg", GitPushSuccess{
				TaskRepositoryID: gitPushAlertTaskRepositoryID,
				Remote:           tc.remote,
				Branch:           tc.branch,
				OccurredAt:       at.Add(time.Minute),
			}); err != nil {
				t.Fatal(err)
			}
			state := readGitPushAlertState(t, repo)
			if state.Active {
				t.Error("newer success for the same canonical repository did not clear the alert")
			}
			message, err := repo.GetMessage(ctx, state.MessageID)
			if err != nil {
				t.Fatal(err)
			}
			if message.Metadata["git_push_alert_active"] != false {
				t.Error("newer success after destination correction left the Fix card active")
			}
		})
	}
}

func TestService_GitPushStatusRejectsInvalidEvidence(t *testing.T) {
	svc, repo := newGitPushAlertTestService(t)
	at := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	recordGitPushFailure(t, svc, at, gitPushAlertTaskRepositoryID, strings.Repeat("rejected\n", 8192))
	cases := []struct {
		name   string
		change func(*GitPushStatusObservation)
	}{
		{"blank repository", func(o *GitPushStatusObservation) { o.TaskRepositoryID = " \t" }},
		{"missing branch", func(o *GitPushStatusObservation) { o.Branch = "" }},
		{"missing upstream", func(o *GitPushStatusObservation) { o.RemoteBranch = "" }},
		{"missing HEAD", func(o *GitPushStatusObservation) { o.HeadCommit = "" }},
		{"missing remote HEAD", func(o *GitPushStatusObservation) { o.RemoteHeadCommit = "" }},
		{"different HEAD", func(o *GitPushStatusObservation) { o.RemoteHeadCommit = "other" }},
		{"unknown ahead", func(o *GitPushStatusObservation) { o.RemoteAheadKnown = false }},
		{"unknown behind", func(o *GitPushStatusObservation) { o.RemoteBehindKnown = false }},
		{"negative ahead", func(o *GitPushStatusObservation) { o.RemoteAhead = -1 }},
		{"maximum behind", func(o *GitPushStatusObservation) { o.RemoteBehind = int(^uint(0) >> 1) }},
		{"zero timestamp", func(o *GitPushStatusObservation) { o.ObservedAt = time.Time{} }},
		{"equal timestamp", func(o *GitPushStatusObservation) { o.ObservedAt = at }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observation := synchronizedGitPushObservation(gitPushAlertTaskRepositoryID, at.Add(time.Minute))
			tc.change(&observation)
			if err := svc.ReconcileGitPushStatus(context.Background(), "sess-msg", observation); err != nil {
				t.Fatal(err)
			}
			if state := readGitPushAlertState(t, repo); !state.Active || state.Revision != 1 {
				t.Fatalf("invalid evidence changed alert: %+v", state)
			}
		})
	}
}
