// Copyright (c) 2026 Vladimir Borisenko (Goth) & AI Gemini.
// Licensed under the MIT License. See LICENSE file in the project root.
//
// File: lexer.go
// Description: Lexical analyzer (tokenizer) for the BIT-C programming language.

package main

import (
	"strings"
	"unicode"
)

// MARK: - Token Definitions and Keyword Table

// TokenType identifies the lexical category of a scanned BIT-C token.
type TokenType string

const (
	TOK_EOF     TokenType = "EOF"     // End of source input
	TOK_ILLEGAL TokenType = "ILLEGAL" // Unrecognized or invalid character
	TOK_IDENT   TokenType = "IDENT"   // Identifier (variable, type, or function name)
	TOK_NUMBER  TokenType = "NUMBER"  // Decimal integer literal
	TOK_HEX     TokenType = "HEX"     // Hexadecimal bitmask literal (0x...)
	TOK_STRING  TokenType = "STRING"  // Double-quoted string literal

	// Core language keywords
	TOK_STRUCT   TokenType = "STRUCT"   // Structure declaration (struct)
	TOK_FN       TokenType = "FN"       // Function or method declaration (fn)
	TOK_TEST     TokenType = "TEST"     // Built-in unit test block (test)
	TOK_IMPORT   TokenType = "IMPORT"   // Module import directive (import)
	TOK_FOR      TokenType = "FOR"      // Range loop keyword (for)
	TOK_PARALLEL TokenType = "PARALLEL" // OpenMP parallel loop modifier (parallel)
	TOK_IN       TokenType = "IN"       // Loop range separator (in)
	TOK_STEP     TokenType = "STEP"     // Loop step modifier (step)
	TOK_IF       TokenType = "IF"       // Conditional branch (if)
	TOK_ELSE     TokenType = "ELSE"     // Alternative conditional branch (else)
	TOK_RETURN   TokenType = "RETURN"   // Function return statement (return)
	TOK_PRINT    TokenType = "PRINT"    // Diagnostic print statement (print)
	TOK_EXTERN   TokenType = "EXTERN"   // External C runtime function declaration (extern)

	// Arithmetic, bitwise, and logical operators
	TOK_ASSIGN  TokenType = "="  // Assignment
	TOK_PLUS    TokenType = "+"  // Addition
	TOK_MINUS   TokenType = "-"  // Subtraction or unary negation
	TOK_STAR    TokenType = "*"  // Multiplication or pointer dereference
	TOK_SLASH   TokenType = "/"  // Integer division
	TOK_PERCENT TokenType = "%"  // Modulo remainder
	TOK_AMP     TokenType = "&"  // Bitwise AND or address-of
	TOK_PIPE    TokenType = "|"  // Bitwise OR
	TOK_CARET   TokenType = "^"  // Bitwise XOR
	TOK_TILDE   TokenType = "~"  // Bitwise NOT
	TOK_SHL     TokenType = "<<" // Bitwise shift left
	TOK_SHR     TokenType = ">>" // Bitwise shift right

	// Comparison operators
	TOK_EQ  TokenType = "==" // Equal
	TOK_NEQ TokenType = "!=" // Not equal
	TOK_LT  TokenType = "<"  // Less than or generic opening bracket
	TOK_GT  TokenType = ">"  // Greater than or generic closing bracket
	TOK_LE  TokenType = "<=" // Less than or equal
	TOK_GE  TokenType = ">=" // Greater than or equal

	// Delimiters and punctuation
	TOK_COLON    TokenType = ":"  // Type annotation separator
	TOK_SEMI     TokenType = ";"  // Statement terminator
	TOK_COMMA    TokenType = ","  // List separator
	TOK_DOT      TokenType = "."  // Struct member access
	TOK_RANGE    TokenType = ".." // Half-open range operator (start..end)
	TOK_ARROW    TokenType = "->" // Function return type arrow
	TOK_LPAREN   TokenType = "("  // Left parenthesis
	TOK_RPAREN   TokenType = ")"  // Right parenthesis
	TOK_LBRACE   TokenType = "{"  // Left curly brace
	TOK_RBRACE   TokenType = "}"  // Right curly brace
	TOK_LBRACKET TokenType = "["  // Left square bracket
	TOK_RBRACKET TokenType = "]"  // Right square bracket
)

// Token represents a single scanned lexical unit with its source line number.
type Token struct {
	Type    TokenType // Lexical category
	Literal string    // Raw matched text
	Line    int       // 1-based source line number
}

// keywords maps reserved BIT-C keywords to their corresponding TokenType.
var keywords = map[string]TokenType{
	"struct":   TOK_STRUCT,
	"fn":       TOK_FN,
	"test":     TOK_TEST,
	"import":   TOK_IMPORT,
	"for":      TOK_FOR,
	"parallel": TOK_PARALLEL,
	"in":       TOK_IN,
	"step":     TOK_STEP,
	"if":       TOK_IF,
	"else":     TOK_ELSE,
	"return":   TOK_RETURN,
	"print":    TOK_PRINT,
	"extern":   TOK_EXTERN,
}

// MARK: - Lexer State and Main Tokenizer Loop

// Lexer holds the scanning state for a BIT-C source buffer.
type Lexer struct {
	input string // Full source text
	pos   int    // Current byte offset in input
	line  int    // Current 1-based line number
}

// NewLexer creates and initializes a Lexer starting at line 1.
func NewLexer(input string) *Lexer {
	return &Lexer{input: input, pos: 0, line: 1}
}

// NextToken advances the scanner and returns the next lexical Token.
func (l *Lexer) NextToken() Token {
	l.skipWhitespaceAndComments()

	if l.pos >= len(l.input) {
		return Token{Type: TOK_EOF, Literal: "", Line: l.line}
	}

	ch := l.input[l.pos]
	nextCh := l.peek()

	// Match two-character operators first.
	twoChar := string([]byte{ch, nextCh})
	twoCharMap := map[string]TokenType{
		"->": TOK_ARROW, "..": TOK_RANGE,
		"==": TOK_EQ, "!=": TOK_NEQ,
		"<=": TOK_LE, ">=": TOK_GE,
		"<<": TOK_SHL, ">>": TOK_SHR,
	}
	if tokType, ok := twoCharMap[twoChar]; ok {
		l.pos += 2
		return Token{Type: tokType, Literal: twoChar, Line: l.line}
	}

	// Match single-character operators and delimiters.
	oneCharMap := map[byte]TokenType{
		'=': TOK_ASSIGN, '+': TOK_PLUS, '-': TOK_MINUS,
		'*': TOK_STAR, '/': TOK_SLASH, '%': TOK_PERCENT,
		'&': TOK_AMP, '|': TOK_PIPE, '^': TOK_CARET, '~': TOK_TILDE,
		'<': TOK_LT, '>': TOK_GT,
		':': TOK_COLON, ';': TOK_SEMI, ',': TOK_COMMA, '.': TOK_DOT,
		'(': TOK_LPAREN, ')': TOK_RPAREN,
		'{': TOK_LBRACE, '}': TOK_RBRACE,
		'[': TOK_LBRACKET, ']': TOK_RBRACKET,
	}
	if tokType, ok := oneCharMap[ch]; ok {
		l.pos++
		return Token{Type: tokType, Literal: string(ch), Line: l.line}
	}

	if ch == '"' {
		return l.readString()
	}
	if unicode.IsLetter(rune(ch)) || ch == '_' {
		return l.readIdentifier()
	}
	if unicode.IsDigit(rune(ch)) {
		return l.readNumber()
	}

	l.pos++
	return Token{Type: TOK_ILLEGAL, Literal: string(ch), Line: l.line}
}

// MARK: - Literal Scanners and Whitespace Handling

// readString scans a double-quoted string literal (used for import paths and test names).
func (l *Lexer) readString() Token {
	l.pos++ // Skip opening quote
	start := l.pos
	for l.pos < len(l.input) && l.input[l.pos] != '"' && l.input[l.pos] != '\n' {
		l.pos++
	}
	str := l.input[start:l.pos]
	if l.pos < len(l.input) && l.input[l.pos] == '"' {
		l.pos++ // Skip closing quote
	}
	return Token{Type: TOK_STRING, Literal: str, Line: l.line}
}

// readIdentifier scans an alphanumeric/underscore word and checks the keyword table.
func (l *Lexer) readIdentifier() Token {
	start := l.pos
	for l.pos < len(l.input) {
		r := rune(l.input[l.pos])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			l.pos++
		} else {
			break
		}
	}
	word := l.input[start:l.pos]
	if kwType, ok := keywords[word]; ok {
		return Token{Type: kwType, Literal: word, Line: l.line}
	}
	return Token{Type: TOK_IDENT, Literal: word, Line: l.line}
}

// readNumber scans a decimal integer or a 0x-prefixed hexadecimal literal.
func (l *Lexer) readNumber() Token {
	start := l.pos
	if l.input[l.pos] == '0' && (l.peek() == 'x' || l.peek() == 'X') {
		l.pos += 2
		for l.pos < len(l.input) && strings.ContainsRune("0123456789abcdefABCDEF", rune(l.input[l.pos])) {
			l.pos++
		}
		return Token{Type: TOK_HEX, Literal: l.input[start:l.pos], Line: l.line}
	}
	for l.pos < len(l.input) && unicode.IsDigit(rune(l.input[l.pos])) {
		l.pos++
	}
	return Token{Type: TOK_NUMBER, Literal: l.input[start:l.pos], Line: l.line}
}

// peek returns the next byte without advancing the lexer cursor.
func (l *Lexer) peek() byte {
	if l.pos+1 >= len(l.input) {
		return 0
	}
	return l.input[l.pos+1]
}

// skipWhitespaceAndComments advances past spaces, newlines, and '//' line comments.
func (l *Lexer) skipWhitespaceAndComments() {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == '\n' {
			l.line++
			l.pos++
		} else if unicode.IsSpace(rune(ch)) {
			l.pos++
		} else if ch == '/' && l.peek() == '/' {
			for l.pos < len(l.input) && l.input[l.pos] != '\n' {
				l.pos++
			}
		} else {
			break
		}
	}
}
