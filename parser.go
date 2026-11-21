/// @file parser.go
/// @brief Синтаксический анализатор (парсер) ядра языка BIT-C v1.0.
/// @details Реализует рекурсивный спуск для разбора объявлений (struct, fn, import)
/// и алгоритм Пратта (Pratt Parsing) для разбора выражений с учетом приоритета операторов.

package main

import "fmt"

// MARK: Блок 1 - Инициализация парсера, приоритеты операторов и хелперы

// / @brief Уровни приоритета бинарных и постфиксных операторов.
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

// / @brief Таблица соответствия типов токенов их приоритетам.
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

// / @brief Состояние синтаксического анализатора.
type Parser struct {
	lexer   *Lexer ///< Лексический анализатор-источник токенов
	curTok  Token  ///< Текущий рассматриваемый токен
	peekTok Token  ///< Следующий токен (предпросмотр на 1 шаг вперед)
}

// / @brief Создает новый парсер и загружает первые два токена.
func NewParser(l *Lexer) *Parser {
	p := &Parser{lexer: l}
	p.nextToken()
	p.nextToken()
	return p
}

// / @brief Сдвигает окно токенов на один шаг вперед.
func (p *Parser) nextToken() {
	p.curTok = p.peekTok
	p.peekTok = p.lexer.NextToken()
}

// / @brief Проверяет текущий токен на соответствие ожидаемому и сдвигает курсор.
func (p *Parser) expect(t TokenType) Token {
	if p.curTok.Type != t {
		panic(fmt.Sprintf("[Line %d] Syntax error: expected %s, got %s ('%s')",
			p.curTok.Line, t, p.curTok.Type, p.curTok.Literal))
	}
	tok := p.curTok
	p.nextToken()
	return tok
}

// / @brief Считывает идентификатор (разрешает также слова 'in' и 'step' в роли имен).
func (p *Parser) expectIdent() Token {
	if p.curTok.Type == TOK_IDENT || p.curTok.Type == TOK_IN || p.curTok.Type == TOK_STEP {
		tok := p.curTok
		p.nextToken()
		return tok
	}
	panic(fmt.Sprintf("[Line %d] Syntax error: expected identifier, got %s ('%s')",
		p.curTok.Line, p.curTok.Type, p.curTok.Literal))
}

// MARK: Блок 2 - Разбор верхнего уровня программы и сложных типов данных

// / @brief Разбирает весь исходный файл в корневой узел ProgramNode.
func (p *Parser) ParseProgram() *ProgramNode {
	prog := &ProgramNode{}
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
		default:
			panic(fmt.Sprintf("[Line %d] Unexpected top-level token: %s ('%s')",
				p.curTok.Line, p.curTok.Type, p.curTok.Literal))
		}
	}
	return prog
}

// / @brief Разбирает список параметров шаблона в угловых скобках <T1, T2>.
func (p *Parser) parseTypeParams() []string {
	var params []string
	if p.curTok.Type != TOK_LT {
		return params
	}
	p.nextToken() // Пропускаем '<'
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

// / @brief Разбирает описание типа: *Dense<256, 8>[4], bvec<64>, int.
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

// MARK: Блок 3 - Разбор структур данных и Go-методов

// / @brief Разбирает объявление структуры struct Name<...> { field: Type; }.
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

// / @brief Разбирает объявление функции или метода с ресивером fn (self: *T) name(...) -> Ret { ... }.
func (p *Parser) parseFn() *FnNode {
	isExtern := false
	if p.curTok.Type == TOK_EXTERN {
		isExtern = true
		p.nextToken()
	}
	line := p.expect(TOK_FN).Line
	var receiver *Field

	// Если после 'fn' идет '(', значит это метод, привязанный к структуре
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

// MARK: Блок 4 - Разбор блоков кода и исполняемых инструкций (Statements)

// / @brief Разбирает блок инструкций внутри фигурных скобок { ... }.
func (p *Parser) parseBlock() []Stmt {
	var stmts []Stmt
	for p.curTok.Type != TOK_RBRACE && p.curTok.Type != TOK_EOF {
		stmts = append(stmts, p.parseStatement())
	}
	p.expect(TOK_RBRACE)
	return stmts
}

// / @brief Разбирает одну исполняемую инструкцию.
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

	// Если текущий токен — имя и за ним идет ':', это объявление переменной (x: int = 5;)
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

	// Иначе разбираем левое выражение (может оказаться присваиванием или вызовом функции)
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

// / @brief Разбирает условный оператор if cond { ... } else { ... }.
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

// / @brief Разбирает цикл for / parallel for i in start..end step s { ... }.
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

// MARK: Блок 5 - Разбор выражений (Pratt Parser)

// / @brief Разбирает выражение с учетом приоритета операторов.
func (p *Parser) parseExpr(precedence int) Expr {
	left := p.parsePrefix()

	for p.curTok.Type != TOK_SEMI && p.curTok.Type != TOK_EOF {
		// Обработка дженерик-вызова вида nudge<256>(...)
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

// / @brief Разбирает первичные и унарные выражения (числа, имена, скобки, ~x, *p, &x).
func (p *Parser) parsePrefix() Expr {
	line := p.curTok.Line
	switch p.curTok.Type {
	case TOK_NUMBER, TOK_HEX:
		isHex := p.curTok.Type == TOK_HEX
		val := p.curTok.Literal
		p.nextToken()
		return &NumberExpr{Line: line, Value: val, IsHex: isHex}
	case TOK_IDENT, TOK_IN:
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

// / @brief Разбирает аргументы вызова функции или метода (arg1, arg2).
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

// / @brief Проверяет, является ли '<' началом списка шаблонных аргументов перед '('.
func (p *Parser) isGenericCallAhead() bool {
	return p.peekTok.Type == TOK_NUMBER || p.peekTok.Type == TOK_IDENT
}
