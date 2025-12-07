// Package export generates PostgreSQL DDL from SmarterBase schemas.
package export

import (
	"fmt"
	"sort"
	"strings"

	"github.com/adrianmcphee/smarterbase/v2/internal/storage"
)

// ExportDDL generates PostgreSQL CREATE TABLE statements from all schemas
func ExportDDL(store *storage.Store) string {
	tables := store.Schema.ListTables()
	sort.Strings(tables) // Deterministic output

	var sb strings.Builder
	sb.WriteString("-- SmarterBase export to PostgreSQL\n")
	sb.WriteString("-- Generated schema (no migration history)\n\n")

	for i, tableName := range tables {
		table, err := store.Schema.GetTable(tableName)
		if err != nil {
			continue
		}

		sb.WriteString(TableToDDL(table))
		if i < len(tables)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// TableToDDL generates a CREATE TABLE statement for a single table
func TableToDDL(table *storage.Table) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", table.Name))

	for i, col := range table.Columns {
		sb.WriteString("  ")
		sb.WriteString(columnToDDL(&col))
		if i < len(table.Columns)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(");\n")

	return sb.String()
}

// columnToDDL generates DDL for a single column
func columnToDDL(col *storage.Column) string {
	var parts []string

	// Column name
	parts = append(parts, col.Name)

	// Type mapping
	pgType := mapType(col.Type)
	parts = append(parts, pgType)

	// Constraints
	if col.PrimaryKey {
		parts = append(parts, "PRIMARY KEY")
	}
	if col.Unique && !col.PrimaryKey {
		parts = append(parts, "UNIQUE")
	}
	if col.NotNull && !col.PrimaryKey {
		parts = append(parts, "NOT NULL")
	}
	if col.Default != "" {
		parts = append(parts, "DEFAULT", col.Default)
	}

	return strings.Join(parts, " ")
}

// mapType maps SmarterBase types to PostgreSQL types
func mapType(sbType string) string {
	switch strings.ToLower(sbType) {
	case "uuid":
		return "UUID"
	case "text", "string":
		return "TEXT"
	case "int", "integer":
		return "INTEGER"
	case "bigint":
		return "BIGINT"
	case "boolean", "bool":
		return "BOOLEAN"
	case "decimal", "numeric":
		return "DECIMAL"
	case "timestamp", "timestamptz":
		return "TIMESTAMPTZ"
	case "date":
		return "DATE"
	case "json", "jsonb":
		return "JSONB"
	default:
		return "TEXT" // Default to TEXT for unknown types
	}
}

// ExportData generates INSERT statements for all data
func ExportData(store *storage.Store) string {
	tables := store.Schema.ListTables()
	sort.Strings(tables)

	var sb strings.Builder
	sb.WriteString("-- SmarterBase data export\n\n")

	for _, tableName := range tables {
		table, err := store.Schema.GetTable(tableName)
		if err != nil {
			continue
		}

		rows, err := store.Data.Scan(tableName)
		if err != nil || len(rows) == 0 {
			continue
		}

		// Get column names in schema order
		colNames := make([]string, len(table.Columns))
		for i, col := range table.Columns {
			colNames[i] = col.Name
		}

		for _, row := range rows {
			sb.WriteString(rowToInsert(tableName, colNames, row))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// rowToInsert generates an INSERT statement for a single row
func rowToInsert(tableName string, colNames []string, row storage.Row) string {
	values := make([]string, len(colNames))

	for i, colName := range colNames {
		val, ok := row[colName]
		if !ok || val == nil {
			values[i] = "NULL"
			continue
		}

		switch v := val.(type) {
		case string:
			// Escape single quotes
			escaped := strings.ReplaceAll(v, "'", "''")
			values[i] = fmt.Sprintf("'%s'", escaped)
		case float64:
			// JSON numbers are float64
			if v == float64(int(v)) {
				values[i] = fmt.Sprintf("%d", int(v))
			} else {
				values[i] = fmt.Sprintf("%v", v)
			}
		case bool:
			values[i] = fmt.Sprintf("%t", v)
		default:
			values[i] = fmt.Sprintf("'%v'", v)
		}
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);\n",
		tableName,
		strings.Join(colNames, ", "),
		strings.Join(values, ", "))
}

// Export generates both DDL and data
func Export(store *storage.Store) string {
	var sb strings.Builder
	sb.WriteString(ExportDDL(store))
	sb.WriteString("\n")
	sb.WriteString(ExportData(store))
	return sb.String()
}
