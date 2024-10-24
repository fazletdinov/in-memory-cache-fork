package inmemorycache

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	defaultExpiration := 100
	cleanupInterval := 0
	haveLimitMaximumCapacity := true
	capacity := 1024

	cached := &InMemoryCache[string, int]{
		RWMutex:                  sync.RWMutex{},
		defaultExpiration:        time.Duration(defaultExpiration),
		cleanupInterval:          time.Duration(cleanupInterval),
		items:                    make(map[string]CacheItem[string, int], capacity),
		haveLimitMaximumCapacity: haveLimitMaximumCapacity,
		capacity:                 int64(capacity),
		arrayCache:               make([]CacheForArray[string], 0, capacity),
		existingVolume:           0,
	}

	cache := New[string, CacheItem[string, int]](
		time.Duration(defaultExpiration),
		time.Duration(cleanupInterval),
		haveLimitMaximumCapacity,
		int64(capacity),
	)

	assert.Equal(t, cached.defaultExpiration, cache.defaultExpiration)
	assert.Equal(t, cached.cleanupInterval, cache.cleanupInterval)
	assert.Equal(t, cached.existingVolume, cache.existingVolume)
	assert.Equal(t, cached.haveLimitMaximumCapacity, cache.haveLimitMaximumCapacity)
	assert.Equal(t, cached.capacity, cache.capacity)
	assert.Equal(t, cached.arrayCache, cache.arrayCache)
	assert.Equal(t, cached.arrayCache, cache.arrayCache)

	assert.NotEqual(t, cached.items, cache.items)

}
