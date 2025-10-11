package smarterbase

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// TestNewPrometheusMetrics tests creating Prometheus metrics
func TestNewPrometheusMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics(registry)

	if metrics == nil {
		t.Fatal("expected PrometheusMetrics, got nil")
	}

	if metrics.registry != registry {
		t.Error("registry not set correctly")
	}

	// Verify default metrics were registered
	if len(metrics.counters) == 0 {
		t.Error("expected counters to be registered")
	}
	if len(metrics.gauges) == 0 {
		t.Error("expected gauges to be registered")
	}
	if len(metrics.histograms) == 0 {
		t.Error("expected histograms to be registered")
	}
}

// TestNewPrometheusMetricsWithNilRegistry tests using default registry
func TestNewPrometheusMetricsWithNilRegistry(t *testing.T) {
	// Note: This will use the default Prometheus registry
	// We can't easily test this without polluting the global registry
	// So we skip this test or use a custom registry
	t.Skip("Skipping test that would pollute default registry")
}

// TestPrometheusMetricsIncrement tests counter increments
func TestPrometheusMetricsIncrement(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics(registry)

	// Test increment with labels (must match registered label count)
	metrics.Increment(MetricBackendOps, "operation", "get", "backend", "filesystem")
	metrics.Increment(MetricBackendOps, "operation", "put", "backend", "s3")
	metrics.Increment(MetricBackendOps, "operation", "delete", "backend", "filesystem")

	// Verify metrics were recorded (by checking registry)
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Should have at least the backend_operations_total metric
	found := false
	for _, mf := range metricFamilies {
		if strings.Contains(mf.GetName(), "backend_operations_total") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected backend_operations_total metric to be registered")
	}
}

// TestPrometheusMetricsGauge tests gauge operations
func TestPrometheusMetricsGauge(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics(registry)

	// Test gauge (MetricCacheSize has no labels)
	metrics.Gauge(MetricCacheSize, 5.5)
	metrics.Gauge(MetricCacheSize, 2.3)
	metrics.Gauge(MetricTransactionSize, 10)

	// Verify metrics were recorded
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metricFamilies {
		if strings.Contains(mf.GetName(), "cache_size") || strings.Contains(mf.GetName(), "transaction_size") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected gauge metrics to be registered")
	}
}

// TestPrometheusMetricsHistogram tests histogram observations
func TestPrometheusMetricsHistogram(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics(registry)

	// Test histogram with labels (must match registered label count)
	metrics.Histogram(MetricBackendLatency, 100.0, "operation", "get", "backend", "filesystem")
	metrics.Histogram(MetricBackendLatency, 50.0, "operation", "get", "backend", "filesystem")
	metrics.Histogram(MetricBackendLatency, 150.0, "operation", "put", "backend", "s3")

	// Verify metrics were recorded
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metricFamilies {
		if strings.Contains(mf.GetName(), "backend_operation_duration") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected backend operation duration histogram to be registered")
	}
}

// TestPrometheusMetricsTiming tests timing observations
func TestPrometheusMetricsTiming(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics(registry)

	// Test timing with labels (must match registered label count)
	metrics.Timing(MetricBackendLatency, 100*time.Millisecond, "operation", "get", "backend", "filesystem")
	metrics.Timing(MetricBackendLatency, 50*time.Millisecond, "operation", "get", "backend", "filesystem")
	metrics.Timing(MetricBackendLatency, 150*time.Millisecond, "operation", "put", "backend", "s3")

	// Verify histogram was updated (Timing should record to histogram)
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range metricFamilies {
		if strings.Contains(mf.GetName(), "backend_operation_duration") {
			found = true
			// Verify it's a histogram
			if mf.GetType() != 4 { // HISTOGRAM = 4
				t.Errorf("expected histogram type, got %v", mf.GetType())
			}
			break
		}
	}
	if !found {
		t.Error("expected backend operation duration metric")
	}
}

// TestPrometheusMetricsGetRegistry tests registry retrieval
func TestPrometheusMetricsGetRegistry(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics(registry)

	retrieved := metrics.GetRegistry()
	if retrieved != registry {
		t.Error("GetRegistry returned wrong registry")
	}
}

// TestPrometheusMetricsLabelExtraction tests label extraction
func TestPrometheusMetricsLabelExtraction(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics(registry)

	// Test with correct label count (must match registered labels)
	// MetricBackendOps expects "operation" and "backend" labels
	metrics.Increment(MetricBackendOps, "operation", "get", "backend", "filesystem")
	metrics.Increment(MetricBackendOps, "operation", "put", "backend", "s3")

	// MetricIndexHits expects "entity" and "index" labels
	metrics.Increment(MetricIndexHits, "entity", "users", "index", "email")
	metrics.Increment(MetricIndexHits, "entity", "orders", "index", "status")
}

// TestPrometheusMetricsAllMetricTypes tests all registered metric types
func TestPrometheusMetricsAllMetricTypes(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics(registry)

	// Record various metrics
	metrics.Increment(MetricBackendOps, "operation", "get", "backend", "filesystem")
	metrics.Increment(MetricBackendErrors, "operation", "put", "backend", "s3", "error_type", "timeout")
	metrics.Increment(MetricIndexHits, "entity", "users", "index", "email")
	metrics.Increment(MetricIndexMisses, "entity", "orders", "index", "status")
	metrics.Increment(MetricCacheHits, "key_prefix", "user:")
	metrics.Increment(MetricCacheMisses, "key_prefix", "order:")

	metrics.Gauge(MetricTransactionSize, 3.2)
	metrics.Gauge(MetricCacheSize, 1000)

	metrics.Histogram(MetricBackendLatency, 75.0, "operation", "get", "backend", "filesystem")
	metrics.Histogram(MetricQueryDuration, 120.0, "prefix", "products")

	// Gather all metrics
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	// Verify we have multiple metric families
	if len(metricFamilies) < 5 {
		t.Errorf("expected at least 5 metric families, got %d", len(metricFamilies))
	}
}

// TestPrometheusMetricsImplementsInterface verifies interface implementation
func TestPrometheusMetricsImplementsInterface(t *testing.T) {
	var _ Metrics = &PrometheusMetrics{}
}

// TestPrometheusMetricsConcurrency tests concurrent metric updates
func TestPrometheusMetricsConcurrency(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetrics(registry)

	// Run concurrent updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				metrics.Increment(MetricBackendOps, "operation", "concurrent", "backend", "test")
				metrics.Gauge(MetricCacheSize, float64(j))
				metrics.Histogram(MetricBackendLatency, float64(j), "operation", "test", "backend", "memory")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should complete without panic
}
