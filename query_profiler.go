package smarterbase

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// QueryComplexity represents the time complexity of a query
type QueryComplexity string

const (
	ComplexityO1     QueryComplexity = "O(1)"       // Redis index lookup
	ComplexityOLogN  QueryComplexity = "O(log N)"   // Binary search
	ComplexityON     QueryComplexity = "O(N)"       // Full scan
	ComplexityONM    QueryComplexity = "O(N*M)"     // Nested loops
	ComplexityONLogN QueryComplexity = "O(N log N)" // Sort
)

// QueryProfile tracks execution details for a single query
type QueryProfile struct {
	Method       string // "ListUserSessions", "GetVisionCardsByPostcode"
	StartTime    time.Time
	Duration     time.Duration
	Complexity   QueryComplexity // O(1), O(N), O(N*M)
	IndexUsed    string          // "redis:sessions-by-user-id" or "none:full-scan"
	ResultCount  int
	FilterFields []string // ["user_id", "status"]
	FallbackPath bool     // Did we fall back from index to scan?
	StorageOps   int      // Number of backend Get/List operations
	Error        error    // Any error that occurred
}

// QueryProfiler collects and reports query performance
type QueryProfiler struct {
	mu                 sync.RWMutex
	profiles           []QueryProfile
	slowQueryThreshold time.Duration
	enabled            bool
}

// NewQueryProfiler creates a new query profiler
func NewQueryProfiler() *QueryProfiler {
	return &QueryProfiler{
		profiles:           make([]QueryProfile, 0),
		slowQueryThreshold: 100 * time.Millisecond,
		enabled:            true,
	}
}

// SetSlowQueryThreshold sets the duration threshold for slow queries
func (p *QueryProfiler) SetSlowQueryThreshold(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.slowQueryThreshold = d
}

// SetEnabled enables or disables profiling
func (p *QueryProfiler) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = enabled
}

// StartProfile begins profiling a query
func (p *QueryProfiler) StartProfile(method string) *QueryProfile {
	p.mu.RLock()
	enabled := p.enabled
	p.mu.RUnlock()

	if !enabled {
		return nil
	}

	return &QueryProfile{
		Method:       method,
		StartTime:    time.Now(),
		FilterFields: make([]string, 0),
	}
}

// Record records a completed query profile
func (p *QueryProfiler) Record(profile *QueryProfile) {
	if profile == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return
	}

	profile.Duration = time.Since(profile.StartTime)
	p.profiles = append(p.profiles, *profile)
}

// GetProfiles returns all recorded profiles
func (p *QueryProfiler) GetProfiles() []QueryProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a copy
	result := make([]QueryProfile, len(p.profiles))
	copy(result, p.profiles)
	return result
}

// GetSlowQueries returns queries that exceeded the slow query threshold
func (p *QueryProfiler) GetSlowQueries() []QueryProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()

	slow := make([]QueryProfile, 0)
	for _, profile := range p.profiles {
		if profile.Duration > p.slowQueryThreshold {
			slow = append(slow, profile)
		}
	}
	return slow
}

// GetFullScans returns queries that performed full scans
func (p *QueryProfiler) GetFullScans() []QueryProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()

	scans := make([]QueryProfile, 0)
	for _, profile := range p.profiles {
		if profile.Complexity == ComplexityON || profile.Complexity == ComplexityONM {
			scans = append(scans, profile)
		}
	}
	return scans
}

// GetFallbacks returns queries that fell back to full scans
func (p *QueryProfiler) GetFallbacks() []QueryProfile {
	p.mu.RLock()
	defer p.mu.RUnlock()

	fallbacks := make([]QueryProfile, 0)
	for _, profile := range p.profiles {
		if profile.FallbackPath {
			fallbacks = append(fallbacks, profile)
		}
	}
	return fallbacks
}

// Clear clears all recorded profiles
func (p *QueryProfiler) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.profiles = make([]QueryProfile, 0)
}

// Summary returns a summary of query performance
type ProfileSummary struct {
	TotalQueries    int
	SlowQueries     int
	FullScans       int
	Fallbacks       int
	AverageDuration time.Duration
	P50Duration     time.Duration
	P95Duration     time.Duration
	P99Duration     time.Duration
	ByMethod        map[string]MethodStats
	ByComplexity    map[QueryComplexity]int
}

type MethodStats struct {
	Count           int
	TotalDuration   time.Duration
	AverageDuration time.Duration
	MaxDuration     time.Duration
	MinDuration     time.Duration
	FullScans       int
	Fallbacks       int
}

// GetSummary returns a statistical summary of all profiles
func (p *QueryProfiler) GetSummary() ProfileSummary {
	p.mu.RLock()
	defer p.mu.RUnlock()

	summary := ProfileSummary{
		TotalQueries: len(p.profiles),
		ByMethod:     make(map[string]MethodStats),
		ByComplexity: make(map[QueryComplexity]int),
	}

	if len(p.profiles) == 0 {
		return summary
	}

	// Calculate stats
	var totalDuration time.Duration
	durations := make([]time.Duration, 0, len(p.profiles))

	for _, profile := range p.profiles {
		totalDuration += profile.Duration
		durations = append(durations, profile.Duration)

		if profile.Duration > p.slowQueryThreshold {
			summary.SlowQueries++
		}

		if profile.Complexity == ComplexityON || profile.Complexity == ComplexityONM {
			summary.FullScans++
		}

		if profile.FallbackPath {
			summary.Fallbacks++
		}

		// By complexity
		summary.ByComplexity[profile.Complexity]++

		// By method
		stats := summary.ByMethod[profile.Method]
		stats.Count++
		stats.TotalDuration += profile.Duration
		if stats.Count == 1 || profile.Duration > stats.MaxDuration {
			stats.MaxDuration = profile.Duration
		}
		if stats.Count == 1 || profile.Duration < stats.MinDuration {
			stats.MinDuration = profile.Duration
		}
		if profile.Complexity == ComplexityON || profile.Complexity == ComplexityONM {
			stats.FullScans++
		}
		if profile.FallbackPath {
			stats.Fallbacks++
		}
		summary.ByMethod[profile.Method] = stats
	}

	// Calculate average durations
	summary.AverageDuration = totalDuration / time.Duration(len(p.profiles))
	for method, stats := range summary.ByMethod {
		stats.AverageDuration = stats.TotalDuration / time.Duration(stats.Count)
		summary.ByMethod[method] = stats
	}

	// Calculate percentiles
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})
	summary.P50Duration = durations[len(durations)*50/100]
	summary.P95Duration = durations[len(durations)*95/100]
	summary.P99Duration = durations[len(durations)*99/100]

	return summary
}

// PrintSummary prints a formatted summary to stdout
func (p *QueryProfiler) PrintSummary() {
	summary := p.GetSummary()

	fmt.Println("\n=== Query Performance Summary ===")
	fmt.Printf("Total Queries:     %d\n", summary.TotalQueries)
	fmt.Printf("Slow Queries:      %d (%.1f%%)\n", summary.SlowQueries,
		float64(summary.SlowQueries)*100/float64(summary.TotalQueries))
	fmt.Printf("Full Scans:        %d (%.1f%%)\n", summary.FullScans,
		float64(summary.FullScans)*100/float64(summary.TotalQueries))
	fmt.Printf("Fallbacks:         %d (%.1f%%)\n", summary.Fallbacks,
		float64(summary.Fallbacks)*100/float64(summary.TotalQueries))

	fmt.Println("\n=== Duration Stats ===")
	fmt.Printf("Average:           %v\n", summary.AverageDuration)
	fmt.Printf("P50:               %v\n", summary.P50Duration)
	fmt.Printf("P95:               %v\n", summary.P95Duration)
	fmt.Printf("P99:               %v\n", summary.P99Duration)

	fmt.Println("\n=== By Complexity ===")
	for complexity, count := range summary.ByComplexity {
		pct := float64(count) * 100 / float64(summary.TotalQueries)
		fmt.Printf("%-10s %5d (%.1f%%)\n", complexity, count, pct)
	}

	fmt.Println("\n=== By Method (top 10 slowest) ===")
	// Sort by average duration
	type methodWithStats struct {
		method string
		stats  MethodStats
	}
	methods := make([]methodWithStats, 0, len(summary.ByMethod))
	for method, stats := range summary.ByMethod {
		methods = append(methods, methodWithStats{method, stats})
	}
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].stats.AverageDuration > methods[j].stats.AverageDuration
	})

	limit := 10
	if len(methods) < limit {
		limit = len(methods)
	}

	for i := 0; i < limit; i++ {
		m := methods[i]
		fmt.Printf("%-40s count=%4d avg=%8v max=%8v scans=%3d fallbacks=%3d\n",
			m.method, m.stats.Count, m.stats.AverageDuration, m.stats.MaxDuration,
			m.stats.FullScans, m.stats.Fallbacks)
	}
}

// Context key for query profiler
type profilerKey struct{}

// WithProfiler attaches a profiler to the context
func WithProfiler(ctx context.Context, profiler *QueryProfiler) context.Context {
	return context.WithValue(ctx, profilerKey{}, profiler)
}

// GetProfilerFromContext retrieves the profiler from context
func GetProfilerFromContext(ctx context.Context) *QueryProfiler {
	if profiler, ok := ctx.Value(profilerKey{}).(*QueryProfiler); ok {
		return profiler
	}
	// Return a disabled profiler if none exists
	p := NewQueryProfiler()
	p.SetEnabled(false)
	return p
}
