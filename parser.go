package main

import "fmt"

// MARK: - Operator Precedence and Parser State

// Operator binding precedence levels for Pratt expression parsing.
const (
	PREC_LOWEST  = iota
	PREC_COMPARE // ==, !=, <, >, <=, >=
	PREC_BIT_OR  // |, ^
	PREC_BIT_AND // &
	PREC_SHIFT   // <<, >>
	PREC_SUM     // +, -
	PREC_PRODUCT // *, /, %
	PREC_PREFIX  // ~x, -x, *ptr, &val
	PREC_CALL    // func(), obj.field, arr[i]
)

// precedences maps infix/postfix TokenTypes to their binding power.
var precedences = map[TokenType]int{
	TOK_EQ: PREC_COMPARE, TOK_NEQ: PREC_COMPARE,
	TOK_LT: PREC_COMPARE, TOK_GT: PREC_COMPARE,
	TOK_LE: PREC_COMPARE, TOK_GE: PREC_COMPARE,
	TOK_PIPE: PREC_BIT_OR, TOK_CARET: PREC_BIT_OR,
	TOK_AMP: PREC_BIT_AND,
	TOK_SHL: PREC_SHIFT, TOK_SHR: PREC_SHIFT,
	TOK_PLUS: PREC_SUM, TOK_MINUS: PREC_SUM,
	TOK_STAR: PREC_PRODUCT, TOK_SLASH: PREC_PRODUCT, TOK_PERCENT: PREC_PRODUCT,
	TOK_LPAREN: PREC_CALL, TOK_DOT: PREC_CALL, TOK_LBRACKET: PREC_CALL,
}

// Parser implements a recursive-descent and Pratt parser for BIT-C source files.
type Parser struct {
	lexer   *Lexer // Underlying lexical scanner
	curTok  Token  // Current token under inspection
	peekTok Token  // One-token lookahead buffer
}

// NewParser initializes a Parser and preloads the two-token lookahead window.
func NewParser(l *Lexer) *Parser {
	p := &Parser{lexer: l}
	p.nextToken()
	p.nextToken()
	return p
}

// nextToken advances curTok and pulls the next token from the Lexer.
func (p *Parser) nextToken() {
	p.curTok = p.peekTok
	p.peekTok = p.lexer.NextToken()
}

// expect verifies that curTok matches t, advances the cursor, and returns the token.
func (p *Parser) expect(t TokenType) Token {
	if p.curTok.Type != t {
		panic(fmt.Sprintf("[Line %d] Syntax error: expected %s, got %s ('%s')",
			p.curTok.Line, t, p.curTok.Type, p.curTok.Literal))
	}
	tok := p.curTok
	p.nextToken()
	return tok
}

// expectIdent consumes an identifier token (also permitting contextual keywords 'in'/'step').
func (p *Parser) expectIdent() Token {
	if p.curTok.Type == TOK_IDENT || p.curTok.Type == TOK_IN || p.curTok.Type == TOK_STEP {
		tok := p.curTok
		p.nextToken()
		return tok
	}
	panic(fmt.Sprintf("[Line %d] Syntax error: expected identifier, got %s ('%s')",
		p.curTok.Line, p.curTok.Type, p.curTok.Literal))
}

// MARK: - Top-Level Parsing and Type Expressions

// ParseProgram parses an entire BIT-C source file into a root ProgramNode.
func (p *Parser) ParseProgram() *ProgramNode {
	prog := &ProgramNode{}
	testIdx := 0
	for p.curTok.Type != TOK_EOF {
		switch p.curTok.Type {
		case TOK_IMPORT:
			p.nextToken()
			path := p.expect(TOK_STRING).Literal
			if p.curTok.Type == TOK_SEMI {
				p.nextToken()
			}
			prog.Imports = append(prog.Imports, path)
		case TOK_STRUCT:
			prog.Structs = append(prog.Structs, p.parseStruct())
		case TOK_FN, TOK_EXTERN:
			prog.Funcs = append(prog.Funcs, p.parseFn())
		case TOK_TEST:
			testIdx++
			prog.Funcs = append(prog.Funcs, p.parseTest(testIdx))
		default:
			panic(fmt.Sprintf("[Line %d] Unexpected top-level token: %s ('%s')",
				p.curTok.Line, p.curTok.Type, p.curTok.Literal))
		}
	}
	return prog
}

// parseTest parses a built-in unit test block: test "description" { ... }
// and lowers it into an internal zero-argument FnNode named __bitc_test_<idx>.
func (p *Parser) parseTest(idx int) *FnNode {
	line := p.expect(TOK_TEST).Line
	_ = p.expect(TOK_STRING).Literal // Test description stored for runner integration
	p.expect(TOK_LBRACE)
	body := p.parseBlock()
	return &FnNode{
		Line:       line,
		Name:       fmt.Sprintf("__bitc_test_%d", idx),
		ReturnType: TypeExpr{Line: line, Name: "void"},
		Body:       body,
	}
}

// parseTypeParams parses generic arguments enclosed in angle brackets <T1, T2>.
func (p *Parser) parseTypeParams() []string {
	var params []string
	if p.curTok.Type != TOK_LT {
		return params
	}
	p.nextToken() // Skip '<'
	for p.curTok.Type != TOK_GT && p.curTok.Type != TOK_EOF {
		if p.curTok.Type == TOK_IDENT || p.curTok.Type == TOK_NUMBER {
			params = append(params, p.curTok.Literal)
			p.nextToken()
		} else {
			panic(fmt.Sprintf("[Line %d] Invalid type parameter: '%s'", p.curTok.Line, p.curTok.Literal))
		}
		if p.curTok.Type == TOK_COMMA {
			p.nextToken()
		}
	}
	p.expect(TOK_GT)
	return params
}

// parseType parses type annotations such as *Dense<256, 8>[4], bvec<64>, or int.
func (p *Parser) parseType() TypeExpr {
	line := p.curTok.Line
	isPtr := false
	if p.curTok.Type == TOK_STAR {
		isPtr = true
		p.nextToken()
	}
	name := p.expectIdent().Literal
	typeParams := p.parseTypeParams()
	arrSize := ""
	if p.curTok.Type == TOK_LBRACKET {
		p.nextToken()
		if p.curTok.Type == TOK_NUMBER || p.curTok.Type == TOK_IDENT {
			arrSize = p.curTok.Literal
			p.nextToken()
		}
		p.expect(TOK_RBRACKET)
	}
	return TypeExpr{
		Line: line, Name: name, IsPointer: isPtr,
		ArraySize: arrSize, TypeParams: typeParams,
	}
}

// MARK: - Struct and Function Declarations

// parseStruct parses a structure definition: struct Name<...> { field: Type; }.
func (p *Parser) parseStruct() *StructNode {
	line := p.expect(TOK_STRUCT).Line
	name := p.expectIdent().Literal
	tParams := p.parseTypeParams()
	p.expect(TOK_LBRACE)

	var fields []Field
	for p.curTok.Type != TOK_RBRACE && p.curTok.Type != TOK_EOF {
		fLine := p.curTok.Line
		fName := p.expectIdent().Literal
		p.expect(TOK_COLON)
		fType := p.parseType()
		p.expect(TOK_SEMI)
		fields = append(fields, Field{Line: fLine, Name: fName, Type: fType})
	}
	p.expect(TOK_RBRACE)
	return &StructNode{Line: line, Name: name, TypeParams: tParams, Fields: fields}
}

// parseFn parses standalone functions, extern C prototypes, and receiver methods.
func (p *Parser) parseFn() *FnNode {
	isExtern := false
	if p.curTok.Type == TOK_EXTERN {
		isExtern = true
		p.nextToken()
	}
	line := p.expect(TOK_FN).Line
	var receiver *Field

	// Parse Go-style method receiver when '(' immediately follows 'fn'.
	if p.curTok.Type == TOK_LPAREN {
		p.nextToken()
		rLine := p.curTok.Line
		rName := p.expectIdent().Literal
		p.expect(TOK_COLON)
		rType := p.parseType()
		p.expect(TOK_RPAREN)
		receiver = &Field{Line: rLine, Name: rName, Type: rType}
	}

	name := p.expectIdent().Literal
	tParams := p.parseTypeParams()
	p.expect(TOK_LPAREN)

	var params []Field
	for p.curTok.Type != TOK_RPAREN && p.curTok.Type != TOK_EOF {
		pLine := p.curTok.Line
		pName := p.expectIdent().Literal
		p.expect(TOK_COLON)
		pType := p.parseType()
		params = append(params, Field{Line: pLine, Name: pName, Type: pType})
		if p.curTok.Type == TOK_COMMA {
			p.nextToken()
		}
	}
	p.expect(TOK_RPAREN)

	retType := TypeExpr{Line: line, Name: "void"}
	if p.curTok.Type == TOK_ARROW {
		p.nextToken()
		retType = p.parseType()
	}
	if isExtern {
		p.expect(TOK_SEMI)
		return &FnNode{
			Line: line, IsExtern: true, Name: name,
			TypeParams: tParams, Params: params, ReturnType: retType,
		}
	}
	p.expect(TOK_LBRACE)
	body := p.parseBlock()

	return &FnNode{
		Line: line, Receiver: receiver, Name: name,
		TypeParams: tParams, Params: params, ReturnType: retType, Body: body,
	}
}

// MARK: - Block and Statement Parsing

// parseBlock parses a brace-delimited list of statements until '}'.
func (p *Parser) parseBlock() []Stmt {
	var stmts []Stmt
	for p.curTok.Type != TOK_RBRACE && p.curTok.Type != TOK_EOF {
		stmts = append(stmts, p.parseStatement())
	}
	p.expect(TOK_RBRACE)
	return stmts
}

// parseStatement dispatches and parses a single executable statement.
func (p *Parser) parseStatement() Stmt {
	line := p.curTok.Line
	switch p.curTok.Type {
	case TOK_IF:
		return p.parseIfStmt()
	case TOK_FOR, TOK_PARALLEL:
		return p.parseForStmt()
	case TOK_RETURN:
		p.nextToken()
		if p.curTok.Type == TOK_SEMI {
			p.nextToken()
			return &ReturnStmt{Line: line}
		}
		val := p.parseExpr(PREC_LOWEST)
		p.expect(TOK_SEMI)
		return &ReturnStmt{Line: line, Value: val}
	case TOK_PRINT:
		p.nextToken()
		val := p.parseExpr(PREC_LOWEST)
		p.expect(TOK_SEMI)
		return &PrintStmt{Line: line, Value: val}
	}

	// Variable declaration: name: Type = init;
	if (p.curTok.Type == TOK_IDENT || p.curTok.Type == TOK_IN) && p.peekTok.Type == TOK_COLON {
		name := p.expectIdent().Literal
		p.expect(TOK_COLON)
		vType := p.parseType()
		var initVal Expr
		if p.curTok.Type == TOK_ASSIGN {
			p.nextToken()
			initVal = p.parseExpr(PREC_LOWEST)
		}
		p.expect(TOK_SEMI)
		return &VarDeclStmt{Line: line, Name: name, Type: vType, Value: initVal}
	}

	// Assignment or standalone expression statement.
	left := p.parseExpr(PREC_LOWEST)
	if p.curTok.Type == TOK_ASSIGN {
		p.nextToken()
		right := p.parseExpr(PREC_LOWEST)
		p.expect(TOK_SEMI)
		return &AssignStmt{Line: line, Target: left, Value: right}
	}
	p.expect(TOK_SEMI)
	return &ExprStmt{Line: line, Expr: left}
}

// parseIfStmt parses an if-else conditional statement.
func (p *Parser) parseIfStmt() *IfStmt {
	line := p.expect(TOK_IF).Line
	cond := p.parseExpr(PREC_LOWEST)
	p.expect(TOK_LBRACE)
	thenBody := p.parseBlock()
	var elseBody []Stmt
	if p.curTok.Type == TOK_ELSE {
		p.nextToken()
		p.expect(TOK_LBRACE)
		elseBody = p.parseBlock()
	}
	return &IfStmt{Line: line, Cond: cond, ThenBody: thenBody, ElseBody: elseBody}
}

// parseForStmt parses sequential and OpenMP parallel bounded range loops.
func (p *Parser) parseForStmt() *ForStmt {
	line := p.curTok.Line
	isParallel := false
	if p.curTok.Type == TOK_PARALLEL {
		isParallel = true
		p.nextToken()
	}
	p.expect(TOK_FOR)
	iter := p.expectIdent().Literal
	p.expect(TOK_IN)
	start := p.parseExpr(PREC_COMPARE)
	p.expect(TOK_RANGE)
	end := p.parseExpr(PREC_COMPARE)
	var step Expr
	if p.curTok.Type == TOK_STEP {
		p.nextToken()
		step = p.parseExpr(PREC_COMPARE)
	}
	p.expect(TOK_LBRACE)
	body := p.parseBlock()
	return &ForStmt{
		Line: line, IsParallel: isParallel, Iterator: iter,
		Start: start, End: end, Step: step, Body: body,
	}
}

// MARK: - Pratt Expression Parsing

// parseExpr parses an expression respecting operator binding precedences.
func (p *Parser) parseExpr(precedence int) Expr {
	left := p.parsePrefix()

	for p.curTok.Type != TOK_SEMI && p.curTok.Type != TOK_EOF {
		// Handle explicit generic call syntax: nudge<256>(...)
		if p.curTok.Type == TOK_LT && p.isGenericCallAhead() {
			tArgs := p.parseTypeParams()
			left = p.parseCallExpr(left, tArgs)
			continue
		}

		nextPrec, ok := precedences[p.curTok.Type]
		if !ok || precedence >= nextPrec {
			break
		}

		switch p.curTok.Type {
		case TOK_LPAREN:
			left = p.parseCallExpr(left, nil)
		case TOK_DOT:
			line := p.curTok.Line
			p.nextToken()
			field := p.expectIdent().Literal
			left = &FieldExpr{Line: line, Obj: left, Field: field}
		case TOK_LBRACKET:
			line := p.curTok.Line
			p.nextToken()
			idx := p.parseExpr(PREC_LOWEST)
			p.expect(TOK_RBRACKET)
			left = &IndexExpr{Line: line, Obj: left, Index: idx}
		default:
			line := p.curTok.Line
			op := p.curTok.Literal
			p.nextToken()
			right := p.parseExpr(nextPrec)
			left = &BinaryExpr{Line: line, Left: left, Op: op, Right: right}
		}
	}
	return left
}

// parsePrefix parses primary expressions and prefix unary operators.
func (p *Parser) parsePrefix() Expr {
	line := p.curTok.Line
	switch p.curTok.Type {
	case TOK_NUMBER, TOK_HEX:
		isHex := p.curTok.Type == TOK_HEX
		val := p.curTok.Literal
		p.nextToken()
		return &NumberExpr{Line: line, Value: val, IsHex: isHex}
	case TOK_IDENT, TOK_IN, TOK_STEP:
		name := p.curTok.Literal
		p.nextToken()
		return &IdentExpr{Line: line, Name: name}
	case TOK_LPAREN:
		p.nextToken()
		expr := p.parseExpr(PREC_LOWEST)
		p.expect(TOK_RPAREN)
		return expr
	case TOK_TILDE, TOK_MINUS, TOK_STAR, TOK_AMP:
		op := p.curTok.Literal
		p.nextToken()
		right := p.parseExpr(PREC_PREFIX)
		return &UnaryExpr{Line: line, Op: op, Right: right}
	}
	panic(fmt.Sprintf("[Line %d] Unexpected token in expression: %s ('%s')",
		p.curTok.Line, p.curTok.Type, p.curTok.Literal))
}

// parseCallExpr parses a parenthesized argument list for a function or method call.
func (p *Parser) parseCallExpr(callee Expr, tArgs []string) *CallExpr {
	line := p.expect(TOK_LPAREN).Line
	var args []Expr
	for p.curTok.Type != TOK_RPAREN && p.curTok.Type != TOK_EOF {
		args = append(args, p.parseExpr(PREC_LOWEST))
		if p.curTok.Type == TOK_COMMA {
			p.nextToken()
		}
	}
	p.expect(TOK_RPAREN)
	return &CallExpr{Line: line, Callee: callee, TypeArgs: tArgs, Args: args}
}

// isGenericCallAhead checks whether '<' begins generic type arguments before '('.
func (p *Parser) isGenericCallAhead() bool {
	return p.peekTok.Type == TOK_NUMBER || p.peekTok.Type == TOK_IDENT
}
