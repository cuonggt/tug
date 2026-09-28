package auth_test

import (
	"testing"

	"github.com/cuonggt/tug/auth"
	"github.com/cuonggt/tug/auth/throttletest"
)

func TestTheMemoryAThrottleCountsInKeepsAStoresPromises(t *testing.T) {
	throttletest.TestStore(t, func(t *testing.T) auth.ThrottleStore {
		return auth.MemoryThrottles()
	})
}
