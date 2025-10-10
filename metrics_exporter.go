package smarterbase

import (
	"context"
	"time"
)

// MetricsRecorder is an interface for recording query metrics
// This allows smarterbase to be decoupled from specific metrics implementations
type MetricsRecorder interface {
	RecordQueryProfile(method string, complexity string, duration float64, storageOps int, resultCount int, isFullScan bool, isFallback bool, indexUsed string)
}

// MetricsExporter exports query profiler metrics to a MetricsRecorder (e.g., Prometheus)
type MetricsExporter struct {
	profiler *QueryProfiler
	recorder MetricsRecorder
	interval time.Duration
	stopCh   chan struct{}
}

// NewMetricsExporter creates a new metrics exporter
func NewMetricsExporter(profiler *QueryProfiler, recorder MetricsRecorder, interval time.Duration) *MetricsExporter {
	return &MetricsExporter{
		profiler: profiler,
		recorder: recorder,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins exporting metrics periodically
func (e *MetricsExporter) Start(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.export()
		case <-e.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop stops the exporter
func (e *MetricsExporter) Stop() {
	close(e.stopCh)
}

// export reads profiles and sends them to the recorder
func (e *MetricsExporter) export() {
	profiles := e.profiler.GetProfiles()

	for _, profile := range profiles {
		if profile.Error != nil {
			continue // Skip failed queries
		}

		isFullScan := profile.Complexity == ComplexityON || profile.Complexity == ComplexityONM

		e.recorder.RecordQueryProfile(
			profile.Method,
			string(profile.Complexity),
			profile.Duration.Seconds(),
			profile.StorageOps,
			profile.ResultCount,
			isFullScan,
			profile.FallbackPath,
			profile.IndexUsed,
		)
	}

	// Clear profiles after export to avoid re-exporting
	e.profiler.Clear()
}

// ExportOnce exports metrics once (useful for testing or manual export)
func (e *MetricsExporter) ExportOnce() {
	e.export()
}
