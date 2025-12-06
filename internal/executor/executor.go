// Package executor parses and executes SQL statements against the storage layer.
package executor

import (
	"fmt"
	"strings"

	"github.com/adrianmcphee/smarterbase/v2/internal/storage"
	"github.com/xwb1989/sqlparser"
)

// Result represents the result of executing a SQL statement
type Result struct {
	Columns      []string
	Rows         [][]string
	RowsAffected int
	LastInsertID string
	Message      string
}

// Executor executes SQL statements
type Executor struct {
	store *storage.Store
}

// NewExecutor creates a new SQL executor
func NewExecutor(store *storage.Store) *Executor {
	return &Executor{store: store}
}

// Execute parses and executes a SQL statement
func (e *Executor) Execute(sql string) (*Result, error) {
	// Handle empty queries
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return &Result{Message: "OK"}, nil
	}

	// Remove trailing semicolon for parser
	sql = strings.TrimSuffix(sql, ";")

	stmt, err := sqlparser.Parse(sql)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	switch s := stmt.(type) {
	case *sqlparser.DDL:
		return e.executeDDL(s)
	case *sqlparser.Select:
		return e.executeSelect(s)
	case *sqlparser.Insert:
		return e.executeInsert(s)
	case *sqlparser.Update:
		return e.executeUpdate(s)
	case *sqlparser.Delete:
		return e.executeDelete(s)
	default:
		return nil, fmt.Errorf("unsupported statement type: %T", stmt)
	}
}

// executeDDL handles CREATE TABLE, DROP TABLE, etc.
func (e *Executor) executeDDL(stmt *sqlparser.DDL) (*Result, error) {
	switch stmt.Action {
	case sqlparser.CreateStr:
		return e.executeCreateTable(stmt)
	case sqlparser.DropStr:
		return e.executeDropTable(stmt)
	default:
		return nil, fmt.Errorf("unsupported DDL action: %s", stmt.Action)
	}
}

// executeCreateTable handles CREATE TABLE statements
func (e *Executor) executeCreateTable(stmt *sqlparser.DDL) (*Result, error) {
	tableName := stmt.NewName.Name.String()

	// Parse column definitions
	var columns []storage.Column
	for _, col := range stmt.TableSpec.Columns {
		column := storage.Column{
			Name: col.Name.String(),
			Type: col.Type.Type,
		}

		// Check for constraints
		if col.Type.NotNull {
			column.NotNull = true
		}

		// Check for PRIMARY KEY in column options
		if col.Type.KeyOpt == 1 { // colKeyPrimary
			column.PrimaryKey = true
		}

		columns = append(columns, column)
	}

	// Check table-level constraints (like PRIMARY KEY)
	for _, idx := range stmt.TableSpec.Indexes {
		if idx.Info.Primary {
			// Mark the column as primary key
			for i, col := range columns {
				for _, idxCol := range idx.Columns {
					if col.Name == idxCol.Column.String() {
						columns[i].PrimaryKey = true
					}
				}
			}
		}
	}

	table := &storage.Table{
		Name:    tableName,
		Columns: columns,
	}

	if err := e.store.Schema.CreateTable(table); err != nil {
		return nil, err
	}

	return &Result{Message: fmt.Sprintf("CREATE TABLE %s", tableName)}, nil
}

// executeDropTable handles DROP TABLE statements
func (e *Executor) executeDropTable(stmt *sqlparser.DDL) (*Result, error) {
	tableName := stmt.Table.Name.String()

	if err := e.store.Schema.DropTable(tableName); err != nil {
		return nil, err
	}

	return &Result{Message: fmt.Sprintf("DROP TABLE %s", tableName)}, nil
}

// executeSelect handles SELECT statements
func (e *Executor) executeSelect(stmt *sqlparser.Select) (*Result, error) {
	// Get table name from FROM clause
	if len(stmt.From) != 1 {
		return nil, fmt.Errorf("only single table SELECT supported")
	}

	tableName, err := getTableName(stmt.From[0])
	if err != nil {
		return nil, err
	}

	// Get table schema for column info
	table, err := e.store.Schema.GetTable(tableName)
	if err != nil {
		return nil, err
	}

	// Scan all rows
	rows, err := e.store.Data.Scan(tableName)
	if err != nil {
		return nil, err
	}

	// Determine which columns to return
	var columns []string
	selectAll := false

	for _, expr := range stmt.SelectExprs {
		switch e := expr.(type) {
		case *sqlparser.StarExpr:
			selectAll = true
		case *sqlparser.AliasedExpr:
			if col, ok := e.Expr.(*sqlparser.ColName); ok {
				columns = append(columns, col.Name.String())
			}
		}
	}

	if selectAll {
		columns = make([]string, len(table.Columns))
		for i, col := range table.Columns {
			columns[i] = col.Name
		}
	}

	// Apply WHERE clause filter
	filteredRows := rows
	if stmt.Where != nil {
		filteredRows = make([]storage.Row, 0)
		for _, row := range rows {
			if matchesWhere(row, stmt.Where.Expr) {
				filteredRows = append(filteredRows, row)
			}
		}
	}

	// Convert to string matrix for result
	resultRows := make([][]string, len(filteredRows))
	for i, row := range filteredRows {
		resultRows[i] = make([]string, len(columns))
		for j, col := range columns {
			if val, ok := row[col]; ok {
				resultRows[i][j] = fmt.Sprintf("%v", val)
			} else {
				resultRows[i][j] = ""
			}
		}
	}

	return &Result{
		Columns: columns,
		Rows:    resultRows,
		Message: fmt.Sprintf("SELECT %d", len(resultRows)),
	}, nil
}

// executeInsert handles INSERT statements
func (e *Executor) executeInsert(stmt *sqlparser.Insert) (*Result, error) {
	tableName := stmt.Table.Name.String()

	// Get column names
	var columns []string
	for _, col := range stmt.Columns {
		columns = append(columns, col.String())
	}

	// Get values
	rows, ok := stmt.Rows.(sqlparser.Values)
	if !ok {
		return nil, fmt.Errorf("only VALUES clause supported for INSERT")
	}

	var lastID string
	for _, valTuple := range rows {
		row := make(storage.Row)

		for i, val := range valTuple {
			if i < len(columns) {
				colName := columns[i]
				row[colName] = evalExpr(val)
			}
		}

		id, err := e.store.Data.Insert(tableName, row)
		if err != nil {
			return nil, err
		}
		lastID = id
	}

	return &Result{
		RowsAffected: len(rows),
		LastInsertID: lastID,
		Message:      fmt.Sprintf("INSERT 0 %d", len(rows)),
	}, nil
}

// executeUpdate handles UPDATE statements
func (e *Executor) executeUpdate(stmt *sqlparser.Update) (*Result, error) {
	if len(stmt.TableExprs) != 1 {
		return nil, fmt.Errorf("only single table UPDATE supported")
	}

	tableName, err := getTableName(stmt.TableExprs[0])
	if err != nil {
		return nil, err
	}

	// Get all rows and filter by WHERE
	rows, err := e.store.Data.Scan(tableName)
	if err != nil {
		return nil, err
	}

	// Build updates map
	updates := make(storage.Row)
	for _, expr := range stmt.Exprs {
		colName := expr.Name.Name.String()
		updates[colName] = evalExpr(expr.Expr)
	}

	// Apply updates to matching rows
	affected := 0
	for _, row := range rows {
		if stmt.Where == nil || matchesWhere(row, stmt.Where.Expr) {
			id, ok := row["id"].(string)
			if !ok {
				continue
			}
			if err := e.store.Data.Update(tableName, id, updates); err != nil {
				return nil, err
			}
			affected++
		}
	}

	return &Result{
		RowsAffected: affected,
		Message:      fmt.Sprintf("UPDATE %d", affected),
	}, nil
}

// executeDelete handles DELETE statements
func (e *Executor) executeDelete(stmt *sqlparser.Delete) (*Result, error) {
	if len(stmt.TableExprs) != 1 {
		return nil, fmt.Errorf("only single table DELETE supported")
	}

	tableName, err := getTableName(stmt.TableExprs[0])
	if err != nil {
		return nil, err
	}

	// Get all rows and filter by WHERE
	rows, err := e.store.Data.Scan(tableName)
	if err != nil {
		return nil, err
	}

	// Delete matching rows
	affected := 0
	for _, row := range rows {
		if stmt.Where == nil || matchesWhere(row, stmt.Where.Expr) {
			id, ok := row["id"].(string)
			if !ok {
				continue
			}
			if err := e.store.Data.Delete(tableName, id); err != nil {
				return nil, err
			}
			affected++
		}
	}

	return &Result{
		RowsAffected: affected,
		Message:      fmt.Sprintf("DELETE %d", affected),
	}, nil
}

// Helper functions

func getTableName(expr sqlparser.TableExpr) (string, error) {
	switch t := expr.(type) {
	case *sqlparser.AliasedTableExpr:
		if tbl, ok := t.Expr.(sqlparser.TableName); ok {
			return tbl.Name.String(), nil
		}
	}
	return "", fmt.Errorf("could not determine table name")
}

func evalExpr(expr sqlparser.Expr) any {
	switch e := expr.(type) {
	case *sqlparser.SQLVal:
		switch e.Type {
		case sqlparser.StrVal:
			return string(e.Val)
		case sqlparser.IntVal:
			return string(e.Val)
		case sqlparser.FloatVal:
			return string(e.Val)
		}
	case *sqlparser.NullVal:
		return nil
	case *sqlparser.FuncExpr:
		// Handle gen_random_uuid7()
		if strings.ToLower(e.Name.String()) == "gen_random_uuid7" {
			return storage.GenerateUUIDv7()
		}
	}
	return nil
}

func matchesWhere(row storage.Row, expr sqlparser.Expr) bool {
	switch e := expr.(type) {
	case *sqlparser.ComparisonExpr:
		left := getColumnValue(row, e.Left)
		right := evalExpr(e.Right)

		switch e.Operator {
		case "=":
			return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right)
		case "!=", "<>":
			return fmt.Sprintf("%v", left) != fmt.Sprintf("%v", right)
		}
	case *sqlparser.AndExpr:
		return matchesWhere(row, e.Left) && matchesWhere(row, e.Right)
	case *sqlparser.OrExpr:
		return matchesWhere(row, e.Left) || matchesWhere(row, e.Right)
	}
	return true
}

func getColumnValue(row storage.Row, expr sqlparser.Expr) any {
	switch e := expr.(type) {
	case *sqlparser.ColName:
		return row[e.Name.String()]
	}
	return nil
}
