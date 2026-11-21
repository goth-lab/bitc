package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// MARK: - Semantic Context and Module Loader

// SemaContext holds resolved symbol tables, monomorphized AST declarations,
// inferred expression types, and call target mappings for the entire program.
type SemaContext struct {
	StructTemplates map[string]*StructNode // Parsed generic struct templates
	FuncTemplates   map[string]*FnNode     // Parsed function, method, and test templates
	MonoStructs     map[string]*StructNode // Specialized structs (e.g., Dense_256_8)
	MonoStructOrder []string               // Dependency-ordered struct names (children first)
	MonoFuncs       map[string]*FnNode     // Specialized functions and methods
	ExprTypes       map[Expr]TypeExpr      // Inferred TypeExpr for every AST expression
	ResolvedCalls   map[*CallExpr]string   // Resolved C target identifier for each call
	TestFuncs       []string               // Ordered list of compiled __bitc_test_* names
}

// LoadProgramWithImports parses the root BIT-C source file and recursively merges
// all transitive 'import' dependencies into a single unified ProgramNode.
func LoadProgramWithImports(entryPath string) (*ProgramNode, error) {
	visited := make(map[string]bool)
	master := &ProgramNode{}
	if err := loadFileRecursive(entryPath, visited, master); err != nil {
		return nil, err
	}
	return master, nil
}

// loadFileRecursive reads a single .bc module, prevents cyclic imports via visited,
// and appends its structs and functions to master.
func loadFileRecursive(filePath string, visited map[string]bool, master *ProgramNode) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}
	if visited[absPath] {
		return nil
	}
	visited[absPath] = true

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to import '%s': %w", filePath, err)
	}

	prog := NewParser(NewLexer(string(data))).ParseProgram()
	baseDir := filepath.Dir(absPath)

	for _, imp := range prog.Imports {
		impPath := filepath.Join(baseDir, imp)
		if err := loadFileRecursive(impPath, visited, master); err != nil {
			return err
		}
	}

	master.Structs = append(master.Structs, prog.Structs...)
	master.Funcs = append(master.Funcs, prog.Funcs...)
	return nil
}

// MARK: - Program Analysis and Struct Monomorphization

// AnalyzeProgram performs semantic analysis and template monomorphization.
// When isTestMode is true, all '__bitc_test_*' blocks are instantiated instead of 'main'.
func AnalyzeProgram(prog *ProgramNode, isTestMode bool) (*SemaContext, error) {
	ctx := &SemaContext{
		StructTemplates: make(map[string]*StructNode),
		FuncTemplates:   make(map[string]*FnNode),
		MonoStructs:     make(map[string]*StructNode),
		MonoFuncs:       make(map[string]*FnNode),
		ExprTypes:       make(map[Expr]TypeExpr),
		ResolvedCalls:   make(map[*CallExpr]string),
	}

	for _, s := range prog.Structs {
		ctx.StructTemplates[s.Name] = s
	}
	for _, fn := range prog.Funcs {
		ctx.FuncTemplates[fnKey(fn)] = fn
	}

	if isTestMode {
		// Instantiate every registered unit test block in deterministic order.
		var testKeys []string
		for k := range ctx.FuncTemplates {
			if strings.HasPrefix(k, "__bitc_test_") {
				testKeys = append(testKeys, k)
			}
		}
		sort.Strings(testKeys)
		for _, tk := range testKeys {
			if _, err := ctx.instantiateFunc(ctx.FuncTemplates[tk], nil); err != nil {
				return nil, err
			}
			ctx.TestFuncs = append(ctx.TestFuncs, tk)
		}
		return ctx, nil
	}

	mainFn, ok := ctx.FuncTemplates["main"]
	if !ok {
		return nil, fmt.Errorf("missing entry point: 'fn main()' not found")
	}
	if _, err := ctx.instantiateFunc(mainFn, nil); err != nil {
		return nil, err
	}
	return ctx, nil
}

// fnKey computes a unique symbol table key for a function or receiver method.
func fnKey(fn *FnNode) string {
	if fn.Receiver != nil {
		return fn.Receiver.Type.Name + "." + fn.Name
	}
	return fn.Name
}

// instantiateStruct creates a concrete specialization of a generic struct
// (e.g., Dense<256, 8> -> Dense_256_8) in dependency order and instantiates its methods.
func (ctx *SemaContext) instantiateStruct(t TypeExpr) (TypeExpr, error) {
	if _, alreadyMono := ctx.MonoStructs[t.Name]; alreadyMono {
		return t, nil
	}
	if t.Name == "bvec" || t.Name == "slice" || t.Name == "int" || t.Name == "i32" ||
		t.Name == "i16" || t.Name == "i8" || t.Name == "u8" || t.Name == "bit" || t.Name == "void" {
		return t, nil
	}

	baseName := t.Name
	typeParams := t.TypeParams
	if len(typeParams) == 0 {
		if tmpl, ok := ctx.StructTemplates[baseName]; ok {
			ctx.MonoStructs[baseName] = tmpl
			ctx.MonoStructOrder = append(ctx.MonoStructOrder, baseName)
		}
		return t, nil
	}

	tmpl, ok := ctx.StructTemplates[baseName]
	if !ok {
		return t, fmt.Errorf("[Line %d] unknown generic struct '%s'", t.Line, baseName)
	}
	if len(tmpl.TypeParams) != len(typeParams) {
		return t, fmt.Errorf("[Line %d] struct '%s' expects %d type args, got %d",
			t.Line, baseName, len(tmpl.TypeParams), len(typeParams))
	}

	var safeParts []string
	for _, tp := range typeParams {
		safeParts = append(safeParts, sanitizeMonoPart(tp))
	}
	monoName := baseName + "_" + strings.Join(safeParts, "_")
	resType := TypeExpr{Line: t.Line, Name: monoName, IsPointer: t.IsPointer, ArraySize: t.ArraySize}

	if _, exists := ctx.MonoStructs[monoName]; exists {
		return resType, nil
	}

	subst := make(map[string]string)
	for i, pName := range tmpl.TypeParams {
		subst[pName] = typeParams[i]
	}

	monoStruct := &StructNode{Line: tmpl.Line, Name: monoName}
	ctx.MonoStructs[monoName] = monoStruct

	// Recursively instantiate all field types first so child structs precede parent in C.
	for _, f := range tmpl.Fields {
		subFieldType := substituteType(f.Type, subst)
		instFieldType, err := ctx.instantiateStruct(subFieldType)
		if err != nil {
			return resType, err
		}
		monoStruct.Fields = append(monoStruct.Fields, Field{
			Line: f.Line, Name: f.Name, Type: instFieldType,
		})
	}
	ctx.MonoStructOrder = append(ctx.MonoStructOrder, monoName)

	// Instantiate all methods bound to this struct template.
	for _, fn := range ctx.FuncTemplates {
		if fn.Receiver != nil && fn.Receiver.Type.Name == tmpl.Name {
			if _, err := ctx.instantiateFunc(fn, subst); err != nil {
				return resType, err
			}
		}
	}
	return resType, nil
}

// MARK: - Function Monomorphization and Template Substitution

// instantiateFunc clones a function or method template with concrete type arguments
// and runs type checking over its body statements.
func (ctx *SemaContext) instantiateFunc(tmpl *FnNode, parentSubst map[string]string) (*FnNode, error) {
	subst := make(map[string]string)
	for k, v := range parentSubst {
		subst[k] = v
	}

	monoName := tmpl.Name
	if tmpl.Receiver != nil {
		recBase := tmpl.Receiver.Type.Name
		if len(tmpl.Receiver.Type.TypeParams) > 0 {
			var resolved []string
			for _, tp := range tmpl.Receiver.Type.TypeParams {
				if val, ok := subst[tp]; ok {
					resolved = append(resolved, sanitizeMonoPart(val))
				} else {
					resolved = append(resolved, sanitizeMonoPart(tp))
				}
			}
			recBase = recBase + "_" + strings.Join(resolved, "_")
		}
		monoName = recBase + "_" + tmpl.Name
	} else if len(tmpl.TypeParams) > 0 {
		var resolved []string
		for _, tp := range tmpl.TypeParams {
			resolved = append(resolved, subst[tp])
		}
		monoName = tmpl.Name + "_" + strings.Join(resolved, "_")
	}

	if existing, exists := ctx.MonoFuncs[monoName]; exists {
		return existing, nil
	}

	monoFn := &FnNode{
		Line:       tmpl.Line,
		IsExtern:   tmpl.IsExtern,
		Name:       monoName,
		ReturnType: substituteType(tmpl.ReturnType, subst),
	}

	scope := make(map[string]TypeExpr)
	if tmpl.Receiver != nil {
		recType, err := ctx.instantiateStruct(substituteType(tmpl.Receiver.Type, subst))
		if err != nil {
			return nil, err
		}
		monoFn.Receiver = &Field{Line: tmpl.Receiver.Line, Name: tmpl.Receiver.Name, Type: recType}
		scope[tmpl.Receiver.Name] = recType
	}
	for _, p := range tmpl.Params {
		pType, err := ctx.instantiateStruct(substituteType(p.Type, subst))
		if err != nil {
			return nil, err
		}
		monoFn.Params = append(monoFn.Params, Field{Line: p.Line, Name: p.Name, Type: pType})
		scope[p.Name] = pType
	}

	ctx.MonoFuncs[monoName] = monoFn
	if tmpl.IsExtern {
		return monoFn, nil
	}

	monoFn.Body = substituteStmts(tmpl.Body, subst)
	if err := ctx.checkStmts(monoFn.Body, scope, monoFn.ReturnType); err != nil {
		return nil, fmt.Errorf("in function '%s': %w", monoName, err)
	}
	return monoFn, nil
}

// substituteType replaces template parameter names inside a TypeExpr with concrete values,
// expanding nested generic arguments (e.g., T -> bvec<64>) while still substituting ArraySize.
func substituteType(t TypeExpr, subst map[string]string) TypeExpr {
	res := t
	if val, ok := subst[res.Name]; ok {
		if base, rest, found := strings.Cut(val, "<"); found {
			res.Name = base
			inner := strings.TrimSuffix(rest, ">")
			res.TypeParams = strings.Split(inner, ",")
		} else {
			res.Name = val
		}
	}
	if val, ok := subst[res.ArraySize]; ok {
		res.ArraySize = val
	}
	if len(t.TypeParams) > 0 {
		res.TypeParams = make([]string, len(t.TypeParams))
		for i, tp := range t.TypeParams {
			if val, ok := subst[tp]; ok {
				res.TypeParams[i] = val
			} else {
				res.TypeParams[i] = tp
			}
		}
	}
	return res
}

// MARK: - AST Cloning and Constant Substitution

// substituteStmts deep-clones statements while replacing template constants.
func substituteStmts(stmts []Stmt, subst map[string]string) []Stmt {
	var res []Stmt
	for _, s := range stmts {
		switch st := s.(type) {
		case *VarDeclStmt:
			res = append(res, &VarDeclStmt{
				Line: st.Line, Name: st.Name,
				Type: substituteType(st.Type, subst), Value: substituteExpr(st.Value, subst),
			})
		case *AssignStmt:
			res = append(res, &AssignStmt{
				Line: st.Line, Target: substituteExpr(st.Target, subst), Value: substituteExpr(st.Value, subst),
			})
		case *ExprStmt:
			res = append(res, &ExprStmt{Line: st.Line, Expr: substituteExpr(st.Expr, subst)})
		case *IfStmt:
			res = append(res, &IfStmt{
				Line: st.Line, Cond: substituteExpr(st.Cond, subst),
				ThenBody: substituteStmts(st.ThenBody, subst), ElseBody: substituteStmts(st.ElseBody, subst),
			})
		case *ForStmt:
			res = append(res, &ForStmt{
				Line: st.Line, IsParallel: st.IsParallel, Iterator: st.Iterator,
				Start: substituteExpr(st.Start, subst), End: substituteExpr(st.End, subst),
				Step: substituteExpr(st.Step, subst), Body: substituteStmts(st.Body, subst),
			})
		case *ReturnStmt:
			res = append(res, &ReturnStmt{Line: st.Line, Value: substituteExpr(st.Value, subst)})
		case *PrintStmt:
			res = append(res, &PrintStmt{Line: st.Line, Value: substituteExpr(st.Value, subst)})
		}
	}
	return res
}

// substituteExpr deep-clones expressions and converts matched template identifiers into NumberExprs.
func substituteExpr(e Expr, subst map[string]string) Expr {
	if e == nil {
		return nil
	}
	switch ex := e.(type) {
	case *IdentExpr:
		if val, ok := subst[ex.Name]; ok {
			return &NumberExpr{Line: ex.Line, Value: val}
		}
		return &IdentExpr{Line: ex.Line, Name: ex.Name}
	case *NumberExpr:
		return &NumberExpr{Line: ex.Line, Value: ex.Value, IsHex: ex.IsHex}
	case *UnaryExpr:
		return &UnaryExpr{Line: ex.Line, Op: ex.Op, Right: substituteExpr(ex.Right, subst)}
	case *BinaryExpr:
		return &BinaryExpr{Line: ex.Line, Left: substituteExpr(ex.Left, subst), Op: ex.Op, Right: substituteExpr(ex.Right, subst)}
	case *FieldExpr:
		return &FieldExpr{Line: ex.Line, Obj: substituteExpr(ex.Obj, subst), Field: ex.Field}
	case *IndexExpr:
		return &IndexExpr{Line: ex.Line, Obj: substituteExpr(ex.Obj, subst), Index: substituteExpr(ex.Index, subst)}
	case *CallExpr:
		var args []Expr
		for _, a := range ex.Args {
			args = append(args, substituteExpr(a, subst))
		}
		var tArgs []string
		for _, ta := range ex.TypeArgs {
			if val, ok := subst[ta]; ok {
				tArgs = append(tArgs, val)
			} else {
				tArgs = append(tArgs, ta)
			}
		}
		return &CallExpr{Line: ex.Line, Callee: substituteExpr(ex.Callee, subst), TypeArgs: tArgs, Args: args}
	}
	return e
}

// MARK: - Statement Type Checking, Expression Inference, and Call Resolution

// checkStmts validates types across a block of statements in the given variable scope.
func (ctx *SemaContext) checkStmts(stmts []Stmt, scope map[string]TypeExpr, expectedRet TypeExpr) error {
	for _, s := range stmts {
		switch st := s.(type) {
		case *VarDeclStmt:
			instType, err := ctx.instantiateStruct(st.Type)
			if err != nil {
				return err
			}
			st.Type = instType
			if st.Value != nil {
				if _, err := ctx.inferExpr(st.Value, scope); err != nil {
					return err
				}
			}
			scope[st.Name] = st.Type
		case *AssignStmt:
			if _, err := ctx.inferExpr(st.Target, scope); err != nil {
				return err
			}
			if _, err := ctx.inferExpr(st.Value, scope); err != nil {
				return err
			}
		case *ExprStmt:
			if _, err := ctx.inferExpr(st.Expr, scope); err != nil {
				return err
			}
		case *IfStmt:
			if _, err := ctx.inferExpr(st.Cond, scope); err != nil {
				return err
			}
			if err := ctx.checkStmts(st.ThenBody, scope, expectedRet); err != nil {
				return err
			}
			if err := ctx.checkStmts(st.ElseBody, scope, expectedRet); err != nil {
				return err
			}
		case *ForStmt:
			scope[st.Iterator] = TypeExpr{Line: st.Line, Name: "int"}
			if err := ctx.checkStmts(st.Body, scope, expectedRet); err != nil {
				return err
			}
		case *ReturnStmt:
			if st.Value != nil {
				if _, err := ctx.inferExpr(st.Value, scope); err != nil {
					return err
				}
			}
		case *PrintStmt:
			if _, err := ctx.inferExpr(st.Value, scope); err != nil {
				return err
			}
		}
	}
	return nil
}

// inferExpr computes and caches the resulting TypeExpr for any expression AST node.
func (ctx *SemaContext) inferExpr(e Expr, scope map[string]TypeExpr) (TypeExpr, error) {
	if e == nil {
		return TypeExpr{Name: "void"}, nil
	}
	var res TypeExpr
	switch ex := e.(type) {
	case *NumberExpr:
		res = TypeExpr{Line: ex.Line, Name: "int"}
	case *IdentExpr:
		t, ok := scope[ex.Name]
		if !ok {
			return res, fmt.Errorf("[Line %d] undefined identifier '%s'", ex.Line, ex.Name)
		}
		res = t
	case *UnaryExpr:
		rt, err := ctx.inferExpr(ex.Right, scope)
		if err != nil {
			return res, err
		}
		switch ex.Op {
		case "*":
			rt.IsPointer = false
		case "&":
			rt.IsPointer = true
		}
		res = rt
	case *BinaryExpr:
		lt, err := ctx.inferExpr(ex.Left, scope)
		if err != nil {
			return res, err
		}
		if _, err := ctx.inferExpr(ex.Right, scope); err != nil {
			return res, err
		}
		if ex.Op == ">" || ex.Op == "<" || ex.Op == "==" || ex.Op == "!=" || ex.Op == "<=" || ex.Op == ">=" {
			res = TypeExpr{Line: ex.Line, Name: "bit"}
		} else {
			res = lt
		}
	case *IndexExpr:
		ot, err := ctx.inferExpr(ex.Obj, scope)
		if err != nil {
			return res, err
		}
		if _, err := ctx.inferExpr(ex.Index, scope); err != nil {
			return res, err
		}
		if ot.ArraySize != "" {
			ot.ArraySize = ""
			res = ot
		} else if ot.Name == "bvec" {
			res = TypeExpr{Line: ex.Line, Name: "bit"}
		}
	case *FieldExpr:
		ot, err := ctx.inferExpr(ex.Obj, scope)
		if err != nil {
			return res, err
		}
		st, ok := ctx.MonoStructs[ot.Name]
		if !ok {
			return res, fmt.Errorf("[Line %d] type '%s' has no fields", ex.Line, ot.Name)
		}
		for _, f := range st.Fields {
			if f.Name == ex.Field {
				res = f.Type
				break
			}
		}
	case *CallExpr:
		t, err := ctx.resolveCall(ex, scope)
		if err != nil {
			return res, err
		}
		res = t
	}
	ctx.ExprTypes[e] = res
	return res, nil
}

// resolveCall resolves built-in intrinsics (match, count), functions, and receiver methods.
func (ctx *SemaContext) resolveCall(call *CallExpr, scope map[string]TypeExpr) (TypeExpr, error) {
	if ident, ok := call.Callee.(*IdentExpr); ok {
		if ident.Name == "match" {
			if len(call.Args) != 2 {
				return TypeExpr{}, fmt.Errorf("[Line %d] match() expects 2 arguments", call.Line)
			}
			t1, err := ctx.inferExpr(call.Args[0], scope)
			if err != nil {
				return TypeExpr{}, err
			}
			t2, err := ctx.inferExpr(call.Args[1], scope)
			if err != nil {
				return TypeExpr{}, err
			}
			if t1.Name != "bvec" || t2.Name != "bvec" || t1.TypeParams[0] != t2.TypeParams[0] {
				return TypeExpr{}, fmt.Errorf("[Line %d] match() requires two bvec<N> of equal width", call.Line)
			}
			ctx.ResolvedCalls[call] = "match"
			return t1, nil
		}
		if ident.Name == "count" {
			if len(call.Args) != 1 {
				return TypeExpr{}, fmt.Errorf("[Line %d] count() expects 1 argument", call.Line)
			}
			t1, err := ctx.inferExpr(call.Args[0], scope)
			if err != nil {
				return TypeExpr{}, err
			}
			if t1.Name != "bvec" {
				return TypeExpr{}, fmt.Errorf("[Line %d] count() requires a bvec<N> argument", call.Line)
			}
			ctx.ResolvedCalls[call] = "count"
			return TypeExpr{Line: call.Line, Name: "int"}, nil
		}

		for _, a := range call.Args {
			if _, err := ctx.inferExpr(a, scope); err != nil {
				return TypeExpr{}, err
			}
		}

		tmpl, ok := ctx.FuncTemplates[ident.Name]
		if !ok {
			return TypeExpr{}, fmt.Errorf("[Line %d] undefined function '%s'", call.Line, ident.Name)
		}
		subst := make(map[string]string)
		for i, tp := range tmpl.TypeParams {
			if i < len(call.TypeArgs) {
				subst[tp] = call.TypeArgs[i]
			}
		}
		monoFn, err := ctx.instantiateFunc(tmpl, subst)
		if err != nil {
			return TypeExpr{}, err
		}
		ctx.ResolvedCalls[call] = monoFn.Name
		return monoFn.ReturnType, nil
	}

	if fieldEx, ok := call.Callee.(*FieldExpr); ok {
		objType, err := ctx.inferExpr(fieldEx.Obj, scope)
		if err != nil {
			return TypeExpr{}, err
		}
		for _, a := range call.Args {
			if _, err := ctx.inferExpr(a, scope); err != nil {
				return TypeExpr{}, err
			}
		}
		methodName := objType.Name + "_" + fieldEx.Field
		monoFn, ok := ctx.MonoFuncs[methodName]
		if !ok {
			return TypeExpr{}, fmt.Errorf("[Line %d] type '%s' has no method '%s'", call.Line, objType.Name, fieldEx.Field)
		}
		ctx.ResolvedCalls[call] = monoFn.Name
		return monoFn.ReturnType, nil
	}

	return TypeExpr{}, fmt.Errorf("[Line %d] invalid call target", call.Line)
}

// getBitWidth extracts the integer bit width from a bvec<N> TypeExpr (returns 0 otherwise).
func getBitWidth(t TypeExpr) int {
	if t.Name == "bvec" && len(t.TypeParams) > 0 {
		w, _ := strconv.Atoi(t.TypeParams[0])
		return w
	}
	return 0
}

// sanitizeMonoPart converts a generic type argument (e.g., "bvec<64>") into a C-safe identifier ("bvec_64").
func sanitizeMonoPart(arg string) string {
	s := strings.ReplaceAll(arg, "<", "_")
	s = strings.ReplaceAll(s, ">", "")
	s = strings.ReplaceAll(s, ",", "_")
	return s
}
