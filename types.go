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

type InMemoryCache struct {
	sync.RWMutex
	defaultExpiration time.Duration
	cleanupInterval   time.Duration
	items             map[string]CacheItem
}

type CacheItem struct {
	Key        string      `json:"Key"`
	Value      interface{} `json:"Value"`
	Created    time.Time   `json:"Created"`
	Expiration int64       `json:"Expiration"`
}

type CacheSize struct {
	Len    int
	Weight uintptr
}

type CacheBackupItem struct {
	Key        string
	Value      interface{}
	Created    string
	Expiration string
}
