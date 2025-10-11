package smarterbase

import (
	"context"
	"testing"
	"time"
)

// TestNewQueryProfiler tests profiler creation
func TestNewQueryProfiler(t *testing.T) {
	profiler := NewQueryProfiler()
	if profiler == nil {
		t.Fatal("expected profiler, got nil")
	}

	if !profiler.enabled {
		t.Error("profiler should be enabled by default")
	}

	if profiler.slowQueryThreshold != 100*time.Millisecond {
		t.Errorf("expected default threshold 100ms, got %v", profiler.slowQueryThreshold)
	}
}

// TestSetSlowQueryThreshold tests threshold configuration
func TestSetSlowQueryThreshold(t *testing.T) {
	profiler := NewQueryProfiler()

	profiler.SetSlowQueryThreshold(500 * time.Millisecond)

	profiler.mu.RLock()
	threshold := profiler.slowQueryThreshold
	profiler.mu.RUnlock()

	if threshold != 500*time.Millisecond {
		t.Errorf("expected threshold 500ms, got %v", threshold)
	}
}

// TestSetEnabled tests enabling/disabling profiler
func TestSetEnabled(t *testing.T) {
	profiler := NewQueryProfiler()

	profiler.SetEnabled(false)
	profiler.mu.RLock()
	enabled := profiler.enabled
	profiler.mu.RUnlock()
	if enabled {
		t.Error("expected profiler to be disabled")
	}

	profiler.SetEnabled(true)
	profiler.mu.RLock()
	enabled = profiler.enabled
	profiler.mu.RUnlock()
	if !enabled {
		t.Error("expected profiler to be enabled")
	}
}

// TestStartProfileWhenEnabled tests profile creation when enabled
func TestStartProfileWhenEnabled(t *testing.T) {
	profiler := NewQueryProfiler()
	profiler.SetEnabled(true)

	profile := profiler.StartProfile("TestMethod")
	if profile == nil {
		t.Fatal("expected profile, got nil")
	}

	if profile.Method != "TestMethod" {
		t.Errorf("expected method 'TestMethod', got '%s'", profile.Method)
	}

	if profile.StartTime.IsZero() {
		t.Error("expected start time to be set")
	}

	if profile.FilterFields == nil {
		t.Error("expected filter fields to be initialized")
	}
}

// TestStartProfileWhenDisabled tests profile creation when disabled
func TestStartProfileWhenDisabled(t *testing.T) {
	profiler := NewQueryProfiler()
	profiler.SetEnabled(false)

	profile := profiler.StartProfile("TestMethod")
	if profile != nil {
		t.Error("expected nil profile when disabled, got profile")
	}
}

// TestRecordProfile tests recording a profile
func TestRecordProfile(t *testing.T) {
	profiler := NewQueryProfiler()

	profile := profiler.StartProfile("TestQuery")
	time.Sleep(10 * time.Millisecond)

	profile.Complexity = ComplexityO1
	profile.IndexUsed = "redis:test-index"
	profile.ResultCount = 5

	profiler.Record(profile)

	profiles := profiler.GetProfiles()
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	recorded := profiles[0]
	if recorded.Method != "TestQuery" {
		t.Errorf("expected method 'TestQuery', got '%s'", recorded.Method)
	}

	if recorded.Duration == 0 {
		t.Error("expected duration to be recorded")
	}

	if recorded.Complexity != ComplexityO1 {
		t.Errorf("expected complexity O(1), got %v", recorded.Complexity)
	}

	if recorded.ResultCount != 5 {
		t.Errorf("expected result count 5, got %d", recorded.ResultCount)
	}
}

// TestRecordNilProfile tests recording nil profile
func TestRecordNilProfile(t *testing.T) {
	profiler := NewQueryProfiler()

	// Should not panic
	profiler.Record(nil)

	profiles := profiler.GetProfiles()
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}

// TestGetProfiles tests retrieving all profiles
func TestGetProfiles(t *testing.T) {
	profiler := NewQueryProfiler()

	// Record multiple profiles
	for i := 0; i < 3; i++ {
		profile := profiler.StartProfile("Query" + string(rune(i+'0')))
		profile.Complexity = ComplexityON
		profiler.Record(profile)
	}

	profiles := profiler.GetProfiles()
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}

	// Verify it's a copy (modifying returned slice doesn't affect original)
	profiles[0].Method = "Modified"
	originalProfiles := profiler.GetProfiles()
	if originalProfiles[0].Method == "Modified" {
		t.Error("GetProfiles should return a copy, not the original slice")
	}
}

// TestGetSlowQueries tests filtering slow queries
func TestGetSlowQueries(t *testing.T) {
	profiler := NewQueryProfiler()
	profiler.SetSlowQueryThreshold(50 * time.Millisecond)

	// Record fast query
	fast := profiler.StartProfile("FastQuery")
	time.Sleep(10 * time.Millisecond)
	profiler.Record(fast)

	// Record slow query
	slow := profiler.StartProfile("SlowQuery")
	time.Sleep(60 * time.Millisecond)
	profiler.Record(slow)

	slowQueries := profiler.GetSlowQueries()
	if len(slowQueries) != 1 {
		t.Fatalf("expected 1 slow query, got %d", len(slowQueries))
	}

	if slowQueries[0].Method != "SlowQuery" {
		t.Errorf("expected slow query 'SlowQuery', got '%s'", slowQueries[0].Method)
	}
}

// TestGetFullScans tests filtering full scan queries
func TestGetFullScans(t *testing.T) {
	profiler := NewQueryProfiler()

	// Record O(1) query
	indexed := profiler.StartProfile("IndexedQuery")
	indexed.Complexity = ComplexityO1
	profiler.Record(indexed)

	// Record O(N) query
	fullScan := profiler.StartProfile("FullScanQuery")
	fullScan.Complexity = ComplexityON
	profiler.Record(fullScan)

	// Record O(N*M) query
	nestedScan := profiler.StartProfile("NestedScanQuery")
	nestedScan.Complexity = ComplexityONM
	profiler.Record(nestedScan)

	scans := profiler.GetFullScans()
	if len(scans) != 2 {
		t.Fatalf("expected 2 full scans, got %d", len(scans))
	}

	// Verify both O(N) and O(N*M) are included
	complexities := make(map[QueryComplexity]bool)
	for _, scan := range scans {
		complexities[scan.Complexity] = true
	}

	if !complexities[ComplexityON] || !complexities[ComplexityONM] {
		t.Error("expected both O(N) and O(N*M) queries in full scans")
	}
}

// TestGetFallbacks tests filtering fallback queries
func TestGetFallbacks(t *testing.T) {
	profiler := NewQueryProfiler()

	// Record regular query
	regular := profiler.StartProfile("RegularQuery")
	regular.FallbackPath = false
	profiler.Record(regular)

	// Record fallback query
	fallback := profiler.StartProfile("FallbackQuery")
	fallback.FallbackPath = true
	profiler.Record(fallback)

	fallbacks := profiler.GetFallbacks()
	if len(fallbacks) != 1 {
		t.Fatalf("expected 1 fallback, got %d", len(fallbacks))
	}

	if fallbacks[0].Method != "FallbackQuery" {
		t.Errorf("expected fallback 'FallbackQuery', got '%s'", fallbacks[0].Method)
	}
}

// TestClearProfiles tests clearing all profiles
func TestClearProfiles(t *testing.T) {
	profiler := NewQueryProfiler()

	// Record some profiles
	for i := 0; i < 5; i++ {
		profile := profiler.StartProfile("Query" + string(rune(i+'0')))
		profiler.Record(profile)
	}

	if len(profiler.GetProfiles()) != 5 {
		t.Fatal("expected 5 profiles before clear")
	}

	profiler.Clear()

	if len(profiler.GetProfiles()) != 0 {
		t.Error("expected 0 profiles after clear")
	}
}

// TestGetSummary tests summary generation
func TestGetSummary(t *testing.T) {
	profiler := NewQueryProfiler()
	profiler.SetSlowQueryThreshold(50 * time.Millisecond)

	// Record various queries
	for i := 0; i < 3; i++ {
		profile := profiler.StartProfile("Query" + string(rune(i+'0')))
		if i == 0 {
			time.Sleep(10 * time.Millisecond)
			profile.Complexity = ComplexityO1
			profile.IndexUsed = "redis:test"
		} else if i == 1 {
			time.Sleep(60 * time.Millisecond)
			profile.Complexity = ComplexityON
			profile.FallbackPath = true
		} else {
			time.Sleep(20 * time.Millisecond)
			profile.Complexity = ComplexityONM
		}
		profiler.Record(profile)
	}

	summary := profiler.GetSummary()
	if summary.TotalQueries != 3 {
		t.Errorf("expected 3 total queries, got %d", summary.TotalQueries)
	}

	if summary.SlowQueries != 1 {
		t.Errorf("expected 1 slow query, got %d", summary.SlowQueries)
	}

	if summary.FullScans != 2 {
		t.Errorf("expected 2 full scans, got %d", summary.FullScans)
	}

	if summary.Fallbacks != 1 {
		t.Errorf("expected 1 fallback, got %d", summary.Fallbacks)
	}
}

// TestPrintSummary tests summary printing (doesn't panic)
func TestPrintSummary(t *testing.T) {
	profiler := NewQueryProfiler()

	// Record some queries
	profile := profiler.StartProfile("TestQuery")
	profile.Complexity = ComplexityO1
	profile.IndexUsed = "redis:test"
	profiler.Record(profile)

	// Should not panic
	profiler.PrintSummary()
}

// TestWithProfiler tests context integration
func TestWithProfiler(t *testing.T) {
	profiler := NewQueryProfiler()
	ctx := context.Background()

	ctx = WithProfiler(ctx, profiler)

	retrieved := GetProfilerFromContext(ctx)
	if retrieved != profiler {
		t.Error("profiler not correctly stored in context")
	}
}

// TestGetProfilerFromContextWithoutProfiler tests retrieving from empty context
func TestGetProfilerFromContextWithoutProfiler(t *testing.T) {
	ctx := context.Background()

	profiler := GetProfilerFromContext(ctx)
	if profiler == nil {
		t.Fatal("expected disabled profiler, got nil")
	}

	// Should return a disabled profiler
	profiler.mu.RLock()
	enabled := profiler.enabled
	profiler.mu.RUnlock()

	if enabled {
		t.Error("profiler from empty context should be disabled")
	}
}

// TestQueryProfilerConcurrency tests concurrent access
func TestQueryProfilerConcurrency(t *testing.T) {
	profiler := NewQueryProfiler()

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				profile := profiler.StartProfile("ConcurrentQuery")
				if profile != nil {
					profile.Complexity = ComplexityO1
					profiler.Record(profile)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	profiles := profiler.GetProfiles()
	if len(profiles) != 1000 {
		t.Errorf("expected 1000 profiles, got %d", len(profiles))
	}
}

// TestQueryComplexityConstants tests complexity constants
func TestQueryComplexityConstants(t *testing.T) {
	constants := []QueryComplexity{
		ComplexityO1,
		ComplexityOLogN,
		ComplexityON,
		ComplexityONM,
		ComplexityONLogN,
	}

	for _, c := range constants {
		if string(c) == "" {
			t.Error("complexity constant is empty")
		}
	}
}
