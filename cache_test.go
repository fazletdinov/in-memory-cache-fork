package inmemorycache

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func SetUp() *InMemoryCache[string, map[string]string] {
	defaultExpiration := 100
	cleanupInterval := 0
	haveLimitMaximumCapacity := true
	capacity := 1024

	cache := New[string, map[string]string](
		time.Duration(defaultExpiration),
		time.Duration(cleanupInterval),
		haveLimitMaximumCapacity,
		int64(capacity),
	)
	return cache
}

func SetUPCreateCache(cache *InMemoryCache[string, map[string]string], t *testing.T) []struct {
	key      string
	value    map[string]string
	duration time.Duration
	expected bool
} {
	testcases := []struct {
		key      string
		value    map[string]string
		duration time.Duration
		expected bool
	}{
		{"Rus", map[string]string{"Not Found": "Не найдено"}, time.Minute * 5, true},
		{"Bel", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 5, true},
		{"Kz", map[string]string{"Not Found": "Табылмады"}, time.Minute * 5, true},
	}

	for _, tc := range testcases {
		t.Run(tc.key, func(t *testing.T) {
			ok := cache.Set(tc.key, tc.value, tc.duration)
			assert.Equal(t, ok, tc.expected, fmt.Sprintf("Set(%s, %v, %v) = %v; ожидалось %v", tc.key, tc.value, tc.duration, ok, tc.expected))
		})
	}
	return testcases
}

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

func TestSet(t *testing.T) {
	cache := SetUp()

	testcases := []struct {
		key      string
		value    map[string]string
		duration time.Duration
		expected bool
	}{
		{"A", map[string]string{"Not Found": "Не найдено"}, time.Minute * 3, true},
		{"B", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 4, true},
		{"C", map[string]string{"Not Found": "Табылмады"}, time.Minute * 5, true},
		{"D", map[string]string{"Not Found": "Не найдено"}, time.Minute * 6, true},
		{"E", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 7, true},
		{"F", map[string]string{"Not Found": "Табылмады"}, time.Minute * 8, true},
		{"G", map[string]string{"Not Found": "Не найдено"}, time.Minute * 9, true},
	}

	for _, tc := range testcases {
		t.Run(tc.key, func(t *testing.T) {
			ok := cache.Set(tc.key, tc.value, tc.duration)
			assert.Equal(t, ok, tc.expected, fmt.Sprintf("Set(%s, %v, %v) = %v; ожидалось %v", tc.key, tc.value, tc.duration, ok, tc.expected))
		})
	}
}

func TestGet(t *testing.T) {
	cache := SetUp()
	testcases := SetUPCreateCache(cache, t)

	for _, tc := range testcases {
		t.Run(tc.key, func(t *testing.T) {
			result, ok := cache.Get(tc.key)
			assert.Equal(t, ok, true, fmt.Sprintf("Get(%s) = %v; ожидалось %v", tc.key, result, tc.value))
		})
	}
}

func TestCacheSize(t *testing.T) {
	cache := SetUp()
	_ = SetUPCreateCache(cache, t)

	t.Run("CacheSize", func(t *testing.T) {
		result := cache.CacheSize()
		assert.Equal(t, CacheSize{
			Len:    len(cache.items),
			Weight: unsafe.Sizeof(cache.items),
		}, result)
	})

}

func TestFlahAll(t *testing.T) {
	cache := SetUp()
	_ = SetUPCreateCache(cache, t)

	assert.NotEqual(t, len(cache.items), 0)
	t.Run("FlahAll", func(t *testing.T) {
		cache.FlashAll()
		assert.Equal(t, len(cache.items), 0)
	})

}

func TestDelete(t *testing.T) {
	cache := SetUp()
	testcases := SetUPCreateCache(cache, t)

	for _, tc := range testcases {
		t.Run(tc.key, func(t *testing.T) {
			err := cache.Delete(tc.key)
			assert.NoError(t, err)
		})
	}
}

func TestRenameKey(t *testing.T) {
	cache := SetUp()
	testcases := SetUPCreateCache(cache, t)

	for index, tc := range testcases {
		t.Run(tc.key, func(t *testing.T) {
			err := cache.RenameKey(tc.key, fmt.Sprintf("%s-%d", tc.key, index))
			assert.NoError(t, err)
		})
	}
}

func TestDeleteDueToOverflow(t *testing.T) {
	cache := SetUp()

	testcases := []struct {
		key      string
		value    map[string]string
		duration time.Duration
		expected bool
	}{
		{"A", map[string]string{"Not Found": "Не найдено"}, time.Minute * 30, true},
		{"B", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 4, true},
		{"C", map[string]string{"Not Found": "Табылмады"}, time.Minute * 5, true},
		{"D", map[string]string{"Not Found": "Не найдено"}, time.Minute * 6, true},
		{"E", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 7, true},
		{"F", map[string]string{"Not Found": "Табылмады"}, time.Minute * 8, true},
		{"G", map[string]string{"Not Found": "Не найдено"}, time.Minute * 9, true},
		{"H", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 10, true},
		{"I", map[string]string{"Not Found": "Табылмады"}, time.Minute * 110, true},
		{"K", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 12, true},
		{"L", map[string]string{"Not Found": "Табылмады"}, time.Minute * 13, true},
		{"M", map[string]string{"Not Found": "Не найдено"}, time.Minute * 14, true},
		{"N", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 15, true},
		{"O", map[string]string{"Not Found": "Не найдено"}, time.Minute * 16, true},
		{"P", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 17, true},
		{"Q", map[string]string{"Not Found": "Табылмады"}, time.Minute * 18, true},
		{"R", map[string]string{"Not Found": "Не найдено"}, time.Minute * 19, true},
		{"S", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 20, true},
		{"T", map[string]string{"Not Found": "Табылмады"}, time.Minute * 21, true},
		{"Y", map[string]string{"Not Found": "Не найдено"}, time.Minute * 22, true},
		{"v", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 123, true},
		{"W", map[string]string{"Not Found": "Табылмады"}, time.Minute * 24, true},
		{"X", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 25, true},
		{"u", map[string]string{"Not Found": "Табылмады"}, time.Minute * 26, true},
		{"Z", map[string]string{"Not Found": "Не найдено"}, time.Minute * 27, true},
	}

	for _, tc := range testcases {
		t.Run(tc.key, func(t *testing.T) {
			ok := cache.Set(tc.key, tc.value, tc.duration)
			assert.Equal(t, ok, tc.expected, fmt.Sprintf("Set(%s, %v, %v) = %v; ожидалось %v", tc.key, tc.value, tc.duration, ok, tc.expected))
		})
	}

	oldLenArrayCache := len(cache.arrayCache)
	oldLenItems := len(cache.items)

	t.Run("deleteDueToOverflow", func(t *testing.T) {
		cache.deleteDueToOverflow()
		assert.Equal(t, sort.SliceIsSorted(cache.arrayCache, func(i, j int) bool {
			return cache.arrayCache[i].Expiration < cache.arrayCache[j].Expiration
		}), true)
	})

	currentLenArrayCache := len(cache.arrayCache)
	currentLenItems := len(cache.items)

	assert.NotEqual(t, oldLenArrayCache, currentLenArrayCache)
	assert.NotEqual(t, oldLenItems, currentLenItems)

}
