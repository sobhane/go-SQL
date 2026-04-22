package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sobhane/golang-database/lexer"
)

// ==========================================================================
// PARSER — Converts Tokens Into an AST (Abstract Syntax Tree)
// ==========================================================================
//
// WHAT DOES THE PARSER DO?
// ────────────────────────
// The lexer gives us a flat list of tokens like:
//   [SELECT] [name] [FROM] [users] [WHERE] [id] [=] [1] [EOF]
//
// The parser reads these tokens left-to-right and builds a TREE:
//   SelectStmt{Columns: [name], Table: "users", Where: BinaryExpr{id, =, 1}}
//
// The executor then walks this tree to perform the actual database operation.
//
// PARSING TECHNIQUE: RECURSIVE DESCENT
// ─────────────────────────────────────
// This is the most popular parsing technique for SQL. The idea is elegant:
//
//   1. Each SQL grammar rule has its OWN FUNCTION.
//      - SELECT → parseSelect()
//      - INSERT → parseInsert()
//      - WHERE expressions → parseExpression()
//
//   2. Each function CONSUMES the tokens it needs, then RETURNS an AST node.
//
//   3. When a rule contains SUB-RULES, it CALLS those functions.
//      For example: parseSelect() calls parseExpression() for the WHERE clause.
//      That's the "recursive" part — functions calling other parsing functions.
//
// STEP-BY-STEP EXAMPLE: "SELECT name FROM users WHERE id = 1"
// ───────────────────────────────────────────────────────────
//   1. Parse() calls parseStatement()
//   2. parseStatement() sees the first token is SELECT → calls parseSelect()
//   3. parseSelect():
//      a. expect(SELECT) ✓ — advance past it
//      b. parseSelectColumns() → reads "name" → returns [ColumnRef("name")]
//      c. expect(FROM) ✓ — advance past it
//      d. reads "users" → TableName = "users"
//      e. sees WHERE → advance past it, calls parseExpression()
//   4. parseExpression():
//      a. calls parseOrExpression() → parseAndExpression() → parseComparison()
//      b. parseComparison() calls parsePrimaryExpression() → reads "id" → ColumnRef("id")
//      c. sees "=" → op = "="
//      d. parsePrimaryExpression() → reads "1" → IntLiteral(1)
//      e. returns BinaryExpr{ColumnRef("id"), "=", IntLiteral(1)}
//   5. parseSelect() assembles everything into a SelectStmt and returns it

// Parser holds the state needed to parse a token stream.
type Parser struct {
	tokens  []lexer.Token // the complete list of tokens from the lexer
	pos     int           // index of the current token we're examining (0-based)
	current lexer.Token   // shortcut: the token at tokens[pos] (avoids repeated indexing)
}

// New creates a new Parser from a slice of tokens.
//
// Usage:
//
//	l := lexer.New("SELECT * FROM persons;")
//	tokens := l.Tokenize()
//	p := parser.New(tokens)
//	stmt, err := p.Parse()
func New(tokens []lexer.Token) *Parser {
	p := &Parser{tokens: tokens, pos: 0}
	if len(tokens) > 0 {
		p.current = tokens[0] // load the first token so we're ready to parse
	}
	return p
}

// Parse is the main entry point. It parses the entire token stream and
// returns the resulting AST node (a Statement).
//
// Returns an error if the SQL is malformed.
func (p *Parser) Parse() (Statement, error) {
	return p.parseStatement()
}

// ==========================================================================
// TOKEN NAVIGATION — how the parser moves through tokens
// ==========================================================================
//
// The parser reads tokens LEFT TO RIGHT, one at a time.
// It NEVER goes backward. Three helpers control movement:
//
//   advance()  — Move to the next token (consume the current one)
//   peek()     — Look at the NEXT token without moving (lookahead)
//   expect(T)  — Assert current token is T, then advance. Error if not.

// advance moves the parser forward by one token.
//
// After calling advance():
//   - pos is incremented by 1
//   - current is updated to the new token at that position
//   - if we've gone past the end, current becomes EOF
//
// Example: if pos=2 and tokens=[SELECT, name, FROM, users, EOF]
//   before advance(): current = FROM (index 2)
//   after  advance(): current = users (index 3)
func (p *Parser) advance() {
	p.pos++
	if p.pos < len(p.tokens) {
		p.current = p.tokens[p.pos]
	} else {
		// We've reached the end of the token stream.
		// Set current to EOF so parsing logic can detect the end.
		p.current = lexer.Token{Type: lexer.TOKEN_EOF}
	}
}

// peek returns the NEXT token without advancing the parser position.
//
// This is "lookahead" — used when we need to see what's coming
// to decide what to do NOW, without actually consuming the token.
//
// Example use case in INSERT parsing:
//   "INSERT INTO persons (name, email) VALUES (...)"
//                        ^ current is '('
//   We peek() to see if the next token is an IDENT (column list)
//   or a literal (values). This tells us which path to take.
func (p *Parser) peek() lexer.Token {
	if p.pos+1 < len(p.tokens) {
		return p.tokens[p.pos+1]
	}
	return lexer.Token{Type: lexer.TOKEN_EOF}
}

// expect asserts that the current token is of the expected type, then advances.
//
// This is used to enforce SQL grammar rules. For example, a SELECT statement
// must have the word FROM after the column list. If it doesn't, that's a
// syntax error:
//
//   p.expect(lexer.TOKEN_FROM)
//     → if current is FROM: ✓ advance past it
//     → if current is anything else: return error
//       "expected FROM, got IDENT ("users") at position 4"
//
// The error message tells the user exactly what went wrong and where.
func (p *Parser) expect(t lexer.TokenType) error {
	if p.current.Type != t {
		return fmt.Errorf("expected %s, got %s (%q) at position %d",
			t.String(), p.current.Type.String(), p.current.Literal, p.pos)
	}
	p.advance()
	return nil
}

// ==========================================================================
// STATEMENT DISPATCH — look at the first token and parse accordingly
// ==========================================================================

// parseStatement looks at the first token to determine which SQL command
// to parse, then delegates to the appropriate parsing function.
//
// Token → Function mapping:
//   SELECT → parseSelect()
//   INSERT → parseInsert()
//   CREATE → parseCreate()
//   UPDATE → parseUpdate()
//   DELETE → parseDelete()
//   DROP   → parseDrop()
//   other  → error: "unexpected token"
func (p *Parser) parseStatement() (Statement, error) {
	switch p.current.Type {
	case lexer.TOKEN_SELECT:
		return p.parseSelect()
	case lexer.TOKEN_INSERT:
		return p.parseInsert()
	case lexer.TOKEN_CREATE:
		return p.parseCreate()
	case lexer.TOKEN_UPDATE:
		return p.parseUpdate()
	case lexer.TOKEN_DELETE:
		return p.parseDelete()
	case lexer.TOKEN_DROP:
		return p.parseDrop()
	default:
		return nil, fmt.Errorf("unexpected token: %s (%q)", p.current.Type.String(), p.current.Literal)
	}
}

// ==========================================================================
// SELECT — the most complex statement to parse
// ==========================================================================
//
// Full grammar:
//   SELECT <columns>
//   FROM <table>
//   [<join_type> JOIN <table> ON <condition>]   ← zero or more
//   [WHERE <condition>]                         ← optional
//   [ORDER BY <column> [ASC|DESC], ...]         ← optional
//   [LIMIT <integer>]                           ← optional
//   [;]                                         ← optional
//
// The parser reads each clause IN ORDER. Required parts use expect()
// (which errors if missing). Optional parts use "if" checks.

func (p *Parser) parseSelect() (*SelectStmt, error) {
	stmt := &SelectStmt{Limit: -1} // -1 means "no limit"

	// ── Step 1: Consume the "SELECT" keyword ──
	if err := p.expect(lexer.TOKEN_SELECT); err != nil {
		return nil, err
	}

	// ── Step 2: Parse the column list ──
	// Examples: "*", "name", "name, email", "users.name AS n"
	columns, err := p.parseSelectColumns()
	if err != nil {
		return nil, err
	}
	stmt.Columns = columns

	// ── Step 3: Consume the "FROM" keyword (required) ──
	if err := p.expect(lexer.TOKEN_FROM); err != nil {
		return nil, err
	}

	// ── Step 4: Read the table name ──
	if p.current.Type != lexer.TOKEN_IDENT {
		return nil, fmt.Errorf("expected table name, got %s", p.current.Type.String())
	}
	stmt.TableName = p.current.Literal
	p.advance()

	// ── Step 5: Parse optional JOINs (can have multiple) ──
	// Keep parsing JOINs as long as we see a join keyword.
	// Supports: JOIN, INNER JOIN, LEFT JOIN, RIGHT JOIN
	for p.current.Type == lexer.TOKEN_INNER || p.current.Type == lexer.TOKEN_LEFT ||
		p.current.Type == lexer.TOKEN_RIGHT || p.current.Type == lexer.TOKEN_JOIN {
		join, err := p.parseJoin()
		if err != nil {
			return nil, err
		}
		stmt.Joins = append(stmt.Joins, join)
	}

	// ── Step 6: Parse optional WHERE clause ──
	// If present, parse the expression that follows. Otherwise, Where stays nil.
	if p.current.Type == lexer.TOKEN_WHERE {
		p.advance() // skip the WHERE keyword
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		stmt.Where = expr
	}

	// ── Step 7: Parse optional ORDER BY clause ──
	if p.current.Type == lexer.TOKEN_ORDER {
		p.advance() // skip ORDER
		if err := p.expect(lexer.TOKEN_BY); err != nil {
			return nil, err
		}
		orderBy, err := p.parseOrderBy()
		if err != nil {
			return nil, err
		}
		stmt.OrderBy = orderBy
	}

	// ── Step 8: Parse optional LIMIT ──
	if p.current.Type == lexer.TOKEN_LIMIT {
		p.advance() // skip LIMIT
		if p.current.Type != lexer.TOKEN_INT_LIT {
			return nil, fmt.Errorf("expected integer after LIMIT, got %s", p.current.Type.String())
		}
		limit, err := strconv.Atoi(p.current.Literal)
		if err != nil {
			return nil, fmt.Errorf("invalid LIMIT value: %s", p.current.Literal)
		}
		stmt.Limit = limit
		p.advance()
	}

	// ── Step 9: Skip optional trailing semicolon ──
	if p.current.Type == lexer.TOKEN_SEMICOLON {
		p.advance()
	}

	return stmt, nil
}

// parseSelectColumns parses the comma-separated column list after SELECT.
//
// This uses the COMMA PATTERN — a loop that:
//   1. Parses one item
//   2. Checks for a comma
//   3. If comma → continue loop (more items)
//   4. If no comma → break (end of list)
//
// This same pattern is reused for VALUES lists, SET assignments,
// ORDER BY lists, and column definitions in CREATE TABLE.
//
// Supports:
//   "*"                → StarExpr (all columns)
//   "name"             → ColumnRef{Column: "name"}
//   "users.name"       → ColumnRef{Table: "users", Column: "name"}
//   "users.*"          → StarExpr (all columns from 'users')
//   "name AS full_name" → ColumnRef with Alias "full_name"
func (p *Parser) parseSelectColumns() ([]SelectColumn, error) {
	var columns []SelectColumn

	for {
		var col SelectColumn

		if p.current.Type == lexer.TOKEN_STAR {
			// ── Case 1: SELECT * ──
			col.Expr = &StarExpr{}
			p.advance()

		} else if p.current.Type == lexer.TOKEN_IDENT {
			// ── Case 2: SELECT column_name ──
			// Start by assuming it's a simple column reference like "name"
			ref := &ColumnRef{Column: p.current.Literal}
			p.advance()

			// Check for DOT notation: table.column or table.*
			// Example: "users.name" → the "users" part is actually the table,
			// and "name" is the column. We need to reclassify.
			if p.current.Type == lexer.TOKEN_DOT {
				p.advance() // skip the dot
				if p.current.Type != lexer.TOKEN_IDENT && p.current.Type != lexer.TOKEN_STAR {
					return nil, fmt.Errorf("expected column name after '.', got %s", p.current.Type.String())
				}
				if p.current.Type == lexer.TOKEN_STAR {
					// "users.*" → all columns from 'users' table
					col.Expr = &StarExpr{}
					ref.Table = ref.Column // what we thought was the column is actually the table
					p.advance()
				} else {
					// "users.name" → qualified column reference
					ref.Table = ref.Column         // "users" was the table
					ref.Column = p.current.Literal // "name" is the actual column
					p.advance()
				}
			}

			// If we haven't set col.Expr yet (no star), use the column ref
			if col.Expr == nil {
				col.Expr = ref
			}

		} else {
			return nil, fmt.Errorf("expected column name or *, got %s (%q)", p.current.Type.String(), p.current.Literal)
		}

		// ── Check for optional AS alias ──
		// Example: "SELECT name AS full_name FROM ..."
		if p.current.Type == lexer.TOKEN_AS {
			p.advance() // skip AS
			if p.current.Type != lexer.TOKEN_IDENT {
				return nil, fmt.Errorf("expected alias after AS, got %s", p.current.Type.String())
			}
			col.Alias = p.current.Literal
			p.advance()
		}

		columns = append(columns, col)

		// ── COMMA PATTERN: More columns? ──
		if p.current.Type != lexer.TOKEN_COMMA {
			break // no comma → end of column list
		}
		p.advance() // skip comma, loop to parse next column
	}

	return columns, nil
}

// parseJoin parses a single JOIN clause.
//
// Grammar:  [INNER|LEFT|RIGHT] JOIN tablename ON condition
//
// Called from parseSelect() when it detects a join keyword.
// The join type defaults to "INNER" if only "JOIN" is written.
func (p *Parser) parseJoin() (JoinClause, error) {
	join := JoinClause{JoinType: "INNER"} // default if just "JOIN" is used

	// ── Step 1: Determine join type ──
	// "INNER JOIN", "LEFT JOIN", "RIGHT JOIN", or just "JOIN"
	switch p.current.Type {
	case lexer.TOKEN_INNER:
		join.JoinType = "INNER"
		p.advance()
	case lexer.TOKEN_LEFT:
		join.JoinType = "LEFT"
		p.advance()
	case lexer.TOKEN_RIGHT:
		join.JoinType = "RIGHT"
		p.advance()
	}

	// ── Step 2: Consume "JOIN" keyword (required) ──
	if err := p.expect(lexer.TOKEN_JOIN); err != nil {
		return join, err
	}

	// ── Step 3: Read the joined table name ──
	if p.current.Type != lexer.TOKEN_IDENT {
		return join, fmt.Errorf("expected table name after JOIN, got %s", p.current.Type.String())
	}
	join.TableName = p.current.Literal
	p.advance()

	// ── Step 4: Consume "ON" keyword and parse the join condition ──
	// Example: ON persons.id = skills.person_id
	if err := p.expect(lexer.TOKEN_ON); err != nil {
		return join, err
	}

	cond, err := p.parseExpression()
	if err != nil {
		return join, err
	}
	join.Condition = cond

	return join, nil
}

// parseOrderBy parses the column list after ORDER BY.
//
// Grammar:  column [ASC|DESC] [, column [ASC|DESC], ...]
//
// Uses the same COMMA PATTERN as parseSelectColumns.
// ASC is the default if neither ASC nor DESC is specified.
func (p *Parser) parseOrderBy() ([]OrderByClause, error) {
	var clauses []OrderByClause

	for {
		// Read column name
		if p.current.Type != lexer.TOKEN_IDENT {
			return nil, fmt.Errorf("expected column name in ORDER BY, got %s", p.current.Type.String())
		}
		clause := OrderByClause{Column: p.current.Literal}
		p.advance()

		// Check for optional ASC/DESC keyword
		if p.current.Type == lexer.TOKEN_DESC {
			clause.Desc = true
			p.advance()
		} else if p.current.Type == lexer.TOKEN_ASC {
			// ASC is the default, but we still need to consume the token
			p.advance()
		}

		clauses = append(clauses, clause)

		// COMMA PATTERN: more columns?
		if p.current.Type != lexer.TOKEN_COMMA {
			break
		}
		p.advance()
	}

	return clauses, nil
}

// ==========================================================================
// INSERT INTO — add new rows
// ==========================================================================
//
// Grammar (two forms):
//   Form 1: INSERT INTO tablename VALUES (v1, v2, ...);
//   Form 2: INSERT INTO tablename (col1, col2) VALUES (v1, v2, ...);
//
// Form 2 has an explicit column list — we use peek() to distinguish
// the two forms (see step 3 below).

func (p *Parser) parseInsert() (*InsertStmt, error) {
	stmt := &InsertStmt{}

	// ── Step 1: Consume "INSERT INTO" ──
	if err := p.expect(lexer.TOKEN_INSERT); err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TOKEN_INTO); err != nil {
		return nil, err
	}

	// ── Step 2: Read the table name ──
	if p.current.Type != lexer.TOKEN_IDENT {
		return nil, fmt.Errorf("expected table name, got %s", p.current.Type.String())
	}
	stmt.TableName = p.current.Literal
	p.advance()

	// ── Step 3: Optional explicit column list ──
	// We need to distinguish between:
	//   INSERT INTO t (col1, col2) VALUES (...)   ← column list
	//   INSERT INTO t VALUES (1, 2, 3)            ← no column list
	//
	// Trick: if the current token is '(' AND the next token is an identifier
	// (not a number or string), then it must be a column list.
	// We use peek() to achieve this lookahead without consuming tokens.
	if p.current.Type == lexer.TOKEN_LPAREN && p.peek().Type == lexer.TOKEN_IDENT {
		p.advance() // skip the opening '('

		// COMMA PATTERN: read comma-separated column names
		for {
			if p.current.Type != lexer.TOKEN_IDENT {
				return nil, fmt.Errorf("expected column name, got %s", p.current.Type.String())
			}
			stmt.Columns = append(stmt.Columns, p.current.Literal)
			p.advance()
			if p.current.Type != lexer.TOKEN_COMMA {
				break
			}
			p.advance() // skip comma
		}
		if err := p.expect(lexer.TOKEN_RPAREN); err != nil {
			return nil, err
		}
	}

	// ── Step 4: Consume "VALUES (" ──
	if err := p.expect(lexer.TOKEN_VALUES); err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TOKEN_LPAREN); err != nil {
		return nil, err
	}

	// ── Step 5: Parse the value list ──
	// COMMA PATTERN: each value is parsed as a primary expression
	// (integers, strings, booleans, NULL — no complex expressions in VALUES)
	for {
		expr, err := p.parsePrimaryExpression()
		if err != nil {
			return nil, err
		}
		stmt.Values = append(stmt.Values, expr)

		if p.current.Type != lexer.TOKEN_COMMA {
			break
		}
		p.advance()
	}

	// ── Step 6: Consume closing ")" ──
	if err := p.expect(lexer.TOKEN_RPAREN); err != nil {
		return nil, err
	}

	// ── Step 7: Skip optional semicolon ──
	if p.current.Type == lexer.TOKEN_SEMICOLON {
		p.advance()
	}

	return stmt, nil
}

// ==========================================================================
// CREATE TABLE — define a new table schema
// ==========================================================================
//
// Grammar:
//   CREATE TABLE tablename (
//       col1 TYPE [PRIMARY KEY] [NOT NULL],
//       col2 TYPE [PRIMARY KEY] [NOT NULL],
//       ...
//       [PRIMARY KEY(col_name)]        ← table-level PK constraint
//   );
//
// Two ways to specify a primary key:
//   1. Inline:      "id INT PRIMARY KEY"  (parsed by parseColumnDef)
//   2. Table-level: "PRIMARY KEY(id)"     (parsed here in the main loop)

func (p *Parser) parseCreate() (*CreateTableStmt, error) {
	stmt := &CreateTableStmt{}

	// ── Step 1: Consume "CREATE TABLE" ──
	if err := p.expect(lexer.TOKEN_CREATE); err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TOKEN_TABLE); err != nil {
		return nil, err
	}

	// ── Step 2: Read table name ──
	if p.current.Type != lexer.TOKEN_IDENT {
		return nil, fmt.Errorf("expected table name, got %s", p.current.Type.String())
	}
	stmt.TableName = p.current.Literal
	p.advance()

	// ── Step 3: Consume opening "(" ──
	if err := p.expect(lexer.TOKEN_LPAREN); err != nil {
		return nil, err
	}

	// ── Step 4: Parse column definitions (COMMA PATTERN) ──
	for {
		// Check for table-level constraint: PRIMARY KEY(col_name)
		// This is an alternative to inline "col INT PRIMARY KEY"
		if p.current.Type == lexer.TOKEN_PRIMARY {
			p.advance() // skip PRIMARY
			if err := p.expect(lexer.TOKEN_KEY); err != nil {
				return nil, err
			}
			if err := p.expect(lexer.TOKEN_LPAREN); err != nil {
				return nil, err
			}
			if p.current.Type != lexer.TOKEN_IDENT {
				return nil, fmt.Errorf("expected column name in PRIMARY KEY(), got %s", p.current.Type.String())
			}
			stmt.PrimaryKey = p.current.Literal
			p.advance()
			if err := p.expect(lexer.TOKEN_RPAREN); err != nil {
				return nil, err
			}
		} else {
			// Parse a regular column definition: name TYPE [modifiers]
			col, err := p.parseColumnDef()
			if err != nil {
				return nil, err
			}
			stmt.Columns = append(stmt.Columns, col)

			// If this column was declared with inline PRIMARY KEY,
			// propagate it to the statement-level PrimaryKey field
			if col.PrimaryKey && stmt.PrimaryKey == "" {
				stmt.PrimaryKey = col.Name
			}
		}

		// COMMA PATTERN: more column definitions?
		if p.current.Type != lexer.TOKEN_COMMA {
			break
		}
		p.advance() // skip comma
	}

	// ── Step 5: Consume closing ")" ──
	if err := p.expect(lexer.TOKEN_RPAREN); err != nil {
		return nil, err
	}

	// ── Step 6: Skip optional semicolon ──
	if p.current.Type == lexer.TOKEN_SEMICOLON {
		p.advance()
	}

	return stmt, nil
}

// parseColumnDef parses a single column definition inside CREATE TABLE.
//
// Grammar: column_name DATA_TYPE [PRIMARY KEY] [NOT NULL]
//
// Examples:
//   "id INT PRIMARY KEY"    → ColumnDef{Name:"id", DataType:"INT", PrimaryKey:true}
//   "name TEXT NOT NULL"    → ColumnDef{Name:"name", DataType:"TEXT", NotNull:true}
//   "email TEXT"            → ColumnDef{Name:"email", DataType:"TEXT"}
//
// Modifiers (PRIMARY KEY, NOT NULL) can appear in any order and are optional.
func (p *Parser) parseColumnDef() (ColumnDef, error) {
	col := ColumnDef{}

	// ── Part 1: Column name (required) ──
	if p.current.Type != lexer.TOKEN_IDENT {
		return col, fmt.Errorf("expected column name, got %s (%q)", p.current.Type.String(), p.current.Literal)
	}
	col.Name = p.current.Literal
	p.advance()

	// ── Part 2: Data type (required) ──
	// Must be one of our three supported types: INT, TEXT, BOOL
	switch p.current.Type {
	case lexer.TOKEN_INT:
		col.DataType = "INT"
	case lexer.TOKEN_TEXT:
		col.DataType = "TEXT"
	case lexer.TOKEN_BOOL:
		col.DataType = "BOOL"
	default:
		return col, fmt.Errorf("expected data type (INT, TEXT, BOOL), got %s (%q)", p.current.Type.String(), p.current.Literal)
	}
	p.advance()

	// ── Part 3: Optional modifiers (loop until we see something else) ──
	// Modifiers can appear in any order:
	//   "PRIMARY KEY NOT NULL" ✓
	//   "NOT NULL PRIMARY KEY" ✓
	for {
		if p.current.Type == lexer.TOKEN_PRIMARY {
			// "PRIMARY KEY" — two tokens that form one modifier
			p.advance() // skip PRIMARY
			if err := p.expect(lexer.TOKEN_KEY); err != nil {
				return col, err
			}
			col.PrimaryKey = true
		} else if p.current.Type == lexer.TOKEN_NOT {
			// "NOT NULL" — two tokens that form one modifier
			p.advance() // skip NOT
			if err := p.expect(lexer.TOKEN_NULL); err != nil {
				return col, err
			}
			col.NotNull = true
		} else {
			break // no more modifiers — done with this column
		}
	}

	return col, nil
}

// ==========================================================================
// UPDATE — modify existing rows
// ==========================================================================
//
// Grammar: UPDATE tablename SET col1 = val1 [, col2 = val2, ...] [WHERE condition];

func (p *Parser) parseUpdate() (*UpdateStmt, error) {
	stmt := &UpdateStmt{}

	// ── Step 1: Consume "UPDATE" ──
	if err := p.expect(lexer.TOKEN_UPDATE); err != nil {
		return nil, err
	}

	// ── Step 2: Read table name ──
	if p.current.Type != lexer.TOKEN_IDENT {
		return nil, fmt.Errorf("expected table name, got %s", p.current.Type.String())
	}
	stmt.TableName = p.current.Literal
	p.advance()

	// ── Step 3: Consume "SET" ──
	if err := p.expect(lexer.TOKEN_SET); err != nil {
		return nil, err
	}

	// ── Step 4: Parse assignments (COMMA PATTERN) ──
	// Each assignment is: column_name = value
	// Example: "SET email = 'new@email.com', name = 'Alice'"
	for {
		// Read column name
		if p.current.Type != lexer.TOKEN_IDENT {
			return nil, fmt.Errorf("expected column name in SET, got %s", p.current.Type.String())
		}
		colName := p.current.Literal
		p.advance()

		// Consume "=" (required between column and value)
		if err := p.expect(lexer.TOKEN_EQ); err != nil {
			return nil, err
		}

		// Parse the new value (a primary expression: literal, NULL, etc.)
		val, err := p.parsePrimaryExpression()
		if err != nil {
			return nil, err
		}

		stmt.Assignments = append(stmt.Assignments, Assignment{Column: colName, Value: val})

		// COMMA PATTERN: more assignments?
		if p.current.Type != lexer.TOKEN_COMMA {
			break
		}
		p.advance()
	}

	// ── Step 5: Parse optional WHERE clause ──
	if p.current.Type == lexer.TOKEN_WHERE {
		p.advance()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		stmt.Where = expr
	}

	// ── Step 6: Skip optional semicolon ──
	if p.current.Type == lexer.TOKEN_SEMICOLON {
		p.advance()
	}

	return stmt, nil
}

// ==========================================================================
// DELETE — remove rows
// ==========================================================================
//
// Grammar: DELETE FROM tablename [WHERE condition];
//
// This is one of the simplest statements to parse.

func (p *Parser) parseDelete() (*DeleteStmt, error) {
	stmt := &DeleteStmt{}

	// ── Step 1: Consume "DELETE FROM" ──
	if err := p.expect(lexer.TOKEN_DELETE); err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TOKEN_FROM); err != nil {
		return nil, err
	}

	// ── Step 2: Read table name ──
	if p.current.Type != lexer.TOKEN_IDENT {
		return nil, fmt.Errorf("expected table name, got %s", p.current.Type.String())
	}
	stmt.TableName = p.current.Literal
	p.advance()

	// ── Step 3: Parse optional WHERE clause ──
	if p.current.Type == lexer.TOKEN_WHERE {
		p.advance()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		stmt.Where = expr
	}

	// ── Step 4: Skip optional semicolon ──
	if p.current.Type == lexer.TOKEN_SEMICOLON {
		p.advance()
	}

	return stmt, nil
}

// ==========================================================================
// DROP TABLE — delete a table
// ==========================================================================
//
// Grammar: DROP TABLE tablename;
//
// The simplest statement — just two keywords and a name.

func (p *Parser) parseDrop() (*DropTableStmt, error) {
	stmt := &DropTableStmt{}

	// Consume "DROP TABLE"
	if err := p.expect(lexer.TOKEN_DROP); err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TOKEN_TABLE); err != nil {
		return nil, err
	}

	// Read table name
	if p.current.Type != lexer.TOKEN_IDENT {
		return nil, fmt.Errorf("expected table name, got %s", p.current.Type.String())
	}
	stmt.TableName = p.current.Literal
	p.advance()

	// Skip optional semicolon
	if p.current.Type == lexer.TOKEN_SEMICOLON {
		p.advance()
	}

	return stmt, nil
}

// ==========================================================================
// EXPRESSION PARSING — the heart of WHERE clauses
// ==========================================================================
//
// Expressions are things like: id = 1, age > 18 AND name = 'Alice'
//
// THE CHALLENGE: OPERATOR PRECEDENCE
// ──────────────────────────────────
// In math, * is evaluated before + (2 + 3 * 4 = 14, not 20).
// In SQL, AND is evaluated before OR:
//
//   WHERE a = 1 OR b = 2 AND c = 3
//   means: a = 1 OR (b = 2 AND c = 3)    ← AND binds tighter
//   NOT:   (a = 1 OR b = 2) AND c = 3    ← wrong!
//
// PRECEDENCE LEVELS (lowest → highest):
//   Level 1: OR                   ← evaluated LAST  (weakest binding)
//   Level 2: AND                  ← evaluated second
//   Level 3: = != < > <= >=       ← evaluated third
//   Level 4: literals, columns, () ← evaluated FIRST (strongest binding)
//
// HOW WE ENFORCE THIS:
// Each level has its own function. Each function calls the NEXT HIGHER level.
// This naturally builds the tree with correct structure:
//
//   parseExpression()          ← entry point
//     └─ parseOrExpression()   ← Level 1: handles OR
//          └─ parseAndExpression()  ← Level 2: handles AND
//               └─ parseComparison()    ← Level 3: handles = != < > <= >=
//                    └─ parsePrimaryExpression()  ← Level 4: values
//
// WHY DOES THIS WORK?
// Higher-precedence operators are parsed DEEPER in the recursion,
// so they end up LOWER in the tree. And the executor evaluates
// bottom-up, so deeper nodes are evaluated FIRST.
//
// EXAMPLE: "a = 1 OR b = 2 AND c = 3"
//
//   parseOrExpression() handles the OR, splitting into:
//     Left:  "a = 1"  (parsed by parseAndExpression → parseComparison)
//     Right: "b = 2 AND c = 3" (parsed by parseAndExpression, which handles AND)
//
//   Result tree:
//        OR
//       /  \
//    (a=1)  AND
//          /   \
//       (b=2) (c=3)
//
//   Executor evaluates bottom-up: first (b=2), then (c=3), then AND, then (a=1), then OR.
//   So AND is evaluated before OR. ✓

// parseExpression is the entry point for all expression parsing.
// It simply delegates to parseOrExpression (the lowest-precedence level).
func (p *Parser) parseExpression() (Expression, error) {
	return p.parseOrExpression()
}

// parseOrExpression handles the OR operator (Level 1 — lowest precedence).
//
// Grammar: and_expr [OR and_expr] [OR and_expr] ...
//
// It parses the left side first (calling the next-higher level),
// then keeps consuming OR and right sides in a loop.
//
// The loop makes OR left-associative:
//   "a OR b OR c" → OR(OR(a, b), c)
func (p *Parser) parseOrExpression() (Expression, error) {
	// Parse left side by calling the next-higher precedence level
	left, err := p.parseAndExpression()
	if err != nil {
		return nil, err
	}

	// Keep consuming OR operators
	for p.current.Type == lexer.TOKEN_OR {
		p.advance() // skip OR
		right, err := p.parseAndExpression()
		if err != nil {
			return nil, err
		}
		// Wrap left and right into a BinaryExpr, then make IT the new left.
		// This builds a left-leaning tree if there are multiple ORs.
		left = &BinaryExpr{Left: left, Op: "OR", Right: right}
	}

	return left, nil
}

// parseAndExpression handles the AND operator (Level 2).
//
// Grammar: comparison [AND comparison] [AND comparison] ...
//
// Same pattern as parseOrExpression, but one level higher.
// Since parseOrExpression calls this function, AND automatically
// binds tighter than OR.
func (p *Parser) parseAndExpression() (Expression, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}

	for p.current.Type == lexer.TOKEN_AND {
		p.advance() // skip AND
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Op: "AND", Right: right}
	}

	return left, nil
}

// parseComparison handles comparison operators (Level 3).
//
// Grammar: primary [OP primary]   where OP is = != < > <= >=
//
// Unlike OR and AND, comparisons are NOT chained in a loop.
// "a = 1 = 2" doesn't make sense in SQL — it's just one comparison.
// So we parse: left, optional operator, optional right.
func (p *Parser) parseComparison() (Expression, error) {
	// Parse the left side (a primary expression: literal, column, or parenthesized)
	left, err := p.parsePrimaryExpression()
	if err != nil {
		return nil, err
	}

	// Check if there's a comparison operator after the left side
	var op string
	switch p.current.Type {
	case lexer.TOKEN_EQ:
		op = "="
	case lexer.TOKEN_NEQ:
		op = "!="
	case lexer.TOKEN_LT:
		op = "<"
	case lexer.TOKEN_GT:
		op = ">"
	case lexer.TOKEN_LTE:
		op = "<="
	case lexer.TOKEN_GTE:
		op = ">="
	default:
		// No comparison operator found.
		// This is valid — it means we just have a standalone primary expression.
		// Example: in "WHERE active", there's no operator, just a column ref.
		return left, nil
	}

	// Consume the operator and parse the right side
	p.advance()
	right, err := p.parsePrimaryExpression()
	if err != nil {
		return nil, err
	}

	return &BinaryExpr{Left: left, Op: op, Right: right}, nil
}

// parsePrimaryExpression handles the highest-precedence expressions (Level 4).
// These are the ATOMIC building blocks — things that can't be broken down further.
//
// What it handles:
//   42               → IntLiteral{Value: 42}
//   'hello'          → StringLiteral{Value: "hello"}
//   TRUE / FALSE     → BoolLiteral{Value: true/false}
//   NULL             → NullLiteral{}
//   column_name      → ColumnRef{Column: "column_name"}
//   table.column     → ColumnRef{Table: "table", Column: "column"}
//   (sub_expression) → recursively calls parseExpression()
//   *                → StarExpr{}
//
// PARENTHESES are handled here! When we see '(', we recurse into
// parseExpression() and parse whatever is inside. This lets users
// override precedence: "(a OR b) AND c" evaluates OR before AND.
func (p *Parser) parsePrimaryExpression() (Expression, error) {
	switch p.current.Type {

	// ── Integer literal: 42, 100, 0 ──
	case lexer.TOKEN_INT_LIT:
		val, err := strconv.ParseInt(p.current.Literal, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer: %s", p.current.Literal)
		}
		p.advance()
		return &IntLiteral{Value: val}, nil

	// ── String literal: 'hello', 'Go Developer' ──
	case lexer.TOKEN_STRING_LIT:
		val := p.current.Literal
		p.advance()
		return &StringLiteral{Value: val}, nil

	// ── Boolean literals ──
	case lexer.TOKEN_TRUE:
		p.advance()
		return &BoolLiteral{Value: true}, nil

	case lexer.TOKEN_FALSE:
		p.advance()
		return &BoolLiteral{Value: false}, nil

	// ── NULL literal ──
	case lexer.TOKEN_NULL:
		p.advance()
		return &NullLiteral{}, nil

	// ── Column reference: "name" or "table.name" ──
	case lexer.TOKEN_IDENT:
		name := p.current.Literal
		p.advance()

		// Check for DOT → qualified name like "persons.id"
		if p.current.Type == lexer.TOKEN_DOT {
			p.advance() // skip the dot
			if p.current.Type != lexer.TOKEN_IDENT {
				return nil, fmt.Errorf("expected column name after '.', got %s", p.current.Type.String())
			}
			colName := p.current.Literal
			p.advance()
			// "persons" was the table, "id" is the column
			return &ColumnRef{Table: name, Column: colName}, nil
		}

		// Simple column reference (no table qualifier)
		return &ColumnRef{Column: strings.ToLower(name)}, nil

	// ── Parenthesized expression: (sub_expression) ──
	// This is what makes "WHERE (a OR b) AND c" work!
	// We recurse into parseExpression() to parse the inner content,
	// then expect a closing ')'.
	case lexer.TOKEN_LPAREN:
		p.advance() // skip '('
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expect(lexer.TOKEN_RPAREN); err != nil {
			return nil, err
		}
		return expr, nil

	// ── Star wildcard: * ──
	case lexer.TOKEN_STAR:
		p.advance()
		return &StarExpr{}, nil

	// ── Anything else is an error ──
	default:
		return nil, fmt.Errorf("unexpected token in expression: %s (%q)", p.current.Type.String(), p.current.Literal)
	}
}
