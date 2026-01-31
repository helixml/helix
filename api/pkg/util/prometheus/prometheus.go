package promutil

import (
	"time"
)

// Simple stubs for prometheus utilities used by revdial
// These can be expanded later with actual prometheus metrics if needed

// Timer represents a prometheus timer (stub implementation)
type Timer struct {
	start time.Time
}

// NewTimer creates a new timer
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// ObserveDuration observes the duration since timer creation
func (t *Timer) ObserveDuration() {
	// Stub implementation - could log duration or send to prometheus
	_ = time.Since(t.start)
}

// Counter represents a prometheus counter (stub implementation)
type Counter struct{}

// Inc increments the counter
func (c *Counter) Inc() {
	// Stub implementation
}

// NewCounter creates a new counter
func NewCounter() *Counter {
	return &Counter{}
}

// Gauge represents a prometheus gauge (stub implementation)
type Gauge struct{}

// Add adds to the gauge
func (g *Gauge) Add(val float64) {
	// Stub implementation
}

// Sub subtracts from the gauge
func (g *Gauge) Sub(val float64) {
	// Stub implementation
}

// Set sets the gauge value
func (g *Gauge) Set(val float64) {
	// Stub implementation
}

// DeviceConnectionCount is a global gauge for device connections
var DeviceConnectionCount = &Gauge{}