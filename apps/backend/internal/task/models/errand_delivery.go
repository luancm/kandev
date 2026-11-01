package models

import "errors"

// ErrErrandClaimCommitUnknown retains ownership when SQL cannot prove whether
// the claim committed. It must not be retried under a different token.
var ErrErrandClaimCommitUnknown = errors.New("errand claim commit outcome unknown")

// ErrandInventory captures every row used to prepare a parent's response,
// including rows omitted from its rendered status block.
func ErrandInventory(rows []*Errand) []ErrandClaimCandidate {
	inventory := make([]ErrandClaimCandidate, 0, len(rows))
	for _, row := range rows {
		inventory = append(inventory, ErrandClaimCandidate{ID: row.ID, Version: row.Version})
	}
	return inventory
}

// ErrandParentAdmission binds a dispatch reservation to trusted sender and
// durable turn/submission identities. It is not a tool argument.
type ErrandParentAdmission struct {
	TaskID          string
	ParentSessionID string
	ErrandID        string
	SenderSessionID string
	TurnID          string
	SubmissionID    string
}

// ErrandAdmissionState records a reservation or provider admission evidence.
// Unknown retains the reservation; only definite refusal or acceptance resolves it.
type ErrandAdmissionState string

const (
	ErrandAdmissionReserved ErrandAdmissionState = "reserved"
	ErrandAdmissionAccepted ErrandAdmissionState = "accepted"
	ErrandAdmissionRefused  ErrandAdmissionState = "refused"
	ErrandAdmissionUnknown  ErrandAdmissionState = "unknown"
)

// ErrandAdmissionReservation distinguishes a newly committed reservation from
// replay. An existing reservation is not permission to dispatch again; recovery
// must resolve it from durable submission/turn evidence.
type ErrandAdmissionReservation struct {
	Created bool
	State   ErrandAdmissionState
}

// ErrandResultClaim is the immutable ownership captured by ClaimResults.
// Release must use this identity, not a newly loaded result's identity.
type ErrandResultClaim struct {
	ErrandID                  string
	ResultRevision            string
	ParentAdmissionGeneration int64
	Token                     string
	Route                     ErrandDeliveryRoute
}
