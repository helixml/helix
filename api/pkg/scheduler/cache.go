package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

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
	mu     sync.Mutex
	fetch  func() (T, error)
	config CacheConfig
	done   chan struct{}
}

func NewCache[T any](ctx context.Context, fetch func() (T, error), config CacheConfig) *Cache[T] {
	c := &Cache[T]{
		fetch:  fetch,
		config: config,
		done:   make(chan struct{}),
	}

	go c.backgroundUpdate(ctx)
	return c
}

func (c *Cache[T]) backgroundUpdate(ctx context.Context) {
	for {
		select {
		case <-time.After(c.config.updateInterval):
			c.update()
		case <-ctx.Done():
			c.Close()
			return
		case <-c.done:
			return
		}
	}
}

func (c *Cache[T]) update() {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := c.fetch()
	newValue := &CacheValue[T]{
		data:      data,
		timestamp: time.Now(),
		err:       err,
	}
	c.value.Store(newValue)
}

func (c *Cache[T]) Get() (T, error) {
	value := c.value.Load()
	if value == nil {
		// First time access - need to fetch
		c.update()
		value = c.value.Load()
	}
	return value.data, value.err
}

func (c *Cache[T]) Close() {
	close(c.done)
}
