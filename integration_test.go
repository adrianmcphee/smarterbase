package smarterbase

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestIntegration_EndToEnd validates complete workflows with real components
func TestIntegration_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Setup mini-redis for testing
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer redisClient.Close()

	// Setup filesystem backend
	backend := NewFilesystemBackend(t.TempDir())

	// Setup store with observability
	logger := &StdLogger{}
	metrics := NewInMemoryMetrics()
	store := NewStoreWithObservability(backend, logger, metrics)

	// Setup Redis indexer
	redisIndexer := NewRedisIndexer(redisClient)

	// Register multi-value index for testing
	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:       "test-entities",
		EntityType: "entities",
		ExtractFunc: func(objectKey string, data []byte) ([]IndexEntry, error) {
			var entity map[string]interface{}
			if err := json.Unmarshal(data, &entity); err != nil {
				return nil, err
			}
			category, _ := entity["category"].(string)
			return []IndexEntry{{
				IndexName:  "category",
				IndexValue: category,
			}}, nil
		},
	})

	// Setup index manager
	indexManager := NewIndexManager(store).WithRedisIndexer(redisIndexer)

	t.Run("CompleteWorkflow_CreateUpdateDelete", func(t *testing.T) {
		key := "entities/test-1.json"
		entity := map[string]interface{}{
			"id":       "test-1",
			"name":     "Test Entity",
			"category": "typeA",
			"value":    100,
		}

		// Create
		err := indexManager.Create(ctx, key, entity)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// Verify stored
		var retrieved map[string]interface{}
		err = store.GetJSON(ctx, key, &retrieved)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if retrieved["name"] != entity["name"] {
			t.Errorf("Expected name %v, got %v", entity["name"], retrieved["name"])
		}

		// Verify Redis index updated
		keys, err := redisIndexer.Query(ctx, "entities", "category", "typeA")
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}

		if len(keys) != 1 || keys[0] != key {
			t.Errorf("Expected [%s], got %v", key, keys)
		}

		// Update
		entity["value"] = 200
		entity["category"] = "typeB"
		err = indexManager.Update(ctx, key, entity)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		// Verify update
		err = store.GetJSON(ctx, key, &retrieved)
		if err != nil {
			t.Fatalf("Get after update failed: %v", err)
		}

		if int(retrieved["value"].(float64)) != 200 {
			t.Errorf("Expected value 200, got %v", retrieved["value"])
		}

		// Verify index updated (typeA should be empty, typeB should have the entity)
		keysA, _ := redisIndexer.Query(ctx, "entities", "category", "typeA")
		if len(keysA) != 0 {
			t.Errorf("Expected typeA index to be empty, got %v", keysA)
		}

		keysB, err := redisIndexer.Query(ctx, "entities", "category", "typeB")
		if err != nil {
			t.Fatalf("Query for typeB failed: %v", err)
		}

		if len(keysB) != 1 || keysB[0] != key {
			t.Errorf("Expected [%s] in typeB, got %v", key, keysB)
		}

		// Delete
		err = indexManager.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Verify deleted
		err = store.GetJSON(ctx, key, &retrieved)
		if err == nil {
			t.Error("Expected error when getting deleted entity")
		}

		// Verify index cleaned up
		keysB, _ = redisIndexer.Query(ctx, "entities", "category", "typeB")
		if len(keysB) != 0 {
			t.Errorf("Expected empty index after delete, got %v", keysB)
		}
	})

	t.Run("IndexHealthMonitoring_DriftDetection", func(t *testing.T) {
		// Create entity with index
		key := "entities/test-health.json"
		entity := map[string]interface{}{
			"id":       "test-health",
			"category": "health-test",
		}

		err := indexManager.Create(ctx, key, entity)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// Setup health monitor
		monitor := NewIndexHealthMonitor(store, redisIndexer).
			WithInterval(100 * time.Millisecond).
			WithSampleSize(10).
			WithDriftThreshold(1.0)

		// Run health check - should be clean
		report, err := monitor.Check(ctx, "entities")
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}

		if report.DriftPercentage > 0 {
			t.Errorf("Expected no drift, got %.2f%% (%d missing)", report.DriftPercentage, report.MissingInRedis)
		}

		// Simulate drift by manually removing index entry
		// Get the data first to properly remove from index
		data, _ := store.Backend().Get(ctx, key)
		redisIndexer.RemoveFromIndexes(ctx, key, data)

		// Check again - should detect drift
		report, err = monitor.Check(ctx, "entities")
		if err != nil {
			t.Fatalf("Check after drift failed: %v", err)
		}

		if report.DriftPercentage == 0 {
			t.Error("Expected drift to be detected")
		}

		if report.MissingInRedis != 1 {
			t.Errorf("Expected 1 missing entry, got %d", report.MissingInRedis)
		}

		if len(report.MissingKeys) != 1 || report.MissingKeys[0] != key {
			t.Errorf("Expected missing key %s, got %v", key, report.MissingKeys)
		}

		// Repair drift
		err = monitor.RepairDrift(ctx, report)
		if err != nil {
			t.Fatalf("RepairDrift failed: %v", err)
		}

		// Verify repair worked
		keys, err := redisIndexer.Query(ctx, "entities", "category", "health-test")
		if err != nil {
			t.Fatalf("Query after repair failed: %v", err)
		}

		if len(keys) != 1 || keys[0] != key {
			t.Errorf("Expected [%s] after repair, got %v", key, keys)
		}

		// Final health check should be clean again
		report, err = monitor.Check(ctx, "entities")
		if err != nil {
			t.Fatalf("Final check failed: %v", err)
		}

		if report.DriftPercentage > 0 {
			t.Errorf("Expected no drift after repair, got %.2f%%", report.DriftPercentage)
		}
	})

	t.Run("LoadTestingFramework", func(t *testing.T) {
		config := LoadTestConfig{
			Duration:    500 * time.Millisecond,
			Concurrency: 5,
			OperationMix: OperationMix{
				ReadPercent:   70,
				WritePercent:  25,
				DeletePercent: 5,
			},
			KeyPrefix: "loadtest",
			KeyCount:  20,
		}

		tester := NewLoadTester(store, config)
		results, err := tester.Run(ctx)
		if err != nil {
			t.Fatalf("Load test failed: %v", err)
		}

		if results.TotalOperations == 0 {
			t.Error("Expected some operations to be performed")
		}

		if results.OperationsPerSec == 0 {
			t.Error("Expected non-zero operations per second")
		}

		t.Logf("Load test results: %d ops, %.0f ops/sec, %d successful, %d failed",
			results.TotalOperations,
			results.OperationsPerSec,
			results.SuccessfulOps,
			results.FailedOps)
	})

	t.Run("MetricsTracking", func(t *testing.T) {
		// Perform some operations
		key := "metrics/test.json"
		data := map[string]interface{}{"value": 123}

		store.PutJSON(ctx, key, data)
		store.GetJSON(ctx, key, &data)
		store.Delete(ctx, key)

		// Check metrics were recorded
		if metrics.Counters[MetricPutSuccess] == 0 {
			t.Error("Expected PutSuccess metric to be recorded")
		}

		if metrics.Counters[MetricGetSuccess] == 0 {
			t.Error("Expected GetSuccess metric to be recorded")
		}

		if metrics.Counters[MetricDeleteSuccess] == 0 {
			t.Error("Expected DeleteSuccess metric to be recorded")
		}

		if len(metrics.Timings[MetricPutDuration]) == 0 {
			t.Error("Expected PutDuration timing to be recorded")
		}
	})

	t.Run("QueryPerformance", func(t *testing.T) {
		// Seed test data
		for i := 0; i < 50; i++ {
			key := "query-perf/item-" + string(rune('0'+i)) + ".json"
			entity := map[string]interface{}{
				"id":    i,
				"value": i * 10,
			}
			store.PutJSON(ctx, key, entity)
		}

		// Query with filter
		var results []map[string]interface{}
		err := store.Query("query-perf/").
			FilterJSON(func(obj map[string]interface{}) bool {
				value, ok := obj["value"].(float64)
				return ok && value > 250
			}).
			SortByField("value", false).
			Limit(10).
			All(ctx, &results)

		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}

		if len(results) != 10 {
			t.Errorf("Expected 10 results, got %d", len(results))
		}

		// Verify metrics tracked query
		if metrics.Histograms[MetricQueryResults] == nil {
			t.Error("Expected query results histogram to be tracked")
		}
	})
}

// TestIntegration_S3Backend validates S3 backend with Redis locking
// NOTE: Full S3 integration tests moved to s3_integration_test.go
// This stub remains for backward compatibility
func TestIntegration_S3Backend(t *testing.T) {
	t.Skip("S3 integration tests moved to s3_integration_test.go - run TestIntegration_S3Backend_MinIO instead")
}
