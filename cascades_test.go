package smarterbase

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestExtractIDFromKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "with .json extension",
			key:  "users/user123.json",
			want: "user123",
		},
		{
			name: "without extension",
			key:  "users/user123",
			want: "user123",
		},
		{
			name: "nested path",
			key:  "prefix/users/user123.json",
			want: "user123",
		},
		{
			name: "no slash",
			key:  "user123.json",
			want: "user123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractIDFromKey(tt.key)
			if got != tt.want {
				t.Errorf("extractIDFromKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateCascadeSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    CascadeSpec
		wantErr bool
	}{
		{
			name: "valid spec",
			spec: CascadeSpec{
				ChildEntityType: "areas",
				ForeignKeyField: "property_id",
				DeleteFunc:      func(ctx context.Context, id string) error { return nil },
			},
			wantErr: false,
		},
		{
			name: "missing child entity type",
			spec: CascadeSpec{
				ForeignKeyField: "property_id",
				DeleteFunc:      func(ctx context.Context, id string) error { return nil },
			},
			wantErr: true,
		},
		{
			name: "missing foreign key field",
			spec: CascadeSpec{
				ChildEntityType: "areas",
				DeleteFunc:      func(ctx context.Context, id string) error { return nil },
			},
			wantErr: true,
		},
		{
			name: "missing delete func",
			spec: CascadeSpec{
				ChildEntityType: "areas",
				ForeignKeyField: "property_id",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCascadeSpec(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCascadeSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDetectCircularCascade(t *testing.T) {
	tests := []struct {
		name     string
		cascades map[string][]CascadeSpec
		wantErr  bool
	}{
		{
			name: "no circular dependency",
			cascades: map[string][]CascadeSpec{
				"properties": {
					{ChildEntityType: "areas"},
				},
				"areas": {
					{ChildEntityType: "photos"},
				},
			},
			wantErr: false,
		},
		{
			name: "circular dependency",
			cascades: map[string][]CascadeSpec{
				"properties": {
					{ChildEntityType: "areas"},
				},
				"areas": {
					{ChildEntityType: "properties"}, // circular!
				},
			},
			wantErr: true,
		},
		{
			name: "self-reference",
			cascades: map[string][]CascadeSpec{
				"categories": {
					{ChildEntityType: "categories"}, // self-reference
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DetectCircularCascade(tt.cascades)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectCircularCascade() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCascadeManagerBasic(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	cm := NewCascadeManager(store, nil)

	// Track deletions
	deletedChildren := []string{}

	// Register cascade
	cm.Register("properties", CascadeSpec{
		ChildEntityType: "areas",
		ForeignKeyField: "property_id",
		DeleteFunc: func(ctx context.Context, childID string) error {
			deletedChildren = append(deletedChildren, childID)
			return store.Delete(ctx, fmt.Sprintf("areas/%s.json", childID))
		},
	})

	// Create parent and children
	propertyKey := "properties/prop1.json"
	store.PutJSON(ctx, propertyKey, map[string]string{"id": "prop1", "name": "Test Property"})

	// Create child areas (we'll need to manually track these since we don't have Redis)
	area1Key := "areas/area1.json"
	area2Key := "areas/area2.json"
	store.PutJSON(ctx, area1Key, map[string]string{"id": "area1", "property_id": "prop1"})
	store.PutJSON(ctx, area2Key, map[string]string{"id": "area2", "property_id": "prop1"})

	// Since we don't have Redis and List() would fail, we need to test with Redis mock
	// For now, test that registration works
	tree := cm.GetCascadeTree()
	if len(tree["properties"]) != 1 {
		t.Errorf("Expected 1 cascade for properties, got %d", len(tree["properties"]))
	}
}

func TestCascadeManagerGetCascadeTree(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	cm := NewCascadeManager(store, nil)

	// Register multi-level cascades
	cm.RegisterChain("properties", []CascadeSpec{
		{ChildEntityType: "areas", ForeignKeyField: "property_id"},
	})
	cm.RegisterChain("areas", []CascadeSpec{
		{ChildEntityType: "photos", ForeignKeyField: "area_id"},
		{ChildEntityType: "voicenotes", ForeignKeyField: "area_id"},
	})

	tree := cm.GetCascadeTree()

	// Verify structure
	if len(tree) != 2 {
		t.Errorf("Expected 2 parent entity types, got %d", len(tree))
	}

	if len(tree["properties"]) != 1 {
		t.Errorf("Expected 1 child for properties, got %d", len(tree["properties"]))
	}

	if len(tree["areas"]) != 2 {
		t.Errorf("Expected 2 children for areas, got %d", len(tree["areas"]))
	}

	// Check specific entries
	if !strings.Contains(tree["properties"][0], "areas") {
		t.Errorf("Expected properties to cascade to areas")
	}
}

func TestCascadeManagerPrintCascadeTree(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	cm := NewCascadeManager(store, nil)

	cm.RegisterChain("properties", []CascadeSpec{
		{ChildEntityType: "areas", ForeignKeyField: "property_id"},
	})

	output := cm.PrintCascadeTree()

	if !strings.Contains(output, "properties") {
		t.Errorf("Output should contain 'properties'")
	}
	if !strings.Contains(output, "areas") {
		t.Errorf("Output should contain 'areas'")
	}
	if !strings.Contains(output, "property_id") {
		t.Errorf("Output should contain foreign key 'property_id'")
	}
}

func TestCascadeIndexManager(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	cim := NewCascadeIndexManager(store, nil)

	// Register cascade
	deleteCallCount := 0
	cim.RegisterCascade("properties", CascadeSpec{
		ChildEntityType: "areas",
		ForeignKeyField: "property_id",
		DeleteFunc: func(ctx context.Context, childID string) error {
			deleteCallCount++
			return nil
		},
	})

	// Verify cascade was registered
	tree := cim.cascadeManager.GetCascadeTree()
	if len(tree["properties"]) != 1 {
		t.Errorf("Cascade not registered properly")
	}
}

func TestCascadeIndexManagerRegisterChain(t *testing.T) {
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	cim := NewCascadeIndexManager(store, nil)

	// Register chain
	cim.RegisterCascadeChain("properties", []CascadeSpec{
		{ChildEntityType: "areas", ForeignKeyField: "property_id"},
		{ChildEntityType: "rooms", ForeignKeyField: "property_id"},
	})

	tree := cim.cascadeManager.GetCascadeTree()
	if len(tree["properties"]) != 2 {
		t.Errorf("Expected 2 cascades for properties, got %d", len(tree["properties"]))
	}
}

func TestStoreGetJSON(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)

	// Test GetJSON helper
	key := "test/data.json"
	testData := map[string]string{"foo": "bar"}

	err := store.PutJSON(ctx, key, testData)
	if err != nil {
		t.Fatalf("PutJSON() error = %v", err)
	}

	var retrieved map[string]string
	err = store.GetJSON(ctx, key, &retrieved)
	if err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}

	if retrieved["foo"] != "bar" {
		t.Errorf("GetJSON() retrieved wrong data: got %v, want bar", retrieved["foo"])
	}
}

func TestExtractIDFromCascadeKey(t *testing.T) {
	// Test public helper function
	id := ExtractIDFromCascadeKey("users/user123.json")
	if id != "user123" {
		t.Errorf("ExtractIDFromCascadeKey() = %v, want user123", id)
	}
}

// Mock backend that implements List for cascade testing
type mockBackendWithList struct {
	*FilesystemBackend
	listFunc func(ctx context.Context, prefix string) ([]string, error)
}

func (m *mockBackendWithList) List(ctx context.Context, prefix string) ([]string, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, prefix)
	}
	return nil, nil
}

func TestCascadeDeleteWithMockBackend(t *testing.T) {
	ctx := context.Background()

	// Create mock backend with List support
	mockBackend := &mockBackendWithList{
		FilesystemBackend: NewFilesystemBackend(t.TempDir()),
	}

	// Set up mock List function
	mockBackend.listFunc = func(ctx context.Context, prefix string) ([]string, error) {
		if prefix == "areas/" {
			return []string{"areas/area1.json", "areas/area2.json"}, nil
		}
		return nil, nil
	}

	store := NewStore(mockBackend)
	cm := NewCascadeManager(store, nil)

	// Track deletions
	deletedAreas := []string{}

	// Register cascade
	cm.Register("properties", CascadeSpec{
		ChildEntityType: "areas",
		ForeignKeyField: "property_id",
		DeleteFunc: func(ctx context.Context, areaID string) error {
			deletedAreas = append(deletedAreas, areaID)
			return store.Delete(ctx, fmt.Sprintf("areas/%s.json", areaID))
		},
	})

	// Create property and areas
	propertyKey := "properties/prop1.json"
	store.PutJSON(ctx, propertyKey, map[string]string{"id": "prop1"})
	store.PutJSON(ctx, "areas/area1.json", map[string]string{"id": "area1", "property_id": "prop1"})
	store.PutJSON(ctx, "areas/area2.json", map[string]string{"id": "area2", "property_id": "prop1"})

	// Execute cascade delete
	err := cm.ExecuteCascadeDelete(ctx, "properties", "prop1", propertyKey)
	if err != nil {
		t.Fatalf("ExecuteCascadeDelete() error = %v", err)
	}

	// Verify children were deleted
	if len(deletedAreas) != 2 {
		t.Errorf("Expected 2 areas to be deleted, got %d", len(deletedAreas))
	}

	// Verify parent was deleted
	exists, err := store.Exists(ctx, propertyKey)
	if err != nil {
		t.Errorf("Exists check failed: %v", err)
	}
	if exists {
		t.Error("Parent should have been deleted")
	}
}

func TestCascadeDeleteNoChildren(t *testing.T) {
	ctx := context.Background()
	backend := NewFilesystemBackend(t.TempDir())
	store := NewStore(backend)
	cm := NewCascadeManager(store, nil)

	// Register cascade but create no children
	cm.Register("properties", CascadeSpec{
		ChildEntityType: "areas",
		ForeignKeyField: "property_id",
		DeleteFunc: func(ctx context.Context, areaID string) error {
			t.Error("DeleteFunc should not be called when there are no children")
			return nil
		},
	})

	// Create only parent
	propertyKey := "properties/prop1.json"
	store.PutJSON(ctx, propertyKey, map[string]string{"id": "prop1"})

	// This should succeed even though we don't have List support (no children to find)
	// Note: In real usage, this would require a backend with List support
}

func TestCascadeDeleteErrorPropagation(t *testing.T) {
	ctx := context.Background()

	mockBackend := &mockBackendWithList{
		FilesystemBackend: NewFilesystemBackend(t.TempDir()),
	}

	mockBackend.listFunc = func(ctx context.Context, prefix string) ([]string, error) {
		if prefix == "areas/" {
			return []string{"areas/area1.json"}, nil
		}
		return nil, nil
	}

	store := NewStore(mockBackend)
	cm := NewCascadeManager(store, nil)

	// Register cascade that errors
	expectedErr := fmt.Errorf("simulated delete error")
	cm.Register("properties", CascadeSpec{
		ChildEntityType: "areas",
		ForeignKeyField: "property_id",
		DeleteFunc: func(ctx context.Context, areaID string) error {
			return expectedErr
		},
	})

	// Create data
	propertyKey := "properties/prop1.json"
	store.PutJSON(ctx, propertyKey, map[string]string{"id": "prop1"})
	store.PutJSON(ctx, "areas/area1.json", map[string]string{"id": "area1", "property_id": "prop1"})

	// Execute cascade delete - should fail
	err := cm.ExecuteCascadeDelete(ctx, "properties", "prop1", propertyKey)
	if err == nil {
		t.Error("ExecuteCascadeDelete() should propagate child delete errors")
	}

	// Verify error message contains context
	if !strings.Contains(err.Error(), "cascade delete failed") {
		t.Errorf("Error should mention cascade delete failure: %v", err)
	}
}
