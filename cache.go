package inmemorycache

import (
	"fmt"
	"sort"
	"sync"
	"time"
	"unsafe"
)

const percentagePermanentlyFreeMemoryFromSpecifiedValue int32 = 88

func New[K comparable, V any](
	defaultExpiration,
	cleanupInterval time.Duration,
	haveLimitMaximumCapacity bool,
	capacity int64,
) *InMemoryCache[K, V] {
	var items map[K]CacheItem[K, V]

	if haveLimitMaximumCapacity {
		items = make(map[K]CacheItem[K, V], capacity)
	} else {
		items = make(map[K]CacheItem[K, V])
	}
	cache := &InMemoryCache[K, V]{
		RWMutex:                  sync.RWMutex{},
		defaultExpiration:        defaultExpiration,
		cleanupInterval:          cleanupInterval,
		items:                    items,
		haveLimitMaximumCapacity: haveLimitMaximumCapacity,
		capacity:                 capacity,
		arrayCache:               make([]CacheForArray[K], 0, capacity),
		existingVolume:           0,
	}

	if cleanupInterval > 0 {
		go cache.gC()
	}

	return cache
}

func (c *InMemoryCache[K, V]) Set(key K, value V, duration time.Duration) bool {
	var expiration int64

	if duration > 0 {
		expiration = time.Now().Add(duration).UnixNano()
	}

	if duration < 0 {
		expiration = int64(NoExpiration)
	}

	if duration == DefaultExpiration {
		expiration = time.Now().Add(c.defaultExpiration).UnixNano()
	}

	if c.haveLimitMaximumCapacity && !c.checkCapacity(key, value) {
		c.deleteDueToOverflow()
		return false
	}

	c.Lock()

	defer c.Unlock()

	c.items[key] = CacheItem[K, V]{
		Key:        key,
		Value:      value,
		Created:    time.Now(),
		Expiration: expiration,
	}

	c.arrayCache = append(c.arrayCache, CacheForArray[K]{
		Key:        key,
		Expiration: expiration,
	})

	return true
}

func (c *InMemoryCache[K, V]) Get(key K) (*CacheItem[K, V], bool) {
	c.RLock()

	defer c.RUnlock()

	item, found := c.items[key]

	if !found {
		return nil, found
	}

	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			return nil, false
		}
	}

	return &CacheItem[K, V]{
		Value:      item.Value,
		Created:    item.Created,
		Expiration: item.Expiration,
	}, true
}

func (c *InMemoryCache[K, V]) Delete(key K) error {
	c.RLock()

	defer c.RUnlock()

	if _, found := c.Get(key); !found {
		return fmt.Errorf("item with key %v not exists", key)
	}

	for index, value := range c.arrayCache {
		if key == value.Key {
			c.arrayCache = append(c.arrayCache[:index], c.arrayCache[index+1:]...)
		}
	}

	delete(c.items, key)

	return nil
}

func (c *InMemoryCache[K, V]) RenameKey(key K, newKey K) error {
	item, found := c.Get(key)
	if !found {
		return fmt.Errorf("item with key %v not exists", key)
	}

	if errDeleteItem := c.Delete(key); errDeleteItem != nil {
		return errDeleteItem
	}

	c.Set(newKey, item.Value, time.Duration(item.Expiration-item.Created.UnixNano()))

	return nil
}

func (c *InMemoryCache[K, V]) CacheSize() CacheSize {
	c.Lock()

	defer c.Unlock()

	cacheWeight := unsafe.Sizeof(c.items)
	cacheLen := len(c.items)

	return CacheSize{
		Len:    cacheLen,
		Weight: cacheWeight,
	}
}

func (c *InMemoryCache[K, V]) FlashAll() {
	c.items = make(map[K]CacheItem[K, V])
}

func (c *InMemoryCache[K, V]) gC() {
	for {
		<-time.After(c.cleanupInterval)

		if c.items == nil {
			return
		}

		if keys := c.expiredKeys(); len(keys) != 0 {
			c.clearItems(keys)
		}
	}
}

func (c *InMemoryCache[K, V]) expiredKeys() (keys []K) {

	c.RLock()

	defer c.RUnlock()

	for k, i := range c.items {
		if time.Now().UnixNano() > i.Expiration && i.Expiration > 0 {
			keys = append(keys, k)
		}
	}

	return
}

func (c *InMemoryCache[K, V]) clearItems(keys []K) {
	c.Lock()

	defer c.Unlock()

	for _, k := range keys {
		delete(c.items, k)
	}
}

func (c *InMemoryCache[K, V]) checkCapacity(key K, value V) bool {
	currentSize := uint32(unsafe.Sizeof(key)) + uint32(unsafe.Sizeof(value))
	c.existingVolume += currentSize

	if int32(float32(c.existingVolume)/float32(c.capacity)*100) > percentagePermanentlyFreeMemoryFromSpecifiedValue {
		c.existingVolume -= currentSize
		return false
	}
	return true
}

func (c *InMemoryCache[K, V]) deleteDueToOverflow() {
	c.sliceSorting()

	c.RLock()
	defer c.RUnlock()

	for index := 0; index < len(c.arrayCache)/2; index++ {
		c.arrayCache = append(c.arrayCache[:index], c.arrayCache[index+1:]...)
		c.Delete(c.arrayCache[index].Key)
	}
}

func (c *InMemoryCache[K, V]) sliceSorting() {
	sort.Slice(c.arrayCache, func(i, j int) bool {
		return c.arrayCache[i].Expiration < c.arrayCache[j].Expiration
	})
}
