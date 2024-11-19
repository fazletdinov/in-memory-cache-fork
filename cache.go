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

	if c.haveLimitMaximumCapacity && c.checkCapacity(key, value) {
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
	keyValueSize := uint32(unsafe.Sizeof(key)) + uint32(unsafe.Sizeof(value))
	currentSize := c.memSize()
	return int32(keyValueSize+currentSize/uint32(c.capacity)*100) > percentagePermanentlyFreeMemoryFromSpecifiedValue
}

func (c *InMemoryCache[K, V]) deleteDueToOverflow() {
	c.RLock()
	defer c.RUnlock()

	c.sliceSorting()
}

func (c *InMemoryCache[K, V]) sliceSorting() {
	var sortedArray []*CacheForArray[K, V]

	for key, value := range c.items {
		sortedArray = append(sortedArray, &CacheForArray[K, V]{
			Key:   key,
			Value: value,
		})
	}

	// сортировка списка по полю Expiration
	sort.Slice(sortedArray, func(i, j int) bool {
		return sortedArray[i].Value.Expiration > sortedArray[j].Value.Expiration
	})

	// создание новой отсортированной map
	sortedMap := make(map[K]CacheItem[K, V])
	for i, pair := range sortedArray {
		sortedMap[pair.Key] = pair.Value
		// удаление 50% элементов по возрастанию
		if i >= len(sortedArray)/2 {
			delete(sortedMap, pair.Key)
		}
	}
	c.items = sortedMap
}

func (c *InMemoryCache[K, V]) memSize() uint32 {
	var size uint32 = 0
	for key, value := range c.items {
		size += uint32(unsafe.Sizeof(key)) + uint32(unsafe.Sizeof(value))
	}
	return size

}
