/// @file ast.go
/// @brief Определение узлов абстрактного синтаксического дерева (AST) языка BIT-C v1.0.
/// @details Содержит структуры для представления типов данных, выражений (Expr),
/// инструкций управления потоком (Stmt) и объявлений верхнего уровня (Decl).

package main

// MARK: Блок 1 - Базовые интерфейсы узлов и система типов

// / @brief Базовый интерфейс любого узла синтаксического дерева.
type Node interface {
	PosLine() int ///< Возвращает номер строки в исходном файле для диагностики ошибок
}

// / @brief Базовый интерфейс для любых выражений (возвращают значение).
type Expr interface {
	Node
	exprNode()
}

// / @brief Базовый интерфейс для исполняемых инструкций (не возвращают значение).
type Stmt interface {
	Node
	stmtNode()
}

// / @brief Представление типа данных в языке BIT-C.
// / @details Поддерживает примитивы (int, bvec<256>), указатели (*T),
// / статические массивы (T[N]), срезы (slice<T>) и дженерик-структуры (Dense<256, 8>).
type TypeExpr struct {
	Line       int      ///< Номер строки объявления типа
	Name       string   ///< Базовое имя типа (например, "bvec", "int", "Dense", "slice")
	IsPointer  bool     ///< Флаг указателя (*Type)
	ArraySize  string   ///< Размер статического массива (пусто, если это не массив)
	TypeParams []string ///< Параметры в угловых скобках <256, 8> или <N>
}

func (t TypeExpr) PosLine() int { return t.Line }

// MARK: Блок 2 - Узлы верхнего уровня (Программа, Импорты, Структуры, Функции)

// / @brief Корневой узел всей скомпилированной программы BIT-C.
type ProgramNode struct {
	Imports []string      ///< Список импортированных файлов стандартной библиотеки
	Structs []*StructNode ///< Объявления структур данных
	Funcs   []*FnNode     ///< Объявления функций и привязанных к структурам методов
}

// / @brief Описание отдельного поля структуры или параметра функции.
type Field struct {
	Line int      ///< Номер строки
	Name string   ///< Имя поля или параметра
	Type TypeExpr ///< Тип данных
}

// / @brief Объявление структуры данных (struct Name<T1, T2> { ... }).
type StructNode struct {
	Line       int      ///< Номер строки
	Name       string   ///< Имя структуры
	TypeParams []string ///< Имена параметров шаблона (например, ["InBits", "OutBits"])
	Fields     []Field  ///< Список полей структуры
}

func (s *StructNode) PosLine() int { return s.Line }

// / @brief Объявление функции или Go-метода (fn (self: *T) name(...) -> Ret { ... }).
type FnNode struct {
	Line       int      ///< Номер строки
	Receiver   *Field   ///< Получатель метода (nil для обычных функций)
	Name       string   ///< Имя функции или метода
	TypeParams []string ///< Параметры шаблона функции (например, <N>)
	Params     []Field  ///< Список входных аргументов
	ReturnType TypeExpr ///< Возвращаемый тип (Name == "void", если не указан)
	Body       []Stmt   ///< Тело функции (список инструкций)
	IsExtern   bool     ///< Флаг внешней системной функции (без тела на BIT-C)
}

func (f *FnNode) PosLine() int { return f.Line }

// MARK: Блок 3 - Исполняемые инструкции (Statements)

// / @brief Объявление локальной переменной (x: int = 10; или arr: bvec<64>[4];).
type VarDeclStmt struct {
	Line  int      ///< Номер строки
	Name  string   ///< Имя переменной
	Type  TypeExpr ///< Явно указанный тип переменной
	Value Expr     ///< Начальное значение (nil, если инициализируется нулями)
}

func (s *VarDeclStmt) PosLine() int { return s.Line }
func (s *VarDeclStmt) stmtNode()    {}

// / @brief Инструкция присваивания (target = value;).
// / @details Целью может быть переменная, поле структуры (a.b), элемент массива (a[i]) или указатель (*p).
type AssignStmt struct {
	Line   int  ///< Номер строки
	Target Expr ///< Левая часть присваивания (L-value)
	Value  Expr ///< Правая вычисляемая часть (R-value)
}

func (s *AssignStmt) PosLine() int { return s.Line }
func (s *AssignStmt) stmtNode()    {}

// / @brief Вызов функции или метода как самостоятельная инструкция (без присваивания).
type ExprStmt struct {
	Line int  ///< Номер строки
	Expr Expr ///< Вычисляемое выражение (обычно CallExpr)
}

func (s *ExprStmt) PosLine() int { return s.Line }
func (s *ExprStmt) stmtNode()    {}

// / @brief Условное ветвление (if cond { ... } else { ... }).
type IfStmt struct {
	Line     int    ///< Номер строки
	Cond     Expr   ///< Условие проверки
	ThenBody []Stmt ///< Ветка истины
	ElseBody []Stmt ///< Ветка лжи (может быть пустой)
}

func (s *IfStmt) PosLine() int { return s.Line }
func (s *IfStmt) stmtNode()    {}

// / @brief Цикл по диапазону (for i in 0..N step S { ... } или parallel for ...).
type ForStmt struct {
	Line       int    ///< Номер строки
	IsParallel bool   ///< Флаг распараллеливания по ядрам CPU (parallel for)
	Iterator   string ///< Имя переменной-счетчика
	Start      Expr   ///< Начальное значение диапазона
	End        Expr   ///< Конечное значение диапазона (не включительно)
	Step       Expr   ///< Шаг итерации (nil, если шаг по умолчанию равен 1)
	Body       []Stmt ///< Тело цикла
}

func (s *ForStmt) PosLine() int { return s.Line }
func (s *ForStmt) stmtNode()    {}

// / @brief Возврат значения из функции (return expr;) и отладочный вывод (print expr;).
type ReturnStmt struct {
	Line  int  ///< Номер строки
	Value Expr ///< Возвращаемое выражение (nil для пустого return;)
}

func (s *ReturnStmt) PosLine() int { return s.Line }
func (s *ReturnStmt) stmtNode()    {}

type PrintStmt struct {
	Line  int  ///< Номер строки
	Value Expr ///< Выражение для вывода в консоль
}

func (s *PrintStmt) PosLine() int { return s.Line }
func (s *PrintStmt) stmtNode()    {}

// MARK: Блок 4 - Атомарные выражения (Expressions)

// / @brief Числовой или шестнадцатеричный литерал (42, 0xFF00).
type NumberExpr struct {
	Line  int    ///< Номер строки
	Value string ///< Строковое представление числа
	IsHex bool   ///< Флаг шестнадцатеричного формата (0x...)
}

func (e *NumberExpr) PosLine() int { return e.Line }
func (e *NumberExpr) exprNode()    {}

// / @brief Обращение к переменной по имени.
type IdentExpr struct {
	Line int    ///< Номер строки
	Name string ///< Имя идентификатора
}

func (e *IdentExpr) PosLine() int { return e.Line }
func (e *IdentExpr) exprNode()    {}

// / @brief Унарная операция (~x, -x, *ptr, &val).
type UnaryExpr struct {
	Line  int    ///< Номер строки
	Op    string ///< Оператор ("~", "-", "*", "&")
	Right Expr   ///< Операнд
}

func (e *UnaryExpr) PosLine() int { return e.Line }
func (e *UnaryExpr) exprNode()    {}

// / @brief Бинарная операция (a + b, a ^ b, a & b, a > b, a << 1).
type BinaryExpr struct {
	Line  int    ///< Номер строки
	Left  Expr   ///< Левый операнд
	Op    string ///< Оператор ("+", "-", "*", "/", "%", "&", "|", "^", "<<", ">>", "==", "!=", "<", ">", "<=", ">=")
	Right Expr   ///< Правый операнд
}

func (e *BinaryExpr) PosLine() int { return e.Line }
func (e *BinaryExpr) exprNode()    {}

// / @brief Вызов функции, метода или встроенного интринсика (match, count).
type CallExpr struct {
	Line     int      ///< Номер строки
	Callee   Expr     ///< Вызываемая сущность (IdentExpr или FieldExpr для obj.method)
	TypeArgs []string ///< Явные параметры шаблона при вызове (например, nudge<256>(...))
	Args     []Expr   ///< Список передаваемых аргументов
}

func (e *CallExpr) PosLine() int { return e.Line }
func (e *CallExpr) exprNode()    {}

// / @brief Доступ к полю или методу структуры через точку (obj.field).
type FieldExpr struct {
	Line  int    ///< Номер строки
	Obj   Expr   ///< Выражение-владелец структуры
	Field string ///< Имя поля или метода
}

func (e *FieldExpr) PosLine() int { return e.Line }
func (e *FieldExpr) exprNode()    {}

// / @brief Индексация массива или отдельного бита вектора (arr[i] или vec[0]).
type IndexExpr struct {
	Line  int  ///< Номер строки
	Obj   Expr ///< Массив или битовый вектор
	Index Expr ///< Выражение индекса
}

func (e *IndexExpr) PosLine() int { return e.Line }
func (e *IndexExpr) exprNode()    {}
