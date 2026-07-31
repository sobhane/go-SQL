# 🗄️ GoSQL — A SQL Database Engine Built from Scratch in Go

A fully functional relational database engine written in pure Go with **zero external dependencies**. It supports a real SQL dialect, stores data persistently to disk using a B+Tree, and gives you an interactive REPL (shell) to run queries — just like SQLite, but built from the ground up for learning.

---

## ✨ Features

- **Real SQL parsing** — Lexer → Parser → AST, just like production databases
- **B+Tree storage** — Data is stored in a sorted tree structure on disk (4KB pages)
- **Persistent storage** — Your data survives restarts (saved to a `.db` file)
- **Interactive REPL** — Type SQL commands and see results instantly
- **No dependencies** — Only the Go standard library. Nothing to install.

---

## 🚀 Getting Started

### Prerequisites

- [Go](https://go.dev/dl/) 1.24 or later

### Run It

```bash
# Clone the repository
git clone https://github.com/sobhane/golang-database.git
cd golang-database

# Run the database (creates mydb.db in the current directory)
go run main.go

# Or specify a custom database file
go run main.go my_project.db
```

You'll see the interactive prompt:

```
Welcome to MyDB — Your own SQL database engine built in Go!
Type .help for help, .quit to exit.

mydb>
```

### Build a Binary (Optional)

```bash
go build -o mydb.exe .
./mydb.exe
```

---

## 📖 SQL Commands

### Create a Table

```sql
CREATE TABLE users (
    id INT PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT,
    active BOOL
);
```

**Supported data types:** `INT`, `TEXT`, `BOOL`
**Supported constraints:** `PRIMARY KEY`, `NOT NULL`

### Insert Rows

```sql
-- Provide values for all columns (in order)
INSERT INTO users VALUES (1, 'Alice', 'alice@example.com', TRUE);

-- Or specify which columns to fill
INSERT INTO users (id, name) VALUES (2, 'Bob');
```

### Query Data

```sql
-- Get all rows
SELECT * FROM users;

-- Pick specific columns
SELECT name, email FROM users;

-- Filter with WHERE
SELECT * FROM users WHERE id = 1;

-- Combine conditions
SELECT * FROM users WHERE active = TRUE AND id > 0;

-- Sort results
SELECT * FROM users ORDER BY name ASC;

-- Limit output
SELECT * FROM users LIMIT 5;

-- Column aliases
SELECT name AS username FROM users;
```

### Update Rows

```sql
UPDATE users SET email = 'newalice@example.com' WHERE id = 1;
UPDATE users SET active = FALSE, name = 'Robert' WHERE id = 2;
```

### Delete Rows

```sql
DELETE FROM users WHERE id = 2;
```

### Drop a Table

```sql
DROP TABLE users;
```

### Joins

```sql
-- Inner join (default)
SELECT users.name, orders.total
FROM users
JOIN orders ON users.id = orders.user_id;

-- Left join (keeps all rows from the left table)
SELECT users.name, orders.total
FROM users
LEFT JOIN orders ON users.id = orders.user_id;
```

### Supported Operators

| Operator | Meaning              |
|----------|----------------------|
| `=`      | Equal                |
| `!=`     | Not equal            |
| `<`      | Less than            |
| `>`      | Greater than         |
| `<=`     | Less than or equal   |
| `>=`     | Greater than or equal|
| `AND`    | Logical AND          |
| `OR`     | Logical OR           |

---

## 🔧 REPL Dot Commands

| Command   | Description                        |
|-----------|------------------------------------|
| `.help`   | Show available commands            |
| `.tables` | List all tables in the database    |
| `.schema` | Show column details for all tables |
| `.quit`   | Exit the REPL                      |

---

## 🏗️ How It Works — Architecture Overview

Every SQL query goes through a **four-stage pipeline**, the same approach used by real databases:

```
SQL String → [Lexer] → Tokens → [Parser] → AST → [Executor] → Result
                                                      ↕
                                                  [Storage]
                                                  (B+Tree + Pager)
```

### 1. Lexer (`lexer/`)

The lexer (also called a tokenizer) reads your SQL string character by character and breaks it into **tokens** — the smallest meaningful units.

```
Input:  "SELECT name FROM users WHERE id = 1"

Output: [SELECT] [name] [FROM] [users] [WHERE] [id] [=] [1] [EOF]
```

- **Case-insensitive** — `select`, `SELECT`, and `Select` all produce the same token
- Handles string literals (`'hello'`), integers (`42`), operators (`<=`, `!=`), and all SQL keywords

📄 Files: `lexer/token.go` (token type definitions), `lexer/lexer.go` (tokenization logic)

### 2. Parser (`parser/`)

The parser reads the flat list of tokens and builds a structured **Abstract Syntax Tree (AST)** — a tree that represents the meaning of the SQL.

```
Tokens: [SELECT] [name] [FROM] [users] [WHERE] [id] [=] [1]

AST:
  SelectStmt
  ├── Columns: [ColumnRef("name")]
  ├── TableName: "users"
  └── Where: BinaryExpr
      ├── Left:  ColumnRef("id")
      ├── Op:    "="
      └── Right: IntLiteral(1)
```

Uses **recursive descent parsing** — each grammar rule (SELECT, INSERT, CREATE, etc.) has its own function that consumes the relevant tokens.

📄 Files: `parser/ast.go` (AST node definitions), `parser/parser.go` (parsing logic)

### 3. Engine (`engine/`)

The engine is the brain of the database. It contains:

- **Executor** — Takes an AST node and performs the operation (scan rows, filter, sort, insert, delete)
- **Catalog** — A registry of all tables and their schemas (column names, types, primary keys). Stored on page 1 of the database file.
- **Table** — Handles row-level operations: serializing Go values to bytes, inserting into the B+Tree, formatting results as ASCII tables.

📄 Files: `engine/executor.go`, `engine/catalog.go`, `engine/table.go`

### 4. Storage (`storage/`)

The storage layer handles persistent, on-disk data:

- **Pager** — Manages a file divided into fixed-size **4KB pages** (like SQLite). Reads/writes pages by number, with an in-memory cache to avoid repeated disk I/O. Page 0 is a file header (magic bytes `MYDB` + page count).
- **B+Tree** — A self-balancing tree where all data lives in **leaf nodes** and internal nodes are for navigation. Supports `Search`, `Insert`, `Delete`, and `Scan` — all in O(log n) time. Nodes split automatically when full.

📄 Files: `storage/pager.go`, `storage/btree.go`

---

## 📁 Project Structure

```
golang-database/
├── main.go              # Entry point — opens DB file, starts REPL
├── go.mod               # Go module definition (zero dependencies)
├── lexer/
│   ├── token.go         # Token type definitions and keyword lookup
│   └── lexer.go         # Character-by-character SQL tokenizer
├── parser/
│   ├── ast.go           # AST node types (Statement + Expression interfaces)
│   └── parser.go        # Recursive descent parser (tokens → AST)
├── engine/
│   ├── executor.go      # Query execution (AST → results)
│   ├── catalog.go       # Table metadata registry (schemas, primary keys)
│   └── table.go         # Row serialization and ASCII table formatting
├── storage/
│   ├── pager.go         # Page-based file I/O with caching
│   └── btree.go         # B+Tree implementation (search, insert, delete, scan)
└── repl/
    └── repl.go          # Interactive Read-Eval-Print Loop
```

---

## 🧪 Try It Out — Quick Example Session

```
mydb> CREATE TABLE persons (id INT PRIMARY KEY, name TEXT NOT NULL, email TEXT);
Table 'persons' created.

mydb> INSERT INTO persons VALUES (1, 'Alice', 'alice@example.com');
1 row inserted.

mydb> INSERT INTO persons VALUES (2, 'Bob', 'bob@example.com');
1 row inserted.

mydb> INSERT INTO persons VALUES (3, 'Charlie', NULL);
1 row inserted.

mydb> SELECT * FROM persons;
+----+---------+-------------------+
| id | name    | email             |
+----+---------+-------------------+
| 1  | Alice   | alice@example.com |
| 2  | Bob     | bob@example.com   |
| 3  | Charlie | NULL              |
+----+---------+-------------------+
(3 rows)

mydb> SELECT name FROM persons WHERE id > 1 ORDER BY name DESC;
+---------+
| name    |
+---------+
| Charlie |
| Bob     |
+---------+
(2 rows)

mydb> UPDATE persons SET email = 'charlie@example.com' WHERE id = 3;
1 row(s) updated.

mydb> DELETE FROM persons WHERE id = 1;
1 row(s) deleted.

mydb> .tables
  persons

mydb> .schema

Table: persons
  Primary Key: id
  Columns:
    id                   INT   PRIMARY KEY
    name                 TEXT  NOT NULL
    email                TEXT

mydb> .quit
Goodbye!
```

---

## 📚 Learning Resources

This project follows the same architecture used by real databases. If you want to dive deeper:

- [Let's Build a Simple Database](https://cstack.github.io/db_tutorial/) — A step-by-step C tutorial that inspired this project's approach
- [SQLite Architecture](https://www.sqlite.org/arch.html) — How SQLite organizes its virtual machine, B-Tree, and pager
- [Crafting Interpreters](https://craftinginterpreters.com/) — Covers lexing, parsing, and ASTs (for programming languages, but the concepts map directly to SQL)

---

## 📄 License

This project is open source and available for learning and experimentation.
