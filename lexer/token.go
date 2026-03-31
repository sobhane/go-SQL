package lexer

// TokenType represents the type of a lexical token.
type TokenType int

const (
	// Special tokens
	TOKEN_ILLEGAL TokenType = iota
	TOKEN_EOF
	TOKEN_WS // whitespace (skipped during lexing)

	// Literals
	TOKEN_IDENT      // column names, table names
	TOKEN_INT_LIT    // 123
	TOKEN_STRING_LIT // 'hello'

	// Operators & Delimiters
	TOKEN_COMMA     // ,
	TOKEN_SEMICOLON // ;
	TOKEN_LPAREN    // (
	TOKEN_RPAREN    // )
	TOKEN_STAR      // *
	TOKEN_EQ        // =
	TOKEN_NEQ       // !=
	TOKEN_LT        // <
	TOKEN_GT        // >
	TOKEN_LTE       // <=
	TOKEN_GTE       // >=
	TOKEN_DOT       // .

	// SQL Keywords
	TOKEN_SELECT
	TOKEN_FROM
	TOKEN_WHERE
	TOKEN_INSERT
	TOKEN_INTO
	TOKEN_VALUES
	TOKEN_UPDATE
	TOKEN_SET
	TOKEN_DELETE
	TOKEN_CREATE
	TOKEN_TABLE
	TOKEN_DROP
	TOKEN_ALTER
	TOKEN_ADD
	TOKEN_PRIMARY
	TOKEN_KEY
	TOKEN_NOT
	TOKEN_NULL
	TOKEN_AND
	TOKEN_OR
	TOKEN_ORDER
	TOKEN_BY
	TOKEN_ASC
	TOKEN_DESC
	TOKEN_INT
	TOKEN_TEXT
	TOKEN_BOOL
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_JOIN
	TOKEN_INNER
	TOKEN_LEFT
	TOKEN_RIGHT
	TOKEN_ON
	TOKEN_AS
	TOKEN_FOREIGN
	TOKEN_REFERENCES
	TOKEN_CASCADE
	TOKEN_LIMIT
)

// Token represents a single lexical token with its type and literal value.
type Token struct {
	Type    TokenType
	Literal string
}

// keywords maps SQL keyword strings to their token types.
// All lookups are done in uppercase so lexing is case-insensitive.
var keywords = map[string]TokenType{
	"SELECT":     TOKEN_SELECT,
	"FROM":       TOKEN_FROM,
	"WHERE":      TOKEN_WHERE,
	"INSERT":     TOKEN_INSERT,
	"INTO":       TOKEN_INTO,
	"VALUES":     TOKEN_VALUES,
	"UPDATE":     TOKEN_UPDATE,
	"SET":        TOKEN_SET,
	"DELETE":     TOKEN_DELETE,
	"CREATE":     TOKEN_CREATE,
	"TABLE":      TOKEN_TABLE,
	"DROP":       TOKEN_DROP,
	"ALTER":      TOKEN_ALTER,
	"ADD":        TOKEN_ADD,
	"PRIMARY":    TOKEN_PRIMARY,
	"KEY":        TOKEN_KEY,
	"NOT":        TOKEN_NOT,
	"NULL":       TOKEN_NULL,
	"AND":        TOKEN_AND,
	"OR":         TOKEN_OR,
	"ORDER":      TOKEN_ORDER,
	"BY":         TOKEN_BY,
	"ASC":        TOKEN_ASC,
	"DESC":       TOKEN_DESC,
	"INT":        TOKEN_INT,
	"TEXT":       TOKEN_TEXT,
	"BOOL":       TOKEN_BOOL,
	"TRUE":       TOKEN_TRUE,
	"FALSE":      TOKEN_FALSE,
	"JOIN":       TOKEN_JOIN,
	"INNER":      TOKEN_INNER,
	"LEFT":       TOKEN_LEFT,
	"RIGHT":      TOKEN_RIGHT,
	"ON":         TOKEN_ON,
	"AS":         TOKEN_AS,
	"FOREIGN":    TOKEN_FOREIGN,
	"REFERENCES": TOKEN_REFERENCES,
	"CASCADE":    TOKEN_CASCADE,
	"LIMIT":      TOKEN_LIMIT,
}

// LookupIdent checks if an identifier is a keyword.
// Returns the keyword token type if found, otherwise TOKEN_IDENT.
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TOKEN_IDENT
}

// String returns a human-readable name for a token type.
func (t TokenType) String() string {
	switch t {
	case TOKEN_ILLEGAL:
		return "ILLEGAL"
	case TOKEN_EOF:
		return "EOF"
	case TOKEN_IDENT:
		return "IDENT"
	case TOKEN_INT_LIT:
		return "INT"
	case TOKEN_STRING_LIT:
		return "STRING"
	case TOKEN_COMMA:
		return ","
	case TOKEN_SEMICOLON:
		return ";"
	case TOKEN_LPAREN:
		return "("
	case TOKEN_RPAREN:
		return ")"
	case TOKEN_STAR:
		return "*"
	case TOKEN_EQ:
		return "="
	case TOKEN_NEQ:
		return "!="
	case TOKEN_LT:
		return "<"
	case TOKEN_GT:
		return ">"
	case TOKEN_LTE:
		return "<="
	case TOKEN_GTE:
		return ">="
	case TOKEN_DOT:
		return "."
	case TOKEN_SELECT:
		return "SELECT"
	case TOKEN_FROM:
		return "FROM"
	case TOKEN_WHERE:
		return "WHERE"
	case TOKEN_INSERT:
		return "INSERT"
	case TOKEN_INTO:
		return "INTO"
	case TOKEN_VALUES:
		return "VALUES"
	case TOKEN_UPDATE:
		return "UPDATE"
	case TOKEN_SET:
		return "SET"
	case TOKEN_DELETE:
		return "DELETE"
	case TOKEN_CREATE:
		return "CREATE"
	case TOKEN_TABLE:
		return "TABLE"
	case TOKEN_DROP:
		return "DROP"
	case TOKEN_ALTER:
		return "ALTER"
	case TOKEN_ADD:
		return "ADD"
	case TOKEN_PRIMARY:
		return "PRIMARY"
	case TOKEN_KEY:
		return "KEY"
	case TOKEN_NOT:
		return "NOT"
	case TOKEN_NULL:
		return "NULL"
	case TOKEN_AND:
		return "AND"
	case TOKEN_OR:
		return "OR"
	case TOKEN_ORDER:
		return "ORDER"
	case TOKEN_BY:
		return "BY"
	case TOKEN_ASC:
		return "ASC"
	case TOKEN_DESC:
		return "DESC"
	case TOKEN_INT:
		return "INT_TYPE"
	case TOKEN_TEXT:
		return "TEXT_TYPE"
	case TOKEN_BOOL:
		return "BOOL_TYPE"
	case TOKEN_TRUE:
		return "TRUE"
	case TOKEN_FALSE:
		return "FALSE"
	case TOKEN_JOIN:
		return "JOIN"
	case TOKEN_INNER:
		return "INNER"
	case TOKEN_LEFT:
		return "LEFT"
	case TOKEN_RIGHT:
		return "RIGHT"
	case TOKEN_ON:
		return "ON"
	case TOKEN_AS:
		return "AS"
	case TOKEN_FOREIGN:
		return "FOREIGN"
	case TOKEN_REFERENCES:
		return "REFERENCES"
	case TOKEN_CASCADE:
		return "CASCADE"
	case TOKEN_LIMIT:
		return "LIMIT"
	default:
		return "UNKNOWN"
	}
}
