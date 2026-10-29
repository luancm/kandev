package errands

import (
	"expvar"

	"github.com/kandev/kandev/internal/task/models"
)

// StartOutcome is the closed label set of errands_started_total.
type StartOutcome string

const (
	StartOutcomeStarted      StartOutcome = "started"
	StartOutcomeQueued       StartOutcome = "queued"
	StartOutcomeLimitReached StartOutcome = "limit_reached"
	StartOutcomeRejected     StartOutcome = "rejected"
	StartOutcomeFailed       StartOutcome = "failed"
)

// The counters are process-local expvar maps exposed through /debug/vars in
// dev mode. Labels come from closed sets; task, session, errand, and profile
// identifiers are never label values.
var (
	startedTotal  = expvar.NewMap("errands_started_total")
	settledTotal  = expvar.NewMap("errands_settled_total")
	deliveryTotal = expvar.NewMap("errands_result_delivery_total")
)

func countStarted(outcome StartOutcome) {
	switch outcome {
	case StartOutcomeStarted, StartOutcomeQueued, StartOutcomeLimitReached, StartOutcomeRejected, StartOutcomeFailed:
		startedTotal.Add("outcome="+string(outcome), 1)
	}
}

// countSettled records a settlement. Only the closed done, failed, and
// cancelled statuses are counted; any other status is ignored.
func countSettled(status models.ErrandStatus) {
	switch status {
	case models.ErrandStatusDone, models.ErrandStatusFailed, models.ErrandStatusCancelled:
		settledTotal.Add("status="+string(status), 1)
	}
}

// countDelivery records one result handed to the parent. The route set is
// closed; task, session, and errand identifiers are never labels.
func countDelivery(route models.ErrandDeliveryRoute) {
	switch route {
	case models.ErrandDeliveryAwait, models.ErrandDeliveryTurnContext, models.ErrandDeliveryNotification:
		deliveryTotal.Add("route="+string(route), 1)
	}
}
