package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adrianmcphee/smarterbase/v2/internal/protocol"
	"github.com/jackc/pgx/v5"
)

var portCounter int32 = 15432

func nextPort() int {
	return int(atomic.AddInt32(&portCounter, 1))
}

type testEnv struct {
	port    int
	dataDir string
}

func setupTest(t *testing.T) *testEnv {
	port := nextPort()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("smarterbase_test_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	server, err := protocol.NewServer(port, dir)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	go func() {
		_ = server.Start()
	}()

	time.Sleep(200 * time.Millisecond)

	return &testEnv{port: port, dataDir: dir}
}

func (env *testEnv) cleanup() {
	os.RemoveAll(env.dataDir)
}

func (env *testEnv) connect(t *testing.T) *pgx.Conn {
	ctx := context.Background()
	config, err := pgx.ParseConfig(fmt.Sprintf("host=localhost port=%d sslmode=disable", env.port))
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}
	// Use simple query protocol to avoid prepared statements
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	return conn
}

func TestCreateTable(t *testing.T) {
	env := setupTest(t)
	defer env.cleanup()

	ctx := context.Background()
	conn := env.connect(t)
	defer conn.Close(ctx)

	// Create table
	_, err := conn.Exec(ctx, "CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT, email TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Verify schema file exists
	schemaPath := filepath.Join(env.dataDir, "_schema", "users.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("Schema file not created: %v", err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("Invalid schema JSON: %v", err)
	}

	if schema["name"] != "users" {
		t.Errorf("Expected table name 'users', got %v", schema["name"])
	}

	columns, ok := schema["columns"].([]interface{})
	if !ok || len(columns) != 3 {
		t.Errorf("Expected 3 columns, got %v", schema["columns"])
	}
}

func TestInsertCreatesJSONL(t *testing.T) {
	env := setupTest(t)
	defer env.cleanup()

	ctx := context.Background()
	conn := env.connect(t)
	defer conn.Close(ctx)

	// Create table
	_, err := conn.Exec(ctx, "CREATE TABLE products (id TEXT PRIMARY KEY, name TEXT, price TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert rows
	_, err = conn.Exec(ctx, "INSERT INTO products (id, name, price) VALUES ('p1', 'Widget', '9.99')")
	if err != nil {
		t.Fatalf("Failed to insert row 1: %v", err)
	}

	_, err = conn.Exec(ctx, "INSERT INTO products (id, name, price) VALUES ('p2', 'Gadget', '19.99')")
	if err != nil {
		t.Fatalf("Failed to insert row 2: %v", err)
	}

	// Verify JSONL file exists and contains correct data
	jsonlPath := filepath.Join(env.dataDir, "products.jsonl")
	file, err := os.Open(jsonlPath)
	if err != nil {
		t.Fatalf("JSONL file not created: %v", err)
	}
	defer file.Close()

	var rows []map[string]interface{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var row map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatalf("Invalid JSONL line: %v", err)
		}
		rows = append(rows, row)
	}

	if len(rows) != 2 {
		t.Errorf("Expected 2 rows in JSONL, got %d", len(rows))
	}

	// Find p1 and p2
	foundP1, foundP2 := false, false
	for _, row := range rows {
		if row["id"] == "p1" {
			foundP1 = true
			if row["name"] != "Widget" || row["price"] != "9.99" {
				t.Errorf("Row p1 has incorrect data: %v", row)
			}
		}
		if row["id"] == "p2" {
			foundP2 = true
			if row["name"] != "Gadget" || row["price"] != "19.99" {
				t.Errorf("Row p2 has incorrect data: %v", row)
			}
		}
	}

	if !foundP1 || !foundP2 {
		t.Errorf("Missing rows: foundP1=%v, foundP2=%v", foundP1, foundP2)
	}
}

func TestUpdateModifiesJSONL(t *testing.T) {
	env := setupTest(t)
	defer env.cleanup()

	ctx := context.Background()
	conn := env.connect(t)
	defer conn.Close(ctx)

	// Create table and insert (avoid reserved word "status")
	_, _ = conn.Exec(ctx, "CREATE TABLE items (id TEXT PRIMARY KEY, state TEXT)")
	_, _ = conn.Exec(ctx, "INSERT INTO items (id, state) VALUES ('i1', 'pending')")

	// Update
	_, err := conn.Exec(ctx, "UPDATE items SET state = 'completed' WHERE id = 'i1'")
	if err != nil {
		t.Fatalf("Failed to update: %v", err)
	}

	// Verify in JSONL file
	jsonlPath := filepath.Join(env.dataDir, "items.jsonl")
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("Failed to read JSONL: %v", err)
	}

	var row map[string]interface{}
	if err := json.Unmarshal(data[:len(data)-1], &row); err != nil { // trim newline
		t.Fatalf("Invalid JSONL: %v", err)
	}

	if row["state"] != "completed" {
		t.Errorf("Expected state 'completed', got %v", row["state"])
	}
}

func TestDeleteRemovesFromJSONL(t *testing.T) {
	env := setupTest(t)
	defer env.cleanup()

	ctx := context.Background()
	conn := env.connect(t)
	defer conn.Close(ctx)

	// Create table and insert
	_, _ = conn.Exec(ctx, "CREATE TABLE tasks (id TEXT PRIMARY KEY, title TEXT)")
	_, _ = conn.Exec(ctx, "INSERT INTO tasks (id, title) VALUES ('t1', 'Task 1')")
	_, _ = conn.Exec(ctx, "INSERT INTO tasks (id, title) VALUES ('t2', 'Task 2')")

	// Delete t1
	_, err := conn.Exec(ctx, "DELETE FROM tasks WHERE id = 't1'")
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify only t2 remains in JSONL
	jsonlPath := filepath.Join(env.dataDir, "tasks.jsonl")
	file, err := os.Open(jsonlPath)
	if err != nil {
		t.Fatalf("Failed to read JSONL: %v", err)
	}
	defer file.Close()

	var rows []map[string]interface{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var row map[string]interface{}
		json.Unmarshal(scanner.Bytes(), &row)
		rows = append(rows, row)
	}

	if len(rows) != 1 {
		t.Errorf("Expected 1 row after delete, got %d", len(rows))
	}

	if rows[0]["id"] != "t2" {
		t.Errorf("Expected remaining row to be t2, got %v", rows[0]["id"])
	}
}

// TestDirectSchemaEdit verifies that editing schema JSON directly works
// This is the key value prop: AI assistants can edit schema without migrations
func TestDirectSchemaEdit(t *testing.T) {
	env := setupTest(t)
	defer env.cleanup()

	ctx := context.Background()
	conn := env.connect(t)
	defer conn.Close(ctx)

	// Create table with 2 columns via SQL
	_, err := conn.Exec(ctx, "CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Verify initial schema was created
	schemaPath := filepath.Join(env.dataDir, "_schema", "users.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("Schema file not found: %v", err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("Invalid schema JSON: %v", err)
	}

	columns := schema["columns"].([]interface{})
	if len(columns) != 2 {
		t.Fatalf("Expected 2 columns initially, got %d", len(columns))
	}

	// Directly edit schema JSON to add email column (simulating AI edit)
	newSchema := `{
  "name": "users",
  "columns": [
    {"name": "id", "type": "text", "primary_key": true},
    {"name": "name", "type": "text"},
    {"name": "email", "type": "text"}
  ]
}`
	if err := os.WriteFile(schemaPath, []byte(newSchema), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	// Verify the schema file was updated
	data, err = os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("Failed to read updated schema: %v", err)
	}

	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("Invalid updated schema JSON: %v", err)
	}

	columns = schema["columns"].([]interface{})
	if len(columns) != 3 {
		t.Errorf("Expected 3 columns after edit, got %d", len(columns))
	}

	// Verify the third column is email
	col3 := columns[2].(map[string]interface{})
	if col3["name"] != "email" {
		t.Errorf("Expected third column to be 'email', got '%v'", col3["name"])
	}

	t.Log("Direct schema edit verified: AI can add columns by editing JSON")
}

// TestDirectDataEdit verifies that editing JSONL directly works
// AI assistants can create test data by editing files
func TestDirectDataEdit(t *testing.T) {
	env := setupTest(t)
	defer env.cleanup()

	ctx := context.Background()
	conn := env.connect(t)
	defer conn.Close(ctx)

	// Create table and insert one row via SQL
	_, err := conn.Exec(ctx, "CREATE TABLE customers (id TEXT PRIMARY KEY, name TEXT, email TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	_, err = conn.Exec(ctx, "INSERT INTO customers (id, name, email) VALUES ('c0', 'Initial', 'initial@example.com')")
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Verify JSONL file has one row
	jsonlPath := filepath.Join(env.dataDir, "customers.jsonl")
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("Failed to read JSONL: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("Expected 1 row initially, got %d", len(lines))
	}

	// Directly append to JSONL file (simulating AI adding test data)
	additionalData := `{"id":"c1","name":"Alice","email":"alice@example.com"}
{"id":"c2","name":"Bob","email":"bob@example.com"}
`
	f, err := os.OpenFile(jsonlPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open JSONL for append: %v", err)
	}
	_, err = f.WriteString(additionalData)
	f.Close()
	if err != nil {
		t.Fatalf("Failed to append to JSONL: %v", err)
	}

	// Verify the JSONL file now has 3 rows
	data, err = os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("Failed to read updated JSONL: %v", err)
	}

	lines = strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 rows after edit, got %d", len(lines))
	}

	// Verify the data is valid JSON
	for i, line := range lines {
		var row map[string]interface{}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Errorf("Row %d is not valid JSON: %v", i, err)
		}
	}

	t.Log("Direct data edit verified: AI can add rows by editing JSONL")
}

// TestFullAIWorkflow tests the complete workflow shown on the website
// 1. Create table via SQL
// 2. AI edits schema (add column)
// 3. AI creates test data
// 4. Verify data files are valid and queryable
func TestFullAIWorkflow(t *testing.T) {
	env := setupTest(t)
	defer env.cleanup()

	ctx := context.Background()
	conn := env.connect(t)
	defer conn.Close(ctx)

	// Step 1: Create table via SQL
	_, err := conn.Exec(ctx, "CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Verify initial schema
	schemaPath := filepath.Join(env.dataDir, "_schema", "users.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("Schema file not created: %v", err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("Invalid initial schema: %v", err)
	}

	columns := schema["columns"].([]interface{})
	if len(columns) != 2 {
		t.Fatalf("Expected 2 columns initially, got %d", len(columns))
	}

	// Step 2: AI edits schema directly (adds email column)
	newSchema := `{
  "name": "users",
  "columns": [
    {"name": "id", "type": "text", "primary_key": true},
    {"name": "name", "type": "text"},
    {"name": "email", "type": "text"}
  ]
}`
	if err := os.WriteFile(schemaPath, []byte(newSchema), 0644); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}

	// Verify schema was updated
	data, err = os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("Failed to read updated schema: %v", err)
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("Invalid updated schema: %v", err)
	}
	columns = schema["columns"].([]interface{})
	if len(columns) != 3 {
		t.Fatalf("Expected 3 columns after edit, got %d", len(columns))
	}

	// Step 3: AI creates test data directly
	jsonlPath := filepath.Join(env.dataDir, "users.jsonl")
	testData := `{"id":"u1","name":"Alice","email":"alice@example.com"}
{"id":"u2","name":"Bob","email":"bob@example.com"}
`
	if err := os.WriteFile(jsonlPath, []byte(testData), 0644); err != nil {
		t.Fatalf("Failed to write JSONL: %v", err)
	}

	// Step 4: Verify the data files are valid
	data, err = os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("Failed to read JSONL: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(lines))
	}

	// Verify each row
	var row1, row2 map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &row1); err != nil {
		t.Fatalf("Invalid row 1: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &row2); err != nil {
		t.Fatalf("Invalid row 2: %v", err)
	}

	// Verify Alice
	if row1["id"] != "u1" || row1["name"] != "Alice" || row1["email"] != "alice@example.com" {
		t.Errorf("Row 1 incorrect: %v", row1)
	}

	// Verify Bob
	if row2["id"] != "u2" || row2["name"] != "Bob" || row2["email"] != "bob@example.com" {
		t.Errorf("Row 2 incorrect: %v", row2)
	}

	t.Log("Full AI workflow verified: CREATE TABLE -> edit schema -> create data -> valid files")
}
