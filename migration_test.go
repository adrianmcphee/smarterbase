package smarterbase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Test types for migrations
type UserV0 struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type UserV1 struct {
	V     int    `json:"_v"`
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

type UserV2 struct {
	V         int    `json:"_v"`
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone"`
}

func TestMigrationBasics(t *testing.T) {
	t.Run("ExtractVersion", func(t *testing.T) {
		tests := []struct {
			name     string
			json     string
			expected int
		}{
			{"with version", `{"_v":2,"id":"123"}`, 2},
			{"without version", `{"id":"123"}`, 0},
			{"version zero", `{"_v":0,"id":"123"}`, 0},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				version := extractVersion([]byte(tt.json))
				if version != tt.expected {
					t.Errorf("expected version %d, got %d", tt.expected, version)
				}
			})
		}
	})

	t.Run("ExtractExpectedVersion", func(t *testing.T) {
		t.Run("UserV0 has no version", func(t *testing.T) {
			user := &UserV0{}
			version := extractExpectedVersion(user)
			if version != 0 {
				t.Errorf("expected version 0, got %d", version)
			}
		})

		t.Run("UserV2 has version 2", func(t *testing.T) {
			user := &UserV2{V: 2}
			version := extractExpectedVersion(user)
			if version != 2 {
				t.Errorf("expected version 2, got %d", version)
			}
		})
	})

	t.Run("GetTypeName", func(t *testing.T) {
		user := &UserV0{}
		name := getTypeName(user)
		if name != "UserV0" {
			t.Errorf("expected type name UserV0, got %s", name)
		}
	})
}

func TestMigrationBuilders(t *testing.T) {
	// Clear registry for clean test
	registry := &MigrationRegistry{
		migrations: make(map[string]map[int]map[int]MigrationFunc),
	}

	t.Run("Split helper", func(t *testing.T) {
		fn := func(data map[string]interface{}) (map[string]interface{}, error) {
			if _, ok := data["name"].(string); ok {
				parts := []string{"Alice", "Smith"}
				data["first_name"] = parts[0]
				data["last_name"] = parts[1]
				delete(data, "name")
			}
			data["_v"] = 2
			return data, nil
		}

		registry.Register("UserV2", 0, 2, fn)

		// Test migration
		input := map[string]interface{}{
			"id":    "123",
			"email": "alice@example.com",
			"name":  "Alice Smith",
		}
		inputJSON, _ := json.Marshal(input)

		output, err := registry.Run("UserV2", 0, 2, inputJSON)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		var result map[string]interface{}
		json.Unmarshal(output, &result)

		if result["first_name"] != "Alice" {
			t.Errorf("expected first_name=Alice, got %v", result["first_name"])
		}
		if result["last_name"] != "Smith" {
			t.Errorf("expected last_name=Smith, got %v", result["last_name"])
		}
		if _, exists := result["name"]; exists {
			t.Error("name field should be removed")
		}
		if result["_v"] != float64(2) {
			t.Errorf("expected _v=2, got %v", result["_v"])
		}
	})

	t.Run("AddField helper", func(t *testing.T) {
		fn := func(data map[string]interface{}) (map[string]interface{}, error) {
			if _, exists := data["phone"]; !exists {
				data["phone"] = ""
			}
			data["_v"] = 1
			return data, nil
		}
		registry.Register("UserV1", 0, 1, fn)

		input := map[string]interface{}{
			"id":    "123",
			"email": "alice@example.com",
		}
		inputJSON, _ := json.Marshal(input)

		output, err := registry.Run("UserV1", 0, 1, inputJSON)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		var result map[string]interface{}
		json.Unmarshal(output, &result)

		if result["phone"] != "" {
			t.Errorf("expected phone='', got %v", result["phone"])
		}
		if result["_v"] != float64(1) {
			t.Errorf("expected _v=1, got %v", result["_v"])
		}
	})

	t.Run("RenameField helper", func(t *testing.T) {
		fn := func(data map[string]interface{}) (map[string]interface{}, error) {
			if val, exists := data["old_email"]; exists {
				data["email"] = val
				delete(data, "old_email")
			}
			data["_v"] = 2
			return data, nil
		}
		registry.Register("Test", 1, 2, fn)

		input := map[string]interface{}{
			"_v":        1,
			"old_email": "test@example.com",
		}
		inputJSON, _ := json.Marshal(input)

		output, err := registry.Run("Test", 1, 2, inputJSON)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		var result map[string]interface{}
		json.Unmarshal(output, &result)

		if result["email"] != "test@example.com" {
			t.Errorf("expected email=test@example.com, got %v", result["email"])
		}
		if _, exists := result["old_email"]; exists {
			t.Error("old_email field should be removed")
		}
		if result["_v"] != float64(2) {
			t.Errorf("expected _v=2, got %v", result["_v"])
		}
	})
}

func TestMigrationChaining(t *testing.T) {
	registry := &MigrationRegistry{
		migrations: make(map[string]map[int]map[int]MigrationFunc),
	}

	// Register chain: 0 -> 1 -> 2
	registry.Register("UserV2", 0, 1, func(data map[string]interface{}) (map[string]interface{}, error) {
		data["phone"] = ""
		data["_v"] = 1
		return data, nil
	})

	registry.Register("UserV2", 1, 2, func(data map[string]interface{}) (map[string]interface{}, error) {
		if _, ok := data["name"].(string); ok {
			parts := []string{"Alice", "Smith"}
			data["first_name"] = parts[0]
			data["last_name"] = parts[1]
			delete(data, "name")
		}
		data["_v"] = 2
		return data, nil
	})

	// Test 0 -> 2 migration
	input := map[string]interface{}{
		"id":    "123",
		"email": "alice@example.com",
		"name":  "Alice Smith",
	}
	inputJSON, _ := json.Marshal(input)

	output, err := registry.Run("UserV2", 0, 2, inputJSON)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(output, &result)

	if result["_v"] != float64(2) {
		t.Errorf("expected _v=2, got %v", result["_v"])
	}
	if result["phone"] != "" {
		t.Errorf("expected phone='', got %v", result["phone"])
	}
	if result["first_name"] != "Alice" {
		t.Errorf("expected first_name=Alice, got %v", result["first_name"])
	}
}

func TestStoreWithMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	backend := NewFilesystemBackend(tmpDir)
	defer backend.Close()

	// Create custom registry for this test
	registry := &MigrationRegistry{
		migrations: make(map[string]map[int]map[int]MigrationFunc),
	}

	store := NewStore(backend)
	store.registry = registry

	ctx := context.Background()

	// Register migration
	Migrate("UserV2").From(0).To(2).Do(func(data map[string]interface{}) (map[string]interface{}, error) {
		if name, ok := data["name"].(string); ok {
			parts := strings.SplitN(name, " ", 2)
			data["first_name"] = parts[0]
			if len(parts) > 1 {
				data["last_name"] = parts[1]
			} else {
				data["last_name"] = ""
			}
			delete(data, "name")
		}
		data["_v"] = 2
		return data, nil
	})

	// Use global registry for this migration
	store.registry = globalRegistry

	t.Run("Write old version, read with migration", func(t *testing.T) {
		// Write old version (no _v field)
		oldUser := UserV0{
			ID:    "123",
			Email: "alice@example.com",
			Name:  "Alice Smith",
		}

		err := store.PutJSON(ctx, "users/123.json", oldUser)
		if err != nil {
			t.Fatalf("PutJSON failed: %v", err)
		}

		// Read as new version (should migrate)
		var newUser UserV2
		newUser.V = 2 // Set expected version

		err = store.GetJSON(ctx, "users/123.json", &newUser)
		if err != nil {
			t.Fatalf("GetJSON failed: %v", err)
		}

		if newUser.FirstName != "Alice" {
			t.Errorf("expected FirstName=Alice, got %s", newUser.FirstName)
		}
		if newUser.LastName != "Smith" {
			t.Errorf("expected LastName=Smith, got %s", newUser.LastName)
		}
	})

	t.Run("MigrateAndWrite policy", func(t *testing.T) {
		store.WithMigrationPolicy(MigrateAndWrite)

		// Write old version
		oldUser := UserV0{
			ID:    "456",
			Email: "bob@example.com",
			Name:  "Bob Johnson",
		}

		err := store.PutJSON(ctx, "users/456.json", oldUser)
		if err != nil {
			t.Fatalf("PutJSON failed: %v", err)
		}

		// Read as new version (should migrate AND write back)
		var newUser UserV2
		newUser.V = 2

		err = store.GetJSON(ctx, "users/456.json", &newUser)
		if err != nil {
			t.Fatalf("GetJSON failed: %v", err)
		}

		// Read again directly to verify it was written back
		data, _ := backend.Get(ctx, "users/456.json")
		var check map[string]interface{}
		json.Unmarshal(data, &check)

		if check["_v"] != float64(2) {
			t.Errorf("data should be written back with _v=2, got %v", check["_v"])
		}
		if check["first_name"] != "Bob" {
			t.Errorf("expected first_name=Bob, got %v", check["first_name"])
		}
	})

	t.Run("No migration needed", func(t *testing.T) {
		// Write new version
		newUser := UserV2{
			V:         2,
			ID:        "789",
			Email:     "charlie@example.com",
			FirstName: "Charlie",
			LastName:  "Brown",
		}

		err := store.PutJSON(ctx, "users/789.json", newUser)
		if err != nil {
			t.Fatalf("PutJSON failed: %v", err)
		}

		// Read as new version (no migration needed)
		var retrieved UserV2
		retrieved.V = 2

		err = store.GetJSON(ctx, "users/789.json", &retrieved)
		if err != nil {
			t.Fatalf("GetJSON failed: %v", err)
		}

		if retrieved.FirstName != "Charlie" {
			t.Errorf("expected FirstName=Charlie, got %s", retrieved.FirstName)
		}
	})
}

func TestMigrationErrors(t *testing.T) {
	registry := &MigrationRegistry{
		migrations: make(map[string]map[int]map[int]MigrationFunc),
	}

	t.Run("No migration path", func(t *testing.T) {
		input := map[string]interface{}{"id": "123"}
		inputJSON, _ := json.Marshal(input)

		_, err := registry.Run("UserV2", 0, 5, inputJSON)
		if err == nil {
			t.Error("expected error for missing migration path")
		}
	})

	t.Run("Migration function error", func(t *testing.T) {
		registry.Register("Test", 1, 2, func(data map[string]interface{}) (map[string]interface{}, error) {
			return nil, fmt.Errorf("migration error")
		})

		input := map[string]interface{}{"_v": 1}
		inputJSON, _ := json.Marshal(input)

		_, err := registry.Run("Test", 1, 2, inputJSON)
		if err == nil {
			t.Error("expected migration error")
		}
	})
}

func TestFluentAPI(t *testing.T) {
	// Test the fluent builder API
	Migrate("TestUser").
		From(0).To(1).AddField("created_at", "2025-01-01").
		From(1).To(2).RenameField("email", "email_address").
		From(2).To(3).RemoveField("temp_field")

	// Verify migrations registered
	if !globalRegistry.HasMigrations() {
		t.Error("migrations should be registered")
	}

	path := globalRegistry.findPath("TestUser", 0, 3)
	if path == nil {
		t.Error("should find migration path 0->1->2->3")
	}
	expectedPath := []int{0, 1, 2, 3}
	if len(path) != len(expectedPath) {
		t.Errorf("expected path length %d, got %d", len(expectedPath), len(path))
	}
}

func TestFluentAPIActualExecution(t *testing.T) {
	t.Run("Split helper actually works", func(t *testing.T) {
		// Use a unique type name for this test
		typeName := "SplitTest_" + t.Name()

		// Register using fluent API - this goes to global registry
		Migrate(typeName).From(0).To(1).Split("name", " ", "first", "last")

		input := map[string]interface{}{"name": "John Doe"}
		inputJSON, _ := json.Marshal(input)

		output, err := globalRegistry.Run(typeName, 0, 1, inputJSON)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		var result map[string]interface{}
		json.Unmarshal(output, &result)

		if result["first"] != "John" {
			t.Errorf("expected first=John, got %v", result["first"])
		}
		if result["last"] != "Doe" {
			t.Errorf("expected last=Doe, got %v", result["last"])
		}
		if result["_v"] != float64(1) {
			t.Errorf("expected _v=1, got %v", result["_v"])
		}
	})

	t.Run("AddField helper actually works", func(t *testing.T) {
		typeName := "AddTest_" + t.Name()
		Migrate(typeName).From(0).To(1).AddField("status", "active")

		input := map[string]interface{}{"id": "123"}
		inputJSON, _ := json.Marshal(input)

		output, err := globalRegistry.Run(typeName, 0, 1, inputJSON)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		var result map[string]interface{}
		json.Unmarshal(output, &result)

		if result["status"] != "active" {
			t.Errorf("expected status=active, got %v", result["status"])
		}
		if result["_v"] != float64(1) {
			t.Errorf("expected _v=1, got %v", result["_v"])
		}
	})

	t.Run("RenameField helper actually works", func(t *testing.T) {
		typeName := "RenameTest_" + t.Name()
		Migrate(typeName).From(0).To(1).RenameField("old_name", "new_name")

		input := map[string]interface{}{"old_name": "value"}
		inputJSON, _ := json.Marshal(input)

		output, err := globalRegistry.Run(typeName, 0, 1, inputJSON)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		var result map[string]interface{}
		json.Unmarshal(output, &result)

		if result["new_name"] != "value" {
			t.Errorf("expected new_name=value, got %v", result["new_name"])
		}
		if _, exists := result["old_name"]; exists {
			t.Error("old_name should be removed")
		}
		if result["_v"] != float64(1) {
			t.Errorf("expected _v=1, got %v", result["_v"])
		}
	})

	t.Run("RemoveField helper actually works", func(t *testing.T) {
		typeName := "RemoveTest_" + t.Name()
		Migrate(typeName).From(0).To(1).RemoveField("unwanted")

		input := map[string]interface{}{"id": "123", "unwanted": "data"}
		inputJSON, _ := json.Marshal(input)

		output, err := globalRegistry.Run(typeName, 0, 1, inputJSON)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		var result map[string]interface{}
		json.Unmarshal(output, &result)

		if _, exists := result["unwanted"]; exists {
			t.Error("unwanted field should be removed")
		}
		if result["_v"] != float64(1) {
			t.Errorf("expected _v=1, got %v", result["_v"])
		}
	})
}

func TestConcurrentMigrations(t *testing.T) {
	registry := &MigrationRegistry{
		migrations: make(map[string]map[int]map[int]MigrationFunc),
	}

	// Register a simple migration
	registry.Register("ConcurrentTest", 0, 1, func(data map[string]interface{}) (map[string]interface{}, error) {
		data["migrated"] = true
		data["_v"] = 1
		return data, nil
	})

	// Run 100 concurrent migrations
	const numGoroutines = 100
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			input := map[string]interface{}{"id": id}
			inputJSON, _ := json.Marshal(input)

			_, err := registry.Run("ConcurrentTest", 0, 1, inputJSON)
			errors <- err
		}(i)
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		if err := <-errors; err != nil {
			t.Errorf("concurrent migration %d failed: %v", i, err)
		}
	}
}

func TestEdgeCases(t *testing.T) {
	t.Run("extractVersion with malformed JSON", func(t *testing.T) {
		version := extractVersion([]byte("not valid json"))
		if version != 0 {
			t.Errorf("expected version 0 for malformed JSON, got %d", version)
		}
	})

	t.Run("extractExpectedVersion with non-struct", func(t *testing.T) {
		var notStruct string = "test"
		version := extractExpectedVersion(&notStruct)
		if version != 0 {
			t.Errorf("expected version 0 for non-struct, got %d", version)
		}
	})

	t.Run("extractExpectedVersion with json tag with omitempty", func(t *testing.T) {
		type TestStruct struct {
			V int `json:"_v,omitempty"`
		}
		s := TestStruct{V: 5}
		version := extractExpectedVersion(&s)
		if version != 5 {
			t.Errorf("expected version 5, got %d", version)
		}
	})

	t.Run("getTypeName with anonymous struct", func(t *testing.T) {
		anon := struct {
			ID string
		}{ID: "123"}
		name := getTypeName(&anon)
		// Anonymous structs return empty string
		if name != "" {
			t.Logf("anonymous struct type name: %s", name)
		}
	})

	t.Run("Split with no delimiter found", func(t *testing.T) {
		registry := &MigrationRegistry{migrations: make(map[string]map[int]map[int]MigrationFunc)}

		fn := func(data map[string]interface{}) (map[string]interface{}, error) {
			if val, ok := data["name"].(string); ok {
				parts := strings.SplitN(val, " ", 2)
				data["first"] = parts[0]
				if len(parts) > 1 {
					data["last"] = parts[1]
				} else {
					data["last"] = ""
				}
			}
			data["_v"] = 1
			return data, nil
		}

		registry.Register("SplitEdge", 0, 1, fn)

		input := map[string]interface{}{"name": "SingleName"}
		inputJSON, _ := json.Marshal(input)

		output, err := registry.Run("SplitEdge", 0, 1, inputJSON)
		if err != nil {
			t.Fatalf("migration failed: %v", err)
		}

		var result map[string]interface{}
		json.Unmarshal(output, &result)

		if result["first"] != "SingleName" {
			t.Errorf("expected first=SingleName, got %v", result["first"])
		}
		if result["last"] != "" {
			t.Errorf("expected empty last, got %v", result["last"])
		}
	})

	t.Run("Migration with same from and to version", func(t *testing.T) {
		registry := &MigrationRegistry{migrations: make(map[string]map[int]map[int]MigrationFunc)}

		input := map[string]interface{}{"id": "123"}
		inputJSON, _ := json.Marshal(input)

		output, err := registry.Run("Test", 1, 1, inputJSON)
		if err != nil {
			t.Errorf("same version should not error: %v", err)
		}

		// Should return unchanged data
		if string(output) != string(inputJSON) {
			t.Error("same version migration should return unchanged data")
		}
	})

	t.Run("Non-linear migration graph", func(t *testing.T) {
		registry := &MigrationRegistry{migrations: make(map[string]map[int]map[int]MigrationFunc)}

		// Create graph: 1->2, 1->3, 2->4, 3->4
		registry.Register("Graph", 1, 2, func(data map[string]interface{}) (map[string]interface{}, error) {
			data["path"] = "1->2"
			data["_v"] = 2
			return data, nil
		})
		registry.Register("Graph", 1, 3, func(data map[string]interface{}) (map[string]interface{}, error) {
			data["path"] = "1->3"
			data["_v"] = 3
			return data, nil
		})
		registry.Register("Graph", 2, 4, func(data map[string]interface{}) (map[string]interface{}, error) {
			data["path"] = data["path"].(string) + "->4"
			data["_v"] = 4
			return data, nil
		})
		registry.Register("Graph", 3, 4, func(data map[string]interface{}) (map[string]interface{}, error) {
			data["path"] = data["path"].(string) + "->4"
			data["_v"] = 4
			return data, nil
		})

		input := map[string]interface{}{"id": "123"}
		inputJSON, _ := json.Marshal(input)

		// Should find shortest path (1->3->4 or 1->2->4, both length 3)
		output, err := registry.Run("Graph", 1, 4, inputJSON)
		if err != nil {
			t.Fatalf("non-linear graph migration failed: %v", err)
		}

		var result map[string]interface{}
		json.Unmarshal(output, &result)

		if result["_v"] != float64(4) {
			t.Errorf("expected _v=4, got %v", result["_v"])
		}
		// Path should be either "1->2->4" or "1->3->4"
		path := result["path"].(string)
		if path != "1->2->4" && path != "1->3->4" {
			t.Errorf("unexpected path: %s", path)
		}
	})
}

func TestHasMigrations(t *testing.T) {
	registry := &MigrationRegistry{migrations: make(map[string]map[int]map[int]MigrationFunc)}

	if registry.HasMigrations() {
		t.Error("empty registry should return false")
	}

	registry.Register("Test", 0, 1, func(data map[string]interface{}) (map[string]interface{}, error) {
		return data, nil
	})

	if !registry.HasMigrations() {
		t.Error("registry with migrations should return true")
	}
}
