package inmemorycache

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"
	"unsafe"
)

const (
	maxRecursionDepth = 32
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
//
//		// Остановка GC вручную, если нужно
//		cache.StopGC()
//	}
//
// Примечание:
//   - Вы можете завершить работу GC вызовом StopGC() вручную или через отмену переданного контекста.
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

	// Запускаем сборщик мусора, если указан интервал очистки
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
	return c.calculateValueSizeWithDepth(value, 0)
}

// calculateValueSize рекурсивно вычисляет размер значения
func (c *InMemoryCache[K, V]) calculateValueSizeWithDepth(value interface{}, depth int) uint64 {
	// Защита от циклических ссылок
	if depth > maxRecursionDepth {
		return 0
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		return uint64(v.Len())

	case reflect.Slice, reflect.Array:
		size := uint64(0)
		for i := 0; i < v.Len(); i++ {
			size += c.calculateValueSizeWithDepth(v.Index(i).Interface(), depth+1)
		}
		return size + uint64(v.Cap())*uint64(v.Type().Elem().Size())

	case reflect.Map:
		size := uint64(0)
		for _, key := range v.MapKeys() {
			size += c.calculateValueSizeWithDepth(key.Interface(), depth+1) +
				c.calculateValueSizeWithDepth(v.MapIndex(key).Interface(), depth+1)
		}
		return size

	case reflect.Struct:
		size := uint64(0)
		for i := 0; i < v.NumField(); i++ {
			size += c.calculateValueSizeWithDepth(v.Field(i).Interface(), depth+1)
		}
		return size

	case reflect.Ptr:
		if v.IsNil() {
			return 0
		}
		return c.calculateValueSizeWithDepth(v.Elem().Interface(), depth+1)

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
	c.currentSize += itemSize

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
	if c.currentSize+requiredSize <= uint64(float64(c.capacity)*MaxCapacityThreshold) {
		return true
	}

	// Сортируем элементы по времени истечения
	sorted := make([]*CacheForArray[K, V], 0, len(c.items))
	for k, v := range c.items {
		sorted = append(sorted, &CacheForArray[K, V]{k, v})
	}

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Value.Expiration == sorted[j].Value.Expiration {
			return sorted[i].Value.Created.Before(sorted[j].Value.Created)
		}
		return sorted[i].Value.Expiration < sorted[j].Value.Expiration
	})

	// Удаляем самые старые элементы, пока не освободим место
	for _, item := range sorted {
		if c.currentSize+requiredSize <= uint64(float64(c.capacity)*MaxCapacityThreshold) {
			break
		}

		itemSize := c.calculateItemSize(item.Key, item.Value.Value)
		delete(c.items, item.Key)
		c.currentSize -= itemSize
	}

	return c.currentSize+requiredSize <= uint64(float64(c.capacity)*MaxCapacityThreshold)
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
	c.currentSize -= itemSize

	return nil
}

func (c *InMemoryCache[K, V]) RenameKey(oldKey K, newKey K) error {
	c.Lock()
	defer c.Unlock()

	// Проверяем существование старого ключа
	item, found := c.items[oldKey]
	if !found {
		return fmt.Errorf("item with key %v not exists", oldKey)
	}

	// Проверяем, не существует ли уже новый ключ
	if _, exists := c.items[newKey]; exists {
		return fmt.Errorf("key %v already exists", newKey)
	}

	itemSize := c.calculateItemSize(oldKey, item.Value)
	newItemSize := c.calculateItemSize(newKey, item.Value)

	// Проверяем capacity, если включено
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
	c.currentSize = c.currentSize - itemSize + newItemSize

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

func (c *InMemoryCache[K, V]) FlushAll() {
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
			c.currentSize -= itemSize
		}
	}
}
