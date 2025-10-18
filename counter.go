package smarterbase

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Counter provides atomic counter operations with Redis backend.
// Useful for generating sequential IDs, tracking counts, etc.
type Counter struct {
	redis   *redis.Client
	key     string
	logger  Logger
	metrics Metrics
}

// NewCounter creates a new Redis-backed atomic counter
func NewCounter(redis *redis.Client, key string, logger Logger, metrics Metrics) *Counter {
	if logger == nil {
		logger = &NoOpLogger{}
	}
	if metrics == nil {
		metrics = &NoOpMetrics{}
	}

	return &Counter{
		redis:   redis,
		key:     key,
		logger:  logger,
		metrics: metrics,
	}
}

// Increment atomically increments the counter and returns the new value
func (c *Counter) Increment(ctx context.Context) (int64, error) {
	if c.redis == nil {
		return 0, fmt.Errorf("redis not available")
	}

	val, err := c.redis.Incr(ctx, c.key).Result()
	if err != nil {
		c.metrics.Increment(MetricCounterError, "operation", "increment", "key", c.key)
		return 0, fmt.Errorf("failed to increment counter: %w", err)
	}

	c.metrics.Increment(MetricCounterIncrement, "key", c.key)
	return val, nil
}

// Get returns the current counter value
func (c *Counter) Get(ctx context.Context) (int64, error) {
	if c.redis == nil {
		return 0, fmt.Errorf("redis not available")
	}

	val, err := c.redis.Get(ctx, c.key).Result()
	if err == redis.Nil {
		// Counter doesn't exist yet, return 0
		return 0, nil
	}
	if err != nil {
		c.metrics.Increment(MetricCounterError, "operation", "get", "key", c.key)
		return 0, fmt.Errorf("failed to get counter: %w", err)
	}

	intVal, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid counter value: %w", err)
	}

	return intVal, nil
}

// Set sets the counter to a specific value
// ⚠️ USE WITH CAUTION: Only for migrations or recovery operations
func (c *Counter) Set(ctx context.Context, value int64) error {
	if c.redis == nil {
		return fmt.Errorf("redis not available")
	}

	err := c.redis.Set(ctx, c.key, value, 0).Err()
	if err != nil {
		c.metrics.Increment(MetricCounterError, "operation", "set", "key", c.key)
		return fmt.Errorf("failed to set counter: %w", err)
	}

	c.logger.Info("counter value set", "key", c.key, "value", value)
	c.metrics.Increment(MetricCounterSet, "key", c.key)
	return nil
}

// Reset resets the counter to zero
func (c *Counter) Reset(ctx context.Context) error {
	return c.Set(ctx, 0)
}

// Delete removes the counter
func (c *Counter) Delete(ctx context.Context) error {
	if c.redis == nil {
		return fmt.Errorf("redis not available")
	}

	err := c.redis.Del(ctx, c.key).Err()
	if err != nil {
		c.metrics.Increment(MetricCounterError, "operation", "delete", "key", c.key)
		return fmt.Errorf("failed to delete counter: %w", err)
	}

	c.logger.Info("counter deleted", "key", c.key)
	c.metrics.Increment(MetricCounterDelete, "key", c.key)
	return nil
}

// CounterAudit provides auditing and verification of counter values
type CounterAudit struct {
	redis   *redis.Client
	logger  Logger
	metrics Metrics
}

// NewCounterAudit creates a new counter audit utility
func NewCounterAudit(redis *redis.Client, logger Logger, metrics Metrics) *CounterAudit {
	if logger == nil {
		logger = &NoOpLogger{}
	}
	if metrics == nil {
		metrics = &NoOpMetrics{}
	}

	return &CounterAudit{
		redis:   redis,
		logger:  logger,
		metrics: metrics,
	}
}

// CounterInfo contains information about a counter
type CounterInfo struct {
	Key          string
	Value        int64
	LastModified time.Time
	MemoryUsage  int64 // Memory used in bytes
	TTL          time.Duration
}

// GetCounterInfo retrieves detailed information about a counter
func (ca *CounterAudit) GetCounterInfo(ctx context.Context, key string) (*CounterInfo, error) {
	if ca.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}

	info := &CounterInfo{
		Key: key,
	}

	// Get value
	val, err := ca.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get counter value: %w", err)
	}

	intVal, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid counter value: %w", err)
	}
	info.Value = intVal

	// Get TTL
	ttl, err := ca.redis.TTL(ctx, key).Result()
	if err != nil {
		ca.logger.Warn("failed to get TTL", "key", key, "error", err)
	} else {
		info.TTL = ttl
	}

	// Try to get memory usage (requires Redis 4.0+)
	memUsage, err := ca.redis.MemoryUsage(ctx, key).Result()
	if err != nil {
		ca.logger.Debug("failed to get memory usage", "key", key, "error", err)
	} else {
		info.MemoryUsage = memUsage
	}

	return info, nil
}

// ListCounters lists all counters matching a pattern
func (ca *CounterAudit) ListCounters(ctx context.Context, pattern string) ([]string, error) {
	if ca.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}

	var counters []string
	var cursor uint64

	for {
		var keys []string
		var err error
		keys, cursor, err = ca.redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan counters: %w", err)
		}

		counters = append(counters, keys...)

		if cursor == 0 {
			break
		}
	}

	return counters, nil
}

// AuditReport contains the results of a counter audit
type AuditReport struct {
	Timestamp        time.Time
	TotalCounters    int
	InvalidCounters  []string // Counters with non-integer values
	NegativeCounters []string // Counters with negative values (usually unexpected)
	LargeCounters    []string // Counters with unusually large values
	ZeroCounters     []string // Counters with zero value
	CounterValues    map[string]int64
	Warnings         []string
}

// AuditOptions configures the audit process
type AuditOptions struct {
	Pattern        string // Redis key pattern (e.g., "counter:*")
	LargeThreshold int64  // Values above this are flagged as large
	CheckNegative  bool   // Flag negative values as warnings
	CheckZero      bool   // Include zero-value counters in report
}

// DefaultAuditOptions returns sensible defaults for counter auditing
func DefaultAuditOptions() *AuditOptions {
	return &AuditOptions{
		Pattern:        "counter:*",
		LargeThreshold: 1000000, // 1 million
		CheckNegative:  true,
		CheckZero:      false,
	}
}

// Audit performs a comprehensive audit of counters
func (ca *CounterAudit) Audit(ctx context.Context, opts *AuditOptions) (*AuditReport, error) {
	if opts == nil {
		opts = DefaultAuditOptions()
	}

	report := &AuditReport{
		Timestamp:        time.Now(),
		InvalidCounters:  make([]string, 0),
		NegativeCounters: make([]string, 0),
		LargeCounters:    make([]string, 0),
		ZeroCounters:     make([]string, 0),
		CounterValues:    make(map[string]int64),
		Warnings:         make([]string, 0),
	}

	// List all counters
	counters, err := ca.ListCounters(ctx, opts.Pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list counters: %w", err)
	}

	report.TotalCounters = len(counters)

	// Audit each counter
	for _, key := range counters {
		val, err := ca.redis.Get(ctx, key).Result()
		if err == redis.Nil {
			continue // Counter was deleted during scan
		}
		if err != nil {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("Failed to read counter %s: %v", key, err))
			continue
		}

		// Try to parse as integer
		intVal, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			report.InvalidCounters = append(report.InvalidCounters, key)
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("Counter %s has invalid value: %s", key, val))
			continue
		}

		report.CounterValues[key] = intVal

		// Check for negative values
		if opts.CheckNegative && intVal < 0 {
			report.NegativeCounters = append(report.NegativeCounters, key)
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("Counter %s has negative value: %d", key, intVal))
		}

		// Check for zero values
		if opts.CheckZero && intVal == 0 {
			report.ZeroCounters = append(report.ZeroCounters, key)
		}

		// Check for large values
		if intVal > opts.LargeThreshold {
			report.LargeCounters = append(report.LargeCounters, key)
		}
	}

	// Log summary
	ca.logger.Info("counter audit completed",
		"total", report.TotalCounters,
		"invalid", len(report.InvalidCounters),
		"negative", len(report.NegativeCounters),
		"large", len(report.LargeCounters),
		"zero", len(report.ZeroCounters),
	)

	ca.metrics.Gauge(MetricCounterAuditTotal, float64(report.TotalCounters))
	ca.metrics.Gauge(MetricCounterAuditInvalid, float64(len(report.InvalidCounters)))
	ca.metrics.Gauge(MetricCounterAuditNegative, float64(len(report.NegativeCounters)))

	return report, nil
}

// RepairCounter attempts to fix a counter by setting it to the suggested value
func (ca *CounterAudit) RepairCounter(ctx context.Context, key string, suggestedValue int64) error {
	if ca.redis == nil {
		return fmt.Errorf("redis not available")
	}

	// Get current value for logging
	currentVal, _ := ca.redis.Get(ctx, key).Result()

	// Set new value
	err := ca.redis.Set(ctx, key, suggestedValue, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to repair counter: %w", err)
	}

	ca.logger.Info("counter repaired",
		"key", key,
		"old_value", currentVal,
		"new_value", suggestedValue,
	)

	ca.metrics.Increment(MetricCounterRepair, "key", key)

	return nil
}

// Example usage:
//
//	// Create counter
//	counter := smarterbase.NewCounter(redisClient, "counter:case_number:ws_123", logger, metrics)
//
//	// Increment and get next value
//	nextID, err := counter.Increment(ctx)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Next case number: %d\n", nextID)
//
//	// Audit all counters
//	audit := smarterbase.NewCounterAudit(redisClient, logger, metrics)
//	report, err := audit.Audit(ctx, &smarterbase.AuditOptions{
//	    Pattern: "counter:*",
//	    LargeThreshold: 100000,
//	    CheckNegative: true,
//	})
//
//	if len(report.InvalidCounters) > 0 {
//	    fmt.Printf("Found %d invalid counters\n", len(report.InvalidCounters))
//	}
//
//	// Repair a counter
//	err = audit.RepairCounter(ctx, "counter:case_number:ws_123", 1543)
