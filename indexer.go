package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
)

// IndexSpec defines how to extract and maintain an index
type IndexSpec struct {
	Name         string                                 // Index name (e.g., "users-by-email")
	KeyFunc      func(data interface{}) (string, error) // Extract index key from object
	ExtractFunc  func(data []byte) (interface{}, error) // Deserialize object
	IndexKey     func(key string) string                // Generate index storage key
	ReverseIndex bool                                   // If true, maintains itemID → parentID mapping
}

// Indexer manages automatic index updates
type Indexer struct {
	store *Store
	specs map[string]*IndexSpec
}

// NewIndexer creates a new indexer
func NewIndexer(store *Store) *Indexer {
	return &Indexer{
		store: store,
		specs: make(map[string]*IndexSpec),
	}
}

// RegisterIndex adds an index specification
func (idx *Indexer) RegisterIndex(spec *IndexSpec) {
	idx.specs[spec.Name] = spec
}

// UpdateIndexes updates all registered indexes for an object
func (idx *Indexer) UpdateIndexes(ctx context.Context, objectKey string, data []byte) error {
	for _, spec := range idx.specs {
		if err := idx.updateIndex(ctx, spec, objectKey, data); err != nil {
			return fmt.Errorf("failed to update index %s: %w", spec.Name, err)
		}
	}
	return nil
}

// updateIndex updates a single index
func (idx *Indexer) updateIndex(ctx context.Context, spec *IndexSpec, objectKey string, data []byte) error {
	// Extract object
	obj, err := spec.ExtractFunc(data)
	if err != nil {
		return err
	}

	// Get index key - if it returns an error, skip this index (it doesn't apply to this object)
	indexKey, err := spec.KeyFunc(obj)
	if err != nil {
		return nil // Skip indexes that don't apply to this object
	}

	// Store index mapping
	storageKey := spec.IndexKey(indexKey)
	return idx.store.PutJSON(ctx, storageKey, map[string]string{
		"object_key": objectKey,
	})
}

// QueryIndex looks up an object by index
func (idx *Indexer) QueryIndex(ctx context.Context, indexName, indexKey string) (string, error) {
	spec, exists := idx.specs[indexName]
	if !exists {
		return "", fmt.Errorf("unknown index: %s", indexName)
	}

	storageKey := spec.IndexKey(indexKey)
	var mapping map[string]string
	if err := idx.store.GetJSON(ctx, storageKey, &mapping); err != nil {
		return "", err
	}

	return mapping["object_key"], nil
}

// Example usage helper - creates common index patterns
func ReverseIndexSpec(name, indexPrefix string, extractID func(interface{}) string) *IndexSpec {
	return &IndexSpec{
		Name: name,
		KeyFunc: func(data interface{}) (string, error) {
			return extractID(data), nil
		},
		ExtractFunc: func(data []byte) (interface{}, error) {
			var obj map[string]interface{}
			err := json.Unmarshal(data, &obj)
			return obj, err
		},
		IndexKey: func(key string) string {
			return fmt.Sprintf("%s%s.json", indexPrefix, key)
		},
		ReverseIndex: true,
	}
}

// SimpleIndexSpec creates a basic forward index (value → key mapping)
func SimpleIndexSpec(name, indexPrefix string, extractValue func(interface{}) string) *IndexSpec {
	return &IndexSpec{
		Name: name,
		KeyFunc: func(data interface{}) (string, error) {
			return extractValue(data), nil
		},
		ExtractFunc: func(data []byte) (interface{}, error) {
			var obj map[string]interface{}
			err := json.Unmarshal(data, &obj)
			return obj, err
		},
		IndexKey: func(key string) string {
			return fmt.Sprintf("%s%s.json", indexPrefix, key)
		},
	}
}
