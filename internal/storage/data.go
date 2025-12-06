package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Row represents a single row of data
type Row map[string]any

// DataStore manages row data as JSONL files (one file per table)
type DataStore struct {
	dataDir string
	schema  *SchemaStore
	mu      sync.RWMutex
}

// NewDataStore creates a new data store
func NewDataStore(dataDir string, schema *SchemaStore) *DataStore {
	return &DataStore{
		dataDir: dataDir,
		schema:  schema,
	}
}

// tablePath returns the path to a table's JSONL file
func (d *DataStore) tablePath(tableName string) string {
	return filepath.Join(d.dataDir, tableName+".jsonl")
}

// GenerateUUIDv7 generates a UUIDv7 (time-ordered)
func GenerateUUIDv7() string {
	now := time.Now().UnixMilli()

	var u [16]byte

	// Timestamp (48 bits, big-endian)
	u[0] = byte(now >> 40)
	u[1] = byte(now >> 32)
	u[2] = byte(now >> 24)
	u[3] = byte(now >> 16)
	u[4] = byte(now >> 8)
	u[5] = byte(now)

	// Random bits for the rest
	random := uuid.New()
	copy(u[6:], random[6:])

	// Set version to 7
	u[6] = (u[6] & 0x0F) | 0x70

	// Set variant to RFC 4122
	u[8] = (u[8] & 0x3F) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// readAllRows reads all rows from a table's JSONL file
func (d *DataStore) readAllRows(tableName string) ([]Row, error) {
	path := d.tablePath(tableName)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Row{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var rows []Row
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var row Row
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue // Skip invalid lines
		}
		rows = append(rows, row)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return rows, nil
}

// writeAllRows writes all rows to a table's JSONL file atomically
func (d *DataStore) writeAllRows(tableName string, rows []Row) error {
	path := d.tablePath(tableName)
	tempPath := path + ".tmp"

	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	writer := bufio.NewWriter(file)
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			file.Close()
			os.Remove(tempPath)
			return fmt.Errorf("marshal row: %w", err)
		}
		if _, err := writer.Write(data); err != nil {
			file.Close()
			os.Remove(tempPath)
			return fmt.Errorf("write row: %w", err)
		}
		if _, err := writer.WriteString("\n"); err != nil {
			file.Close()
			os.Remove(tempPath)
			return fmt.Errorf("write newline: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		file.Close()
		os.Remove(tempPath)
		return fmt.Errorf("flush: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("close: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

// Insert inserts a new row into a table
func (d *DataStore) Insert(tableName string, row Row) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Verify table exists
	table, err := d.schema.GetTable(tableName)
	if err != nil {
		return "", err
	}

	// Generate ID if not provided
	id, ok := row["id"].(string)
	if !ok || id == "" {
		id = GenerateUUIDv7()
		row["id"] = id
	}

	// Validate columns exist in schema
	columnMap := make(map[string]Column)
	for _, col := range table.Columns {
		columnMap[col.Name] = col
	}

	for colName := range row {
		if _, exists := columnMap[colName]; !exists {
			return "", fmt.Errorf("column %s does not exist in table %s", colName, tableName)
		}
	}

	// Read existing rows
	rows, err := d.readAllRows(tableName)
	if err != nil {
		return "", err
	}

	// Check for duplicate ID
	for _, existing := range rows {
		if existing["id"] == id {
			return "", fmt.Errorf("row with id %s already exists in table %s", id, tableName)
		}
	}

	// Append new row
	rows = append(rows, row)

	// Write all rows
	if err := d.writeAllRows(tableName, rows); err != nil {
		return "", err
	}

	return id, nil
}

// Get retrieves a row by ID
func (d *DataStore) Get(tableName, id string) (Row, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.schema.TableExists(tableName) {
		return nil, fmt.Errorf("table %s does not exist", tableName)
	}

	rows, err := d.readAllRows(tableName)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		if row["id"] == id {
			return row, nil
		}
	}

	return nil, fmt.Errorf("row %s not found in table %s", id, tableName)
}

// Update updates an existing row
func (d *DataStore) Update(tableName, id string, updates Row) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	table, err := d.schema.GetTable(tableName)
	if err != nil {
		return err
	}

	// Validate update columns exist in schema
	columnMap := make(map[string]Column)
	for _, col := range table.Columns {
		columnMap[col.Name] = col
	}

	for colName := range updates {
		if colName == "id" {
			continue
		}
		if _, exists := columnMap[colName]; !exists {
			return fmt.Errorf("column %s does not exist in table %s", colName, tableName)
		}
	}

	// Read all rows
	rows, err := d.readAllRows(tableName)
	if err != nil {
		return err
	}

	// Find and update the row
	found := false
	for i, row := range rows {
		if row["id"] == id {
			for k, v := range updates {
				if k != "id" {
					rows[i][k] = v
				}
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("row %s not found in table %s", id, tableName)
	}

	return d.writeAllRows(tableName, rows)
}

// Delete deletes a row by ID
func (d *DataStore) Delete(tableName, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.schema.TableExists(tableName) {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	rows, err := d.readAllRows(tableName)
	if err != nil {
		return err
	}

	// Filter out the deleted row
	newRows := make([]Row, 0, len(rows))
	found := false
	for _, row := range rows {
		if row["id"] == id {
			found = true
			continue
		}
		newRows = append(newRows, row)
	}

	if !found {
		return fmt.Errorf("row %s not found in table %s", id, tableName)
	}

	return d.writeAllRows(tableName, newRows)
}

// Scan returns all rows in a table
func (d *DataStore) Scan(tableName string) ([]Row, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.schema.TableExists(tableName) {
		return nil, fmt.Errorf("table %s does not exist", tableName)
	}

	return d.readAllRows(tableName)
}

// Count returns the number of rows in a table
func (d *DataStore) Count(tableName string) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.schema.TableExists(tableName) {
		return 0, fmt.Errorf("table %s does not exist", tableName)
	}

	rows, err := d.readAllRows(tableName)
	if err != nil {
		return 0, err
	}

	return len(rows), nil
}
