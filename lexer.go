/// @file lexer.go
/// @brief Лексический анализатор (токенизатор) ядра языка BIT-C v1.0.
/// @details Преобразует исходный текст программы в поток типизированных токенов
/// с отслеживанием номеров строк для точной диагностики ошибок.

package main

import (
	"strings"
	"unicode"
)

// MARK: Блок 1 - Определение типов токенов и структур лексера

// / @brief Тип лексического токена ядра языка BIT-C.
type TokenType string

const (
	TOK_EOF     TokenType = "EOF"     ///< Конец исходного файла
	TOK_ILLEGAL TokenType = "ILLEGAL" ///< Неизвестный или ошибочный символ
	TOK_IDENT   TokenType = "IDENT"   ///< Идентификатор (имя переменной, типа, функции)
	TOK_NUMBER  TokenType = "NUMBER"  ///< Десятичное целое число
	TOK_HEX     TokenType = "HEX"     ///< Шестнадцатеричная битовая маска (0x...)
	TOK_STRING  TokenType = "STRING"  ///< Строковый литерал в двойных кавычках

	// Ключевые слова ядра языка
	TOK_STRUCT   TokenType = "STRUCT"   ///< Объявление структуры (struct)
	TOK_FN       TokenType = "FN"       ///< Объявление функции или метода (fn)
	TOK_IMPORT   TokenType = "IMPORT"   ///< Подключение модуля библиотеки (import)
	TOK_FOR      TokenType = "FOR"      ///< Начало цикла (for)
	TOK_PARALLEL TokenType = "PARALLEL" ///< Модификатор многопоточности (parallel)
	TOK_IN       TokenType = "IN"       ///< Разделитель диапазона в цикле (in)
	TOK_STEP     TokenType = "STEP"     ///< Шаг итерации цикла (step)
	TOK_IF       TokenType = "IF"       ///< Условный оператор (if)
	TOK_ELSE     TokenType = "ELSE"     ///< Альтернативная ветка условия (else)
	TOK_RETURN   TokenType = "RETURN"   ///< Возврат значения из функции (return)
	TOK_PRINT    TokenType = "PRINT"    ///< Отладочный вывод (print)
	TOK_EXTERN   TokenType = "EXTERN"   ///< Объявление внешней системной функции (extern)

	// Арифметические, побитовые и логические операторы
	TOK_ASSIGN  TokenType = "="  ///< Присваивание
	TOK_PLUS    TokenType = "+"  ///< Сложение
	TOK_MINUS   TokenType = "-"  ///< Вычитание
	TOK_STAR    TokenType = "*"  ///< Умножение или разыменование указателя
	TOK_SLASH   TokenType = "/"  ///< Целочисленное деление
	TOK_PERCENT TokenType = "%"  ///< Остаток от деления
	TOK_AMP     TokenType = "&"  ///< Побитовое И или взятие адреса переменной
	TOK_PIPE    TokenType = "|"  ///< Побитовое ИЛИ
	TOK_CARET   TokenType = "^"  ///< Побитовое исключающее ИЛИ (XOR)
	TOK_TILDE   TokenType = "~"  ///< Побитовая инверсия (NOT)
	TOK_SHL     TokenType = "<<" ///< Побитовый сдвиг влево
	TOK_SHR     TokenType = ">>" ///< Побитовый сдвиг вправо

	// Операторы сравнения
	TOK_EQ  TokenType = "==" ///< Равно
	TOK_NEQ TokenType = "!=" ///< Не равно
	TOK_LT  TokenType = "<"  ///< Меньше или открывающая угловая скобка дженерика
	TOK_GT  TokenType = ">"  ///< Больше или закрывающая угловая скобка дженерика
	TOK_LE  TokenType = "<=" ///< Меньше или равно
	TOK_GE  TokenType = ">=" ///< Больше или равно

	// Разделители и скобки
	TOK_COLON    TokenType = ":"  ///< Двоеточие (указание типа)
	TOK_SEMI     TokenType = ";"  ///< Точка с запятой
	TOK_COMMA    TokenType = ","  ///< Запятая
	TOK_DOT      TokenType = "."  ///< Точка (доступ к полю или методу структуры)
	TOK_RANGE    TokenType = ".." ///< Диапазон значений (0..N)
	TOK_ARROW    TokenType = "->" ///< Указание возвращаемого типа функции
	TOK_LPAREN   TokenType = "("  ///< Левая круглая скобка
	TOK_RPAREN   TokenType = ")"  ///< Правая круглая скобка
	TOK_LBRACE   TokenType = "{"  ///< Левая фигурная скобка
	TOK_RBRACE   TokenType = "}"  ///< Правая фигурная скобка
	TOK_LBRACKET TokenType = "["  ///< Левая квадратная скобка
	TOK_RBRACKET TokenType = "]"  ///< Правая квадратная скобка
)

// / @brief Структура отдельного токена исходного кода.
type Token struct {
	Type    TokenType ///< Категория токена
	Literal string    ///< Текстовое представление токена в коде
	Line    int       ///< Номер строки в исходном файле
}

// / @brief Таблица зарезервированных ключевых слов ядра BIT-C.
var keywords = map[string]TokenType{
	"struct":   TOK_STRUCT,
	"fn":       TOK_FN,
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

// MARK: Блок 2 - Инициализация лексера и основной цикл сканирования

// / @brief Состояние лексического анализатора.
type Lexer struct {
	input string ///< Исходный код программы
	pos   int    ///< Текущая позиция курсора в строке input
	line  int    ///< Текущий номер строки
}

// / @brief Создает и инициализирует новый экземпляр лексера.
// / @param input Строка с исходным кодом на языке BIT-C.
// / @return Указатель на готовый к работе Lexer.
func NewLexer(input string) *Lexer {
	return &Lexer{input: input, pos: 0, line: 1}
}

// / @brief Извлекает следующий токен из входного потока.
// / @return Структура Token с типом, текстом и номером строки.
func (l *Lexer) NextToken() Token {
	l.skipWhitespaceAndComments()

	if l.pos >= len(l.input) {
		return Token{Type: TOK_EOF, Literal: "", Line: l.line}
	}

	ch := l.input[l.pos]
	nextCh := l.peek()

	// Проверка двухсимвольных операторов
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

	// Проверка односимвольных операторов и разделителей
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

	// Строковые литералы в кавычках (для путей import)
	if ch == '"' {
		return l.readString()
	}

	// Идентификаторы и ключевые слова (разрешаем символ '_' в именах)
	if unicode.IsLetter(rune(ch)) || ch == '_' {
		return l.readIdentifier()
	}

	// Десятичные и шестнадцатеричные числа
	if unicode.IsDigit(rune(ch)) {
		return l.readNumber()
	}

	l.pos++
	return Token{Type: TOK_ILLEGAL, Literal: string(ch), Line: l.line}
}

// MARK: Блок 3 - Вспомогательные методы чтения литералов и пропуска комментариев

// / @brief Считывает строковый литерал до закрывающей кавычки.
func (l *Lexer) readString() Token {
	l.pos++ // Пропускаем открывающую кавычку
	start := l.pos
	for l.pos < len(l.input) && l.input[l.pos] != '"' && l.input[l.pos] != '\n' {
		l.pos++
	}
	str := l.input[start:l.pos]
	if l.pos < len(l.input) && l.input[l.pos] == '"' {
		l.pos++ // Пропускаем закрывающую кавычку
	}
	return Token{Type: TOK_STRING, Literal: str, Line: l.line}
}

// / @brief Считывает слово и определяет, является ли оно ключевым словом или именем.
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

// / @brief Считывает числовой литерал (обычный десятичный или 0x-шестнадцатеричный).
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

// / @brief Возвращает следующий символ без сдвига курсора.
func (l *Lexer) peek() byte {
	if l.pos+1 >= len(l.input) {
		return 0
	}
	return l.input[l.pos+1]
}

// / @brief Пропускает пробельные символы, переносы строк и однострочные комментарии //.
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
