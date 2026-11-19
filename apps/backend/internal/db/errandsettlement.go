package db

import "errors"

var ErrErrandSettlementPending = errors.New("preceding errand turn has no committed exact settlement decision")
