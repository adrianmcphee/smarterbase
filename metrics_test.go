package smarterbase

import (
	"testing"
	"time"
)

func TestNoOpMetrics(t *testing.T) {
	metrics := &NoOpMetrics{}

	// All calls should be safe (no panics, no output)
	metrics.Increment("test.counter")
	metrics.Gauge("test.gauge", 42.0)
	metrics.Histogram("test.histogram", 100.5)
	metrics.Timing("test.timing", 5*time.Millisecond)

	// With tags
	metrics.Increment("test.counter", "tag1", "tag2")
	metrics.Gauge("test.gauge", 42.0, "env:prod")
	metrics.Histogram("test.histogram", 100.5, "region:us-west")
	metrics.Timing("test.timing", 5*time.Millisecond, "endpoint:/api/users")
}

func TestInMemoryMetrics(t *testing.T) {
	metrics := NewInMemoryMetrics()

	// Test counters
	metrics.Increment("requests")
	metrics.Increment("requests")
	metrics.Increment("requests")
	metrics.Increment("errors")

	if metrics.Counters["requests"] != 3 {
		t.Errorf("requests counter = %d, want 3", metrics.Counters["requests"])
	}
	if metrics.Counters["errors"] != 1 {
		t.Errorf("errors counter = %d, want 1", metrics.Counters["errors"])
	}

	// Test gauges
	metrics.Gauge("memory_usage", 1024.5)
	metrics.Gauge("memory_usage", 2048.75)
	metrics.Gauge("cpu_percent", 75.0)

	if metrics.Gauges["memory_usage"] != 2048.75 {
		t.Errorf("memory_usage gauge = %f, want 2048.75", metrics.Gauges["memory_usage"])
	}
	if metrics.Gauges["cpu_percent"] != 75.0 {
		t.Errorf("cpu_percent gauge = %f, want 75.0", metrics.Gauges["cpu_percent"])
	}

	// Test histograms
	metrics.Histogram("response_size", 100.0)
	metrics.Histogram("response_size", 200.0)
	metrics.Histogram("response_size", 150.0)

	if len(metrics.Histograms["response_size"]) != 3 {
		t.Errorf("response_size histogram length = %d, want 3", len(metrics.Histograms["response_size"]))
	}
	expected := []float64{100.0, 200.0, 150.0}
	for i, v := range metrics.Histograms["response_size"] {
		if v != expected[i] {
			t.Errorf("histogram[%d] = %f, want %f", i, v, expected[i])
		}
	}

	// Test timings
	metrics.Timing("api_latency", 10*time.Millisecond)
	metrics.Timing("api_latency", 15*time.Millisecond)
	metrics.Timing("db_query", 5*time.Millisecond)

	if len(metrics.Timings["api_latency"]) != 2 {
		t.Errorf("api_latency timings length = %d, want 2", len(metrics.Timings["api_latency"]))
	}
	if metrics.Timings["api_latency"][0] != 10*time.Millisecond {
		t.Errorf("api_latency[0] = %v, want 10ms", metrics.Timings["api_latency"][0])
	}
	if metrics.Timings["db_query"][0] != 5*time.Millisecond {
		t.Errorf("db_query[0] = %v, want 5ms", metrics.Timings["db_query"][0])
	}
}

func TestMetricsInterface(t *testing.T) {
	// Verify implementations satisfy the interface
	var _ Metrics = &NoOpMetrics{}
	var _ Metrics = &InMemoryMetrics{}
}

func TestMetricConstants(t *testing.T) {
	// Verify metric name constants are defined
	constants := []string{
		MetricGetSuccess,
		MetricGetError,
		MetricGetDuration,
		MetricPutSuccess,
		MetricPutError,
		MetricPutDuration,
		MetricDeleteSuccess,
		MetricDeleteError,
		MetricDeleteDuration,
		MetricQueryDuration,
		MetricQueryResults,
		MetricIndexUpdate,
		MetricIndexRetries,
		MetricIndexErrors,
		MetricTransactionSuccess,
		MetricTransactionConflict,
		MetricTransactionRollback,
		MetricLockAcquired,
		MetricLockFailed,
		MetricLockDuration,
	}

	for _, name := range constants {
		if name == "" {
			t.Errorf("metric constant is empty")
		}
		if name[:11] != "smarterbase" {
			t.Errorf("metric %q should start with 'smarterbase'", name)
		}
	}
}

func TestInMemoryMetricsWithTags(t *testing.T) {
	metrics := NewInMemoryMetrics()

	// Tags should be accepted but ignored in basic implementation
	metrics.Increment("requests", "method:GET", "status:200")
	metrics.Gauge("memory", 1024.0, "host:server1")
	metrics.Histogram("size", 100.0, "type:json")
	metrics.Timing("latency", 50*time.Millisecond, "endpoint:/api")

	// Verify metrics are still recorded
	if metrics.Counters["requests"] != 1 {
		t.Errorf("counter should be incremented despite tags")
	}
	if metrics.Gauges["memory"] != 1024.0 {
		t.Errorf("gauge should be set despite tags")
	}
}

func BenchmarkInMemoryMetricsIncrement(b *testing.B) {
	metrics := NewInMemoryMetrics()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		metrics.Increment("benchmark.counter")
	}
}

func BenchmarkInMemoryMetricsGauge(b *testing.B) {
	metrics := NewInMemoryMetrics()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		metrics.Gauge("benchmark.gauge", float64(i))
	}
}

func BenchmarkInMemoryMetricsHistogram(b *testing.B) {
	metrics := NewInMemoryMetrics()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		metrics.Histogram("benchmark.histogram", float64(i))
	}
}

func BenchmarkInMemoryMetricsTiming(b *testing.B) {
	metrics := NewInMemoryMetrics()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		metrics.Timing("benchmark.timing", time.Duration(i)*time.Millisecond)
	}
}
