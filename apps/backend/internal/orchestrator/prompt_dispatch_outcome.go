package orchestrator

import (
	"errors"
	"sync"
)

type promptDispatchEvidence uint8

const (
	promptDispatchPending promptDispatchEvidence = iota
	promptDispatchRejected
	promptDispatchNotSent
	promptDispatchAccepted
	promptDispatchUnresolved
)

type promptDispatchResolution struct {
	evidence                promptDispatchEvidence
	admissionErr            error
	publicationErr          error
	ordinaryFailureRollback bool
}

func (r promptDispatchResolution) accepted() bool { return r.evidence == promptDispatchAccepted }

func (r promptDispatchResolution) refused() bool {
	return r.evidence == promptDispatchRejected || (r.evidence == promptDispatchNotSent && r.admissionErr == nil)
}

func (r promptDispatchResolution) rollbackEligible() bool {
	return !r.accepted() && (r.refused() || (r.admissionErr == nil && r.ordinaryFailureRollback))
}

func (r promptDispatchResolution) failure(cause error) error {
	return r.dispatchError(errors.Join(cause, r.publicationErr))
}

func (r promptDispatchResolution) dispatchError(cause error) error {
	err := errors.Join(cause, r.admissionErr)
	if err != nil && r.accepted() {
		return &acceptedPromptDispatchError{err: err}
	}
	return err
}

// promptDispatchOutcome keeps one attempt's evidence separate from failures in
// recording admission or publishing an accepted turn. Acceptance is irreversible.
type promptDispatchOutcome struct {
	mu sync.Mutex
	promptDispatchResolution
	turnID     string
	onAccepted func(turnID string)
}

func (o *promptDispatchOutcome) recordAccepted(publicationErr error) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.evidence = promptDispatchAccepted
	o.publicationErr = errors.Join(o.publicationErr, publicationErr)
}

func (o *promptDispatchOutcome) recordRejection(admissionErr error) {
	o.recordEvidence(promptDispatchRejected, admissionErr)
}

func (o *promptDispatchOutcome) recordNonDelivery(admissionErr error) {
	o.recordEvidence(promptDispatchNotSent, admissionErr)
}

func (o *promptDispatchOutcome) recordUnresolved(admissionErr error) {
	o.recordEvidence(promptDispatchUnresolved, admissionErr)
}

func (o *promptDispatchOutcome) recordEvidence(evidence promptDispatchEvidence, admissionErr error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.evidence != promptDispatchAccepted {
		o.evidence = evidence
	}
	o.admissionErr = errors.Join(o.admissionErr, admissionErr)
}

func (o *promptDispatchOutcome) recordAdmissionError(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.admissionErr = errors.Join(o.admissionErr, err)
}

func (o *promptDispatchOutcome) recordPublicationError(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.publicationErr = errors.Join(o.publicationErr, err)
}

func (o *promptDispatchOutcome) allowOrdinaryFailureRollback() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.ordinaryFailureRollback = true
}

func (o *promptDispatchOutcome) snapshot() promptDispatchResolution {
	if o == nil {
		return promptDispatchResolution{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.promptDispatchResolution
}
