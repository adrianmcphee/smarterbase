package smarterbase

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// PrometheusMetrics implements the Metrics interface using Prometheus
type PrometheusMetrics struct {
	counters   map[string]*prometheus.CounterVec
	gauges     map[string]*prometheus.GaugeVec
	histograms map[string]*prometheus.HistogramVec
	registry   *prometheus.Registry
}

// NewPrometheusMetrics creates a new Prometheus metrics instance
// If registry is nil, uses the default Prometheus registry
func NewPrometheusMetrics(registry *prometheus.Registry) *PrometheusMetrics {
	if registry == nil {
		registry = prometheus.DefaultRegisterer.(*prometheus.Registry)
	}

	pm := &PrometheusMetrics{
		counters:   make(map[string]*prometheus.CounterVec),
		gauges:     make(map[string]*prometheus.GaugeVec),
		histograms: make(map[string]*prometheus.HistogramVec),
		registry:   registry,
	}

	pm.registerDefaultMetrics()
	return pm
}

// registerDefaultMetrics registers all standard Smarterbase metrics
func (p *PrometheusMetrics) registerDefaultMetrics() {
	// Operation counts
	p.counters[MetricBackendOps] = promauto.With(p.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smarterbase",
			Subsystem: "backend",
			Name:      "operations_total",
			Help:      "Total number of backend operations",
		},
		[]string{"operation", "backend"},
	)

	p.counters[MetricBackendErrors] = promauto.With(p.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smarterbase",
			Subsystem: "backend",
			Name:      "errors_total",
			Help:      "Total number of backend errors",
		},
		[]string{"operation", "backend", "error_type"},
	)

	p.counters[MetricIndexHits] = promauto.With(p.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smarterbase",
			Subsystem: "index",
			Name:      "hits_total",
			Help:      "Total number of index hits",
		},
		[]string{"entity", "index"},
	)

	p.counters[MetricIndexMisses] = promauto.With(p.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smarterbase",
			Subsystem: "index",
			Name:      "misses_total",
			Help:      "Total number of index misses",
		},
		[]string{"entity", "index"},
	)

	p.counters[MetricCacheHits] = promauto.With(p.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smarterbase",
			Subsystem: "cache",
			Name:      "hits_total",
			Help:      "Total number of cache hits",
		},
		[]string{"key_prefix"},
	)

	p.counters[MetricCacheMisses] = promauto.With(p.registry).NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smarterbase",
			Subsystem: "cache",
			Name:      "misses_total",
			Help:      "Total number of cache misses",
		},
		[]string{"key_prefix"},
	)

	// Timing histograms
	p.histograms[MetricBackendLatency] = promauto.With(p.registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "smarterbase",
			Subsystem: "backend",
			Name:      "operation_duration_seconds",
			Help:      "Backend operation duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation", "backend"},
	)

	p.histograms[MetricQueryDuration] = promauto.With(p.registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "smarterbase",
			Subsystem: "query",
			Name:      "duration_seconds",
			Help:      "Query execution duration in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"prefix"},
	)

	// Result counts
	p.histograms[MetricQueryResults] = promauto.With(p.registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "smarterbase",
			Subsystem: "query",
			Name:      "results",
			Help:      "Number of results returned by queries",
			Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 10000},
		},
		[]string{"prefix"},
	)

	// Gauge metrics
	p.gauges[MetricTransactionSize] = promauto.With(p.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "smarterbase",
			Subsystem: "transaction",
			Name:      "size",
			Help:      "Number of operations in current transaction",
		},
		[]string{},
	)

	p.gauges[MetricCacheSize] = promauto.With(p.registry).NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "smarterbase",
			Subsystem: "cache",
			Name:      "size_bytes",
			Help:      "Current cache size in bytes",
		},
		[]string{},
	)
}

// Increment increments a Prometheus counter
func (p *PrometheusMetrics) Increment(name string, tags ...string) {
	counter, ok := p.counters[name]
	if !ok {
		// Create dynamic counter if it doesn't exist
		counter = promauto.With(p.registry).NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "smarterbase",
				Name:      name,
				Help:      "Dynamic counter: " + name,
			},
			p.extractLabels(tags),
		)
		p.counters[name] = counter
	}

	labels := p.extractLabelValues(tags)
	counter.With(labels).Inc()
}

// Gauge sets a Prometheus gauge value
func (p *PrometheusMetrics) Gauge(name string, value float64, tags ...string) {
	gauge, ok := p.gauges[name]
	if !ok {
		// Create dynamic gauge if it doesn't exist
		gauge = promauto.With(p.registry).NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "smarterbase",
				Name:      name,
				Help:      "Dynamic gauge: " + name,
			},
			p.extractLabels(tags),
		)
		p.gauges[name] = gauge
	}

	labels := p.extractLabelValues(tags)
	gauge.With(labels).Set(value)
}

// Histogram records a value in a Prometheus histogram
func (p *PrometheusMetrics) Histogram(name string, value float64, tags ...string) {
	histogram, ok := p.histograms[name]
	if !ok {
		// Create dynamic histogram if it doesn't exist
		histogram = promauto.With(p.registry).NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "smarterbase",
				Name:      name,
				Help:      "Dynamic histogram: " + name,
				Buckets:   prometheus.DefBuckets,
			},
			p.extractLabels(tags),
		)
		p.histograms[name] = histogram
	}

	labels := p.extractLabelValues(tags)
	histogram.With(labels).Observe(value)
}

// Timing records a duration in a Prometheus histogram
func (p *PrometheusMetrics) Timing(name string, duration time.Duration, tags ...string) {
	p.Histogram(name, duration.Seconds(), tags...)
}

// extractLabels extracts label names from tags (every even index)
func (p *PrometheusMetrics) extractLabels(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}

	labels := make([]string, 0, len(tags)/2)
	for i := 0; i < len(tags); i += 2 {
		if i < len(tags) {
			labels = append(labels, tags[i])
		}
	}
	return labels
}

// extractLabelValues creates a label map from tags (key-value pairs)
func (p *PrometheusMetrics) extractLabelValues(tags []string) prometheus.Labels {
	if len(tags) == 0 {
		return prometheus.Labels{}
	}

	labels := make(prometheus.Labels)
	for i := 0; i < len(tags)-1; i += 2 {
		labels[tags[i]] = tags[i+1]
	}
	return labels
}

// GetRegistry returns the underlying Prometheus registry
func (p *PrometheusMetrics) GetRegistry() *prometheus.Registry {
	return p.registry
}
