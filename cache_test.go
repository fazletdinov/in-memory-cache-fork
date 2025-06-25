package inmemorycache

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestCacheCreation проверяет создание нового кэша
func TestCacheCreation(t *testing.T) {
	t.Run("Default creation", func(t *testing.T) {
		cache := New[string, int](
			context.Background(),
			time.Minute,
			time.Minute*5,
			false,
			0,
		)
		assert.NotNil(t, cache)
		assert.Equal(t, time.Minute, cache.defaultExpiration)
		assert.Equal(t, time.Minute*5, cache.cleanupInterval)
	})

	t.Run("With capacity limit", func(t *testing.T) {
		cache := New[string, string](
			context.Background(),
			time.Minute,
			0,
			true,
			1024,
		)
		assert.True(t, cache.haveLimitMaximumCapacity)
		assert.Equal(t, uint64(1024), cache.capacity)
	})
}

// TestBasicOperations проверяет базовые операции
func TestBasicOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache := New[string, string](
		ctx,
		time.Minute,
		time.Second,
		false,
		0,
	)

	t.Run("Set and Get", func(t *testing.T) {
		ok := cache.Set("key1", "value1", time.Minute)
		assert.True(t, ok)

		item, found := cache.Get("key1")
		assert.True(t, found)
		assert.Equal(t, "value1", item.Value)
	})

	t.Run("Get non-existent", func(t *testing.T) {
		_, found := cache.Get("nonexistent")
		assert.False(t, found)
	})

	t.Run("Delete", func(t *testing.T) {
		err := cache.Delete("key1")
		assert.NoError(t, err)
		_, found := cache.Get("key1")
		assert.False(t, found)
	})

	t.Run("Delete non-existent", func(t *testing.T) {
		err := cache.Delete("nonexistent")
		assert.Error(t, err)
	})
}

// TestExpiration проверяет работу срока жизни
func TestExpiration(t *testing.T) {
	cache := New[string, string](
		context.Background(),
		time.Millisecond*50,
		time.Millisecond*10,
		false,
		0,
	)

	t.Run("Item expires", func(t *testing.T) {
		cache.Set("temp", "value", time.Millisecond*50)
		time.Sleep(time.Millisecond * 60)
		_, found := cache.Get("temp")
		assert.False(t, found)
	})

	t.Run("No expiration", func(t *testing.T) {
		cache.Set("perm", "value", NoExpiration)
		time.Sleep(time.Millisecond * 60)
		_, found := cache.Get("perm")
		assert.True(t, found)
	})
}

// TestCapacityManagement проверяет работу с ограничением емкости
func TestCapacityManagement(t *testing.T) {
	cache := New[string, string](
		context.Background(),
		time.Minute,
		0,
		true,
		300, // Маленькая емкость для тестирования
	)

	t.Run("Add within capacity", func(t *testing.T) {
		assert.True(t, cache.Set("k1", strings.Repeat("a", 100), time.Minute))
		assert.True(t, cache.Set("k2", strings.Repeat("b", 100), time.Minute))
		assert.Equal(t, 2, len(cache.items))
	})

	t.Run("Exceed capacity", func(t *testing.T) {
		ok := cache.Set("k3", strings.Repeat("c", 150), time.Minute)
		if ok {
			// Проверяем что какие-то элементы были удалены
			assert.Less(t, cache.currentSize, uint64(300))
		} else {
			assert.False(t, ok)
		}
	})
}

// TestRenameKey проверяет переименование ключей
func TestRenameKey(t *testing.T) {
	cache := New[string, string](
		context.Background(),
		time.Minute,
		0,
		false,
		0,
	)

	t.Run("Successful rename", func(t *testing.T) {
		cache.Set("old", "value", time.Minute)
		err := cache.RenameKey("old", "new")
		assert.NoError(t, err)

		_, found := cache.Get("new")
		assert.True(t, found)
		_, found = cache.Get("old")
		assert.False(t, found)
	})

	t.Run("Rename non-existent", func(t *testing.T) {
		err := cache.RenameKey("nonexistent", "new")
		assert.Error(t, err)
	})

	t.Run("Rename to existing", func(t *testing.T) {
		cache.Set("exist1", "value1", time.Minute)
		cache.Set("exist2", "value2", time.Minute)
		err := cache.RenameKey("exist1", "exist2")
		assert.Error(t, err)
	})
}

// TestConcurrentAccess проверяет конкурентный доступ
func TestConcurrentAccess(t *testing.T) {
	cache := New[int, int](
		context.Background(),
		time.Minute,
		0,
		false,
		0,
	)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache.Set(i, i*i, time.Minute)
			_, _ = cache.Get(i)
			_ = cache.Delete(i)
		}(i)
	}
	wg.Wait()
}

// TestCacheSize проверяет корректность расчета размера
func TestCacheSize(t *testing.T) {
	cache := New[string, string](
		context.Background(),
		time.Minute,
		0,
		true,
		1000,
	)

	cache.Set("k1", strings.Repeat("a", 100), time.Minute)
	cache.Set("k2", strings.Repeat("b", 200), time.Minute)

	size := cache.CacheSize()
	assert.Equal(t, 2, size.Len)
	assert.Greater(t, size.Weight, uint64(0))
}

// TestFlushAll проверяет полную очистку кэша
func TestFlashAll(t *testing.T) {
	cache := New[string, string](
		context.Background(),
		time.Minute,
		0,
		false,
		0,
	)

	cache.Set("k1", "v1", time.Minute)
	cache.Set("k2", "v2", time.Minute)
	assert.Equal(t, 2, len(cache.items))

	cache.FlashAll()
	assert.Equal(t, 0, len(cache.items))
}

// TestGC проверяет работу сборщика мусора
func TestGC(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cache := New[string, string](
		ctx,
		time.Millisecond*50,
		time.Millisecond*10,
		false,
		0,
	)

	cache.Set("temp", "value", time.Millisecond*50)
	time.Sleep(time.Millisecond * 60)
	assert.Equal(t, 0, len(cache.items))
}

// TestEdgeCases проверяет граничные случаи
func TestEdgeCases(t *testing.T) {
	t.Run("Zero value cache", func(t *testing.T) {
		var cache *InMemoryCache[string, string]
		assert.Panics(t, func() {
			cache.Set("key", "value", time.Minute)
		})
	})

	t.Run("Empty key", func(t *testing.T) {
		cache := New[string, string](
			context.Background(),
			time.Minute,
			0,
			false,
			0,
		)
		assert.True(t, cache.Set("", "empty", time.Minute))
		_, found := cache.Get("")
		assert.True(t, found)
	})

	t.Run("Nil value", func(t *testing.T) {
		cache := New[string, *string](
			context.Background(),
			time.Minute,
			0,
			false,
			0,
		)
		assert.True(t, cache.Set("nil", nil, time.Minute))
		item, found := cache.Get("nil")
		assert.True(t, found)
		assert.Nil(t, item.Value)
	})
}
