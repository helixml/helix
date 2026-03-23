package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStreamProxyDedup_SwapCancelsPrevious(t *testing.T) {
	var proxies sync.Map

	// First proxy registers
	ctx1, cancel1 := context.WithCancel(context.Background())
	done1 := make(chan struct{})
	proxy1 := &activeStreamProxy{cancel: cancel1, done: done1}
	_, loaded := proxies.Swap("ses_123", proxy1)
	assert.False(t, loaded, "first proxy should not find a previous entry")

	// Simulate proxy1 running
	go func() {
		<-ctx1.Done()
		close(done1)
	}()

	// Second proxy arrives for same session — should cancel first
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	proxy2 := &activeStreamProxy{cancel: cancel2, done: done2}

	prev, loaded := proxies.Swap("ses_123", proxy2)
	assert.True(t, loaded, "second proxy should find previous entry")

	prevProxy := prev.(*activeStreamProxy)
	prevProxy.cancel()

	select {
	case <-prevProxy.done:
		// Previous proxy shut down
	case <-time.After(time.Second):
		t.Fatal("previous proxy did not shut down within 1 second")
	}

	// Verify proxy2 is still active
	select {
	case <-ctx2.Done():
		t.Fatal("proxy2 context should still be active")
	default:
		// Good
	}

	// Cleanup
	cancel2()
	close(done2)
}

func TestStreamProxyDedup_CompareAndDeletePointerIdentity(t *testing.T) {
	var proxies sync.Map

	cancel := func() {}
	done := make(chan struct{})
	proxy1 := &activeStreamProxy{cancel: cancel, done: done}
	proxy2 := &activeStreamProxy{cancel: cancel, done: done}

	proxies.Store("ses_123", proxy1)

	// CompareAndDelete with different pointer should NOT delete
	deleted := proxies.CompareAndDelete("ses_123", proxy2)
	assert.False(t, deleted, "should not delete with different pointer")

	// CompareAndDelete with same pointer should delete
	deleted = proxies.CompareAndDelete("ses_123", proxy1)
	assert.True(t, deleted, "should delete with same pointer")

	// Verify it's gone
	_, loaded := proxies.Load("ses_123")
	assert.False(t, loaded, "entry should be removed after CompareAndDelete")
}

func TestStreamProxyDedup_RapidReconnect(t *testing.T) {
	var proxies sync.Map
	var cancelled []int

	// Simulate 3 rapid connections for the same session
	for i := 0; i < 3; i++ {
		idx := i
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		proxy := &activeStreamProxy{cancel: cancel, done: done}

		go func() {
			<-ctx.Done()
			cancelled = append(cancelled, idx)
			close(done)
		}()

		if prev, loaded := proxies.Swap("ses_123", proxy); loaded {
			prevProxy := prev.(*activeStreamProxy)
			prevProxy.cancel()
			select {
			case <-prevProxy.done:
			case <-time.After(time.Second):
				t.Fatalf("proxy %d did not shut down", idx-1)
			}
		}
	}

	// Only the last proxy should be active
	val, loaded := proxies.Load("ses_123")
	assert.True(t, loaded)
	lastProxy := val.(*activeStreamProxy)

	// Cancel the last one to clean up
	lastProxy.cancel()
	select {
	case <-lastProxy.done:
	case <-time.After(time.Second):
		t.Fatal("last proxy did not shut down")
	}

	// First two should have been cancelled
	assert.Contains(t, cancelled, 0)
	assert.Contains(t, cancelled, 1)
}
