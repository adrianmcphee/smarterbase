package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Store provides high-level operations on top of a Backend
// Completely domain-agnostic - works with any JSON-serializable types
type Store struct {
	backend         Backend
	logger          Logger
	metrics         Metrics
	migrationPolicy MigrationPolicy
	registry        *MigrationRegistry
}

// NewStore creates a new SmarterBase store with no-op logger and metrics
func NewStore(backend Backend) *Store {
	return &Store{
		backend:         backend,
		logger:          &NoOpLogger{},
		metrics:         &NoOpMetrics{},
		migrationPolicy: MigrateOnRead,
		registry:        globalRegistry,
	}
}

// NewStoreWithLogger creates a new store with a custom logger
func NewStoreWithLogger(backend Backend, logger Logger) *Store {
	return &Store{
		backend:         backend,
		logger:          logger,
		metrics:         &NoOpMetrics{},
		migrationPolicy: MigrateOnRead,
		registry:        globalRegistry,
	}
}

// NewStoreWithObservability creates a new store with logging and metrics
func NewStoreWithObservability(backend Backend, logger Logger, metrics Metrics) *Store {
	return &Store{
		backend:         backend,
		logger:          logger,
		metrics:         metrics,
		migrationPolicy: MigrateOnRead,
		registry:        globalRegistry,
	}
}

// SetLogger updates the logger for this store
func (s *Store) SetLogger(logger Logger) {
	s.logger = logger
}

// SetMetrics updates the metrics collector for this store
func (s *Store) SetMetrics(metrics Metrics) {
	s.metrics = metrics
}

// WithMigrationPolicy sets the migration policy for this store
func (s *Store) WithMigrationPolicy(policy MigrationPolicy) *Store {
	s.migrationPolicy = policy
	return s
}

// GetJSON fetches and unmarshals a JSON object from storage, applying migrations if needed.
//
// This is the primary method for reading data from smarterbase. It automatically handles
// schema migrations when the stored data version doesn't match the expected version.
//
// Basic usage:
//
//	var user User
//	err := store.GetJSON(ctx, "users/123.json", &user)
//	if smarterbase.IsNotFound(err) {
//	    // User doesn't exist
//	}
//
// With schema versioning:
//
//	type User struct {
//	    V         int    `json:"_v"`
//	    ID        string `json:"id"`
//	    FirstName string `json:"first_name"`
//	}
//
//	// Register migration
//	smarterbase.Migrate("User").From(0).To(1).
//	    Split("name", " ", "first_name", "last_name")
//
//	// Old data (v0) is automatically migrated to v1
//	var user User
//	user.V = 1  // Expected version
//	store.GetJSON(ctx, "users/old-user.json", &user)
//
// Migration behavior depends on the store's migration policy:
//   - MigrateOnRead (default): Migrates data in memory only
//   - MigrateAndWrite: Migrates data and writes it back to storage
//
// Error handling:
//
//	err := store.GetJSON(ctx, key, &user)
//	if smarterbase.IsNotFound(err) {
//	    // Key doesn't exist in storage
//	} else if err != nil {
//	    // Other error (network, permissions, migration failure, etc.)
//	}
//
// Performance: ~50ns overhead when no migrations are registered. Migration adds 2-5ms per version step.
func (s *Store) GetJSON(ctx context.Context, key string, dest interface{}) error {
	start := time.Now()
	data, err := s.backend.Get(ctx, key)
	s.metrics.Timing(MetricGetDuration, time.Since(start))

	if err != nil {
		s.metrics.Increment(MetricGetError)
		return err
	}

	s.metrics.Increment(MetricGetSuccess)

	// Fast path: no migrations registered
	if !s.registry.HasMigrations() {
		return json.Unmarshal(data, dest)
	}

	// Check versions
	dataVersion := extractVersion(data)
	expectedVersion := extractExpectedVersion(dest)

	// No migration needed
	if dataVersion == expectedVersion {
		return json.Unmarshal(data, dest)
	}

	// Run migrations
	typeName := getTypeName(dest)
	migratedData, err := s.registry.Run(typeName, dataVersion, expectedVersion, data)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// If policy is MigrateAndWrite, write back the migrated data
	if s.migrationPolicy == MigrateAndWrite {
		if putErr := s.backend.Put(ctx, key, migratedData); putErr != nil {
			s.logger.Error("failed to write back migrated data", "key", key, "error", putErr)
		}
	}

	return json.Unmarshal(migratedData, dest)
}

// PutJSON marshals and stores a JSON object to storage.
//
// This is the primary method for writing data to smarterbase. It marshals the value
// to JSON and stores it at the specified key.
//
// Basic usage:
//
//	user := &User{
//	    ID:    smarterbase.NewID(),
//	    Email: "alice@example.com",
//	    Name:  "Alice",
//	}
//	err := store.PutJSON(ctx, "users/"+user.ID, user)
//
// With schema versioning:
//
//	user := &User{
//	    V:         1,  // Current version
//	    ID:        smarterbase.NewID(),
//	    FirstName: "Alice",
//	    LastName:  "Smith",
//	}
//	err := store.PutJSON(ctx, "users/"+user.ID, user)
//
// Important notes:
//   - PutJSON overwrites existing data unconditionally (no ETag check)
//   - For conditional updates, use PutJSONWithETag instead
//   - For race-free updates, use WithAtomicUpdate with distributed locks
//
// Error handling:
//
//	err := store.PutJSON(ctx, key, user)
//	if err != nil {
//	    // Error could be: marshaling failure, network error, permissions, etc.
//	}
func (s *Store) PutJSON(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	start := time.Now()
	err = s.backend.Put(ctx, key, data)
	s.metrics.Timing(MetricPutDuration, time.Since(start))

	if err != nil {
		s.metrics.Increment(MetricPutError)
		return err
	}

	s.metrics.Increment(MetricPutSuccess)
	return nil
}

// PutJSONWithETag stores JSON with optimistic locking using ETag validation.
//
// This method provides optimistic concurrency control. It only writes the data if the
// current ETag matches expectedETag, preventing lost updates from concurrent modifications.
//
// Basic usage pattern (read-modify-write):
//
//	// 1. Read with ETag
//	var user User
//	etag, err := store.GetJSONWithETag(ctx, "users/123", &user)
//
//	// 2. Modify
//	user.Name = "Alice Smith"
//
//	// 3. Write with ETag check
//	newETag, err := store.PutJSONWithETag(ctx, "users/123", &user, etag)
//	if smarterbase.IsConflict(err) {
//	    // Someone else modified the user between read and write
//	    // Retry the operation
//	}
//
// Common pattern with retry:
//
//	config := smarterbase.DefaultRetryConfig()
//	for i := 0; i < config.MaxRetries; i++ {
//	    var user User
//	    etag, err := store.GetJSONWithETag(ctx, key, &user)
//	    if err != nil {
//	        return err
//	    }
//
//	    user.Balance += 100
//
//	    _, err = store.PutJSONWithETag(ctx, key, &user, etag)
//	    if err == nil {
//	        return nil  // Success
//	    }
//	    if !smarterbase.IsConflict(err) {
//	        return err  // Permanent error
//	    }
//	    // ETag conflict - retry
//	}
//
// Important notes:
//   - For critical operations (financial transactions), use WithAtomicUpdate with distributed locks
//   - PutJSONWithETag provides optimistic locking but NOT true isolation
//   - Always use S3BackendWithRedisLock for production multi-writer scenarios
//
// Returns the new ETag on success, or error if write fails or ETag doesn't match.
func (s *Store) PutJSONWithETag(ctx context.Context, key string, value interface{}, expectedETag string) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to marshal: %w", err)
	}
	return s.backend.PutIfMatch(ctx, key, data, expectedETag)
}

// GetJSONWithETag fetches JSON and returns its ETag for optimistic locking, applying migrations if needed.
//
// Use this method when you need to implement optimistic concurrency control. The returned ETag
// can be passed to PutJSONWithETag to ensure the data hasn't changed between read and write.
//
// Basic usage:
//
//	var user User
//	etag, err := store.GetJSONWithETag(ctx, "users/123", &user)
//	if err != nil {
//	    return err
//	}
//
//	// Modify user
//	user.LoginCount++
//
//	// Write with ETag check
//	_, err = store.PutJSONWithETag(ctx, "users/123", &user, etag)
//
// With migrations and MigrateAndWrite policy:
//
//	store.WithMigrationPolicy(smarterbase.MigrateAndWrite)
//	var user User
//	user.V = 2  // Expected version
//	etag, err := store.GetJSONWithETag(ctx, "users/old-user", &user)
//	// Note: If data was migrated and written back, the returned ETag is now stale
//	// You should refetch if you need the current ETag
//
// ETag behavior with migrations:
//   - If no migration needed: Returns current ETag
//   - If migration happens in-memory only: Returns ETag of original data
//   - If MigrateAndWrite policy: ETag becomes stale after write-back (refetch recommended)
//
// Returns the ETag string and unmarshaled data in dest, or error if read/migration fails.
func (s *Store) GetJSONWithETag(ctx context.Context, key string, dest interface{}) (string, error) {
	data, etag, err := s.backend.GetWithETag(ctx, key)
	if err != nil {
		return "", err
	}

	// Fast path: no migrations registered
	if !s.registry.HasMigrations() {
		if err := json.Unmarshal(data, dest); err != nil {
			return "", err
		}
		return etag, nil
	}

	// Check versions
	dataVersion := extractVersion(data)
	expectedVersion := extractExpectedVersion(dest)

	// No migration needed
	if dataVersion == expectedVersion {
		if err := json.Unmarshal(data, dest); err != nil {
			return "", err
		}
		return etag, nil
	}

	// Run migrations
	typeName := getTypeName(dest)
	migratedData, err := s.registry.Run(typeName, dataVersion, expectedVersion, data)
	if err != nil {
		return "", fmt.Errorf("migration failed: %w", err)
	}

	// If policy is MigrateAndWrite, write back the migrated data
	if s.migrationPolicy == MigrateAndWrite {
		if putErr := s.backend.Put(ctx, key, migratedData); putErr != nil {
			s.logger.Error("failed to write back migrated data", "key", key, "error", putErr)
		}
		// Note: After writing back, ETag is now stale. Caller should refetch if needed.
	}

	if err := json.Unmarshal(migratedData, dest); err != nil {
		return "", err
	}

	// Note: ETag is from original data - if MigrateAndWrite was used, ETag is now stale
	return etag, nil
}

// Delete removes an object
func (s *Store) Delete(ctx context.Context, key string) error {
	start := time.Now()
	err := s.backend.Delete(ctx, key)
	s.metrics.Timing(MetricDeleteDuration, time.Since(start))

	if err != nil {
		s.metrics.Increment(MetricDeleteError)
		return err
	}

	s.metrics.Increment(MetricDeleteSuccess)
	return nil
}

// Exists checks if a key exists
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	return s.backend.Exists(ctx, key)
}

// List returns all keys with the given prefix
func (s *Store) List(ctx context.Context, prefix string) ([]string, error) {
	return s.backend.List(ctx, prefix)
}

// ListPaginated processes keys in batches
func (s *Store) ListPaginated(ctx context.Context, prefix string, handler func(keys []string) error) error {
	return s.backend.ListPaginated(ctx, prefix, handler)
}

// Index represents a reverse index mapping
type Index struct {
	Key     string                 // Index storage key
	Entries map[string]string      // itemID -> parentID
	Meta    map[string]interface{} // Optional metadata
}

// GetIndex fetches an index
func (s *Store) GetIndex(ctx context.Context, key string) (*Index, error) {
	idx := &Index{Key: key, Entries: make(map[string]string)}
	err := s.GetJSON(ctx, key, &idx.Entries)
	if err != nil {
		return nil, err
	}
	return idx, nil
}

// PutIndex stores an index
func (s *Store) PutIndex(ctx context.Context, idx *Index) error {
	return s.PutJSON(ctx, idx.Key, idx.Entries)
}

// MarshalObject marshals an object to JSON (utility function)
// Renamed from MarshalJSON to avoid conflict with json.Marshaler interface
func (s *Store) MarshalObject(value interface{}) ([]byte, error) {
	return json.Marshal(value)
}

// Backend returns the underlying backend (for advanced use cases like index repair)
func (s *Store) Backend() Backend {
	return s.backend
}

// UpdateIndex atomically updates an index entry with retry and exponential backoff
func (s *Store) UpdateIndex(ctx context.Context, key string, itemID, parentID string) error {
	config := DefaultRetryConfig()

	for i := 0; i < config.MaxRetries; i++ {
		// Get current index with ETag
		var entries map[string]string
		etag, err := s.GetJSONWithETag(ctx, key, &entries)
		if err != nil {
			// Index doesn't exist, create it
			entries = make(map[string]string)
			etag = ""
		}

		// Update entry
		entries[itemID] = parentID

		// Try to save with ETag
		_, err = s.PutJSONWithETag(ctx, key, entries, etag)
		if err == nil {
			return nil
		}

		// ETag mismatch - retry with backoff and jitter
		if i < config.MaxRetries-1 { // Don't sleep on last iteration
			backoff := config.InitialBackoff * time.Duration(1<<uint(i))
			jitter := time.Duration(float64(backoff) * config.JitterPercent * (1.0 - (float64(i%2) * 0.5)))
			time.Sleep(backoff + jitter)
		}
	}

	err := WithContext(ErrIndexRetries, map[string]interface{}{
		"key":     key,
		"retries": config.MaxRetries,
	})
	s.logger.Error("index update failed after retries",
		"key", key,
		"retries", config.MaxRetries,
		"error", err,
	)
	s.metrics.Increment(MetricIndexErrors)
	s.metrics.Gauge(MetricIndexRetries, float64(config.MaxRetries))
	return err
}

// RemoveFromIndex atomically removes an entry from an index
func (s *Store) RemoveFromIndex(ctx context.Context, key string, itemID string) error {
	config := DefaultRetryConfig()

	for i := 0; i < config.MaxRetries; i++ {
		var entries map[string]string
		etag, err := s.GetJSONWithETag(ctx, key, &entries)
		if err != nil {
			return err // Index doesn't exist
		}

		delete(entries, itemID)

		_, err = s.PutJSONWithETag(ctx, key, entries, etag)
		if err == nil {
			return nil
		}

		if i < config.MaxRetries-1 {
			backoff := config.InitialBackoff * time.Duration(1<<uint(i))
			time.Sleep(backoff)
		}
	}

	err := WithContext(ErrIndexRetries, map[string]interface{}{
		"key":     key,
		"retries": config.MaxRetries,
	})
	s.logger.Error("index removal failed after retries",
		"key", key,
		"retries", config.MaxRetries,
		"error", err,
	)
	return err
}

// Ping checks backend health
func (s *Store) Ping(ctx context.Context) error {
	return s.backend.Ping(ctx)
}

// Close releases resources held by the store and backend
func (s *Store) Close() error {
	return s.backend.Close()
}
