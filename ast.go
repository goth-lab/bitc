// Package main implements the BIT-C compiler frontend, semantic analyzer, and C backend.
package main

// MARK: - Core AST Interfaces and Type Representation

// Node is the base interface implemented by every AST node for source location tracking.
type Node interface {
	// PosLine returns the 1-based source line number where the node starts.
	PosLine() int
}

// Expr is the marker interface for all AST nodes that evaluate to a value.
type Expr interface {
	Node
	exprNode()
}

// Stmt is the marker interface for all executable AST statements.
type Stmt interface {
	Node
	stmtNode()
}

// TypeExpr represents a resolved or parsed type annotation in BIT-C
// (e.g., primitives, bvec<N>, pointers *T, fixed arrays T[N], slices, or generic structs).
type TypeExpr struct {
	Line       int      // Source line number of the type annotation
	Name       string   // Base type identifier (e.g., "bvec", "int", "Dense", "slice")
	IsPointer  bool     // True when prefixed with '*' (e.g., *Dense<256, 8>)
	ArraySize  string   // Static array length expression; empty for non-array types
	TypeParams []string // Generic type or bit-width arguments inside '<...>'
}

// PosLine returns the source line number of the TypeExpr.
func (t TypeExpr) PosLine() int { return t.Line }

// MARK: - Top-Level Declarations (Program, Structs, Functions)

// ProgramNode is the root AST node representing a complete BIT-C compilation unit.
type ProgramNode struct {
	Imports []string      // Relative paths to imported BIT-C modules
	Structs []*StructNode // All struct declarations in the module
	Funcs   []*FnNode     // All standalone functions and struct-bound methods
}

// Field describes a struct member or a function/method parameter.
type Field struct {
	Line int      // Source line number
	Name string   // Identifier of the field or parameter
	Type TypeExpr // Declared type of the field or parameter
}

// StructNode represents a user-defined structure (struct Name<T1, T2> { ... }).
type StructNode struct {
	Line       int      // Source line number
	Name       string   // Struct type name
	TypeParams []string // Generic parameter names (e.g., ["InBits", "OutBits"])
	Fields     []Field  // Ordered list of struct fields
}

// PosLine returns the source line number of the StructNode.
func (s *StructNode) PosLine() int { return s.Line }

// FnNode represents a function, external C declaration, or Go-style receiver method.
type FnNode struct {
	Line       int      // Source line number
	Receiver   *Field   // Optional method receiver (e.g., self: *Dense); nil for functions
	Name       string   // Function or method name
	TypeParams []string // Generic parameter names (e.g., ["N"])
	Params     []Field  // Input parameter list
	ReturnType TypeExpr // Return type ("void" when omitted)
	Body       []Stmt   // Executable body statements (empty when IsExtern is true)
	IsExtern   bool     // True for external C runtime functions (extern fn)
}

// PosLine returns the source line number of the FnNode.
func (f *FnNode) PosLine() int { return f.Line }

// MARK: - Executable Statements

// VarDeclStmt represents a local variable declaration (e.g., x: int = 10;).
type VarDeclStmt struct {
	Line  int      // Source line number
	Name  string   // Local variable name
	Type  TypeExpr // Explicit variable type
	Value Expr     // Initializer expression; nil triggers zero-initialization ({0})
}

func (s *VarDeclStmt) PosLine() int { return s.Line }
func (s *VarDeclStmt) stmtNode()    {}

// AssignStmt represents an assignment to an addressable target (target = value;).
type AssignStmt struct {
	Line   int  // Source line number
	Target Expr // Assignable L-value (variable, field, array slot, or bvec bit)
	Value  Expr // Evaluable R-value expression
}

func (s *AssignStmt) PosLine() int { return s.Line }
func (s *AssignStmt) stmtNode()    {}

// ExprStmt wraps a standalone expression executed for side effects (e.g., call()).
type ExprStmt struct {
	Line int  // Source line number
	Expr Expr // Underlying expression
}

func (s *ExprStmt) PosLine() int { return s.Line }
func (s *ExprStmt) stmtNode()    {}

// IfStmt represents a conditional branch (if cond { ... } else { ... }).
type IfStmt struct {
	Line     int    // Source line number
	Cond     Expr   // Boolean condition expression
	ThenBody []Stmt // Statements executed when Cond evaluates to true
	ElseBody []Stmt // Optional statements executed when Cond evaluates to false
}

func (s *IfStmt) PosLine() int { return s.Line }
func (s *IfStmt) stmtNode()    {}

// ForStmt represents a bounded range loop (for i in start..end step s { ... }).
type ForStmt struct {
	Line       int    // Source line number
	IsParallel bool   // True when prefixed with 'parallel' for OpenMP execution
	Iterator   string // Loop counter variable name
	Start      Expr   // Inclusive lower bound expression
	End        Expr   // Exclusive upper bound expression
	Step       Expr   // Optional increment expression; defaults to 1 when nil
	Body       []Stmt // Loop body statements
}

func (s *ForStmt) PosLine() int { return s.Line }
func (s *ForStmt) stmtNode()    {}

// ReturnStmt represents a function return statement (return expr;).
type ReturnStmt struct {
	Line  int  // Source line number
	Value Expr // Returned expression; nil for void functions
}

func (s *ReturnStmt) PosLine() int { return s.Line }
func (s *ReturnStmt) stmtNode()    {}

// PrintStmt represents a built-in diagnostic print statement (print expr;).
type PrintStmt struct {
	Line  int  // Source line number
	Value Expr // Integer or bit-vector expression to print
}

func (s *PrintStmt) PosLine() int { return s.Line }
func (s *PrintStmt) stmtNode()    {}

// MARK: - Expressions

// NumberExpr represents a decimal or hexadecimal integer literal (e.g., 42, 0xFF).
type NumberExpr struct {
	Line  int    // Source line number
	Value string // Raw literal text
	IsHex bool   // True if literal starts with "0x"
}

func (e *NumberExpr) PosLine() int { return e.Line }
func (e *NumberExpr) exprNode()    {}

// IdentExpr represents a variable or symbol reference by name.
type IdentExpr struct {
	Line int    // Source line number
	Name string // Referenced identifier
}

func (e *IdentExpr) PosLine() int { return e.Line }
func (e *IdentExpr) exprNode()    {}

// UnaryExpr represents a prefix unary operation (~x, -x, *ptr, &val).
type UnaryExpr struct {
	Line  int    // Source line number
	Op    string // Unary operator token
	Right Expr   // Operand expression
}

func (e *UnaryExpr) PosLine() int { return e.Line }
func (e *UnaryExpr) exprNode()    {}

// BinaryExpr represents an infix binary operation (e.g., a ^ b, a + b, a << 1).
type BinaryExpr struct {
	Line  int    // Source line number
	Left  Expr   // Left-hand operand
	Op    string // Binary operator token
	Right Expr   // Right-hand operand
}

func (e *BinaryExpr) PosLine() int { return e.Line }
func (e *BinaryExpr) exprNode()    {}

// CallExpr represents a function call, method invocation, or intrinsic (match/count).
type CallExpr struct {
	Line     int      // Source line number
	Callee   Expr     // Target function (IdentExpr) or method receiver expression (FieldExpr)
	TypeArgs []string // Explicit generic arguments at call site (e.g., nudge<256>)
	Args     []Expr   // Positional call arguments
}

func (e *CallExpr) PosLine() int { return e.Line }
func (e *CallExpr) exprNode()    {}

// FieldExpr represents member access on a struct or pointer (obj.field).
type FieldExpr struct {
	Line  int    // Source line number
	Obj   Expr   // Owner struct or pointer expression
	Field string // Member field or method name
}

func (e *FieldExpr) PosLine() int { return e.Line }
func (e *FieldExpr) exprNode()    {}

// IndexExpr represents array element indexing (arr[i]) or single-bit access (vec[i]).
type IndexExpr struct {
	Line  int  // Source line number
	Obj   Expr // Target array or bvec<N> expression
	Index Expr // Zero-based index expression
}

func (e *IndexExpr) PosLine() int { return e.Line }
func (e *IndexExpr) exprNode()    {}
