package storage

import (
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

// DataStore manages row data as JSON files
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

// rowPath returns the path to a row's JSON file
func (d *DataStore) rowPath(tableName, id string) string {
	return filepath.Join(d.dataDir, tableName, id+".json")
}

// GenerateUUIDv7 generates a UUIDv7 (time-ordered)
func GenerateUUIDv7() string {
	// UUIDv7: timestamp (48 bits) + version (4 bits) + random (12 bits) + variant (2 bits) + random (62 bits)
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

	// Write row file atomically
	data, err := json.MarshalIndent(row, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal row: %w", err)
	}

	rowPath := d.rowPath(tableName, id)
	tempPath := rowPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return "", fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tempPath, rowPath); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("rename temp file: %w", err)
	}

	return id, nil
}

// Get retrieves a row by ID
func (d *DataStore) Get(tableName, id string) (Row, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Verify table exists
	if !d.schema.TableExists(tableName) {
		return nil, fmt.Errorf("table %s does not exist", tableName)
	}

	data, err := os.ReadFile(d.rowPath(tableName, id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("row %s not found in table %s", id, tableName)
		}
		return nil, err
	}

	var row Row
	if err := json.Unmarshal(data, &row); err != nil {
		return nil, err
	}

	return row, nil
}

// Update updates an existing row
func (d *DataStore) Update(tableName, id string, updates Row) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Verify table exists
	table, err := d.schema.GetTable(tableName)
	if err != nil {
		return err
	}

	// Read existing row
	rowPath := d.rowPath(tableName, id)
	data, err := os.ReadFile(rowPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("row %s not found in table %s", id, tableName)
		}
		return err
	}

	var row Row
	if err := json.Unmarshal(data, &row); err != nil {
		return err
	}

	// Validate update columns exist in schema
	columnMap := make(map[string]Column)
	for _, col := range table.Columns {
		columnMap[col.Name] = col
	}

	for colName := range updates {
		if colName == "id" {
			continue // Can't update ID
		}
		if _, exists := columnMap[colName]; !exists {
			return fmt.Errorf("column %s does not exist in table %s", colName, tableName)
		}
	}

	// Apply updates
	for k, v := range updates {
		if k != "id" {
			row[k] = v
		}
	}

	// Write updated row atomically
	data, err = json.MarshalIndent(row, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal row: %w", err)
	}

	tempPath := rowPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tempPath, rowPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// Delete deletes a row by ID
func (d *DataStore) Delete(tableName, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Verify table exists
	if !d.schema.TableExists(tableName) {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	rowPath := d.rowPath(tableName, id)
	if err := os.Remove(rowPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("row %s not found in table %s", id, tableName)
		}
		return err
	}

	return nil
}

// Scan returns all rows in a table (for simple SELECT *)
func (d *DataStore) Scan(tableName string) ([]Row, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Verify table exists
	if !d.schema.TableExists(tableName) {
		return nil, fmt.Errorf("table %s does not exist", tableName)
	}

	tableDir := filepath.Join(d.dataDir, tableName)
	entries, err := os.ReadDir(tableDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Row{}, nil
		}
		return nil, err
	}

	var rows []Row
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(tableDir, entry.Name()))
		if err != nil {
			continue // Skip unreadable files
		}

		var row Row
		if err := json.Unmarshal(data, &row); err != nil {
			continue // Skip invalid JSON
		}

		rows = append(rows, row)
	}

	return rows, nil
}

// Count returns the number of rows in a table
func (d *DataStore) Count(tableName string) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.schema.TableExists(tableName) {
		return 0, fmt.Errorf("table %s does not exist", tableName)
	}

	tableDir := filepath.Join(d.dataDir, tableName)
	entries, err := os.ReadDir(tableDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			count++
		}
	}

	return count, nil
}
