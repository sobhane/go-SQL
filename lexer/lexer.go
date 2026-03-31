package lexer

import (
	"fmt"
	"strings"
	"unicode"
)

// Lexer tokenizes a SQL input string character by character.
//
// HOW IT WORKS:
// The lexer maintains a position (pos) in the input string.
// It reads one character at a time and decides what token to emit.
// Keywords are case-insensitive: "select", "SELECT", and "Select" all produce TOKEN_SELECT.
type Lexer struct {
	input   string // the full SQL string
	pos     int    // current position in input (points to current char)
	readPos int    // next position to read (after current char)
	ch      byte   // current character being examined
}

// New creates a new Lexer for the given SQL input string.
func New(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar() // initialize: load the first character
	return l
}

// readChar advances the lexer by one character.
// If we've reached the end of input, ch is set to 0 (null byte = EOF).
func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0 // ASCII NUL = end of input
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
}

// peekChar looks at the next character WITHOUT advancing the position.
// This is essential for two-character operators like !=, <=, >=.
func (l *Lexer) peekChar() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

// NextToken reads and returns the next token from the input.
// This is the core method — it's called repeatedly until TOKEN_EOF.
func (l *Lexer) NextToken() Token {
	var tok Token

	l.skipWhitespace()

	switch l.ch {
	case ',':
		tok = Token{Type: TOKEN_COMMA, Literal: ","}
	case ';':
		tok = Token{Type: TOKEN_SEMICOLON, Literal: ";"}
	case '(':
		tok = Token{Type: TOKEN_LPAREN, Literal: "("}
	case ')':
		tok = Token{Type: TOKEN_RPAREN, Literal: ")"}
	case '*':
		tok = Token{Type: TOKEN_STAR, Literal: "*"}
	case '.':
		tok = Token{Type: TOKEN_DOT, Literal: "."}
	case '=':
		tok = Token{Type: TOKEN_EQ, Literal: "="}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_LTE, Literal: "<="}
		} else {
			tok = Token{Type: TOKEN_LT, Literal: "<"}
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_GTE, Literal: ">="}
		} else {
			tok = Token{Type: TOKEN_GT, Literal: ">"}
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_NEQ, Literal: "!="}
		} else {
			tok = Token{Type: TOKEN_ILLEGAL, Literal: string(l.ch)}
		}
	case '\'':
		// String literals: 'hello world'
		// We read everything between the quotes.
		str, err := l.readString()
		if err != nil {
			tok = Token{Type: TOKEN_ILLEGAL, Literal: err.Error()}
		} else {
			tok = Token{Type: TOKEN_STRING_LIT, Literal: str}
		}
		return tok // readString already advanced past the closing quote
	case 0:
		tok = Token{Type: TOKEN_EOF, Literal: ""}
	default:
		if isLetter(l.ch) || l.ch == '_' {
			// Read a full word (identifier or keyword)
			literal := l.readIdentifier()
			// Check if it's a SQL keyword (case-insensitive)
			tokType := LookupIdent(strings.ToUpper(literal))
			return Token{Type: tokType, Literal: literal}
		} else if isDigit(l.ch) {
			// Read a full number
			literal := l.readNumber()
			return Token{Type: TOKEN_INT_LIT, Literal: literal}
		} else {
			tok = Token{Type: TOKEN_ILLEGAL, Literal: string(l.ch)}
		}
	}

	l.readChar()
	return tok
}

// Tokenize reads all tokens from the input and returns them as a slice.
// Useful for debugging or when the parser wants all tokens upfront.
func (l *Lexer) Tokenize() []Token {
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == TOKEN_EOF {
			break
		}
	}
	return tokens
}

// --- Helper methods ---

// skipWhitespace advances past any spaces, tabs, newlines.
func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

// readIdentifier reads a contiguous sequence of letters, digits, and underscores.
// Called when we see a letter or underscore as the first character.
func (l *Lexer) readIdentifier() string {
	start := l.pos
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}
	return l.input[start:l.pos]
}

// readNumber reads a contiguous sequence of digits.
func (l *Lexer) readNumber() string {
	start := l.pos
	for isDigit(l.ch) {
		l.readChar()
	}
	return l.input[start:l.pos]
}

// readString reads a single-quoted string literal.
// The opening quote has already been consumed when this is called.
// It handles escaped quotes ('') inside strings.
func (l *Lexer) readString() (string, error) {
	// Skip the opening quote
	l.readChar()
	var result strings.Builder
	for {
		if l.ch == 0 {
			return "", fmt.Errorf("unterminated string literal")
		}
		if l.ch == '\'' {
			// Check for escaped quote ('')
			if l.peekChar() == '\'' {
				result.WriteByte('\'')
				l.readChar() // skip first quote
				l.readChar() // skip second quote
				continue
			}
			// End of string
			l.readChar() // consume closing quote
			break
		}
		result.WriteByte(l.ch)
		l.readChar()
	}
	return result.String(), nil
}

func isLetter(ch byte) bool {
	return unicode.IsLetter(rune(ch))
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}
