package smarterbase

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestIndexHealthMonitor_Creation tests monitor creation and configuration
func TestIndexHealthMonitor_Creation(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	mr := miniredis.RunT(t)
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	redisIndexer := NewRedisIndexer(redisClient)

	// Create monitor
	monitor := NewIndexHealthMonitor(store, redisIndexer)
	if monitor == nil {
		t.Fatal("expected monitor, got nil")
	}

	// Configure monitor
	monitor.WithInterval(1 * time.Minute)
	monitor.WithSampleSize(50)
	monitor.WithDriftThreshold(10.0)

	// Verify configuration worked (implicitly by not crashing)
}

// TestIndexHealthMonitor_Check tests index health checking
func TestIndexHealthMonitor_Check(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	mr := miniredis.RunT(t)
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	redisIndexer := NewRedisIndexer(redisClient)

	// Register index
	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	monitor := NewIndexHealthMonitor(store, redisIndexer).
		WithSampleSize(10).
		WithDriftThreshold(5.0)

	// Create some test data
	for i := 0; i < 5; i++ {
		user := map[string]interface{}{
			"id":    NewID(),
			"email": "user" + string(rune(i+'0')) + "@example.com",
		}
		userData, _ := json.Marshal(user)
		key := "users/user-" + string(rune(i+'0')) + ".json"
		store.PutJSON(ctx, key, user)
		redisIndexer.UpdateIndexes(ctx, key, userData)
	}

	// Check health
	report, err := monitor.Check(ctx, "users")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	if report == nil {
		t.Fatal("expected report, got nil")
	}

	// Should have no drift initially
	if report.DriftPercentage > 0 {
		t.Errorf("expected no drift, got %.2f%%", report.DriftPercentage)
	}
}

// TestIndexHealthMonitor_DetectDrift tests drift detection
func TestIndexHealthMonitor_DetectDrift(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	mr := miniredis.RunT(t)
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	redisIndexer := NewRedisIndexer(redisClient)

	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	monitor := NewIndexHealthMonitor(store, redisIndexer).
		WithSampleSize(10).
		WithDriftThreshold(5.0)

	// Create test data
	for i := 0; i < 5; i++ {
		user := map[string]interface{}{
			"id":    NewID(),
			"email": "user" + string(rune(i+'0')) + "@example.com",
		}
		userData, _ := json.Marshal(user)
		key := "users/user-" + string(rune(i+'0')) + ".json"
		store.PutJSON(ctx, key, user)
		redisIndexer.UpdateIndexes(ctx, key, userData)
	}

	// Simulate drift by removing one index entry
	mr.Del("idx:users:email:user0@example.com")

	// Check for drift
	report, err := monitor.Check(ctx, "users")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	if report.DriftPercentage == 0 {
		t.Error("expected drift to be detected")
	}

	if report.MissingInRedis == 0 {
		t.Error("expected missing entries in Redis")
	}
}

// TestIndexHealthMonitor_RepairDrift tests drift repair
func TestIndexHealthMonitor_RepairDrift(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	mr := miniredis.RunT(t)
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	redisIndexer := NewRedisIndexer(redisClient)

	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	monitor := NewIndexHealthMonitor(store, redisIndexer).
		WithSampleSize(10)

	// Create test data
	testKeys := make([]string, 3)
	for i := 0; i < 3; i++ {
		user := map[string]interface{}{
			"id":    NewID(),
			"email": "user" + string(rune(i+'0')) + "@example.com",
		}
		userData, _ := json.Marshal(user)
		key := "users/user-" + string(rune(i+'0')) + ".json"
		testKeys[i] = key
		store.PutJSON(ctx, key, user)
		redisIndexer.UpdateIndexes(ctx, key, userData)
	}

	// Create drift
	mr.Del("idx:users:email:user0@example.com")

	// Check drift
	report, err := monitor.Check(ctx, "users")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	if report.DriftPercentage == 0 {
		t.Fatal("expected drift before repair")
	}

	// Repair drift
	err = monitor.RepairDrift(ctx, report)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}

	// Check again - drift should be fixed
	reportAfter, err := monitor.Check(ctx, "users")
	if err != nil {
		t.Fatalf("health check after repair failed: %v", err)
	}

	if reportAfter.DriftPercentage > 0 {
		t.Errorf("expected no drift after repair, got %.2f%%", reportAfter.DriftPercentage)
	}
}

// TestIndexHealthMonitor_StartStop tests background monitoring
func TestIndexHealthMonitor_StartStop(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	mr := miniredis.RunT(t)
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	redisIndexer := NewRedisIndexer(redisClient)

	monitor := NewIndexHealthMonitor(store, redisIndexer).
		WithInterval(100 * time.Millisecond). // Short interval for testing
		WithSampleSize(10)

	// Start monitoring
	err := monitor.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start monitor: %v", err)
	}

	// Let it run briefly
	time.Sleep(250 * time.Millisecond)

	// Stop monitoring
	monitor.Stop()

	// Verify it stopped (by checking it doesn't crash)
	time.Sleep(100 * time.Millisecond)
}

// TestIndexHealthMonitor_MultipleEntityTypes tests checking multiple entity types
func TestIndexHealthMonitor_MultipleEntityTypes(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	mr := miniredis.RunT(t)
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	redisIndexer := NewRedisIndexer(redisClient)

	// Register multiple indexes
	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "orders-by-status",
		EntityType:  "orders",
		ExtractFunc: ExtractJSONField("status"),
	})

	monitor := NewIndexHealthMonitor(store, redisIndexer).
		WithSampleSize(5)

	// Create users
	for i := 0; i < 3; i++ {
		user := map[string]interface{}{
			"id":    NewID(),
			"email": "user" + string(rune(i+'0')) + "@example.com",
		}
		userData, _ := json.Marshal(user)
		key := "users/user-" + string(rune(i+'0')) + ".json"
		store.PutJSON(ctx, key, user)
		redisIndexer.UpdateIndexes(ctx, key, userData)
	}

	// Create orders
	for i := 0; i < 3; i++ {
		order := map[string]interface{}{
			"id":     NewID(),
			"status": "pending",
		}
		orderData, _ := json.Marshal(order)
		key := "orders/order-" + string(rune(i+'0')) + ".json"
		store.PutJSON(ctx, key, order)
		redisIndexer.UpdateIndexes(ctx, key, orderData)
	}

	// Check users health
	userReport, err := monitor.Check(ctx, "users")
	if err != nil {
		t.Fatalf("users health check failed: %v", err)
	}

	if userReport.EntityType != "users" {
		t.Errorf("expected entity type 'users', got '%s'", userReport.EntityType)
	}

	// Check orders health
	orderReport, err := monitor.Check(ctx, "orders")
	if err != nil {
		t.Fatalf("orders health check failed: %v", err)
	}

	if orderReport.EntityType != "orders" {
		t.Errorf("expected entity type 'orders', got '%s'", orderReport.EntityType)
	}

	// Check all entities (empty string)
	allReport, err := monitor.Check(ctx, "")
	if err != nil {
		t.Fatalf("all entities health check failed: %v", err)
	}

	if allReport == nil {
		t.Error("expected report for all entities")
	}
}

// TestIndexHealthMonitor_EmptyData tests monitoring with no data
func TestIndexHealthMonitor_EmptyData(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	defer backend.Close()
	store := NewStore(backend)

	mr := miniredis.RunT(t)
	defer mr.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	redisIndexer := NewRedisIndexer(redisClient)

	redisIndexer.RegisterMultiIndex(&MultiIndexSpec{
		Name:        "users-by-email",
		EntityType:  "users",
		ExtractFunc: ExtractJSONField("email"),
	})

	monitor := NewIndexHealthMonitor(store, redisIndexer)

	// Check health with no data
	report, err := monitor.Check(ctx, "users")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	if report.TotalSampled != 0 {
		t.Errorf("expected 0 sampled objects, got %d", report.TotalSampled)
	}

	if report.DriftPercentage != 0 {
		t.Errorf("expected 0%% drift with no data, got %.2f%%", report.DriftPercentage)
	}
}
