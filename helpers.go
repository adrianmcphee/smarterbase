package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Package-level helper functions for convenience

// Now returns the current time (for consistency across the codebase)
func Now() time.Time {
	return time.Now()
}

// PutJSON is a package-level helper for storing JSON
func PutJSON(backend Backend, ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}
	return backend.Put(ctx, key, data)
}

// GetJSON is a package-level helper for retrieving JSON
func GetJSON(backend Backend, ctx context.Context, key string, dest interface{}) error {
	data, err := backend.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}
