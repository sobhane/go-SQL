package parser

// ==========================================================================
// AST (Abstract Syntax Tree) — The Output of the Parser
// ==========================================================================
//
// WHAT IS AN AST?
// ───────────────
// When you type SQL like "SELECT name FROM users WHERE id = 1", it's just
// a string of characters. The computer can't execute a string directly.
// It needs a STRUCTURED REPRESENTATION — that's the AST.
//
// The parser converts flat text into a TREE of nodes. Each node represents
// one meaningful piece of the SQL statement. The executor then walks this
// tree to actually perform the database operation.
//
// EXAMPLE:
// ────────
// SQL:  "SELECT name FROM users WHERE id = 1"
//
// AST:
//   SelectStmt
//   ├── Columns: [ColumnRef("name")]
//   ├── TableName: "users"
//   └── Where: BinaryExpr
//       ├── Left:  ColumnRef("id")
//       ├── Op:    "="
//       └── Right: IntLiteral(1)
//
// WHY TWO INTERFACES?
// ───────────────────
// Everything in our AST is either:
//   1. A Statement — a complete SQL command (SELECT, INSERT, CREATE, etc.)
//   2. An Expression — a value or condition (42, 'hello', id = 1, etc.)
//
// Statements CONTAIN expressions (e.g. a SELECT has a WHERE expression),
// but expressions never contain statements.

// Statement is the interface that all SQL command nodes implement.
//
// The statementNode() method is a "marker" — it has no logic. Its only purpose
// is type safety: Go's compiler won't let you accidentally pass an Expression
// where a Statement is expected, and vice versa.
//
// Types that implement Statement:
//   - *CreateTableStmt   (CREATE TABLE ...)
//   - *DropTableStmt     (DROP TABLE ...)
//   - *InsertStmt        (INSERT INTO ...)
//   - *SelectStmt        (SELECT ... FROM ...)
//   - *UpdateStmt        (UPDATE ... SET ...)
//   - *DeleteStmt        (DELETE FROM ...)
type Statement interface {
	statementNode()
}

// Expression is the interface for all value/condition nodes.
//
// Expressions appear inside statements — in WHERE clauses, VALUES lists,
// and SET assignments. They can be NESTED: a BinaryExpr contains two
// sub-expressions, which can themselves be BinaryExprs, and so on.
//
// Types that implement Expression:
//   - *IntLiteral        (42)
//   - *StringLiteral     ('hello')
//   - *BoolLiteral       (TRUE / FALSE)
//   - *NullLiteral       (NULL)
//   - *ColumnRef         (name, users.name)
//   - *StarExpr          (*)
//   - *BinaryExpr        (id = 1, age > 18, x AND y)
type Expression interface {
	expressionNode()
}

// ==========================================================================
// COLUMN DEFINITION — used exclusively in CREATE TABLE
// ==========================================================================

// ColumnDef represents one column definition inside a CREATE TABLE statement.
//
// SQL syntax:  column_name DATA_TYPE [PRIMARY KEY] [NOT NULL]
//
// Examples:
//   "id INT PRIMARY KEY"       → ColumnDef{Name:"id",    DataType:"INT",  PrimaryKey:true,  NotNull:false}
//   "name TEXT NOT NULL"       → ColumnDef{Name:"name",  DataType:"TEXT", PrimaryKey:false, NotNull:true}
//   "email TEXT"               → ColumnDef{Name:"email", DataType:"TEXT", PrimaryKey:false, NotNull:false}
//   "active BOOL PRIMARY KEY NOT NULL" → ColumnDef{Name:"active", DataType:"BOOL", PrimaryKey:true, NotNull:true}
type ColumnDef struct {
	Name       string // the column name, e.g. "id", "first_name"
	DataType   string // one of: "INT", "TEXT", "BOOL"
	PrimaryKey bool   // true if this column has the PRIMARY KEY constraint
	NotNull    bool   // true if this column has the NOT NULL constraint
}

// ==========================================================================
// SQL STATEMENT NODES — one struct per SQL command
// ==========================================================================

// CreateTableStmt represents a CREATE TABLE command.
//
// SQL syntax:
//   CREATE TABLE tablename (
//       col1 TYPE [constraints],
//       col2 TYPE [constraints],
//       [PRIMARY KEY(col_name)]
//   );
//
// Example:
//   CREATE TABLE persons (id INT PRIMARY KEY, name TEXT NOT NULL, email TEXT);
//
//   → CreateTableStmt{
//       TableName:  "persons",
//       Columns:    [{Name:"id", DataType:"INT", PrimaryKey:true}, ...],
//       PrimaryKey: "id",
//     }
//
// The PrimaryKey field is set when either:
//   - A column is declared with inline PRIMARY KEY:  "id INT PRIMARY KEY"
//   - A table-level constraint is used:              "PRIMARY KEY(id)"
type CreateTableStmt struct {
	TableName  string      // the name of the table to create
	Columns    []ColumnDef // the list of column definitions
	PrimaryKey string      // name of the primary key column (from either inline or table-level)
}

func (s *CreateTableStmt) statementNode() {}

// DropTableStmt represents a DROP TABLE command.
//
// SQL syntax:  DROP TABLE tablename;
// Example:    DROP TABLE persons;
//
//   → DropTableStmt{TableName: "persons"}
type DropTableStmt struct {
	TableName string // the name of the table to drop
}

func (s *DropTableStmt) statementNode() {}

// InsertStmt represents an INSERT INTO command.
//
// SQL syntax (two forms):
//   Form 1: INSERT INTO tablename VALUES (v1, v2, ...);
//   Form 2: INSERT INTO tablename (col1, col2) VALUES (v1, v2, ...);
//
// Example (form 1):
//   INSERT INTO persons VALUES (1, 'Sobhane', 'sobhane@email.com');
//
//   → InsertStmt{
//       TableName: "persons",
//       Columns:   nil,  (empty — means "use all columns in table order")
//       Values:    [IntLiteral(1), StringLiteral("Sobhane"), StringLiteral("sobhane@email.com")],
//     }
//
// Example (form 2):
//   INSERT INTO persons (name, email) VALUES ('Sobhane', 'sobhane@email.com');
//
//   → InsertStmt{
//       TableName: "persons",
//       Columns:   ["name", "email"],
//       Values:    [StringLiteral("Sobhane"), StringLiteral("sobhane@email.com")],
//     }
type InsertStmt struct {
	TableName string       // the table to insert into
	Columns   []string     // optional explicit column list; nil means "all columns"
	Values    []Expression // the values to insert (one Expression per column)
}

func (s *InsertStmt) statementNode() {}

// SelectStmt represents a SELECT query — the most complex statement.
//
// SQL syntax:
//   SELECT columns
//   FROM tablename
//   [JOIN tablename ON condition]     ← optional, can repeat
//   [WHERE condition]                 ← optional
//   [ORDER BY column [ASC|DESC]]      ← optional
//   [LIMIT n]                         ← optional
//
// Example:
//   SELECT first_name, email FROM persons WHERE id = 1 ORDER BY first_name LIMIT 10;
//
//   → SelectStmt{
//       Columns:   [{Expr: ColumnRef("first_name")}, {Expr: ColumnRef("email")}],
//       TableName: "persons",
//       Where:     BinaryExpr{ColumnRef("id"), "=", IntLiteral(1)},
//       OrderBy:   [{Column: "first_name", Desc: false}],
//       Limit:     10,
//       Joins:     nil,
//     }
type SelectStmt struct {
	Columns   []SelectColumn  // which columns to return; a StarExpr means SELECT *
	TableName string          // the main table to query (after FROM)
	Where     Expression      // the filter condition; nil if no WHERE clause
	OrderBy   []OrderByClause // sort rules; empty if no ORDER BY
	Limit     int             // max rows to return; -1 means "no limit"
	Joins     []JoinClause    // list of JOINs; empty if no JOINs
}

func (s *SelectStmt) statementNode() {}

// SelectColumn represents one item in the SELECT column list.
//
// Examples:
//   "name"              → SelectColumn{Expr: ColumnRef("name"),          Alias: ""}
//   "*"                 → SelectColumn{Expr: StarExpr{},                 Alias: ""}
//   "name AS full_name" → SelectColumn{Expr: ColumnRef("name"),          Alias: "full_name"}
//   "users.name"        → SelectColumn{Expr: ColumnRef("users","name"),  Alias: ""}
type SelectColumn struct {
	Expr  Expression // can be ColumnRef, StarExpr, or any expression
	Alias string     // the AS alias; empty string if no alias was specified
}

// OrderByClause represents one column in an ORDER BY clause.
//
// Examples:
//   "ORDER BY name"       → OrderByClause{Column: "name", Desc: false}
//   "ORDER BY name ASC"   → OrderByClause{Column: "name", Desc: false}
//   "ORDER BY name DESC"  → OrderByClause{Column: "name", Desc: true}
type OrderByClause struct {
	Column string // the column name to sort by
	Desc   bool   // true = descending order, false = ascending (the default)
}

// JoinClause represents a JOIN between two tables.
//
// SQL syntax:  [INNER|LEFT|RIGHT] JOIN tablename ON condition
//
// Example:
//   LEFT JOIN skills ON persons.id = skills.person_id
//
//   → JoinClause{
//       JoinType:  "LEFT",
//       TableName: "skills",
//       Condition: BinaryExpr{ColumnRef("persons.id"), "=", ColumnRef("skills.person_id")},
//     }
type JoinClause struct {
	JoinType  string     // "INNER", "LEFT", or "RIGHT"
	TableName string     // the table to join with
	Condition Expression // the ON condition (usually a BinaryExpr comparing two columns)
}

// UpdateStmt represents an UPDATE command.
//
// SQL syntax:  UPDATE tablename SET col1 = val1, col2 = val2 [WHERE condition];
//
// Example:
//   UPDATE persons SET email = 'new@email.com' WHERE id = 1;
//
//   → UpdateStmt{
//       TableName:   "persons",
//       Assignments: [{Column:"email", Value:StringLiteral("new@email.com")}],
//       Where:       BinaryExpr{ColumnRef("id"), "=", IntLiteral(1)},
//     }
//
// WARNING: If Where is nil, ALL rows in the table will be updated!
type UpdateStmt struct {
	TableName   string       // the table to update
	Assignments []Assignment // list of "column = value" pairs
	Where       Expression   // filter for which rows to update; nil = all rows
}

func (s *UpdateStmt) statementNode() {}

// Assignment represents a single "column = value" pair in an UPDATE SET clause.
//
// Example:  "email = 'new@email.com'"
//   → Assignment{Column: "email", Value: StringLiteral("new@email.com")}
type Assignment struct {
	Column string     // the column to update
	Value  Expression // the new value to assign
}

// DeleteStmt represents a DELETE command.
//
// SQL syntax:  DELETE FROM tablename [WHERE condition];
//
// Example:
//   DELETE FROM persons WHERE id = 1;
//
//   → DeleteStmt{
//       TableName: "persons",
//       Where:     BinaryExpr{ColumnRef("id"), "=", IntLiteral(1)},
//     }
//
// WARNING: If Where is nil, ALL rows in the table will be deleted!
type DeleteStmt struct {
	TableName string     // the table to delete from
	Where     Expression // filter for which rows to delete; nil = all rows
}

func (s *DeleteStmt) statementNode() {}

// ==========================================================================
// EXPRESSION NODES — the building blocks of WHERE, VALUES, and SET
// ==========================================================================

// IntLiteral represents a literal integer value in SQL.
//
// Examples:
//   42    → IntLiteral{Value: 42}
//   0     → IntLiteral{Value: 0}
//   -1    → IntLiteral{Value: -1}  (if negative numbers are supported)
type IntLiteral struct {
	Value int64 // the numeric value
}

func (e *IntLiteral) expressionNode() {}

// StringLiteral represents a single-quoted string value in SQL.
//
// Examples:
//   'hello'       → StringLiteral{Value: "hello"}
//   'Go Developer' → StringLiteral{Value: "Go Developer"}
//
// Note: In SQL, strings are delimited by single quotes, not double quotes.
type StringLiteral struct {
	Value string // the string content (without the surrounding quotes)
}

func (e *StringLiteral) expressionNode() {}

// BoolLiteral represents a TRUE or FALSE value in SQL.
//
// Examples:
//   TRUE  → BoolLiteral{Value: true}
//   FALSE → BoolLiteral{Value: false}
type BoolLiteral struct {
	Value bool
}

func (e *BoolLiteral) expressionNode() {}

// NullLiteral represents the SQL NULL value (absence of a value).
//
// Example:
//   NULL → NullLiteral{}
type NullLiteral struct{}

func (e *NullLiteral) expressionNode() {}

// ColumnRef represents a reference to a table column.
//
// Can be a simple column name or a qualified table.column name:
//   "name"        → ColumnRef{Table: "",      Column: "name"}
//   "users.name"  → ColumnRef{Table: "users", Column: "name"}
//
// The Table field is empty when no table qualifier is used.
type ColumnRef struct {
	Table  string // optional: the table name (before the dot); empty if unqualified
	Column string // the column name (after the dot, or the entire identifier)
}

func (e *ColumnRef) expressionNode() {}

// StarExpr represents the * wildcard, used in "SELECT *" to mean "all columns".
//
// Example:
//   SELECT * FROM persons;   → the Columns list will contain a StarExpr
type StarExpr struct{}

func (e *StarExpr) expressionNode() {}

// BinaryExpr represents a binary operation: two expressions connected by an operator.
//
// This is the most important expression node because it handles BOTH:
//   1. Comparisons:   id = 1,  age > 18,  name != 'Alice'
//   2. Logical ops:   (age > 18) AND (name = 'Alice')
//
// Because Left and Right are themselves Expressions, BinaryExpr nodes can
// NEST to form complex conditions:
//
//   "age > 18 AND name = 'Alice'"
//
//   → BinaryExpr{
//       Left:  BinaryExpr{ColumnRef("age"), ">",   IntLiteral(18)},
//       Op:    "AND",
//       Right: BinaryExpr{ColumnRef("name"), "=",  StringLiteral("Alice")},
//     }
//
// Supported operators: "=", "!=", "<", ">", "<=", ">=", "AND", "OR"
type BinaryExpr struct {
	Left  Expression // the left-hand side
	Op    string     // the operator
	Right Expression // the right-hand side
}

func (e *BinaryExpr) expressionNode() {}
