package smarterbase

import (
	"sync"
	"time"
)

// Metrics provides observability for Smarterbase operations
type Metrics interface {
	// Increment increases a counter by 1
	Increment(name string, tags ...string)

	// Gauge sets an absolute value
	Gauge(name string, value float64, tags ...string)

	// Histogram records a value distribution (latency, size, etc)
	Histogram(name string, value float64, tags ...string)

	// Timing records a duration
	Timing(name string, duration time.Duration, tags ...string)
}

// NoOpMetrics is a metrics collector that does nothing
type NoOpMetrics struct{}

// Increment increments a counter (no-op implementation)
func (m *NoOpMetrics) Increment(name string, tags ...string) {}

// Gauge sets a gauge value (no-op implementation)
func (m *NoOpMetrics) Gauge(name string, value float64, tags ...string) {}

// Histogram records a histogram value (no-op implementation)
func (m *NoOpMetrics) Histogram(name string, value float64, tags ...string) {}

// Timing records a timing duration (no-op implementation)
func (m *NoOpMetrics) Timing(name string, duration time.Duration, tags ...string) {}

// InMemoryMetrics stores metrics in memory for testing (thread-safe)
type InMemoryMetrics struct {
	mu         sync.RWMutex
	Counters   map[string]int
	Gauges     map[string]float64
	Histograms map[string][]float64
	Timings    map[string][]time.Duration
}

// NewInMemoryMetrics creates an in-memory metrics collector for testing
func NewInMemoryMetrics() *InMemoryMetrics {
	return &InMemoryMetrics{
		Counters:   make(map[string]int),
		Gauges:     make(map[string]float64),
		Histograms: make(map[string][]float64),
		Timings:    make(map[string][]time.Duration),
	}
}

// Increment increments a counter in memory
func (m *InMemoryMetrics) Increment(name string, tags ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Counters[name]++
}

// Gauge sets a gauge value in memory
func (m *InMemoryMetrics) Gauge(name string, value float64, tags ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Gauges[name] = value
}

// Histogram records a histogram value in memory
func (m *InMemoryMetrics) Histogram(name string, value float64, tags ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Histograms[name] = append(m.Histograms[name], value)
}

// Timing records a timing duration in memory
func (m *InMemoryMetrics) Timing(name string, duration time.Duration, tags ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Timings[name] = append(m.Timings[name], duration)
}

// Common metric names
const (
	MetricGetSuccess          = "smarterbase.get.success"
	MetricGetError            = "smarterbase.get.error"
	MetricGetDuration         = "smarterbase.get.duration"
	MetricPutSuccess          = "smarterbase.put.success"
	MetricPutError            = "smarterbase.put.error"
	MetricPutDuration         = "smarterbase.put.duration"
	MetricDeleteSuccess       = "smarterbase.delete.success"
	MetricDeleteError         = "smarterbase.delete.error"
	MetricDeleteDuration      = "smarterbase.delete.duration"
	MetricQueryDuration       = "smarterbase.query.duration"
	MetricQueryResults        = "smarterbase.query.results"
	MetricIndexUpdate         = "smarterbase.index.update"
	MetricIndexRetries        = "smarterbase.index.retries"
	MetricIndexErrors         = "smarterbase.index.errors"
	MetricTransactionSuccess  = "smarterbase.transaction.success"
	MetricTransactionConflict = "smarterbase.transaction.conflict"
	MetricTransactionRollback = "smarterbase.transaction.rollback"
	MetricLockAcquired        = "smarterbase.lock.acquired"
	MetricLockFailed          = "smarterbase.lock.failed"
	MetricLockDuration        = "smarterbase.lock.duration"
	MetricLockContention      = "smarterbase.lock.contention"    // Number of retries needed
	MetricLockTimeout         = "smarterbase.lock.timeout"       // Locks that timed out
	MetricLockWaitTime        = "smarterbase.lock.wait_duration" // Time spent waiting for locks

	// Additional metrics for Prometheus integration
	MetricBackendOps      = "smarterbase.backend.ops"
	MetricBackendErrors   = "smarterbase.backend.errors"
	MetricBackendLatency  = "smarterbase.backend.latency"
	MetricIndexHits       = "smarterbase.index.hits"
	MetricIndexMisses     = "smarterbase.index.misses"
	MetricCacheHits       = "smarterbase.cache.hits"
	MetricCacheMisses     = "smarterbase.cache.misses"
	MetricTransactionSize = "smarterbase.transaction.size"
	MetricCacheSize       = "smarterbase.cache.size"
)

// Production integrations:
//
// For Prometheus (github.com/prometheus/client_golang):
//   type PrometheusMetrics struct {
//       counters   map[string]prometheus.Counter
//       gauges     map[string]prometheus.Gauge
//       histograms map[string]prometheus.Histogram
//   }
//
// For Datadog (github.com/DataDog/datadog-go/statsd):
//   type DatadogMetrics struct { client *statsd.Client }
//   func (m *DatadogMetrics) Increment(name string, tags ...string) {
//       m.client.Incr(name, tags, 1)
//   }
//
// For StatsD:
//   type StatsDMetrics struct { client *statsd.Client }
//   func (m *StatsDMetrics) Timing(name string, duration time.Duration, tags ...string) {
//       m.client.Timing(name, duration, tags...)
//   }
