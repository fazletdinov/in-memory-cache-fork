package inmemorycache

import (
	"sync"
	"time"
)

const (
	NoExpiration      time.Duration = -1
	DefaultExpiration time.Duration = 0

	TimeLayout           = "02.01.2006_15-04-05"
	MaxCapacityThreshold = 0.88
)

type InMemoryCache[K comparable, V any] struct {
	sync.RWMutex
	defaultExpiration        time.Duration
	cleanupInterval          time.Duration
	items                    map[K]CacheItem[K, V]
	haveLimitMaximumCapacity bool
	capacity                 uint64
	currentSize              uint64
}

type CacheItem[K comparable, V any] struct {
	Key        K         `json:"Key"`
	Value      V         `json:"Value"`
	Created    time.Time `json:"Created"`
	Expiration int64     `json:"Expiration"`
}

type CacheSize struct {
	Len    int
	Weight uint64
}

type CacheForArray[K comparable, V any] struct {
	Key   K
	Value CacheItem[K, V]
}
