package inmemorycache

import (
	"container/list"
	"fmt"
	"sync"
	"time"
	"unsafe"
)

const percentagePermanentlyFreeMemoryFromSpecifiedValue int64 = 88

var existingVolume uint32 = 0

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
		linkedList:               list.New(),
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

	if c.haveLimitMaximumCapacity {
		if !c.checkCapacity(key, value) {
			c.deleteDueToOverflow()
			return false
		}
	}

	c.Lock()

	defer c.Unlock()

	c.items[key] = CacheItem[K, V]{
		Key:        key,
		Value:      value,
		Created:    time.Now(),
		Expiration: expiration,
	}
	c.linkedList.PushBack(key)

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
	c.Lock()

	defer c.Unlock()

	if _, found := c.Get(key); found {
		return fmt.Errorf("item with key %v not exists", key)
	}

	for element := c.linkedList.Front(); element != nil; element = element.Next() {
		if key == element.Value.(K) {
			c.linkedList.Remove(element)
		}
	}

	delete(c.items, key)

	return nil
}

func (c *InMemoryCache[K, V]) RenameKey(key K, newKey K) error {
	c.Lock()

	defer c.Unlock()

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
	existingVolume += currentSize

	if int64((float64(existingVolume)+float64(currentSize))/float64(existingVolume)*100) > int64(percentagePermanentlyFreeMemoryFromSpecifiedValue) {
		existingVolume -= currentSize
		return false
	}
	return true
}

func (c *InMemoryCache[K, V]) deleteDueToOverflow() {
	c.Lock()

	defer c.Unlock()

	for i := 0; i < c.linkedList.Len()/2; i++ {
		item := c.linkedList.Back()
		c.linkedList.Remove(item)
		c.Delete(item.Value.(K))
	}
}
