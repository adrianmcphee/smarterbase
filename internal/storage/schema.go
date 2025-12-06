// Package storage implements JSON file storage for schemas and data.
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Column represents a table column definition
type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	PrimaryKey bool   `json:"primary_key,omitempty"`
	Unique     bool   `json:"unique,omitempty"`
	NotNull    bool   `json:"not_null,omitempty"`
	Default    string `json:"default,omitempty"`
}

// Table represents a table schema
type Table struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

// SchemaStore manages table schemas as JSON files
type SchemaStore struct {
	dataDir string
	mu      sync.RWMutex
	cache   map[string]*Table
}

// NewSchemaStore creates a new schema store
func NewSchemaStore(dataDir string) (*SchemaStore, error) {
	schemaDir := filepath.Join(dataDir, "_schema")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		return nil, fmt.Errorf("create schema dir: %w", err)
	}

	store := &SchemaStore{
		dataDir: dataDir,
		cache:   make(map[string]*Table),
	}

	// Load existing schemas into cache
	if err := store.loadAll(); err != nil {
		return nil, fmt.Errorf("load schemas: %w", err)
	}

	return store, nil
}

// schemaPath returns the path to a table's schema file
func (s *SchemaStore) schemaPath(tableName string) string {
	return filepath.Join(s.dataDir, "_schema", tableName+".json")
}

// loadAll loads all existing schemas into the cache
func (s *SchemaStore) loadAll() error {
	schemaDir := filepath.Join(s.dataDir, "_schema")
	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		tableName := entry.Name()[:len(entry.Name())-5] // remove .json
		table, err := s.loadSchema(tableName)
		if err != nil {
			return fmt.Errorf("load schema %s: %w", tableName, err)
		}
		s.cache[tableName] = table
	}

	return nil
}

// loadSchema loads a single schema from disk
func (s *SchemaStore) loadSchema(tableName string) (*Table, error) {
	data, err := os.ReadFile(s.schemaPath(tableName))
	if err != nil {
		return nil, err
	}

	var table Table
	if err := json.Unmarshal(data, &table); err != nil {
		return nil, err
	}

	return &table, nil
}

// CreateTable creates a new table schema
func (s *SchemaStore) CreateTable(table *Table) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if table already exists
	if _, exists := s.cache[table.Name]; exists {
		return fmt.Errorf("table %s already exists", table.Name)
	}

	// Write schema file atomically (write to temp, then rename)
	data, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal schema: %w", err)
	}

	schemaPath := s.schemaPath(table.Name)
	tempPath := schemaPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tempPath, schemaPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	// Create data directory for table
	tableDir := filepath.Join(s.dataDir, table.Name)
	if err := os.MkdirAll(tableDir, 0755); err != nil {
		return fmt.Errorf("create table dir: %w", err)
	}

	// Update cache
	s.cache[table.Name] = table

	return nil
}

// GetTable returns a table schema by name
func (s *SchemaStore) GetTable(tableName string) (*Table, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	table, exists := s.cache[tableName]
	if !exists {
		return nil, fmt.Errorf("table %s does not exist", tableName)
	}

	return table, nil
}

// TableExists checks if a table exists
func (s *SchemaStore) TableExists(tableName string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.cache[tableName]
	return exists
}

// ListTables returns all table names
func (s *SchemaStore) ListTables() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tables := make([]string, 0, len(s.cache))
	for name := range s.cache {
		tables = append(tables, name)
	}
	return tables
}

// DropTable removes a table schema and all its data
func (s *SchemaStore) DropTable(tableName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.cache[tableName]; !exists {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	// Remove schema file
	if err := os.Remove(s.schemaPath(tableName)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove schema file: %w", err)
	}

	// Remove data directory
	tableDir := filepath.Join(s.dataDir, tableName)
	if err := os.RemoveAll(tableDir); err != nil {
		return fmt.Errorf("remove table dir: %w", err)
	}

	// Update cache
	delete(s.cache, tableName)

	return nil
}
