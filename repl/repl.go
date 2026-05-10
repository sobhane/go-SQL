package repl

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/sobhane/golang-database/engine"
	"github.com/sobhane/golang-database/lexer"
	"github.com/sobhane/golang-database/parser"
)

// REPL — Read-Eval-Print Loop
//
// This is the interactive SQL shell. It works like this:
// 1. READ  — Print "mydb> " and read a line of SQL from the user
// 2. EVAL  — Lex → Parse → Execute the SQL
// 3. PRINT — Show the result (table, message, or error)
// 4. LOOP  — Go back to step 1
//
// Special commands start with a dot (.help, .tables, .quit)

// REPL holds the state for the interactive SQL shell.
type REPL struct {
	executor *engine.Executor
}

// New creates a new REPL with the given executor.
func New(executor *engine.Executor) *REPL {
	return &REPL{executor: executor}
}

// Start begins the interactive REPL loop.
func (r *REPL) Start(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)

	fmt.Fprintln(out, "Welcome to MyDB — Your own SQL database engine built in Go!")
	fmt.Fprintln(out, "Type .help for help, .quit to exit.")
	fmt.Fprintln(out)

	for {
		fmt.Fprint(out, "mydb> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle dot commands
		if strings.HasPrefix(input, ".") {
			r.handleDotCommand(input, out)
			continue
		}

		// Execute SQL
		result, err := r.ExecuteSQL(input)
		if err != nil {
			fmt.Fprintf(out, "Error: %s\n", err)
		} else {
			fmt.Fprintln(out, result)
		}
	}
}

// ExecuteSQL lexes, parses, and executes a SQL string.
func (r *REPL) ExecuteSQL(input string) (string, error) {
	// Step 1: Lex (tokenize)
	l := lexer.New(input)
	tokens := l.Tokenize()

	// Step 2: Parse (tokens → AST)
	p := parser.New(tokens)
	stmt, err := p.Parse()
	if err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	// Step 3: Execute (AST → result)
	result, err := r.executor.Execute(stmt)
	if err != nil {
		return "", fmt.Errorf("execution error: %w", err)
	}

	return result, nil
}

// handleDotCommand processes special dot commands.
func (r *REPL) handleDotCommand(input string, out io.Writer) {
	switch strings.ToLower(input) {
	case ".help":
		fmt.Fprintln(out, `
Available commands:
  .help     Show this help message
  .tables   List all tables
  .schema   Show schema for all tables
  .quit     Exit the REPL

SQL commands:
  CREATE TABLE name (col1 TYPE, col2 TYPE, ...);
  INSERT INTO name VALUES (v1, v2, ...);
  SELECT col1, col2 FROM name WHERE condition;
  UPDATE name SET col = val WHERE condition;
  DELETE FROM name WHERE condition;
  DROP TABLE name;

Data types: INT, TEXT, BOOL
Operators:  =, !=, <, >, <=, >=, AND, OR`)

	case ".tables":
		if len(r.executor.Catalog.Tables) == 0 {
			fmt.Fprintln(out, "No tables found.")
			return
		}
		for name := range r.executor.Catalog.Tables {
			fmt.Fprintf(out, "  %s\n", name)
		}

	case ".schema":
		if len(r.executor.Catalog.Tables) == 0 {
			fmt.Fprintln(out, "No tables found.")
			return
		}
		for _, table := range r.executor.Catalog.Tables {
			fmt.Fprintf(out, "\nTable: %s\n", table.Name)
			fmt.Fprintf(out, "  Primary Key: %s\n", table.PrimaryKey)
			fmt.Fprintln(out, "  Columns:")
			for _, col := range table.Columns {
				flags := ""
				if col.PrimaryKey {
					flags += " PRIMARY KEY"
				}
				if col.NotNull {
					flags += " NOT NULL"
				}
				fmt.Fprintf(out, "    %-20s %-6s%s\n", col.Name, col.DataType, flags)
			}
		}

	case ".quit", ".exit":
		fmt.Fprintln(out, "Goodbye!")
		// The caller should handle exit — we'll signal via a special return
		// For now, we just print. main.go handles os.Exit.
		panic("quit") // caught by main

	default:
		fmt.Fprintf(out, "Unknown command: %s. Type .help for help.\n", input)
	}
}
