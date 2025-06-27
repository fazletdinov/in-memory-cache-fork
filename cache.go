package inmemorycache

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"
	"unsafe"
)

// New создает и возвращает новый экземпляр универсального in-memory кэша с поддержкой TTL, автоматической очистки и контроля памяти.
//
// Параметры:
//
//   - ctx: контекст для управления жизненным циклом кэша.
//     Используется для принудительной остановки сборщика мусора (GC) через отмену контекста.
//     Если контекст отменяется — фоновая горутина GC корректно завершается.
//
//   - defaultExpiration: значение по умолчанию для времени жизни элементов (TTL).
//     Если элемент добавлен с duration = DefaultExpiration, то используется это значение.
//     Значение 0 означает "нет срока действия" по умолчанию.
//
//   - cleanupInterval: интервал между автоматическими запусками сборщика мусора (GC),
//     удаляющего просроченные элементы. Значение 0 отключает автоочистку.
//
//   - haveLimitMaximumCapacity: включает контроль объема кэша по памяти (в байтах).
//     Если true, при превышении установленного объема происходит автоматическое удаление
//     менее приоритетных элементов (например, с ближайшим сроком истечения).
//
//   - capacity: максимально допустимый объем памяти кэша в байтах.
//     Используется только при haveLimitMaximumCapacity = true.
//
// Возвращает:
//   - *InMemoryCache[K, V]: указатель на созданный экземпляр кэша.
//
// Особенности:
//   - Поддерживает обобщённые типы: ключи должны быть comparable, значения — любые (any).
//   - Автоматическая сборка мусора работает в отдельной горутине при cleanupInterval > 0.
//   - Контекст ctx позволяет безопасно завершить GC при остановке приложения или отмене контекста.
//   - Потокобезопасный: все операции защищены внутренним sync.Mutex или sync.RWMutex.
//
// Пример использования:
//
//	func main() {
//		ctx, cancel := context.WithCancel(context.Background())
//		defer cancel()
//
//		cache := New[string, int](
//			ctx,
//			5*time.Minute,   // default TTL
//			1*time.Minute,   // run GC every 1 min
//			true,            // enable memory limit
//			1024*1024,       // 1 MB capacity
//		)
//
//		cache.Set("foo", 42, DefaultExpiration)
//
//		if val, ok := cache.Get("foo"); ok {
//			fmt.Println("Value:", val.Value)
//		}
//	}
//
// Примечание:
//   - Вы можете завершить работу GC через отмену переданного контекста.
//   - Элементы с установленным TTL автоматически удаляются по расписанию или при вызове clearExpired().
func New[K comparable, V any](
	ctx context.Context,
	defaultExpiration,
	cleanupInterval time.Duration,
	haveLimitMaximumCapacity bool,
	capacity uint64,
) *InMemoryCache[K, V] {
	cache := &InMemoryCache[K, V]{
		defaultExpiration:        defaultExpiration,
		cleanupInterval:          cleanupInterval,
		items:                    make(map[K]CacheItem[K, V], capacity),
		haveLimitMaximumCapacity: haveLimitMaximumCapacity,
		capacity:                 capacity,
	}

	if cleanupInterval > 0 {
		go cache.gC(ctx)

	}

	return cache
}

// calculateItemSize вычисляет приблизительный размер элемента в байтах
func (c *InMemoryCache[K, V]) calculateItemSize(key K, value V) uint64 {
	return uint64(unsafe.Sizeof(key)) + c.calculateValueSize(value)
}

func (c *InMemoryCache[K, V]) calculateValueSize(value interface{}) uint64 {
	visited := make(map[uintptr]struct{})
	return c.calculateValueSizeWithUnsafe(reflect.ValueOf(value), visited)
}

// calculateValueSize рекурсивно вычисляет размер значения
func (c *InMemoryCache[K, V]) calculateValueSizeWithUnsafe(v reflect.Value, visited map[uintptr]struct{}) uint64 {
	if !v.IsValid() {
		return 0
	}

	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return 0
		}
		ptr := v.Pointer()
		if _, ok := visited[ptr]; ok {
			return 0
		}
		visited[ptr] = struct{}{}
		return uint64(unsafe.Sizeof(ptr)) + c.calculateValueSizeWithUnsafe(v.Elem(), visited)

	case reflect.String:
		str := v.String()
		ptr := unsafe.StringData(str)
		addr := uintptr(*ptr)

		if _, ok := visited[addr]; ok {
			return 0
		}
		visited[addr] = struct{}{}

		return uint64(len(str)) + uint64(unsafe.Sizeof(str))

	case reflect.Slice:
		ptr := v.Pointer()
		if _, ok := visited[ptr]; ok {
			return 0
		}
		visited[ptr] = struct{}{}

		total := uint64(unsafe.Sizeof(v.Interface()))
		for i := 0; i < v.Len(); i++ {
			total += c.calculateValueSizeWithUnsafe(v.Index(i), visited)
		}
		return total

	case reflect.Map:
		ptr := v.Pointer()
		if _, ok := visited[ptr]; ok {
			return 0
		}
		visited[ptr] = struct{}{}

		total := uint64(unsafe.Sizeof(v.Interface()))
		for _, key := range v.MapKeys() {
			total += c.calculateValueSizeWithUnsafe(key, visited)
			total += c.calculateValueSizeWithUnsafe(v.MapIndex(key), visited)
		}
		return total

	case reflect.Array:
		total := uint64(0)
		for i := 0; i < v.Len(); i++ {
			total += c.calculateValueSizeWithUnsafe(v.Index(i), visited)
		}
		return total

	default:
		return uint64(v.Type().Size())
	}
}

func (c *InMemoryCache[K, V]) Set(key K, value V, duration time.Duration) bool {
	expiration := c.calculateExpiration(duration)
	itemSize := c.calculateItemSize(key, value)

	c.Lock()
	defer c.Unlock()

	if c.haveLimitMaximumCapacity {
		if !c.makeSpaceFor(itemSize) {
			return false
		}
	}

	c.items[key] = CacheItem[K, V]{
		Key:        key,
		Value:      value,
		Created:    time.Now(),
		Expiration: expiration,
	}
	c.adjustCurrentSize(int64(itemSize))

	return true
}

func (c *InMemoryCache[K, V]) calculateExpiration(duration time.Duration) int64 {
	if duration == DefaultExpiration {
		duration = c.defaultExpiration
	}

	switch {
	case duration == NoExpiration:
		return int64(NoExpiration)
	case duration > 0:
		return time.Now().Add(duration).UnixNano()
	default:
		return int64(NoExpiration)
	}
}

func (c *InMemoryCache[K, V]) makeSpaceFor(requiredSize uint64) bool {
	thresholdSize := uint64(float64(c.capacity) * MaxCapacityThreshold)

	if c.currentSize+requiredSize <= thresholdSize {
		return true
	}

	sorted := make([]*CacheForArray[K, V], 0, len(c.items))
	for k, v := range c.items {
		sorted = append(sorted, &CacheForArray[K, V]{Key: k, Value: v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Value.Expiration == sorted[j].Value.Expiration {
			return sorted[i].Value.Created.Before(sorted[j].Value.Created)
		}
		return sorted[i].Value.Expiration < sorted[j].Value.Expiration
	})

	// Удаляем 50% самых старых элементов
	numToDelete := len(sorted) / 2
	for i := 0; i < numToDelete; i++ {
		item := sorted[i]
		itemSize := c.calculateItemSize(item.Key, item.Value.Value)
		delete(c.items, item.Key)
		c.adjustCurrentSize(-int64(itemSize))
	}

	return c.currentSize+requiredSize <= thresholdSize
}

func (c *InMemoryCache[K, V]) Get(key K) (*CacheItem[K, V], bool) {
	c.RLock()
	defer c.RUnlock()

	item, found := c.items[key]
	if !found {
		return nil, false
	}

	if item.Expiration > 0 && time.Now().UnixNano() > item.Expiration {
		return nil, false
	}

	return &item, true
}

func (c *InMemoryCache[K, V]) Delete(key K) error {
	c.Lock()
	defer c.Unlock()

	item, found := c.items[key]
	if !found {
		return fmt.Errorf("item with key %v not exists", key)
	}

	itemSize := c.calculateItemSize(key, item.Value)
	delete(c.items, key)
	c.adjustCurrentSize(-int64(itemSize))

	return nil
}

func (c *InMemoryCache[K, V]) RenameKey(oldKey K, newKey K) error {
	c.Lock()
	defer c.Unlock()

	item, found := c.items[oldKey]
	if !found {
		return fmt.Errorf("item with key %v not exists", oldKey)
	}

	if _, exists := c.items[newKey]; exists {
		return fmt.Errorf("key %v already exists", newKey)
	}

	itemSize := c.calculateItemSize(oldKey, item.Value)
	newItemSize := c.calculateItemSize(newKey, item.Value)

	if c.haveLimitMaximumCapacity {
		if c.currentSize-newItemSize+itemSize > uint64(float64(c.capacity)*MaxCapacityThreshold) {
			if !c.makeSpaceFor(newItemSize - itemSize) {
				return fmt.Errorf("not enough space after rename")
			}
		}
	}

	delete(c.items, oldKey)
	item.Key = newKey
	c.items[newKey] = item
	delta := int64(newItemSize) - int64(itemSize)
	c.adjustCurrentSize(delta)

	return nil
}

func (c *InMemoryCache[K, V]) CacheSize() CacheSize {
	c.RLock()
	defer c.RUnlock()

	return CacheSize{
		Len:    len(c.items),
		Weight: c.currentSize,
	}
}

func (c *InMemoryCache[K, V]) FlashAll() {
	c.Lock()
	defer c.Unlock()

	c.items = make(map[K]CacheItem[K, V])
	c.currentSize = 0
}

func (c *InMemoryCache[K, V]) gC(ctx context.Context) {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.clearExpired()
		case <-ctx.Done():
			return
		}
	}
}

func (c *InMemoryCache[K, V]) clearExpired() {
	now := time.Now().UnixNano()

	c.Lock()
	defer c.Unlock()

	for k, item := range c.items {
		if item.Expiration > 0 && now > item.Expiration {
			itemSize := c.calculateItemSize(k, item.Value)
			delete(c.items, k)
			c.adjustCurrentSize(-int64(itemSize))
		}
	}
}

func (c *InMemoryCache[K, V]) adjustCurrentSize(delta int64) {
	newSize := int64(c.currentSize) + delta
	if newSize < 0 {
		c.currentSize = 0
	} else {
		c.currentSize = uint64(newSize)
	}
}
