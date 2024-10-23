package inmemorycache

import (
	"sync"
	"time"
)

const (
	NoExpiration      time.Duration = -1
	DefaultExpiration time.Duration = 0
)

const TimeLayout = "02.01.2006_15-04-05"

type InMemoryCache[K comparable, V any] struct {
	sync.RWMutex
	defaultExpiration time.Duration
	cleanupInterval   time.Duration
	items             map[K]CacheItem[K, V]
	turnCapacity      bool
	capacity          int
}

type CacheItem[K comparable, V any] struct {
	Key        K         `json:"Key"`
	Value      V         `json:"Value"`
	Created    time.Time `json:"Created"`
	Expiration int64     `json:"Expiration"`
}

type CacheSize struct {
	Len    int
	Weight uintptr
}

type CacheBackupItem[K comparable, V any] struct {
	Key        K
	Value      V
	Created    string
	Expiration string
}
