package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type LockingRunnerMap[T any] struct {
	mu *sync.Mutex
	m  map[string]*Cache[T]
}

func NewLockingRunnerMap[T any]() *LockingRunnerMap[T] {
	return &LockingRunnerMap[T]{
		mu: &sync.Mutex{},
		m:  make(map[string]*Cache[T]),
	}
}

func (m *LockingRunnerMap[T]) GetOrCreateCache(ctx context.Context, key string, fetch func() (T, error), config CacheConfig) *Cache[T] {
	// Check if the cache already exists
	if cache, ok := m.m[key]; ok {
		return cache
	}

	// If it doesn't exist, lock the map and create it
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double check that another lock hasn't just created it
	if cache, ok := m.m[key]; ok {
		return cache
	}

	cache := NewCache(ctx, fetch, config)
	m.m[key] = cache
	return cache
}

func (m *LockingRunnerMap[T]) Set(key string, cache *Cache[T]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = cache
}

func (m *LockingRunnerMap[T]) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.m))
	for key := range m.m {
		keys = append(keys, key)
	}
	return keys
}

func (m *LockingRunnerMap[T]) DeleteCache(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cache, ok := m.m[key]; ok {
		cache.Close()
		delete(m.m, key)
	}
}

type CacheValue[T any] struct {
	data      T
	timestamp time.Time
	err       error
}

type CacheConfig struct {
	updateInterval time.Duration
}

type Cache[T any] struct {
	value  atomic.Pointer[CacheValue[T]]
	fetch  func() (T, error)
	config CacheConfig
	done   chan struct{}
	wg     sync.WaitGroup
}

func NewCache[T any](ctx context.Context, fetch func() (T, error), config CacheConfig) *Cache[T] {
	c := &Cache[T]{
		fetch:  fetch,
		config: config,
		done:   make(chan struct{}),
		wg:     sync.WaitGroup{},
	}

	// Initial fetch to ensure the data is not nil, blocking
	data, err := fetch()
	c.value.Store(&CacheValue[T]{
		data:      data,
		timestamp: time.Now(),
		err:       err,
	})

	go c.backgroundUpdate(ctx)
	return c
}

func (c *Cache[T]) backgroundUpdate(ctx context.Context) {
	c.wg.Add(1)
	defer c.wg.Done()
	for {
		select {
		case <-time.After(c.config.updateInterval):
			data, err := c.fetch()
			c.value.Store(&CacheValue[T]{
				data:      data,
				timestamp: time.Now(),
				err:       err,
			})
		case <-ctx.Done():
			return
		case <-c.done:
			return
		}
	}
}

func (c *Cache[T]) Get() (T, error) {
	value := c.value.Load()
	return value.data, value.err
}

func (c *Cache[T]) Close() {
	close(c.done)
	c.wg.Wait()
}
