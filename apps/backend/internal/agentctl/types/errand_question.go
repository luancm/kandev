package types

import "errors"

// ErrErrandQuestion directs an Errand to return a blocker to its parent.
var ErrErrandQuestion = errors.New("errands cannot ask users questions; return the question or blocker to the parent")
