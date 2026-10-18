package session_test

import (
	"testing"

	"github.com/cuonggt/tug/session"
	"github.com/cuonggt/tug/session/sessiontest"
)

func TestTheMemoryKeeperKeepsAKeepersPromises(t *testing.T) {
	sessiontest.TestStore(t, func(t *testing.T) session.Keeper {
		return &sessiontest.Memory{}
	})
}
