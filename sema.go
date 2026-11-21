/// @file sema.go
/// @brief Семантический анализатор, резолвер импортов и мономорфизатор шаблонов BIT-C v1.0.
/// @details Подгружает модули стандартной библиотеки (import), разворачивает дженерик-структуры
/// и функции (<InBits, OutBits>) в конкретные специализации и проверяет совместимость типов.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MARK: Блок 1 - Структуры контекста проверки и загрузчик модулей import

// / @brief Контекст проверки типов и мономорфизации всей программы.
type SemaContext struct {
	StructTemplates map[string]*StructNode ///< Исходные шаблоны структур
	FuncTemplates   map[string]*FnNode     ///< Исходные шаблоны функций и методов
	MonoStructs     map[string]*StructNode ///< Конкретные специализации структур (Dense_256_8)
	MonoFuncs       map[string]*FnNode     ///< Конкретные специализации функций и методов
	ExprTypes       map[Expr]TypeExpr      ///< Карта выведенных типов для каждого выражения AST
	ResolvedCalls   map[*CallExpr]string   ///< Карта точных имен C-функций для каждого вызова
	LoadedFiles     map[string]bool        ///< Защита от повторного импорта файлов
}

// / @brief Загружает главный файл программы и рекурсивно подтягивает все import.
// / @param entryPath Путь к главному .bc файлу.
// / @return Объединенное дерево ProgramNode со всеми библиотечными функциями.
func LoadProgramWithImports(entryPath string) (*ProgramNode, error) {
	visited := make(map[string]bool)
	master := &ProgramNode{}
	if err := loadFileRecursive(entryPath, visited, master); err != nil {
		return nil, err
	}
	return master, nil
}

// / @brief Вспомогательная рекурсивная функция чтения и парсинга .bc модулей.
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

// MARK: Блок 2 - Точка входа семантики и мономорфизация структур

// / @brief Выполняет полный семантический анализ и специализацию шаблонов.
// / @param prog Объединенное дерево программы.
// / @return Готовый контекст SemaContext для генератора C-кода или ошибка.
func AnalyzeProgram(prog *ProgramNode) (*SemaContext, error) {
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
		key := fnKey(fn)
		ctx.FuncTemplates[key] = fn
	}

	mainFn, ok := ctx.FuncTemplates["main"]
	if !ok {
		return nil, fmt.Errorf("missing entry point: 'fn main()' not found")
	}

	// Запускаем проверку и инстанцирование шаблонов начиная с функции main()
	if _, err := ctx.instantiateFunc(mainFn, nil); err != nil {
		return nil, err
	}
	return ctx, nil
}

// / @brief Формирует уникальный ключ функции или метода (например, "Dense.forward" или "nudge").
func fnKey(fn *FnNode) string {
	if fn.Receiver != nil {
		return fn.Receiver.Type.Name + "." + fn.Name
	}
	return fn.Name
}

// / @brief Создает конкретную копию структуры с подстановкой параметров шаблона (Dense<256, 8> -> Dense_256_8).
func (ctx *SemaContext) instantiateStruct(t TypeExpr) (TypeExpr, error) {
	if t.Name == "bvec" || t.Name == "slice" || t.Name == "int" || t.Name == "bit" || t.Name == "void" {
		return t, nil
	}

	baseName := t.Name
	typeParams := t.TypeParams
	if len(typeParams) == 0 {
		if base, rest, found := strings.Cut(baseName, "_"); found {
			baseName = base
			typeParams = strings.Split(rest, "_")
		}
	}

	if len(typeParams) == 0 {
		if tmpl, ok := ctx.StructTemplates[baseName]; ok {
			ctx.MonoStructs[baseName] = tmpl
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

	monoName := baseName + "_" + strings.Join(typeParams, "_")
	resType := TypeExpr{Line: t.Line, Name: monoName, IsPointer: t.IsPointer, ArraySize: t.ArraySize}

	if _, exists := ctx.MonoStructs[monoName]; exists {
		return resType, nil
	}

	subst := make(map[string]string)
	for i, pName := range tmpl.TypeParams {
		subst[pName] = typeParams[i]
	}

	monoStruct := &StructNode{Line: tmpl.Line, Name: monoName}
	for _, f := range tmpl.Fields {
		subFieldType := substituteType(f.Type, subst)
		monoStruct.Fields = append(monoStruct.Fields, Field{Line: f.Line, Name: f.Name, Type: subFieldType})
	}
	ctx.MonoStructs[monoName] = monoStruct

	for _, fn := range ctx.FuncTemplates {
		if fn.Receiver != nil && fn.Receiver.Type.Name == tmpl.Name {
			if _, err := ctx.instantiateFunc(fn, subst); err != nil {
				return resType, err
			}
		}
	}
	return resType, nil
}

// MARK: Блок 3 - Специализация функций и методов (Мономорфизация)

// / @brief Создает конкретную специализацию функции или метода и проверяет её тело.
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
					resolved = append(resolved, val)
				} else {
					resolved = append(resolved, tp)
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
		recType := substituteType(tmpl.Receiver.Type, subst)
		monoFn.Receiver = &Field{Line: tmpl.Receiver.Line, Name: tmpl.Receiver.Name, Type: recType}
		scope[tmpl.Receiver.Name] = recType
	}

	for _, p := range tmpl.Params {
		pType := substituteType(p.Type, subst)
		monoFn.Params = append(monoFn.Params, Field{Line: p.Line, Name: p.Name, Type: pType})
		scope[p.Name] = pType
	}

	ctx.MonoFuncs[monoName] = monoFn

	if tmpl.IsExtern {
		return monoFn, nil
	}

	// Клонируем и подставляем константы шаблонов внутрь инструкций тела функции
	monoFn.Body = substituteStmts(tmpl.Body, subst)
	if err := ctx.checkStmts(monoFn.Body, scope, monoFn.ReturnType); err != nil {
		return nil, fmt.Errorf("in function '%s': %w", monoName, err)
	}
	return monoFn, nil
}

// / @brief Подставляет значения параметров шаблона в описание типа (bvec<InBits> -> bvec<256>).
func substituteType(t TypeExpr, subst map[string]string) TypeExpr {
	res := t
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
		if res.Name != "bvec" && res.Name != "slice" {
			res.Name = res.Name + "_" + strings.Join(res.TypeParams, "_")
			res.TypeParams = nil
		}
	}
	return res
}

// MARK: Блок 4 - Клонирование и подстановка параметров шаблона в AST

// / @brief Заменяет идентификаторы параметров шаблона (OutBits -> 8) в списке инструкций.
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

// / @brief Заменяет идентификаторы параметров шаблона в выражениях (например, 0..OutBits -> 0..8).
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

// MARK: Блок 5 - Семантическая проверка инструкций и вывод типов выражений

// / @brief Проверяет корректность типов всех инструкций внутри функции.
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

// / @brief Выводит тип выражения и разрешает вызовы шаблонных функций и методов.
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

// / @brief Разрешает вызов встроенных интринсиков (match, count), функций и Go-методов.
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

	// Вызов Go-метода через точку: layer1.forward(x)
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

// / @brief Извлекает числовую ширину битового вектора из TypeExpr (например, bvec<256> -> 256).
func getBitWidth(t TypeExpr) int {
	if t.Name == "bvec" && len(t.TypeParams) > 0 {
		w, _ := strconv.Atoi(t.TypeParams[0])
		return w
	}
	return 0
}
