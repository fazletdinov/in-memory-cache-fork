package inmemorycache

import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

func New[K comparable, V any](
	defaultExpiration, cleanupInterval time.Duration,
) *InMemoryCache[K, V] {

	items := make(map[K]CacheItem[K, V])

	cache := &InMemoryCache[K, V]{
		RWMutex:           sync.RWMutex{},
		defaultExpiration: defaultExpiration,
		cleanupInterval:   cleanupInterval,
		items:             items,
	}

	if cleanupInterval > 0 {
		go cache.gC()
	}

	return cache
}

func (c *InMemoryCache[K, V]) Set(key K, value V, duration time.Duration) {
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

	c.Lock()

	defer c.Unlock()

	c.items[key] = CacheItem[K, V]{
		Key:        key,
		Value:      value,
		Created:    time.Now(),
		Expiration: expiration,
	}
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
