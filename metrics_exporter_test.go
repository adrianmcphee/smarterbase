package smarterbase

import (
	"context"
	"testing"
	"time"
)

// MockMetricsRecorder implements MetricsRecorder for testing
type MockMetricsRecorder struct {
	RecordedProfiles []string
}

func (m *MockMetricsRecorder) RecordQueryProfile(method string, complexity string, duration float64, storageOps int, resultCount int, isFullScan bool, isFallback bool, indexUsed string) {
	m.RecordedProfiles = append(m.RecordedProfiles, method)
}

// TestNewMetricsExporter tests creating a metrics exporter
func TestNewMetricsExporter(t *testing.T) {
	profiler := NewQueryProfiler()
	recorder := &MockMetricsRecorder{}
	exporter := NewMetricsExporter(profiler, recorder, 100*time.Millisecond)

	if exporter == nil {
		t.Fatal("expected exporter, got nil")
	}
}

// TestMetricsExporterStartStop tests starting and stopping exporter
func TestMetricsExporterStartStop(t *testing.T) {
	// Skip this test in short mode as it involves goroutines
	if testing.Short() {
		t.Skip("Skipping background goroutine test in short mode")
	}

	ctx := context.Background()
	profiler := NewQueryProfiler()
	recorder := &MockMetricsRecorder{}
	exporter := NewMetricsExporter(profiler, recorder, 50*time.Millisecond)

	// Record a profile
	profile := profiler.StartProfile("TestQuery")
	profile.Complexity = ComplexityO1
	profiler.Record(profile)

	// Start exporter in background (Start() is blocking)
	go exporter.Start(ctx)

	// Let it run briefly
	time.Sleep(150 * time.Millisecond)

	// Stop exporter
	exporter.Stop()

	// Verify it stopped (by not crashing)
	time.Sleep(100 * time.Millisecond)

	// Verify metrics were recorded
	if len(recorder.RecordedProfiles) == 0 {
		t.Error("expected profiles to be exported")
	}
}

// TestMetricsExporterExportOnce tests one-time export
func TestMetricsExporterExportOnce(t *testing.T) {
	profiler := NewQueryProfiler()
	recorder := &MockMetricsRecorder{}
	exporter := NewMetricsExporter(profiler, recorder, 1*time.Second)

	// Record some profiles
	for i := 0; i < 3; i++ {
		profile := profiler.StartProfile("TestQuery")
		profile.Complexity = ComplexityON
		time.Sleep(10 * time.Millisecond)
		profiler.Record(profile)
	}

	// Export once
	exporter.ExportOnce()

	// Verify metrics were recorded
	if len(recorder.RecordedProfiles) != 3 {
		t.Errorf("expected 3 profiles to be exported, got %d", len(recorder.RecordedProfiles))
	}
}
