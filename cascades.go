package smarterbase

import (
	"context"
	"fmt"
	"strings"
)

// CascadeSpec defines a parent-child relationship for cascade deletes
type CascadeSpec struct {
	ChildEntityType string // e.g., "areas"
	ForeignKeyField string // JSON field name in child that references parent (e.g., "property_id")
	DeleteFunc      func(ctx context.Context, childID string) error
}

// CascadeManager handles declarative cascade delete operations
type CascadeManager struct {
	cascades map[string][]CascadeSpec // parent entity type → child cascade specs
	base     *Store
	indexer  *RedisIndexer
}

// NewCascadeManager creates a new cascade manager
func NewCascadeManager(base *Store, indexer *RedisIndexer) *CascadeManager {
	return &CascadeManager{
		cascades: make(map[string][]CascadeSpec),
		base:     base,
		indexer:  indexer,
	}
}

// Register registers a cascade delete relationship
// Example:
//
//	cm.Register("properties", CascadeSpec{
//	    ChildEntityType: "areas",
//	    ForeignKeyField: "property_id",
//	    DeleteFunc: store.DeleteArea,
//	})
func (cm *CascadeManager) Register(parentEntityType string, spec CascadeSpec) {
	cm.cascades[parentEntityType] = append(cm.cascades[parentEntityType], spec)
}

// RegisterChain registers multiple cascade relationships for an entity
// Example:
//
//	cm.RegisterChain("properties", []CascadeSpec{
//	    {ChildEntityType: "areas", ForeignKeyField: "property_id", DeleteFunc: store.DeleteArea},
//	})
func (cm *CascadeManager) RegisterChain(parentEntityType string, specs []CascadeSpec) {
	for _, spec := range specs {
		cm.Register(parentEntityType, spec)
	}
}

// ExecuteCascadeDelete deletes all children before deleting the parent entity
// Returns error if any child deletion fails (transaction-like behavior)
func (cm *CascadeManager) ExecuteCascadeDelete(
	ctx context.Context,
	parentEntityType string,
	parentID string,
	parentKey string,
) error {
	specs, exists := cm.cascades[parentEntityType]
	if !exists {
		// No cascades registered, just delete parent
		return cm.base.Delete(ctx, parentKey)
	}

	// Delete all children first
	for _, spec := range specs {
		if err := cm.deleteCascadeChildren(ctx, parentID, spec); err != nil {
			return fmt.Errorf("cascade delete failed for %s->%s: %w",
				parentEntityType, spec.ChildEntityType, err)
		}
	}

	// All children deleted successfully, now delete parent
	return cm.base.Delete(ctx, parentKey)
}

// deleteCascadeChildren finds and deletes all children for a given parent
func (cm *CascadeManager) deleteCascadeChildren(
	ctx context.Context,
	parentID string,
	spec CascadeSpec,
) error {
	// Try to find children using Redis index first
	var childKeys []string
	var err error

	if cm.indexer != nil {
		childKeys, err = cm.indexer.Query(ctx, spec.ChildEntityType, spec.ForeignKeyField, parentID)
		if err != nil {
			// Redis query failed, fall back to full scan below
			childKeys = nil
		}
	}

	// If Redis didn't work, fall back to full scan
	if len(childKeys) == 0 {
		prefix := fmt.Sprintf("%s/", spec.ChildEntityType)
		childKeys, err = cm.base.List(ctx, prefix)
		if err != nil {
			return fmt.Errorf("failed to list children: %w", err)
		}

		// Filter by foreign key
		var filtered []string
		for _, key := range childKeys {
			// Read entity and check foreign key
			var data map[string]interface{}
			if err := cm.base.GetJSON(ctx, key, &data); err != nil {
				continue
			}

			if fkValue, ok := data[spec.ForeignKeyField].(string); ok && fkValue == parentID {
				filtered = append(filtered, key)
			}
		}
		childKeys = filtered
	}

	// Delete each child (which may trigger its own cascades)
	for _, childKey := range childKeys {
		// Extract child ID from key (assumes format: entity_type/id.json)
		childID := extractIDFromKey(childKey)

		if err := spec.DeleteFunc(ctx, childID); err != nil {
			return fmt.Errorf("failed to delete child %s: %w", childID, err)
		}
	}

	return nil
}

// extractIDFromKey extracts entity ID from storage key
// Example: "areas/abc123.json" → "abc123"
func extractIDFromKey(key string) string {
	// Remove prefix (everything before last /)
	lastSlash := -1
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			lastSlash = i
			break
		}
	}

	var id string
	if lastSlash == -1 {
		id = key
	} else {
		id = key[lastSlash+1:]
	}

	// Remove .json suffix if present
	if len(id) > 5 && id[len(id)-5:] == ".json" {
		id = id[:len(id)-5]
	}

	return id
}

// CascadeDeleteFunc is a helper type for delete functions
type CascadeDeleteFunc func(ctx context.Context, id string) error

// CascadeIndexManager wraps an IndexManager to add cascade delete support
type CascadeIndexManager struct {
	*IndexManager
	cascadeManager *CascadeManager
}

// NewCascadeIndexManager creates an IndexManager with cascade support
func NewCascadeIndexManager(
	base *Store,
	fileIndexer *Indexer,
	redisIndexer *RedisIndexer,
) *CascadeIndexManager {
	im := NewIndexManager(base).
		WithFileIndexer(fileIndexer).
		WithRedisIndexer(redisIndexer)

	return &CascadeIndexManager{
		IndexManager:   im,
		cascadeManager: NewCascadeManager(base, redisIndexer),
	}
}

// RegisterCascade registers a cascade relationship
func (cim *CascadeIndexManager) RegisterCascade(parentEntityType string, spec CascadeSpec) {
	cim.cascadeManager.Register(parentEntityType, spec)
}

// RegisterCascadeChain registers multiple cascades for an entity
func (cim *CascadeIndexManager) RegisterCascadeChain(parentEntityType string, specs []CascadeSpec) {
	cim.cascadeManager.RegisterChain(parentEntityType, specs)
}

// DeleteWithCascade deletes an entity and all its children
func (cim *CascadeIndexManager) DeleteWithCascade(
	ctx context.Context,
	parentEntityType string,
	key string,
	parentID string,
) error {
	return cim.cascadeManager.ExecuteCascadeDelete(ctx, parentEntityType, parentID, key)
}

// Example usage in a store:
//
//	type PropertyStore struct {
//	    im *CascadeIndexManager
//	}
//
//	func NewPropertyStore(base, indexer, redisIndexer) *PropertyStore {
//	    im := NewCascadeIndexManager(base, indexer, redisIndexer)
//
//	    s := &PropertyStore{im: im}
//
//	    // Register cascade relationships
//	    im.RegisterCascadeChain("properties", []CascadeSpec{
//	        {ChildEntityType: "areas", ForeignKeyField: "property_id", DeleteFunc: s.DeleteArea},
//	    })
//	    im.RegisterCascadeChain("areas", []CascadeSpec{
//	        {ChildEntityType: "photos", ForeignKeyField: "area_id", DeleteFunc: s.DeletePhoto},
//	        {ChildEntityType: "voicenotes", ForeignKeyField: "area_id", DeleteFunc: s.DeleteVoiceNote},
//	    })
//
//	    return s
//	}
//
//	func (s *PropertyStore) DeleteProperty(ctx context.Context, propertyID string) error {
//	    return s.im.DeleteWithCascade(ctx, "properties", s.propertyKey(propertyID), propertyID)
//	}

// ExtractIDFromCascadeKey is a public helper for extracting IDs from keys
// This is useful when implementing custom cascade logic
func ExtractIDFromCascadeKey(key string) string {
	return extractIDFromKey(key)
}

// ValidateCascadeSpec validates that a cascade spec is properly configured
func ValidateCascadeSpec(spec CascadeSpec) error {
	if spec.ChildEntityType == "" {
		return fmt.Errorf("cascade spec missing ChildEntityType")
	}
	if spec.ForeignKeyField == "" {
		return fmt.Errorf("cascade spec missing ForeignKeyField for %s", spec.ChildEntityType)
	}
	if spec.DeleteFunc == nil {
		return fmt.Errorf("cascade spec missing DeleteFunc for %s", spec.ChildEntityType)
	}
	return nil
}

// DetectCircularCascade detects circular cascade dependencies
// This is a helper function to prevent infinite loops in cascade chains
func DetectCircularCascade(cascades map[string][]CascadeSpec) error {
	visited := make(map[string]bool)
	stack := make(map[string]bool)

	var visit func(entityType string) error
	visit = func(entityType string) error {
		if stack[entityType] {
			return fmt.Errorf("circular cascade detected involving %s", entityType)
		}
		if visited[entityType] {
			return nil
		}

		visited[entityType] = true
		stack[entityType] = true

		for _, spec := range cascades[entityType] {
			if err := visit(spec.ChildEntityType); err != nil {
				return err
			}
		}

		stack[entityType] = false
		return nil
	}

	for entityType := range cascades {
		if err := visit(entityType); err != nil {
			return err
		}
	}

	return nil
}

// GetCascadeTree returns a human-readable representation of cascade relationships
// Useful for debugging and documentation
func (cm *CascadeManager) GetCascadeTree() map[string][]string {
	tree := make(map[string][]string)
	for parent, specs := range cm.cascades {
		children := make([]string, len(specs))
		for i, spec := range specs {
			children[i] = fmt.Sprintf("%s (via %s)", spec.ChildEntityType, spec.ForeignKeyField)
		}
		tree[parent] = children
	}
	return tree
}

// PrintCascadeTree prints a human-readable cascade tree
// Useful for debugging
func (cm *CascadeManager) PrintCascadeTree() string {
	var sb strings.Builder
	tree := cm.GetCascadeTree()

	for parent, children := range tree {
		sb.WriteString(fmt.Sprintf("%s:\n", parent))
		for _, child := range children {
			sb.WriteString(fmt.Sprintf("  → %s\n", child))
		}
	}

	return sb.String()
}
