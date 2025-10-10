package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Query provides a fluent interface for querying objects in Smarterbase
type Query struct {
	store      *Store
	prefix     string
	filterFunc func([]byte) bool
	limit      int
	offset     int
	sortFunc   func(a, b []byte) bool
}

// Query creates a new query for objects with the given prefix
func (s *Store) Query(prefix string) *Query {
	return &Query{
		store:  s,
		prefix: prefix,
		limit:  -1, // No limit by default
	}
}

// Filter adds a filter function to the query
// The filter receives raw JSON bytes and should return true if the object matches
func (q *Query) Filter(fn func(data []byte) bool) *Query {
	q.filterFunc = fn
	return q
}

// FilterJSON adds a filter function that works with unmarshaled objects
// This is a convenience wrapper around Filter
func (q *Query) FilterJSON(fn func(obj map[string]interface{}) bool) *Query {
	q.filterFunc = func(data []byte) bool {
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			return false
		}
		return fn(obj)
	}
	return q
}

// Limit sets the maximum number of results to return
func (q *Query) Limit(n int) *Query {
	q.limit = n
	return q
}

// Offset sets the number of results to skip
func (q *Query) Offset(n int) *Query {
	q.offset = n
	return q
}

// Sort adds a sorting function to the query
// The sort function should return true if a should come before b
func (q *Query) Sort(fn func(a, b []byte) bool) *Query {
	q.sortFunc = fn
	return q
}

// SortByField sorts by a JSON field (ascending)
func (q *Query) SortByField(fieldName string, ascending bool) *Query {
	q.sortFunc = func(a, b []byte) bool {
		var objA, objB map[string]interface{}
		if err := json.Unmarshal(a, &objA); err != nil {
			return false
		}
		if err := json.Unmarshal(b, &objB); err != nil {
			return false
		}

		valA, okA := objA[fieldName]
		valB, okB := objB[fieldName]
		if !okA || !okB {
			return false
		}

		// Handle different types
		switch va := valA.(type) {
		case string:
			vb, ok := valB.(string)
			if !ok {
				return false
			}
			if ascending {
				return va < vb
			}
			return va > vb
		case float64:
			vb, ok := valB.(float64)
			if !ok {
				return false
			}
			if ascending {
				return va < vb
			}
			return va > vb
		default:
			return false
		}
	}
	return q
}

// All executes the query and unmarshals all matching objects into dest
// dest should be a pointer to a slice of the appropriate type
func (q *Query) All(ctx context.Context, dest interface{}) error {
	start := time.Now()

	keys, err := q.store.backend.List(ctx, q.prefix)
	if err != nil {
		return err
	}

	var results [][]byte
	processed := 0 // Track how many we've examined (for offset)

	for _, key := range keys {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		data, err := q.store.backend.Get(ctx, key)
		if err != nil {
			continue // Skip objects that can't be read
		}

		// Apply filter if set
		if q.filterFunc != nil && !q.filterFunc(data) {
			continue
		}

		// Apply offset
		if processed < q.offset {
			processed++
			continue
		}

		results = append(results, data)

		// Early exit if we have enough results (only when not sorting)
		// Note: If sorting is enabled, we need all results
		if q.sortFunc == nil && q.limit > 0 && len(results) >= q.limit {
			break
		}
	}

	// Apply sorting if set (we already filtered during collection)
	if q.sortFunc != nil {
		sort.Slice(results, func(i, j int) bool {
			return q.sortFunc(results[i], results[j])
		})

		// Apply limit after sorting
		if q.limit > 0 && len(results) > q.limit {
			results = results[:q.limit]
		}
	}

	// Record query metrics
	duration := time.Since(start)
	q.store.metrics.Timing(MetricQueryDuration, duration, "prefix", q.prefix)
	q.store.metrics.Histogram(MetricQueryResults, float64(len(results)), "prefix", q.prefix)
	q.store.logger.Debug("query executed",
		"prefix", q.prefix,
		"results", len(results),
		"duration_ms", duration.Milliseconds(),
	)

	// Unmarshal all results into dest
	return q.unmarshalResults(results, dest)
}

// First executes the query and returns the first matching object
func (q *Query) First(ctx context.Context, dest interface{}) error {
	q.limit = 1
	keys, err := q.store.backend.List(ctx, q.prefix)
	if err != nil {
		return err
	}

	for _, key := range keys {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		data, err := q.store.backend.Get(ctx, key)
		if err != nil {
			continue
		}

		if q.filterFunc != nil && !q.filterFunc(data) {
			continue
		}

		return json.Unmarshal(data, dest)
	}

	return fmt.Errorf("no matching object found")
}

// Count returns the number of matching objects
func (q *Query) Count(ctx context.Context) (int, error) {
	keys, err := q.store.backend.List(ctx, q.prefix)
	if err != nil {
		return 0, err
	}

	if q.filterFunc == nil {
		return len(keys), nil
	}

	count := 0
	for _, key := range keys {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		data, err := q.store.backend.Get(ctx, key)
		if err != nil {
			continue
		}

		if q.filterFunc(data) {
			count++
		}
	}

	return count, nil
}

// Each executes a function for each matching object
func (q *Query) Each(ctx context.Context, fn func(key string, data []byte) error) error {
	keys, err := q.store.backend.List(ctx, q.prefix)
	if err != nil {
		return err
	}

	processed := 0
	for _, key := range keys {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		data, err := q.store.backend.Get(ctx, key)
		if err != nil {
			continue
		}

		if q.filterFunc != nil && !q.filterFunc(data) {
			continue
		}

		// Apply offset
		if q.offset > 0 {
			q.offset--
			continue
		}

		if err := fn(key, data); err != nil {
			return err
		}

		processed++
		if q.limit > 0 && processed >= q.limit {
			break
		}
	}

	return nil
}

// unmarshalResults unmarshals a slice of byte slices into the destination
func (q *Query) unmarshalResults(results [][]byte, dest interface{}) error {
	// Create a temporary JSON array
	var jsonArray []json.RawMessage
	for _, data := range results {
		jsonArray = append(jsonArray, data)
	}

	arrayJSON, err := json.Marshal(jsonArray)
	if err != nil {
		return err
	}

	return json.Unmarshal(arrayJSON, dest)
}

// QueryBuilder provides common query patterns
type QueryBuilder struct {
	store *Store
}

// NewQueryBuilder creates a new query builder
func NewQueryBuilder(store *Store) *QueryBuilder {
	return &QueryBuilder{store: store}
}

// CreatedAfter finds all objects with a created_at field after the given time
func (qb *QueryBuilder) CreatedAfter(prefix string, after time.Time) *Query {
	return qb.store.Query(prefix).FilterJSON(func(obj map[string]interface{}) bool {
		createdAtStr, ok := obj["created_at"].(string)
		if !ok {
			return false
		}
		createdAt, err := time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return false
		}
		return createdAt.After(after)
	})
}

// FieldEquals finds all objects where a field equals a value
func (qb *QueryBuilder) FieldEquals(prefix, fieldName string, value interface{}) *Query {
	return qb.store.Query(prefix).FilterJSON(func(obj map[string]interface{}) bool {
		fieldVal, ok := obj[fieldName]
		if !ok {
			return false
		}
		return fieldVal == value
	})
}

// FieldContains finds all objects where a string field contains a substring
func (qb *QueryBuilder) FieldContains(prefix, fieldName, substring string) *Query {
	return qb.store.Query(prefix).FilterJSON(func(obj map[string]interface{}) bool {
		fieldVal, ok := obj[fieldName].(string)
		if !ok {
			return false
		}
		return strings.Contains(fieldVal, substring)
	})
}
