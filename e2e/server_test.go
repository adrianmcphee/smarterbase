package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	conn, err := pgx.Connect(ctx, fmt.Sprintf("host=localhost port=%d sslmode=disable", env.port))
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
