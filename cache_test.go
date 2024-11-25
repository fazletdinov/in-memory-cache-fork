package inmemorycache

import (
	"fmt"
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
	}

	cache := New[string, CacheItem[string, int]](
		time.Duration(defaultExpiration),
		time.Duration(cleanupInterval),
		haveLimitMaximumCapacity,
		int64(capacity),
	)

	assert.Equal(t, cached.defaultExpiration, cache.defaultExpiration)
	assert.Equal(t, cached.cleanupInterval, cache.cleanupInterval)
	assert.Equal(t, cached.haveLimitMaximumCapacity, cache.haveLimitMaximumCapacity)
	assert.Equal(t, cached.capacity, cache.capacity)

	assert.NotEqual(t, cached.items, cache.items)

}

func TestSet(t *testing.T) {
	cache := SetUp()
	cache.capacity = 300

	testcases := []struct {
		key      string
		value    map[string]string
		duration time.Duration
		expected bool
	}{
		{"Python", map[string]string{"Not Found": "Не найдено"}, time.Minute * 6, true},
		{"Golang", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 25, true},
		{"Rust", map[string]string{"Not Found": "Табылмады"}, time.Minute * 45, true},
		{"C++", map[string]string{"Not Found": "Не найдено"}, time.Minute * 55, true},
		{"PHP", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 75, true},
		{"JS", map[string]string{"Not Found": "Табылмады"}, time.Minute * 89, false},
		{"Java", map[string]string{"Not Found": "Не найдено"}, time.Minute * 110, true},
		{"Haskel", map[string]string{"Not Found": "Не найдено"}, time.Minute * 130, true},
		{"Django", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 150, true},
		{"FastApi", map[string]string{"Not Found": "Табылмады"}, time.Minute * 170, false},
		{"Flask", map[string]string{"Not Found": "Не найдено"}, time.Minute * 190, true},
		{"Gin", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 230, true},
		{"Fiber", map[string]string{"Not Found": "Табылмады"}, time.Minute * 240, true},
		{"Laravel", map[string]string{"Not Found": "Не найдено"}, time.Minute * 290, false},
		{"Docker", map[string]string{"Not Found": "Не найдено"}, time.Minute * 300, true},
		{"Git", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 320, true},
		{"Asyncio", map[string]string{"Not Found": "Табылмады"}, time.Minute * 340, true},
		{"Gorutin", map[string]string{"Not Found": "Не найдено"}, time.Minute * 360, false},
		{"Kotlin", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 370, true},
		{"Redis", map[string]string{"Not Found": "Табылмады"}, time.Minute * 390, true},
		{"Kubernetes", map[string]string{"Not Found": "Не найдено"}, time.Minute * 430, true},
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

func TestFlashAll(t *testing.T) {
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
		{"Python", map[string]string{"Not Found": "Не найдено"}, time.Minute * 300, true},
		{"Golang", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 250, true},
		{"Rust", map[string]string{"Not Found": "Табылмады"}, time.Minute * 200, true},
		{"C++", map[string]string{"Not Found": "Не найдено"}, time.Minute * 150, true},
		{"Haskel", map[string]string{"Not Found": "Не знойдзена"}, time.Minute * 100, true},
		{"PHP", map[string]string{"Not Found": "Табылмады"}, time.Minute * 70, true},
	}

	for _, tc := range testcases {
		t.Run(tc.key, func(t *testing.T) {
			ok := cache.Set(tc.key, tc.value, tc.duration)
			assert.Equal(t, ok, tc.expected, fmt.Sprintf("Set(%s, %v, %v) = %v; ожидалось %v", tc.key, tc.value, tc.duration, ok, tc.expected))
		})
	}

	oldLenItems := len(cache.items)
	t.Logf("oldLenItems == %#v\n", cache.items)

	t.Run("deleteDueToOverflow", func(t *testing.T) {
		cache.deleteDueToOverflow()
	})

	currentLenItems := len(cache.items)
	t.Logf("currentLenItems == %#v\n", cache.items)

	assert.NotEqual(t, oldLenItems, currentLenItems)

}
