package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sobhane/golang-database/parser"
	"github.com/sobhane/golang-database/storage"
)

// Executor — The Query Engine
//
// HOW QUERY EXECUTION WORKS:
// 1. The parser gives us an AST (Abstract Syntax Tree).
// 2. The executor looks at the AST node type (SELECT? INSERT? CREATE?).
// 3. For each type, it performs the corresponding operation:
//    - CREATE TABLE → register in catalog, allocate B-Tree
//    - INSERT → validate types, serialize row, insert into B-Tree
//    - SELECT → scan B-Tree, filter with WHERE, project columns, sort, format
//    - UPDATE → find matching rows, modify values, re-insert
//    - DELETE → find matching rows, remove from B-Tree

// Executor holds references to the storage layer and catalog.
type Executor struct {
	Pager   *storage.Pager
	Catalog *Catalog
}

// NewExecutor creates a new executor.
func NewExecutor(pager *storage.Pager, catalog *Catalog) *Executor {
	return &Executor{Pager: pager, Catalog: catalog}
}

// Execute takes an AST statement and executes it, returning a result string.
func (e *Executor) Execute(stmt parser.Statement) (string, error) {
	switch s := stmt.(type) {
	case *parser.CreateTableStmt:
		return e.executeCreate(s)
	case *parser.DropTableStmt:
		return e.executeDrop(s)
	case *parser.InsertStmt:
		return e.executeInsert(s)
	case *parser.SelectStmt:
		return e.executeSelect(s)
	case *parser.UpdateStmt:
		return e.executeUpdate(s)
	case *parser.DeleteStmt:
		return e.executeDelete(s)
	default:
		return "", fmt.Errorf("unknown statement type: %T", stmt)
	}
}

// ---------------------------------------------------------------------------
// CREATE TABLE
// ---------------------------------------------------------------------------
func (e *Executor) executeCreate(stmt *parser.CreateTableStmt) (string, error) {
	columns := make([]ColumnInfo, len(stmt.Columns))
	for i, col := range stmt.Columns {
		columns[i] = ColumnInfo{
			Name:       col.Name,
			DataType:   col.DataType,
			PrimaryKey: col.PrimaryKey,
			NotNull:    col.NotNull,
		}
	}

	primaryKey := stmt.PrimaryKey
	// Also check for inline PRIMARY KEY on columns
	if primaryKey == "" {
		for _, col := range columns {
			if col.PrimaryKey {
				primaryKey = col.Name
				break
			}
		}
	}

	_, err := e.Catalog.CreateTable(stmt.TableName, columns, primaryKey)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Table '%s' created.", stmt.TableName), nil
}

// ---------------------------------------------------------------------------
// DROP TABLE
// ---------------------------------------------------------------------------
func (e *Executor) executeDrop(stmt *parser.DropTableStmt) (string, error) {
	if err := e.Catalog.DropTable(stmt.TableName); err != nil {
		return "", err
	}
	return fmt.Sprintf("Table '%s' dropped.", stmt.TableName), nil
}

// ---------------------------------------------------------------------------
// INSERT INTO table VALUES (...)
// ---------------------------------------------------------------------------
func (e *Executor) executeInsert(stmt *parser.InsertStmt) (string, error) {
	table, err := e.Catalog.GetTable(stmt.TableName)
	if err != nil {
		return "", err
	}

	// Determine which columns we're inserting into
	var targetColumns []string
	if len(stmt.Columns) > 0 {
		targetColumns = stmt.Columns
	} else {
		// No explicit columns — values must match all columns in order
		for _, col := range table.Columns {
			targetColumns = append(targetColumns, col.Name)
		}
	}

	if len(stmt.Values) != len(targetColumns) {
		return "", fmt.Errorf("expected %d values, got %d", len(targetColumns), len(stmt.Values))
	}

	// Build the row
	row := make(Row)
	for i, colName := range targetColumns {
		colIdx := table.GetColumnIndex(colName)
		if colIdx == -1 {
			return "", fmt.Errorf("unknown column '%s' in table '%s'", colName, stmt.TableName)
		}

		colInfo := table.Columns[colIdx]
		val, err := evaluateExpression(stmt.Values[i], nil) // no row context for VALUES
		if err != nil {
			return "", fmt.Errorf("error evaluating value for column '%s': %w", colName, err)
		}

		// Type checking
		typedVal, err := coerceType(val, colInfo.DataType)
		if err != nil {
			return "", fmt.Errorf("type error for column '%s': %w", colName, err)
		}

		// NOT NULL check
		if typedVal == nil && colInfo.NotNull {
			return "", fmt.Errorf("column '%s' cannot be NULL", colName)
		}

		row[colName] = typedVal
	}

	if err := InsertRow(e.Pager, table, row); err != nil {
		return "", err
	}

	// Save catalog (NextRowID may have changed)
	if err := e.Catalog.Save(); err != nil {
		return "", err
	}

	return "1 row inserted.", nil
}

// ---------------------------------------------------------------------------
// SELECT
// ---------------------------------------------------------------------------
func (e *Executor) executeSelect(stmt *parser.SelectStmt) (string, error) {
	table, err := e.Catalog.GetTable(stmt.TableName)
	if err != nil {
		return "", err
	}

	// Scan all rows from the table
	rows, err := ScanAllRows(e.Pager, table)
	if err != nil {
		return "", err
	}

	// Handle JOINs
	for _, join := range stmt.Joins {
		joinTable, err := e.Catalog.GetTable(join.TableName)
		if err != nil {
			return "", err
		}
		joinRows, err := ScanAllRows(e.Pager, joinTable)
		if err != nil {
			return "", err
		}

		// Nested Loop Join — the simplest join algorithm
		// For each row in the left table, check every row in the right table.
		// If the ON condition matches, combine them into one row.
		var joinedRows []Row
		for _, leftRow := range rows {
			for _, rightRow := range joinRows {
				// Merge rows (prefix with table name for ambiguity)
				merged := make(Row)
				for k, v := range leftRow {
					merged[k] = v
					merged[stmt.TableName+"."+k] = v
				}
				for k, v := range rightRow {
					merged[k] = v
					merged[join.TableName+"."+k] = v
				}

				// Evaluate join condition
				match, err := evaluateBoolExpression(join.Condition, merged)
				if err != nil {
					return "", fmt.Errorf("error evaluating JOIN condition: %w", err)
				}
				if match {
					joinedRows = append(joinedRows, merged)
				}
			}

			// LEFT JOIN: if no match found, include left row with NULLs
			if join.JoinType == "LEFT" && !hasMatchInJoin(leftRow, joinRows, join, stmt.TableName) {
				merged := make(Row)
				for k, v := range leftRow {
					merged[k] = v
					merged[stmt.TableName+"."+k] = v
				}
				for _, col := range joinTable.Columns {
					merged[col.Name] = nil
					merged[join.TableName+"."+col.Name] = nil
				}
				joinedRows = append(joinedRows, merged)
			}
		}
		rows = joinedRows
	}

	// Apply WHERE filter
	if stmt.Where != nil {
		var filtered []Row
		for _, row := range rows {
			match, err := evaluateBoolExpression(stmt.Where, row)
			if err != nil {
				return "", fmt.Errorf("error evaluating WHERE: %w", err)
			}
			if match {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}

	// Apply ORDER BY
	if len(stmt.OrderBy) > 0 {
		sort.SliceStable(rows, func(i, j int) bool {
			for _, ob := range stmt.OrderBy {
				vi := rows[i][ob.Column]
				vj := rows[j][ob.Column]
				cmp := compareValues(vi, vj)
				if cmp == 0 {
					continue
				}
				if ob.Desc {
					return cmp > 0
				}
				return cmp < 0
			}
			return false
		})
	}

	// Apply LIMIT
	if stmt.Limit >= 0 && stmt.Limit < len(rows) {
		rows = rows[:stmt.Limit]
	}

	// Determine output columns
	var outputColumns []string
	isSelectStar := false
	for _, col := range stmt.Columns {
		if _, ok := col.Expr.(*parser.StarExpr); ok {
			isSelectStar = true
			break
		}
	}

	if isSelectStar {
		// SELECT * — show all columns from the table
		for _, col := range table.Columns {
			outputColumns = append(outputColumns, col.Name)
		}
		// Add joined table columns
		for _, join := range stmt.Joins {
			joinTable, _ := e.Catalog.GetTable(join.TableName)
			if joinTable != nil {
				for _, col := range joinTable.Columns {
					outputColumns = append(outputColumns, join.TableName+"."+col.Name)
				}
			}
		}
	} else {
		for _, col := range stmt.Columns {
			switch expr := col.Expr.(type) {
			case *parser.ColumnRef:
				if expr.Table != "" {
					outputColumns = append(outputColumns, expr.Table+"."+expr.Column)
				} else {
					name := expr.Column
					if col.Alias != "" {
						name = col.Alias
					}
					outputColumns = append(outputColumns, name)
				}
			}
		}
	}

	return FormatTable(outputColumns, rows), nil
}

// ---------------------------------------------------------------------------
// UPDATE
// ---------------------------------------------------------------------------
func (e *Executor) executeUpdate(stmt *parser.UpdateStmt) (string, error) {
	table, err := e.Catalog.GetTable(stmt.TableName)
	if err != nil {
		return "", err
	}

	rows, err := ScanAllRows(e.Pager, table)
	if err != nil {
		return "", err
	}

	count := 0
	for _, row := range rows {
		// Check WHERE
		if stmt.Where != nil {
			match, err := evaluateBoolExpression(stmt.Where, row)
			if err != nil {
				return "", err
			}
			if !match {
				continue
			}
		}

		// Build updates map
		updates := make(map[string]interface{})
		for _, assign := range stmt.Assignments {
			val, err := evaluateExpression(assign.Value, row)
			if err != nil {
				return "", err
			}

			colIdx := table.GetColumnIndex(assign.Column)
			if colIdx == -1 {
				return "", fmt.Errorf("unknown column '%s'", assign.Column)
			}

			typedVal, err := coerceType(val, table.Columns[colIdx].DataType)
			if err != nil {
				return "", err
			}
			updates[assign.Column] = typedVal
		}

		// Get primary key value
		pkVal := row[table.PrimaryKey]
		if pkVal == nil {
			pkVal = row["_rowid"]
		}

		if err := UpdateRow(e.Pager, table, pkVal, updates); err != nil {
			return "", err
		}
		count++
	}

	return fmt.Sprintf("%d row(s) updated.", count), nil
}

// ---------------------------------------------------------------------------
// DELETE
// ---------------------------------------------------------------------------
func (e *Executor) executeDelete(stmt *parser.DeleteStmt) (string, error) {
	table, err := e.Catalog.GetTable(stmt.TableName)
	if err != nil {
		return "", err
	}

	rows, err := ScanAllRows(e.Pager, table)
	if err != nil {
		return "", err
	}

	count := 0
	for _, row := range rows {
		// Check WHERE
		if stmt.Where != nil {
			match, err := evaluateBoolExpression(stmt.Where, row)
			if err != nil {
				return "", err
			}
			if !match {
				continue
			}
		}

		// Get primary key value
		pkVal := row[table.PrimaryKey]
		if pkVal == nil {
			pkVal = row["_rowid"]
		}

		deleted, err := DeleteRow(e.Pager, table, pkVal)
		if err != nil {
			return "", err
		}
		if deleted {
			count++
		}
	}

	return fmt.Sprintf("%d row(s) deleted.", count), nil
}

// ---------------------------------------------------------------------------
// Expression evaluation
// ---------------------------------------------------------------------------

// evaluateExpression computes the value of an expression, given a row context.
func evaluateExpression(expr parser.Expression, row Row) (interface{}, error) {
	switch e := expr.(type) {
	case *parser.IntLiteral:
		return e.Value, nil
	case *parser.StringLiteral:
		return e.Value, nil
	case *parser.BoolLiteral:
		return e.Value, nil
	case *parser.NullLiteral:
		return nil, nil
	case *parser.ColumnRef:
		if row == nil {
			return nil, fmt.Errorf("cannot reference column '%s' without row context", e.Column)
		}
		// Try table.column first, then just column
		if e.Table != "" {
			key := e.Table + "." + e.Column
			if val, ok := row[key]; ok {
				return val, nil
			}
		}
		if val, ok := row[e.Column]; ok {
			return val, nil
		}
		// Column might exist but be NULL
		return nil, nil
	case *parser.BinaryExpr:
		return evaluateBinaryExpr(e, row)
	default:
		return nil, fmt.Errorf("unsupported expression type: %T", expr)
	}
}

// evaluateBoolExpression evaluates an expression that should return a boolean.
func evaluateBoolExpression(expr parser.Expression, row Row) (bool, error) {
	val, err := evaluateExpression(expr, row)
	if err != nil {
		return false, err
	}
	if b, ok := val.(bool); ok {
		return b, nil
	}
	return false, fmt.Errorf("expected boolean expression, got %T", val)
}

// evaluateBinaryExpr evaluates binary expressions (=, !=, <, >, AND, OR, etc.)
func evaluateBinaryExpr(expr *parser.BinaryExpr, row Row) (interface{}, error) {
	left, err := evaluateExpression(expr.Left, row)
	if err != nil {
		return nil, err
	}
	right, err := evaluateExpression(expr.Right, row)
	if err != nil {
		return nil, err
	}

	switch expr.Op {
	case "AND":
		lb, lok := left.(bool)
		rb, rok := right.(bool)
		if !lok || !rok {
			return false, nil
		}
		return lb && rb, nil
	case "OR":
		lb, lok := left.(bool)
		rb, rok := right.(bool)
		if !lok || !rok {
			return false, nil
		}
		return lb || rb, nil
	case "=":
		return compareValues(left, right) == 0, nil
	case "!=":
		return compareValues(left, right) != 0, nil
	case "<":
		return compareValues(left, right) < 0, nil
	case ">":
		return compareValues(left, right) > 0, nil
	case "<=":
		return compareValues(left, right) <= 0, nil
	case ">=":
		return compareValues(left, right) >= 0, nil
	default:
		return nil, fmt.Errorf("unsupported operator: %s", expr.Op)
	}
}

// compareValues compares two values. Returns -1, 0, or 1.
func compareValues(a, b interface{}) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Try numeric comparison
	ai := toInt64(a)
	bi := toInt64(b)
	if ai != nil && bi != nil {
		if *ai < *bi {
			return -1
		}
		if *ai > *bi {
			return 1
		}
		return 0
	}

	// Fall back to string comparison
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	return strings.Compare(as, bs)
}

func toInt64(v interface{}) *int64 {
	switch val := v.(type) {
	case int64:
		return &val
	case int:
		i := int64(val)
		return &i
	default:
		return nil
	}
}

// coerceType converts a value to match the expected column type.
func coerceType(val interface{}, dataType string) (interface{}, error) {
	if val == nil {
		return nil, nil
	}

	switch dataType {
	case "INT":
		switch v := val.(type) {
		case int64:
			return v, nil
		case int:
			return int64(v), nil
		default:
			return nil, fmt.Errorf("cannot convert %T to INT", val)
		}
	case "TEXT":
		switch v := val.(type) {
		case string:
			return v, nil
		default:
			return fmt.Sprintf("%v", v), nil
		}
	case "BOOL":
		switch v := val.(type) {
		case bool:
			return v, nil
		default:
			return nil, fmt.Errorf("cannot convert %T to BOOL", val)
		}
	default:
		return val, nil
	}
}

// hasMatchInJoin checks if a left row has any match in the join table (for LEFT JOIN).
func hasMatchInJoin(leftRow Row, joinRows []Row, join parser.JoinClause, leftTableName string) bool {
	for _, rightRow := range joinRows {
		merged := make(Row)
		for k, v := range leftRow {
			merged[k] = v
			merged[leftTableName+"."+k] = v
		}
		for k, v := range rightRow {
			merged[k] = v
			merged[join.TableName+"."+k] = v
		}
		match, err := evaluateBoolExpression(join.Condition, merged)
		if err == nil && match {
			return true
		}
	}
	return false
}
