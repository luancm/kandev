package orchestrator

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func launchRecoveryFixture(t *testing.T) errandLaunchRecoveryEvidence {
	t.Helper()
	row := &models.Errand{ID: "reserved-child", TaskID: "reserved-task", ParentSessionID: "reserved-parent",
		FirstTurnID: "reserved-first-turn", SubmissionID: "reserved-submission", AgentProfileID: "reserved-profile", Lifecycle: models.ErrandLifecycleQueued}
	session := &models.TaskSession{ID: row.ID, TaskID: row.TaskID, ParentSessionID: row.ParentSessionID,
		Role: models.SessionRoleErrand, AgentProfileID: row.AgentProfileID, QueueIncarnationID: "session-incarnation", State: models.TaskSessionStateCreated}
	require.NoError(t, models.BindErrandLaunchAuthority(session, models.ReservedLaunchIdentity{
		TaskID: row.TaskID, SessionID: row.ID, TurnID: row.FirstTurnID, SubmissionID: row.SubmissionID,
	}, row.ParentSessionID))
	// Exercise the producer's persisted JSON shape, not a normalized test map.
	encoded, err := json.Marshal(session.Metadata)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &session.Metadata))
	turn := &models.Turn{ID: row.FirstTurnID, TaskSessionID: row.ID, TaskID: row.TaskID, ExecutionProfileID: row.AgentProfileID,
		Metadata: map[string]interface{}{models.TurnMetaKeyPromptDispatchPending: true}}
	current := *turn
	current.Metadata = nil
	return errandLaunchRecoveryEvidence{
		Reservation: row, Task: &models.Task{ID: row.TaskID},
		Parent:  &models.TaskSession{ID: row.ParentSessionID, TaskID: row.TaskID, State: models.TaskSessionStateWaitingForInput},
		Session: session, ReservedTurn: turn, CurrentTurn: &current, SubmissionAbsent: true, RuntimeAbsent: true,
	}
}

func recoveryRuntime(e *errandLaunchRecoveryEvidence) {
	e.RuntimeAbsent = false
	e.Runtime = &errandLaunchRuntimeEvidence{ExecutionID: "restored-execution", TaskID: e.Reservation.TaskID,
		SessionID: e.Reservation.ID, IncarnationID: e.Session.QueueIncarnationID,
		TurnID: e.Reservation.FirstTurnID, SubmissionID: normalizeDeliverySubmissionID(e.Reservation.SubmissionID)}
}

func recoverySubmission(e *errandLaunchRecoveryEvidence, state models.DeliverySubmissionState) {
	e.SubmissionAbsent = false
	e.Submission = &models.AgentDeliverySubmission{ID: normalizeDeliverySubmissionID(e.Reservation.SubmissionID),
		SessionID: e.Session.ID, IncarnationID: e.Session.QueueIncarnationID, State: state, DispatchAttemptID: "durable-dispatch-attempt"}
	if state == models.DeliverySubmissionPrepared {
		e.Submission.DispatchAttemptID = ""
	}
}

func recoveryTerminal(e *errandLaunchRecoveryEvidence, state models.DeliverySubmissionState) {
	recoverySubmission(e, state)
	completed := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	e.ReservedTurn.CompletedAt = &completed
	e.Submission.TerminalTurnID = e.Reservation.FirstTurnID
}

func recoveryAbsent(e *errandLaunchRecoveryEvidence) {
	e.Session, e.ReservedTurn, e.CurrentTurn = nil, nil, nil
	e.SessionAbsent, e.ReservedTurnAbsent, e.CurrentTurnAbsent = true, true, true
}

func corruptRecoveryAuthority(e *errandLaunchRecoveryEvidence, key string, value interface{}) {
	e.Session.Metadata[models.SessionMetaKeyErrandLaunchAuthority].(map[string]interface{})[key] = value
}

// @covers AC-TASKS-ERRANDS-001.1 AC-TASKS-ERRANDS-002.1 AC-TASKS-ERRANDS-004.8 AC-TASKS-ERRANDS-008.2
func TestErrandLaunchRecoveryRequiresCompleteAuthority(t *testing.T) {
	for _, tc := range launchRecoveryCases() {
		t.Run(tc.name, func(t *testing.T) {
			evidence := launchRecoveryFixture(t)
			tc.change(&evidence)
			before, err := json.Marshal(evidence)
			require.NoError(t, err)
			state, err := classifyErrandLaunchRecovery(evidence)
			require.Equal(t, tc.want, state, "classification error: %v", err)
			if tc.want == errandLaunchRecoveryBlocked {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			after, err := json.Marshal(evidence)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after), "classification must not mutate evidence")
		})
	}
}

type launchRecoveryCase struct {
	name   string
	want   errandLaunchRecoveryState
	change func(*errandLaunchRecoveryEvidence)
}

func launchRecoveryCases() []launchRecoveryCase {
	cases := []launchRecoveryCase{
		{"confirmed absent with live owner", errandLaunchRecoveryAbsent, recoveryAbsent},
		{"prepared exact unattempted turn", errandLaunchRecoveryPrepared, func(*errandLaunchRecoveryEvidence) {}},
		{"prepared turn absent", errandLaunchRecoveryPrepared, func(e *errandLaunchRecoveryEvidence) {
			e.ReservedTurn = nil
			e.ReservedTurnAbsent = true
			e.CurrentTurn = nil
			e.CurrentTurnAbsent = true
		}},
		{"prepared submission no dispatch attempt", errandLaunchRecoveryPrepared, func(e *errandLaunchRecoveryEvidence) { recoverySubmission(e, models.DeliverySubmissionPrepared) }},
		{"exact runtime without durable receipt", errandLaunchRecoveryAdmitted, recoveryRuntime},
		{"runtime receipt outranks stale prepared submission", errandLaunchRecoveryAdmitted, func(e *errandLaunchRecoveryEvidence) {
			recoveryRuntime(e)
			recoverySubmission(e, models.DeliverySubmissionPrepared)
		}},
		{"initial profile survives later profile selection", errandLaunchRecoveryAdmitted, func(e *errandLaunchRecoveryEvidence) {
			e.Session.AgentProfileID = "later-profile"
			recoverySubmission(e, models.DeliverySubmissionAccepted)
		}},
		{"dead task owner", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryAbsent(e)
			archived := time.Now()
			e.Task.ArchivedAt = &archived
		}},
		{"missing owner", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { recoveryAbsent(e); e.Task = nil }},
		{"missing parent", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Parent = nil }},
		{"dead parent owner", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Parent.State = models.TaskSessionStateCompleted }},
		{"wrong owner task", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Task.ID = "foreign-owner-task" }},
		{"wrong parent task", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Parent.TaskID = "foreign-parent-task" }},
		{"wrong parent id", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Parent.ID = "foreign-owner-parent" }},
		{"errand parent", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Parent.Role = models.SessionRoleErrand }},
		{"cancelled reservation", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Reservation.Lifecycle = models.ErrandLifecycleCancelled }},
		{"read error despite runtime match", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryRuntime(e)
			e.ReadError = errors.New("reserved submission metadata read failed")
		}},
		{"unknown session absence", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { recoveryAbsent(e); e.SessionAbsent = false }},
		{"unknown turn absence", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { recoveryAbsent(e); e.ReservedTurnAbsent = false }},
		{"unknown current turn absence", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { recoveryAbsent(e); e.CurrentTurnAbsent = false }},
		{"unknown submission absence", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { recoveryAbsent(e); e.SubmissionAbsent = false }},
		{"unknown runtime absence", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.RuntimeAbsent = false }},
		{"wrong canonical task", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Session.TaskID = "foreign-session-task" }},
		{"wrong canonical parent", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Session.ParentSessionID = "foreign-session-parent" }},
		{"wrong canonical role same id", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Session.Role = "" }},
		{"primary errand", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Session.IsPrimary = true }},
		{"missing session incarnation", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Session.QueueIncarnationID = "" }},
		{"stored provenance wrong turn", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			corruptRecoveryAuthority(e, "first_turn_id", "foreign-authority-turn")
		}},
		{"stored provenance wrong task", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			corruptRecoveryAuthority(e, "task_id", "foreign-authority-task")
		}},
		{"stored provenance wrong parent", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			corruptRecoveryAuthority(e, "parent_session_id", "foreign-authority-parent")
		}},
		{"stored provenance wrong submission", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			corruptRecoveryAuthority(e, "submission_id", "foreign-authority-submission")
		}},
		{"stored provenance wrong initial profile", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			corruptRecoveryAuthority(e, "agent_profile_id", "foreign-authority-profile")
		}},
		{"stored provenance unsupported version", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { corruptRecoveryAuthority(e, "version", 2) }},
		{"wrong reserved turn id", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.ReservedTurn.ID = "foreign-loaded-turn" }},
		{"wrong reserved turn task", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.ReservedTurn.TaskID = "foreign-turn-task" }},
		{"wrong reserved turn owner", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.ReservedTurn.TaskSessionID = "foreign-turn-session" }},
		{"lifecycle turn", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.ReservedTurn.Metadata[models.TurnMetaKeyLifecycleOnly] = true }},
		{"unattempted not proved by open turn", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.ReservedTurn.Metadata = nil }},
		{"dispatch attempt without receipt", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			e.ReservedTurn.Metadata[models.TurnMetaKeyPromptDispatchAttempted] = true
		}},
		{"prepared cannot explain native conversation", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Session.DownstreamACPSessionID = "native-conversation" }},
		{"running state does not prove admission", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Session.State = models.TaskSessionStateRunning }},
		{"prepared submission with attempted dispatch", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoverySubmission(e, models.DeliverySubmissionPrepared)
			e.Submission.DispatchAttemptID = "attempt-with-unknown-effect"
		}},
		{"accepted requires exact reserved turn", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			e.ReservedTurn = nil
			e.ReservedTurnAbsent = true
		}},
		{"durable foreign incarnation", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			e.Submission.IncarnationID = "foreign-submission-incarnation"
		}},
		{"durable wrong session", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			e.Submission.SessionID = "foreign-submission-session"
		}},
		{"durable wrong submission", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			e.Submission.ID = "prompt:foreign-submission"
		}},
		{"durable raw id is not normalized identity", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			e.Submission.ID = e.Reservation.SubmissionID
		}},
		{"durable unknown state", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoverySubmission(e, models.DeliverySubmissionState("future-state"))
		}},
		{"runtime same session id unreadable receipt", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryRuntime(e)
			e.Runtime.TurnID = ""
			e.Runtime.SubmissionID = ""
		}},
		{"runtime successor is never absence", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryRuntime(e)
			e.Runtime.TurnID = "runtime-successor-turn"
			e.Runtime.SubmissionID = "prompt:runtime-successor-submission"
		}},
		{"runtime wrong task", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { recoveryRuntime(e); e.Runtime.TaskID = "foreign-runtime-task" }},
		{"runtime foreign incarnation", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryRuntime(e)
			e.Runtime.IncarnationID = "foreign-runtime-incarnation"
		}},
		{"runtime wrong submission", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryRuntime(e)
			e.Runtime.SubmissionID = "prompt:foreign-runtime-submission"
		}},
		{"runtime missing execution", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { recoveryRuntime(e); e.Runtime.ExecutionID = "" }},
		{"matching runtime different durable turn", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryRuntime(e)
			recoveryTerminal(e, models.DeliverySubmissionCompleted)
			e.Submission.TerminalTurnID = "durable-successor-turn"
		}},
		{"matching runtime conflicting current turn", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { recoveryRuntime(e); e.CurrentTurn.ID = "current-successor-turn" }},
		{"accepted durable conflicting successor runtime", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			recoveryRuntime(e)
			e.Runtime.TurnID = "live-successor-turn"
		}},
		{"terminal state without turn link", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryTerminal(e, models.DeliverySubmissionCompleted)
			e.Submission.TerminalTurnID = ""
		}},
		{"terminal receipt without terminal turn", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryTerminal(e, models.DeliverySubmissionCompleted)
			e.ReservedTurn.CompletedAt = nil
		}},
		{"terminal turn alone not settled", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			completed := time.Now()
			e.ReservedTurn.CompletedAt = &completed
		}},
		{"legacy session id alone", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			delete(e.Session.Metadata, models.SessionMetaKeyErrandLaunchAuthority)
		}},
		{"legacy durable accepted lacks turn link", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			delete(e.Session.Metadata, models.SessionMetaKeyErrandLaunchAuthority)
			recoverySubmission(e, models.DeliverySubmissionAccepted)
		}},
		{"legacy complete runtime tuple", errandLaunchRecoveryAdmitted, func(e *errandLaunchRecoveryEvidence) {
			delete(e.Session.Metadata, models.SessionMetaKeyErrandLaunchAuthority)
			recoveryRuntime(e)
		}},
		{"legacy complete terminal tuple", errandLaunchRecoverySettled, func(e *errandLaunchRecoveryEvidence) {
			delete(e.Session.Metadata, models.SessionMetaKeyErrandLaunchAuthority)
			recoveryTerminal(e, models.DeliverySubmissionCompleted)
		}},
		{"legacy runtime missing initial profile", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			delete(e.Session.Metadata, models.SessionMetaKeyErrandLaunchAuthority)
			recoveryRuntime(e)
			e.ReservedTurn.ExecutionProfileID = ""
		}},
	}
	for _, state := range []models.DeliverySubmissionState{models.DeliverySubmissionAccepted, models.DeliverySubmissionDispatching, models.DeliverySubmissionInterruptedUnknown} {
		cases = append(cases, launchRecoveryCase{string(state), errandLaunchRecoveryAdmitted, func(e *errandLaunchRecoveryEvidence) { recoverySubmission(e, state) }})
	}
	for _, state := range []models.DeliverySubmissionState{models.DeliverySubmissionCompleted, models.DeliverySubmissionFailed, models.DeliverySubmissionCancelled, models.DeliverySubmissionInterruptedUnknown} {
		cases = append(cases, launchRecoveryCase{string(state) + " exact terminal", errandLaunchRecoverySettled, func(e *errandLaunchRecoveryEvidence) { recoveryTerminal(e, state) }})
	}
	return append(cases, launchRecoveryContradictions()...)
}

func launchRecoveryContradictions() []launchRecoveryCase {
	return []launchRecoveryCase{

		{"nil reservation", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Reservation = nil }},
		{"missing reserved submission", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Reservation.SubmissionID = "" }},
		{"wrong canonical session", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Session.ID = "foreign-canonical-session" }},
		{"stored provenance wrong session", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			corruptRecoveryAuthority(e, "session_id", "foreign-authority-session")
		}},
		{"runtime wrong session", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryRuntime(e)
			e.Runtime.SessionID = "foreign-runtime-session"
		}},
		{"conflicting current turn absence", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.CurrentTurnAbsent = true }},
		{"launched row cannot become absent", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryAbsent(e)
			e.Reservation.Lifecycle = models.ErrandLifecycleLaunched
		}},
		{"same session id alone is not prepared", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryAbsent(e)
			e.Session = &models.TaskSession{ID: "reserved-child"}
			e.SessionAbsent = false
		}},
		{"prepared with retained allocation evidence", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.Session.AgentExecutionID = "previous-allocation" }},
		{"runtime match conflicts with foreign durable incarnation", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoveryRuntime(e)
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			e.Submission.IncarnationID = "conflicting-durable-incarnation"
		}},
		{"matching durable and runtime proof", errandLaunchRecoveryAdmitted, func(e *errandLaunchRecoveryEvidence) {
			recoveryRuntime(e)
			recoverySubmission(e, models.DeliverySubmissionAccepted)
		}},
		{"legacy bound accepted submission", errandLaunchRecoveryAdmitted, func(e *errandLaunchRecoveryEvidence) {
			delete(e.Session.Metadata, models.SessionMetaKeyErrandLaunchAuthority)
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			e.ReservedTurn.Metadata[errandAdmissionTurnMetadataKey] = map[string]interface{}{"submission_id": "reserved-submission", "sender_session_id": "reserved-parent", "state": "accepted"}
		}},
		{"legacy wrong bound parent", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			delete(e.Session.Metadata, models.SessionMetaKeyErrandLaunchAuthority)
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			e.ReservedTurn.Metadata[errandAdmissionTurnMetadataKey] = map[string]interface{}{"submission_id": "reserved-submission", "sender_session_id": "foreign-marker-parent", "state": "accepted"}
		}},
		{"legacy wrong bound submission", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			delete(e.Session.Metadata, models.SessionMetaKeyErrandLaunchAuthority)
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			e.ReservedTurn.Metadata[errandAdmissionTurnMetadataKey] = map[string]interface{}{"submission_id": "foreign-marker-submission", "sender_session_id": "reserved-parent", "state": "accepted"}
		}},
		{"legacy initial prompt alone", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			delete(e.Session.Metadata, models.SessionMetaKeyErrandLaunchAuthority)
			e.Session.Metadata[models.SessionMetaKeyInitialPromptSubmission] = &models.InitialPromptSubmission{Version: models.InitialPromptSubmissionVersion, State: models.InitialPromptSubmissionAccepted, AcceptedAt: "2026-10-30T12:00:00Z"}
		}},
		{"session present and reported absent", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.SessionAbsent = true }},
		{"turn present and reported absent", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { e.ReservedTurnAbsent = true }},
		{"submission present and reported absent", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			recoverySubmission(e, models.DeliverySubmissionAccepted)
			e.SubmissionAbsent = true
		}},
		{"runtime present and reported absent", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) { recoveryRuntime(e); e.RuntimeAbsent = true }},
		{"accepted initial prompt is no reserved submission proof", errandLaunchRecoveryBlocked, func(e *errandLaunchRecoveryEvidence) {
			initial, err := models.NewInitialPromptSubmission("accepted source prompt", false, nil)
			if err != nil {
				panic(err)
			}
			initial.State = models.InitialPromptSubmissionAccepted
			initial.AcceptedAt = "2026-10-30T12:00:00Z"
			e.Session.Metadata[models.SessionMetaKeyInitialPromptSubmission] = initial
		}},
	}
}

// @covers AC-TASKS-ERRANDS-008.2
func TestErrandLaunchAuthorityReadErrorIsPreserved(t *testing.T) {
	evidence := launchRecoveryFixture(t)
	recoveryRuntime(&evidence)
	evidence.ReadError = errors.New("owner read failed")
	state, err := classifyErrandLaunchRecovery(evidence)
	require.Equal(t, errandLaunchRecoveryBlocked, state)
	require.ErrorIs(t, err, evidence.ReadError)
}
