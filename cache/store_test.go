package cache_test

import (
	"testing"

	"github.com/cuonggt/tug/cache"
	"github.com/cuonggt/tug/cache/cachetest"
)

func TestTheMemoryACacheKeepsItsValuesInKeepsAStoresPromises(t *testing.T) {
	cachetest.TestStore(t, func(t *testing.T) cache.Store {
		return cache.MemoryStore()
	})
}
