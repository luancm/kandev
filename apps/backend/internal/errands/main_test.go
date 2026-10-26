package errands

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces that the cascade worker is fully drained by Stop.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
