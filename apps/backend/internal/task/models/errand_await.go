package models

import "github.com/kandev/kandev/internal/agentctl/types/errandawait"

type ErrandAwaitIdentity = errandawait.Identity

// ErrandAwaitDelivery correlates only the results carried by a successful body.
// It is not receiver evidence.
type ErrandAwaitDelivery struct {
	Version int                       `json:"version"`
	Token   string                    `json:"token"`
	Items   []ErrandAwaitDeliveryItem `json:"items"`
}
type ErrandAwaitDeliveryItem struct {
	ErrandID       string `json:"errand_id"`
	ResultRevision string `json:"result_revision"`
}
