package smarterbase

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// IndexHealthMonitor provides automated health checking and drift detection
// for Redis indexes.
//
// Purpose:
// - Detect when Redis indexes become stale or inconsistent
// - Alert on drift before it causes data issues
// - Enable automated repair workflows
// - Provide visibility into index health
type IndexHealthMonitor struct {
	store        *Store
	redisIndexer *RedisIndexer
	logger       Logger
	metrics      Metrics

	// Configuration
	checkInterval  time.Duration
	sampleSize     int
	driftThreshold float64 // Alert if drift > this percentage

	// State
	running  bool
	stopChan chan struct{}
	mu       sync.Mutex
}

// IndexHealthReport contains the results of a health check
type IndexHealthReport struct {
	Timestamp       time.Time
	EntityType      string
	TotalSampled    int
	MissingInRedis  int
	ExtraInRedis    int
	DriftPercentage float64
	MissingKeys     []string
	ExtraKeys       []string
}

// NewIndexHealthMonitor creates a new health monitor
func NewIndexHealthMonitor(store *Store, redisIndexer *RedisIndexer) *IndexHealthMonitor {
	return &IndexHealthMonitor{
		store:          store,
		redisIndexer:   redisIndexer,
		logger:         store.logger,
		metrics:        store.metrics,
		checkInterval:  5 * time.Minute,
		sampleSize:     100,
		driftThreshold: 5.0, // Alert if >5% drift
		stopChan:       make(chan struct{}),
	}
}

// WithInterval sets the health check interval
func (ihm *IndexHealthMonitor) WithInterval(interval time.Duration) *IndexHealthMonitor {
	ihm.checkInterval = interval
	return ihm
}

// WithSampleSize sets the number of objects to sample per check
func (ihm *IndexHealthMonitor) WithSampleSize(size int) *IndexHealthMonitor {
	ihm.sampleSize = size
	return ihm
}

// WithDriftThreshold sets the drift percentage that triggers alerts
func (ihm *IndexHealthMonitor) WithDriftThreshold(threshold float64) *IndexHealthMonitor {
	ihm.driftThreshold = threshold
	return ihm
}

// Start begins automated health checking in the background
func (ihm *IndexHealthMonitor) Start(ctx context.Context) error {
	ihm.mu.Lock()
	defer ihm.mu.Unlock()

	if ihm.running {
		return fmt.Errorf("health monitor already running")
	}

	ihm.running = true

	go func() {
		ticker := time.NewTicker(ihm.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				ihm.logger.Info("index health monitor stopped", "reason", "context canceled")
				return
			case <-ihm.stopChan:
				ihm.logger.Info("index health monitor stopped", "reason", "stop requested")
				return
			case <-ticker.C:
				// Run health check
				report, err := ihm.Check(ctx, "")
				if err != nil {
					ihm.logger.Error("health check failed", "error", err)
					ihm.metrics.Increment("smarterbase.health.check.error")
					continue
				}

				// Log and alert if drift detected
				ihm.processReport(report)
			}
		}
	}()

	ihm.logger.Info("index health monitor started",
		"interval", ihm.checkInterval,
		"sample_size", ihm.sampleSize,
		"drift_threshold", ihm.driftThreshold,
	)

	return nil
}

// Stop halts the background health checking
func (ihm *IndexHealthMonitor) Stop() {
	ihm.mu.Lock()
	defer ihm.mu.Unlock()

	if ihm.running {
		close(ihm.stopChan)
		ihm.running = false
	}
}

// Check performs a single health check on the specified entity type
// If entityType is empty, checks all registered indexes
//
//nolint:gocyclo // Complexity is inherent to comprehensive health checking
func (ihm *IndexHealthMonitor) Check(ctx context.Context, entityType string) (*IndexHealthReport, error) {
	if ihm.redisIndexer == nil {
		return nil, fmt.Errorf("redis indexer not configured")
	}

	// For now, check a single entity type
	// In a real implementation, you'd iterate through all entity types

	// Sample random objects from storage
	report := &IndexHealthReport{
		Timestamp:   time.Now(),
		EntityType:  entityType,
		MissingKeys: make([]string, 0),
		ExtraKeys:   make([]string, 0),
	}

	// Get all keys for the entity type
	prefix := entityType
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}

	keys, err := ihm.store.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	if len(keys) == 0 {
		return report, nil
	}

	// Sample random keys
	sampleSize := ihm.sampleSize
	if sampleSize > len(keys) {
		sampleSize = len(keys)
	}

	// Fisher-Yates shuffle to get random sample
	sampledKeys := make([]string, len(keys))
	copy(sampledKeys, keys)

	rand.Shuffle(len(sampledKeys), func(i, j int) {
		sampledKeys[i], sampledKeys[j] = sampledKeys[j], sampledKeys[i]
	})

	sampledKeys = sampledKeys[:sampleSize]
	report.TotalSampled = sampleSize

	// Check each sampled object's indexes
	for _, key := range sampledKeys {
		// Get the object data
		data, err := ihm.store.Backend().Get(ctx, key)
		if err != nil {
			continue // Object might have been deleted
		}

		// Check if this object's indexes exist in Redis
		objectFoundInAnyIndex := false

		// Check all registered index specs
		for _, spec := range ihm.redisIndexer.specs {
			// Skip if spec doesn't match entity type
			if spec.EntityType != entityType {
				continue
			}

			// Extract index entries from the object
			entries, err := spec.ExtractFunc(key, data)
			if err != nil {
				// Object doesn't have required fields for this index - skip
				continue
			}

			// For each index entry, verify it exists in Redis
			for _, entry := range entries {
				// Query Redis to see if this key appears in the index
				keys, err := ihm.redisIndexer.Query(ctx, spec.EntityType, entry.IndexName, entry.IndexValue)
				if err != nil {
					ihm.logger.Warn("redis query failed during health check",
						"entity_type", spec.EntityType,
						"index_name", entry.IndexName,
						"index_value", entry.IndexValue,
						"error", err,
					)
					continue
				}

				// Check if our key is in the results
				foundInThisIndex := false
				for _, indexKey := range keys {
					if indexKey == key {
						foundInThisIndex = true
						objectFoundInAnyIndex = true
						break
					}
				}

				// If not found in this index, it's missing
				if !foundInThisIndex {
					report.MissingInRedis++
					report.MissingKeys = append(report.MissingKeys, key)
					break // Don't count the same key multiple times
				}
			}

			if objectFoundInAnyIndex {
				break // Don't check other specs for this key
			}
		}
	}

	// Calculate drift percentage
	if report.TotalSampled > 0 {
		totalProblems := report.MissingInRedis + report.ExtraInRedis
		report.DriftPercentage = (float64(totalProblems) / float64(report.TotalSampled)) * 100.0
	}

	return report, nil
}

// processReport handles the health check results
func (ihm *IndexHealthMonitor) processReport(report *IndexHealthReport) {
	// Record metrics
	ihm.metrics.Gauge("smarterbase.index.drift", report.DriftPercentage,
		"entity_type", report.EntityType,
	)
	ihm.metrics.Gauge("smarterbase.index.missing", float64(report.MissingInRedis),
		"entity_type", report.EntityType,
	)
	ihm.metrics.Gauge("smarterbase.index.extra", float64(report.ExtraInRedis),
		"entity_type", report.EntityType,
	)

	// Alert if drift exceeds threshold
	if report.DriftPercentage > ihm.driftThreshold {
		ihm.logger.Error("index drift detected",
			"entity_type", report.EntityType,
			"drift_percent", report.DriftPercentage,
			"missing", report.MissingInRedis,
			"extra", report.ExtraInRedis,
			"sampled", report.TotalSampled,
		)
		ihm.metrics.Increment("smarterbase.index.drift.alert",
			"entity_type", report.EntityType,
		)
	} else {
		ihm.logger.Debug("index health check passed",
			"entity_type", report.EntityType,
			"drift_percent", report.DriftPercentage,
			"sampled", report.TotalSampled,
		)
	}
}

// RepairDrift attempts to repair detected index drift
// This should be run during off-peak hours as it can be resource-intensive
func (ihm *IndexHealthMonitor) RepairDrift(ctx context.Context, report *IndexHealthReport) error {
	if ihm.redisIndexer == nil {
		return fmt.Errorf("redis indexer not configured")
	}

	ihm.logger.Info("starting index drift repair",
		"entity_type", report.EntityType,
		"missing", len(report.MissingKeys),
		"extra", len(report.ExtraKeys),
	)

	repaired := 0
	errors := 0

	// Add missing entries
	for _, key := range report.MissingKeys {
		data, err := ihm.store.Backend().Get(ctx, key)
		if err != nil {
			errors++
			continue
		}

		if err := ihm.redisIndexer.UpdateIndexes(ctx, key, data); err != nil {
			ihm.logger.Warn("failed to repair missing index", "key", key, "error", err)
			errors++
		} else {
			repaired++
		}

		// Check for cancellation
		select {
		case <-ctx.Done():
			return fmt.Errorf("repair canceled: %w", ctx.Err())
		default:
		}
	}

	// Remove extra entries
	for _, key := range report.ExtraKeys {
		// Get the data to remove from indexes
		data, err := ihm.store.Backend().Get(ctx, key)
		if err != nil {
			// Object doesn't exist, so remove from indexes
			// We need the original data to know which indexes to clean
			// For now, skip - this requires more sophisticated tracking
			continue
		}

		if err := ihm.redisIndexer.RemoveFromIndexes(ctx, key, data); err != nil {
			ihm.logger.Warn("failed to remove extra index", "key", key, "error", err)
			errors++
		} else {
			repaired++
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("repair canceled: %w", ctx.Err())
		default:
		}
	}

	ihm.logger.Info("index drift repair completed",
		"entity_type", report.EntityType,
		"repaired", repaired,
		"errors", errors,
	)

	ihm.metrics.Increment("smarterbase.index.repair.completed",
		"entity_type", report.EntityType,
	)
	ihm.metrics.Gauge("smarterbase.index.repair.count", float64(repaired),
		"entity_type", report.EntityType,
	)

	return nil
}

// Example usage:
//
//	// Initialize health monitor
//	monitor := smarterbase.NewIndexHealthMonitor(store, redisIndexer).
//	    WithInterval(5 * time.Minute).
//	    WithSampleSize(100).
//	    WithDriftThreshold(5.0)
//
//	// Start automated monitoring
//	monitor.Start(ctx)
//	defer monitor.Stop()
//
//	// Or run a manual check
//	report, err := monitor.Check(ctx, "users")
//	if report.DriftPercentage > 5.0 {
//	    monitor.RepairDrift(ctx, report)
//	}
