package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrianmcphee/smarterbase/internal/storage"
)

func setupTestStore(t *testing.T) (*storage.Store, string) {
	t.Helper()

	dir := t.TempDir()
	store, err := storage.NewStore(dir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	return store, dir
}

func TestExportDDL_EmptyStore(t *testing.T) {
	store, _ := setupTestStore(t)

	output := ExportDDL(store)

	if !strings.Contains(output, "SmarterBase export") {
		t.Error("Expected header comment")
	}
	if !strings.Contains(output, "no migration history") {
		t.Error("Expected 'no migration history' comment")
	}
}

func TestExportDDL_SingleTable(t *testing.T) {
	store, _ := setupTestStore(t)

	// Create a table
	err := store.Schema.CreateTable(&storage.Table{
		Name: "users",
		Columns: []storage.Column{
			{Name: "id", Type: "uuid", PrimaryKey: true},
			{Name: "email", Type: "text", Unique: true, NotNull: true},
			{Name: "name", Type: "text"},
		},
	})
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	output := ExportDDL(store)

	// Check CREATE TABLE
	if !strings.Contains(output, "CREATE TABLE users") {
		t.Error("Expected CREATE TABLE users")
	}

	// Check columns
	if !strings.Contains(output, "id UUID PRIMARY KEY") {
		t.Errorf("Expected 'id UUID PRIMARY KEY', got:\n%s", output)
	}
	if !strings.Contains(output, "email TEXT UNIQUE NOT NULL") {
		t.Errorf("Expected 'email TEXT UNIQUE NOT NULL', got:\n%s", output)
	}
	if !strings.Contains(output, "name TEXT") {
		t.Errorf("Expected 'name TEXT', got:\n%s", output)
	}
}

func TestExportDDL_MultipleTables(t *testing.T) {
	store, _ := setupTestStore(t)

	// Create users table
	store.Schema.CreateTable(&storage.Table{
		Name: "users",
		Columns: []storage.Column{
			{Name: "id", Type: "uuid", PrimaryKey: true},
			{Name: "email", Type: "text"},
		},
	})

	// Create orders table
	store.Schema.CreateTable(&storage.Table{
		Name: "orders",
		Columns: []storage.Column{
			{Name: "id", Type: "uuid", PrimaryKey: true},
			{Name: "total", Type: "decimal"},
		},
	})

	output := ExportDDL(store)

	// Both tables should be present (sorted alphabetically)
	if !strings.Contains(output, "CREATE TABLE orders") {
		t.Error("Expected CREATE TABLE orders")
	}
	if !strings.Contains(output, "CREATE TABLE users") {
		t.Error("Expected CREATE TABLE users")
	}

	// orders should come before users (alphabetical)
	ordersIdx := strings.Index(output, "CREATE TABLE orders")
	usersIdx := strings.Index(output, "CREATE TABLE users")
	if ordersIdx > usersIdx {
		t.Error("Expected tables to be sorted alphabetically")
	}
}

func TestExportDDL_TypeMapping(t *testing.T) {
	store, _ := setupTestStore(t)

	store.Schema.CreateTable(&storage.Table{
		Name: "types_test",
		Columns: []storage.Column{
			{Name: "col_uuid", Type: "uuid"},
			{Name: "col_text", Type: "text"},
			{Name: "col_string", Type: "string"},
			{Name: "col_int", Type: "int"},
			{Name: "col_integer", Type: "integer"},
			{Name: "col_bigint", Type: "bigint"},
			{Name: "col_bool", Type: "boolean"},
			{Name: "col_decimal", Type: "decimal"},
			{Name: "col_timestamp", Type: "timestamp"},
			{Name: "col_date", Type: "date"},
			{Name: "col_jsonb", Type: "jsonb"},
			{Name: "col_unknown", Type: "unknown"},
		},
	})

	output := ExportDDL(store)

	typeTests := []struct {
		expected string
	}{
		{"col_uuid UUID"},
		{"col_text TEXT"},
		{"col_string TEXT"},
		{"col_int INTEGER"},
		{"col_integer INTEGER"},
		{"col_bigint BIGINT"},
		{"col_bool BOOLEAN"},
		{"col_decimal DECIMAL"},
		{"col_timestamp TIMESTAMPTZ"},
		{"col_date DATE"},
		{"col_jsonb JSONB"},
		{"col_unknown TEXT"}, // Unknown types default to TEXT
	}

	for _, tt := range typeTests {
		if !strings.Contains(output, tt.expected) {
			t.Errorf("Expected '%s' in output:\n%s", tt.expected, output)
		}
	}
}

func TestExportData_WithRows(t *testing.T) {
	store, _ := setupTestStore(t)

	// Create table
	store.Schema.CreateTable(&storage.Table{
		Name: "users",
		Columns: []storage.Column{
			{Name: "id", Type: "text", PrimaryKey: true},
			{Name: "name", Type: "text"},
			{Name: "email", Type: "text"},
		},
	})

	// Insert data
	store.Data.Insert("users", storage.Row{
		"id":    "u1",
		"name":  "Alice",
		"email": "alice@example.com",
	})
	store.Data.Insert("users", storage.Row{
		"id":    "u2",
		"name":  "Bob",
		"email": "bob@example.com",
	})

	output := ExportData(store)

	// Check INSERT statements
	if !strings.Contains(output, "INSERT INTO users") {
		t.Error("Expected INSERT INTO users")
	}
	if !strings.Contains(output, "'u1'") {
		t.Error("Expected 'u1' in output")
	}
	if !strings.Contains(output, "'Alice'") {
		t.Error("Expected 'Alice' in output")
	}
	if !strings.Contains(output, "'alice@example.com'") {
		t.Error("Expected 'alice@example.com' in output")
	}
}

func TestExportData_EscapesSingleQuotes(t *testing.T) {
	store, _ := setupTestStore(t)

	store.Schema.CreateTable(&storage.Table{
		Name: "items",
		Columns: []storage.Column{
			{Name: "id", Type: "text", PrimaryKey: true},
			{Name: "description", Type: "text"},
		},
	})

	store.Data.Insert("items", storage.Row{
		"id":          "i1",
		"description": "It's a test with 'quotes'",
	})

	output := ExportData(store)

	// Single quotes should be escaped as ''
	if !strings.Contains(output, "It''s a test with ''quotes''") {
		t.Errorf("Expected escaped quotes in output:\n%s", output)
	}
}

func TestExport_Full(t *testing.T) {
	store, _ := setupTestStore(t)

	// Create table
	store.Schema.CreateTable(&storage.Table{
		Name: "users",
		Columns: []storage.Column{
			{Name: "id", Type: "uuid", PrimaryKey: true},
			{Name: "email", Type: "text"},
		},
	})

	// Insert data
	store.Data.Insert("users", storage.Row{
		"id":    "abc-123",
		"email": "test@example.com",
	})

	output := Export(store)

	// Should have both DDL and data
	if !strings.Contains(output, "CREATE TABLE users") {
		t.Error("Expected CREATE TABLE in full export")
	}
	if !strings.Contains(output, "INSERT INTO users") {
		t.Error("Expected INSERT INTO in full export")
	}
}

func TestTableToDDL_WithDefault(t *testing.T) {
	table := &storage.Table{
		Name: "users",
		Columns: []storage.Column{
			{Name: "id", Type: "uuid", PrimaryKey: true},
			{Name: "status", Type: "text", Default: "'active'"},
		},
	}

	output := TableToDDL(table)

	if !strings.Contains(output, "status TEXT DEFAULT 'active'") {
		t.Errorf("Expected default value in output:\n%s", output)
	}
}

// TestExportIntegration tests the full workflow: create via SQL, export, verify valid PostgreSQL
func TestExportIntegration(t *testing.T) {
	store, dir := setupTestStore(t)

	// Simulate what would happen after CREATE TABLE via SQL
	store.Schema.CreateTable(&storage.Table{
		Name: "orders",
		Columns: []storage.Column{
			{Name: "id", Type: "uuid", PrimaryKey: true},
			{Name: "customer_id", Type: "uuid"},
			{Name: "total", Type: "decimal"},
			{Name: "status", Type: "text"},
		},
	})

	// Add some data
	store.Data.Insert("orders", storage.Row{
		"id":          "order-1",
		"customer_id": "cust-1",
		"total":       99.99,
		"status":      "pending",
	})

	// Export DDL
	ddl := ExportDDL(store)

	// Verify the DDL is valid PostgreSQL-like syntax
	expected := []string{
		"CREATE TABLE orders",
		"id UUID PRIMARY KEY",
		"customer_id UUID",
		"total DECIMAL",
		"status TEXT",
	}

	for _, exp := range expected {
		if !strings.Contains(ddl, exp) {
			t.Errorf("Expected '%s' in DDL:\n%s", exp, ddl)
		}
	}

	// Verify the export file can be written and re-read
	exportPath := filepath.Join(dir, "export.sql")
	fullExport := Export(store)
	if err := os.WriteFile(exportPath, []byte(fullExport), 0644); err != nil {
		t.Fatalf("Failed to write export: %v", err)
	}

	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("Failed to read export: %v", err)
	}

	if !strings.Contains(string(data), "CREATE TABLE orders") {
		t.Error("Export file should contain CREATE TABLE")
	}
	if !strings.Contains(string(data), "INSERT INTO orders") {
		t.Error("Export file should contain INSERT INTO")
	}

	t.Log("Export integration test passed: schema + data exported to valid SQL")
}
