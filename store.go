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

// GetJSON fetches and unmarshals a JSON object, applying migrations if needed
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

// PutJSON marshals and stores a JSON object
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

// PutJSONWithETag stores JSON with optimistic locking
func (s *Store) PutJSONWithETag(ctx context.Context, key string, value interface{}, expectedETag string) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to marshal: %w", err)
	}
	return s.backend.PutIfMatch(ctx, key, data, expectedETag)
}

// GetJSONWithETag fetches JSON and returns its ETag, applying migrations if needed
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
