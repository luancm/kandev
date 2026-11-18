package clarification

import "errors"

var ErrWaiterSession = errors.New("clarification waiter session is required")
var ErrWaiterSessionCollision = errors.New("clarification pending ID belongs to another session")

func requestWaiterSession(req *Request) (string, error) {
	if req == nil || req.SessionID == "" {
		return "", ErrWaiterSession
	}
	return req.SessionID, nil
}
