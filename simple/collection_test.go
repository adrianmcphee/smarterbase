package simple

import (
	"context"
	"testing"

	"github.com/adrianmcphee/smarterbase"
)

type testItem struct {
	ID   string `json:"id" sb:"id"`
	Name string `json:"name"`
	Tag  string `json:"tag" sb:"index"`
}

func setupTestDB(t *testing.T) *DB {
	t.Helper()

	// Use filesystem backend with temp dir
	backend := smarterbase.NewFilesystemBackend(t.TempDir())
	db, err := Connect(WithBackend(backend))
	if err != nil {
		t.Fatal(err)
	}

	return db
}

// Collection Tests

func TestNewCollection_CreatesCollection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)
	if collection == nil {
		t.Fatal("NewCollection returned nil")
	}

	// The pluralize function preserves case from type name
	if collection.name != "testItems" {
		t.Errorf("Expected collection name 'testItems', got '%s'", collection.name)
	}
}

func TestNewCollection_CustomName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db, "custom_items")
	if collection.name != "custom_items" {
		t.Errorf("Expected collection name 'custom_items', got '%s'", collection.name)
	}
}

func TestCollection_Create(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	item := &testItem{
		Name: "Test Item",
		Tag:  "test",
	}

	created, err := collection.Create(ctx, item)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Check that ID was generated
	if created.ID == "" {
		t.Error("Created item has empty ID")
	}

	// Check that original was not mutated
	if item.ID != "" {
		t.Error("Original item was mutated (ID should be empty)")
	}

	// Check that data was preserved
	if created.Name != "Test Item" {
		t.Errorf("Expected name 'Test Item', got '%s'", created.Name)
	}
}

func TestCollection_CreateWithID(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	item := &testItem{
		ID:   "custom-id",
		Name: "Test Item",
	}

	created, err := collection.Create(ctx, item)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if created.ID != "custom-id" {
		t.Errorf("Expected ID 'custom-id', got '%s'", created.ID)
	}
}

func TestCollection_CreateNil(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	_, err := collection.Create(ctx, nil)
	if err == nil {
		t.Error("Expected error when creating nil item")
	}
}

func TestCollection_Get(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	// Create an item first
	created, err := collection.Create(ctx, &testItem{Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	// Get it back
	found, err := collection.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("Expected ID %s, got %s", created.ID, found.ID)
	}

	if found.Name != "Test" {
		t.Errorf("Expected name 'Test', got '%s'", found.Name)
	}
}

func TestCollection_GetNotFound(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	_, err := collection.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent item")
	}
}

func TestCollection_GetEmptyID(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	_, err := collection.Get(ctx, "")
	if err == nil {
		t.Error("Expected error when getting with empty ID")
	}
}

func TestCollection_Update(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	// Create an item
	created, err := collection.Create(ctx, &testItem{Name: "Original"})
	if err != nil {
		t.Fatal(err)
	}

	// Update it
	created.Name = "Updated"
	err = collection.Update(ctx, created)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	found, err := collection.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}

	if found.Name != "Updated" {
		t.Errorf("Expected name 'Updated', got '%s'", found.Name)
	}
}

func TestCollection_UpdateNil(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	err := collection.Update(ctx, nil)
	if err == nil {
		t.Error("Expected error when updating nil item")
	}
}

func TestCollection_UpdateNoID(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	err := collection.Update(ctx, &testItem{Name: "Test"})
	if err == nil {
		t.Error("Expected error when updating item without ID")
	}
}

func TestCollection_Delete(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	// Create an item
	created, err := collection.Create(ctx, &testItem{Name: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	// Delete it
	err = collection.Delete(ctx, created.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deletion
	_, err = collection.Get(ctx, created.ID)
	if err == nil {
		t.Error("Expected error when getting deleted item")
	}
}

func TestCollection_DeleteEmptyID(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	err := collection.Delete(ctx, "")
	if err == nil {
		t.Error("Expected error when deleting with empty ID")
	}
}

func TestCollection_Count(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	// Initially empty
	count, err := collection.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("Expected count 0, got %d", count)
	}

	// Create some items
	for i := 0; i < 3; i++ {
		_, err := collection.Create(ctx, &testItem{Name: "Test"})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Count should be 3
	count, err = collection.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}

func TestCollection_All(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	// Create some items
	for i := 0; i < 3; i++ {
		_, err := collection.Create(ctx, &testItem{Name: "Test"})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Get all
	items, err := collection.All(ctx)
	if err != nil {
		t.Fatalf("All failed: %v", err)
	}

	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}
}

func TestCollection_Each(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	// Create some items
	for i := 0; i < 3; i++ {
		_, err := collection.Create(ctx, &testItem{Name: "Test"})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Iterate
	count := 0
	err := collection.Each(ctx, func(item *testItem) error {
		count++
		if item.Name != "Test" {
			t.Errorf("Expected name 'Test', got '%s'", item.Name)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Each failed: %v", err)
	}

	if count != 3 {
		t.Errorf("Expected 3 iterations, got %d", count)
	}
}

func TestCollection_Find_NoRedis(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	// Verify Redis is actually not available
	if db.redisIndexer != nil {
		t.Skip("Redis is available in test environment, skipping NoRedis test")
	}

	collection := NewCollection[testItem](db)

	// Without Redis, Find should return error
	_, err := collection.Find(ctx, "tag", "test")
	if err == nil {
		t.Error("Expected error when using Find without Redis")
	}
}

func TestCollection_FindOne_NoRedis(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()

	collection := NewCollection[testItem](db)

	// Without Redis, FindOne should return error
	_, err := collection.FindOne(ctx, "tag", "test")
	if err == nil {
		t.Error("Expected error when using FindOne without Redis")
	}
}

// Helper function tests

func TestPluralize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"User", "Users"},     // Regular plural preserves case
		{"Person", "people"},  // Irregular plural returns lowercase
		{"Child", "children"}, // Irregular plural returns lowercase
		{"City", "Cities"},    // -y ending preserves case
		{"Box", "Boxes"},      // -x ending preserves case
		{"Class", "Classes"},  // -s ending preserves case
		{"Wish", "Wishes"},    // -sh ending preserves case
		{"Goose", "geese"},    // Irregular plural returns lowercase
		{"user", "users"},     // Lowercase input
		{"person", "people"},  // Lowercase irregular
	}

	for _, tt := range tests {
		result := pluralize(tt.input)
		if result != tt.expected {
			t.Errorf("pluralize(%s) = %s, want %s", tt.input, result, tt.expected)
		}
	}
}

func TestGetTypeName(t *testing.T) {
	type MyType struct{}

	var v MyType
	name := getTypeName(v)
	if name != "MyType" {
		t.Errorf("Expected 'MyType', got '%s'", name)
	}

	var p *MyType
	name = getTypeName(p)
	if name != "MyType" {
		t.Errorf("Expected 'MyType' for pointer, got '%s'", name)
	}
}

func TestIsVowel(t *testing.T) {
	vowels := []rune{'a', 'e', 'i', 'o', 'u'}
	for _, v := range vowels {
		if !isVowel(v) {
			t.Errorf("Expected %c to be a vowel", v)
		}
	}

	consonants := []rune{'b', 'c', 'd', 'f', 'g'}
	for _, c := range consonants {
		if isVowel(c) {
			t.Errorf("Expected %c to not be a vowel", c)
		}
	}
}

func TestContains(t *testing.T) {
	slice := []string{"foo", "bar", "baz"}

	if !contains(slice, "bar") {
		t.Error("Expected slice to contain 'bar'")
	}

	if contains(slice, "qux") {
		t.Error("Expected slice to not contain 'qux'")
	}
}
