package queuetest_test

import (
	"testing"

	"github.com/cuonggt/tug/queue"
	"github.com/cuonggt/tug/queue/queuetest"
)

func TestMemoryKeepsTheQueuesPromises(t *testing.T) {
	queuetest.TestStore(t, func(t *testing.T) queue.Store { return &queuetest.Memory{} })
}
