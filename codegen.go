/// @file codegen.go
/// @brief Генератор низкоуровневого C-кода (Backend) для компилятора BIT-C v1.0.
/// @details Транслирует мономорфизированное AST в выровненные структуры C,
/// статические inline-функции и аппаратные SIMD-интринсики процессора.

package main

import (
	"fmt"
	"sort"
	"strings"
)

// MARK: Блок 1 - Инициализация генератора и базовый рантайм кремния

// / @brief Состояние генератора C-кода.
type CodeGenerator struct {
	ctx       *SemaContext    ///< Контекст типов и специализаций из семантического анализатора
	sb        strings.Builder ///< Буфер выходного C-кода
	wideSizes map[int]bool    ///< Набор использованных ширин широких векторов (> 64 бит)
	tempID    int             ///< Счетчик временных переменных C
}

// / @brief Генерирует полный исходный файл на языке C из проверенного AST.
func GenerateCCode(ctx *SemaContext) string {
	g := &CodeGenerator{
		ctx:       ctx,
		wideSizes: make(map[int]bool),
	}

	g.collectWideVectors()
	g.emitHeader()
	g.emitStructs()
	g.emitFuncPrototypes()
	g.emitFuncs()

	return g.sb.String()
}

// / @brief Собирает все ширины bvec<N> (> 64 бит), встречающиеся в программе.
func (g *CodeGenerator) collectWideVectors() {
	for _, t := range g.ctx.ExprTypes {
		w := getBitWidth(t)
		if w > 64 {
			g.wideSizes[w] = true
		}
	}
	for _, st := range g.ctx.MonoStructs {
		for _, f := range st.Fields {
			w := getBitWidth(f.Type)
			if w > 64 {
				g.wideSizes[w] = true
			}
		}
	}
}

// / @brief Записывает заголовки C, генератор случайных битов и структуры широких векторов.
func (g *CodeGenerator) emitHeader() {
	g.sb.WriteString("#include <stdint.h>\n#include <stdio.h>\n#include <stdlib.h>\n#include <time.h>\n\n")
	g.sb.WriteString(`static uint64_t _rng_state = 88172645463325252ULL;
static inline uint64_t _rand_u64() {
    uint64_t x = _rng_state;
    x ^= x << 13; x ^= x >> 7; x ^= x << 17;
    _rng_state = x;
    return x;
}
static inline uint64_t rand_mask() {
	return _rand_u64() & _rand_u64() & _rand_u64();
}
static inline int64_t now_ns() {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (int64_t)ts.tv_sec * 1000000000LL + (int64_t)ts.tv_nsec;
}
static inline void save_raw(int64_t slot, void* ptr, int64_t bytes) {
	char name[64];
	snprintf(name, sizeof(name), "weights_slot_%lld.bin", (long long)slot);
	FILE* f = fopen(name, "wb");
	if (f) { fwrite(ptr, 1, (size_t)bytes, f); fclose(f); }
}
static inline void load_raw(int64_t slot, void* ptr, int64_t bytes) {
	char name[64];
	snprintf(name, sizeof(name), "weights_slot_%lld.bin", (long long)slot);
	FILE* f = fopen(name, "rb");
	if (f) { size_t r = fread(ptr, 1, (size_t)bytes, f); (void)r; fclose(f); }
}
`)
	var widths []int
	for w := range g.wideSizes {
		widths = append(widths, w)
	}
	sort.Ints(widths)
	for _, w := range widths {
		words := w / 64
		g.sb.WriteString(fmt.Sprintf("typedef struct __attribute__((aligned(32))) { uint64_t w[%d]; } bvec%d_t;\n", words, w))
	}
	g.sb.WriteString("\n")
}

// MARK: Блок 2 - Генерация пользовательских структур и прототипов функций

// / @brief Генерирует C-структуры из мономорфизированных шаблонов (Dense_256_8).
func (g *CodeGenerator) emitStructs() {
	for _, st := range g.ctx.MonoStructs {
		g.sb.WriteString("typedef struct {\n")
		for _, f := range st.Fields {
			g.sb.WriteString(fmt.Sprintf("    %s;\n", g.formatVarDecl(f.Name, f.Type)))
		}
		g.sb.WriteString(fmt.Sprintf("} %s;\n\n", st.Name))
	}
}

// / @brief Генерирует предварительные объявления всех функций (forward declarations).
func (g *CodeGenerator) emitFuncPrototypes() {
	for _, fn := range g.ctx.MonoFuncs {
		if fn.Name == "main" || fn.IsExtern {
			continue
		}
		g.sb.WriteString(g.buildFnSignature(fn))
		g.sb.WriteString(";\n")
	}
	g.sb.WriteString("\n")
}

// / @brief Генерирует реализации всех специализированных функций и методов.
func (g *CodeGenerator) emitFuncs() {
	for _, fn := range g.ctx.MonoFuncs {
		if fn.IsExtern {
			continue
		}
		g.sb.WriteString(g.buildFnSignature(fn))
		g.sb.WriteString(" {\n")
		g.emitStmts(fn.Body, "    ")
		if fn.Name == "main" {
			g.sb.WriteString("    return 0;\n")
		}
		g.sb.WriteString("}\n\n")
	}
}

// / @brief Формирует заголовок C-функции с учетом ресивера метода (self: *T).
func (g *CodeGenerator) buildFnSignature(fn *FnNode) string {
	if fn.Name == "main" {
		return "int main()"
	}
	var params []string
	if fn.Receiver != nil {
		params = append(params, g.formatVarDecl(fn.Receiver.Name, fn.Receiver.Type))
	}
	for _, p := range fn.Params {
		params = append(params, g.formatVarDecl(p.Name, p.Type))
	}
	ret := g.mapCType(fn.ReturnType)
	return fmt.Sprintf("static inline %s %s(%s)", ret, fn.Name, strings.Join(params, ", "))
}

// MARK: Блок 3 - Генерация исполняемых инструкций (Statements)

// / @brief Генерирует C-код для списка инструкций с заданным отступом.
func (g *CodeGenerator) emitStmts(stmts []Stmt, indent string) {
	for _, s := range stmts {
		switch st := s.(type) {
		case *VarDeclStmt:
			decl := g.formatVarDecl(st.Name, st.Type)
			if st.Value != nil {
				valStr := g.emitExpr(st.Value, indent)
				g.sb.WriteString(fmt.Sprintf("%s%s = %s;\n", indent, decl, valStr))
			} else {
				g.sb.WriteString(fmt.Sprintf("%s%s = {0};\n", indent, decl))
			}

		case *AssignStmt:
			// Запись отдельного бита в скалярный вектор (только если это НЕ массив!)
			if idxEx, ok := st.Target.(*IndexExpr); ok {
				objType := g.ctx.ExprTypes[idxEx.Obj]
				if objType.ArraySize == "" && objType.Name == "bvec" && getBitWidth(objType) <= 64 {
					vecStr := g.emitExpr(idxEx.Obj, indent)
					idxStr := g.emitExpr(idxEx.Index, indent)
					valStr := g.emitExpr(st.Value, indent)
					g.sb.WriteString(fmt.Sprintf("%sif (%s) { %s |= (1ULL << (%s)); } else { %s &= ~(1ULL << (%s)); }\n",
						indent, valStr, vecStr, idxStr, vecStr, idxStr))
					continue
				}
			}
			targetStr := g.emitExpr(st.Target, indent)
			valStr := g.emitExpr(st.Value, indent)
			fmt.Fprintf(&g.sb, "%s%s = %s;\n", indent, targetStr, valStr)

		case *ExprStmt:
			exprStr := g.emitExpr(st.Expr, indent)
			g.sb.WriteString(fmt.Sprintf("%s%s;\n", indent, exprStr))

		case *IfStmt:
			condStr := g.emitExpr(st.Cond, indent)
			g.sb.WriteString(fmt.Sprintf("%sif (%s) {\n", indent, condStr))
			g.emitStmts(st.ThenBody, indent+"    ")
			if len(st.ElseBody) > 0 {
				g.sb.WriteString(fmt.Sprintf("%s} else {\n", indent))
				g.emitStmts(st.ElseBody, indent+"    ")
			}
			g.sb.WriteString(fmt.Sprintf("%s}\n", indent))

		case *ForStmt:
			startStr := g.emitExpr(st.Start, indent)
			endStr := g.emitExpr(st.End, indent)
			stepStr := "1"
			if st.Step != nil {
				stepStr = g.emitExpr(st.Step, indent)
			}
			g.sb.WriteString(fmt.Sprintf("%sfor (int %s = %s; %s < %s; %s += %s) {\n",
				indent, st.Iterator, startStr, st.Iterator, endStr, st.Iterator, stepStr))
			g.emitStmts(st.Body, indent+"    ")
			if st.IsParallel {
				g.sb.WriteString(fmt.Sprintf("%s#if defined(_OPENMP)\n%s#pragma omp parallel for\n%s#endif\n", indent, indent, indent))
			}
			g.sb.WriteString(fmt.Sprintf("%s}\n", indent))

		case *ReturnStmt:
			if st.Value != nil {
				valStr := g.emitExpr(st.Value, indent)
				g.sb.WriteString(fmt.Sprintf("%sreturn %s;\n", indent, valStr))
			} else {
				g.sb.WriteString(fmt.Sprintf("%sreturn;\n", indent))
			}

		case *PrintStmt:
			valStr := g.emitExpr(st.Value, indent)
			tmp := g.nextTemp()
			fmt.Fprintf(&g.sb, "%sint64_t %s = (int64_t)(%s);\n", indent, tmp, valStr)
			fmt.Fprintf(&g.sb, "%sprintf(\"[BIT-C] 0x%%llX (dec: %%lld)\\n\", (unsigned long long)%s, (long long)%s);\n",
				indent, tmp, tmp)
		}
	}
}

// MARK: Блок 4 - Генерация выражений и аппаратных интринсиков

// / @brief Генерирует C-выражение (при необходимости разворачивая широкие SIMD-векторы во временные переменные).
func (g *CodeGenerator) emitExpr(e Expr, indent string) string {
	switch ex := e.(type) {
	case *NumberExpr:
		if ex.IsHex {
			return ex.Value + "ULL"
		}
		return ex.Value

	case *IdentExpr:
		return ex.Name

	case *UnaryExpr:
		rType := g.ctx.ExprTypes[ex.Right]
		rStr := g.emitExpr(ex.Right, indent)
		w := getBitWidth(rType)
		if ex.Op == "~" && w > 64 {
			tmp := g.nextTemp()
			words := w / 64
			g.sb.WriteString(fmt.Sprintf("%sbvec%d_t %s;\n", indent, w, tmp))
			g.sb.WriteString(fmt.Sprintf("%sfor (int _k = 0; _k < %d; _k++) { %s.w[_k] = ~(%s).w[_k]; }\n",
				indent, words, tmp, rStr))
			return tmp
		}
		return fmt.Sprintf("(%s%s)", ex.Op, rStr)

	case *BinaryExpr:
		lType := g.ctx.ExprTypes[ex.Left]
		lStr := g.emitExpr(ex.Left, indent)
		rStr := g.emitExpr(ex.Right, indent)
		w := getBitWidth(lType)
		if w > 64 && (ex.Op == "^" || ex.Op == "&" || ex.Op == "|") {
			tmp := g.nextTemp()
			words := w / 64
			fmt.Fprintf(&g.sb, "%sbvec%d_t %s;\n", indent, w, tmp)
			fmt.Fprintf(&g.sb, "%sfor (int _k = 0; _k < %d; _k++) { %s.w[_k] = (%s).w[_k] %s (%s).w[_k]; }\n",
				indent, words, tmp, lStr, ex.Op, rStr)
			return tmp
		}
		return fmt.Sprintf("(%s %s %s)", lStr, ex.Op, rStr)

	case *FieldExpr:
		objType := g.ctx.ExprTypes[ex.Obj]
		objStr := g.emitExpr(ex.Obj, indent)
		if objType.IsPointer {
			return fmt.Sprintf("%s->%s", objStr, ex.Field)
		}
		return fmt.Sprintf("%s.%s", objStr, ex.Field)

	case *IndexExpr:
		objType := g.ctx.ExprTypes[ex.Obj]
		objStr := g.emitExpr(ex.Obj, indent)
		idxStr := g.emitExpr(ex.Index, indent)
		if objType.ArraySize == "" && objType.Name == "bvec" && getBitWidth(objType) <= 64 {
			return fmt.Sprintf("((%s >> (%s)) & 1ULL)", objStr, idxStr)
		}
		return fmt.Sprintf("%s[%s]", objStr, idxStr)

	case *CallExpr:
		return g.emitCallExpr(ex, indent)
	}
	return ""
}

// MARK: Блок 5 - Генерация вызовов функций, методов, интринсиков и типов C

// / @brief Генерирует C-код для вызова функции, Go-метода или интринсика.
func (g *CodeGenerator) emitCallExpr(call *CallExpr, indent string) string {
	targetName := g.ctx.ResolvedCalls[call]

	if targetName == "match" {
		t1 := g.ctx.ExprTypes[call.Args[0]]
		aStr := g.emitExpr(call.Args[0], indent)
		bStr := g.emitExpr(call.Args[1], indent)
		w := getBitWidth(t1)
		if w > 64 {
			tmp := g.nextTemp()
			words := w / 64
			g.sb.WriteString(fmt.Sprintf("%sbvec%d_t %s;\n", indent, w, tmp))
			g.sb.WriteString(fmt.Sprintf("%sfor (int _k = 0; _k < %d; _k++) { %s.w[_k] = ~((%s).w[_k] ^ (%s).w[_k]); }\n",
				indent, words, tmp, aStr, bStr))
			return tmp
		}
		mask := uint64((1 << w) - 1)
		if w >= 64 {
			mask = 0xFFFFFFFFFFFFFFFF
		}
		return fmt.Sprintf("((~(%s ^ %s)) & 0x%XULL)", aStr, bStr, mask)
	}

	if targetName == "count" {
		t1 := g.ctx.ExprTypes[call.Args[0]]
		aStr := g.emitExpr(call.Args[0], indent)
		w := getBitWidth(t1)
		if w > 64 {
			tmp := g.nextTemp()
			words := w / 64
			g.sb.WriteString(fmt.Sprintf("%sint %s = 0;\n", indent, tmp))
			g.sb.WriteString(fmt.Sprintf("%sfor (int _k = 0; _k < %d; _k++) { %s += __builtin_popcountll((%s).w[_k]); }\n",
				indent, words, tmp, aStr))
			return tmp
		}
		return fmt.Sprintf("__builtin_popcountll((uint64_t)(%s))", aStr)
	}

	var args []string
	// Если это вызов метода структуры (obj.method(...)), передаем &obj первым аргументом
	if fieldEx, ok := call.Callee.(*FieldExpr); ok {
		objType := g.ctx.ExprTypes[fieldEx.Obj]
		objStr := g.emitExpr(fieldEx.Obj, indent)
		if objType.IsPointer {
			args = append(args, objStr)
		} else {
			args = append(args, "&"+objStr)
		}
	}

	for _, a := range call.Args {
		args = append(args, g.emitExpr(a, indent))
	}
	return fmt.Sprintf("%s(%s)", targetName, strings.Join(args, ", "))
}

// / @brief Создает уникальное имя временной переменной в C.
func (g *CodeGenerator) nextTemp() string {
	g.tempID++
	return fmt.Sprintf("_tmp_%d", g.tempID)
}

// / @brief Форматирует объявление переменной или поля на языке C (с учетом массивов и указателей).
func (g *CodeGenerator) formatVarDecl(name string, t TypeExpr) string {
	base := g.mapCType(t)
	if t.ArraySize != "" {
		return fmt.Sprintf("%s %s[%s]", base, name, t.ArraySize)
	}
	return fmt.Sprintf("%s %s", base, name)
}

// / @brief Преобразует TypeExpr языка BIT-C в нативный тип C.
func (g *CodeGenerator) mapCType(t TypeExpr) string {
	ptr := ""
	if t.IsPointer {
		ptr = "*"
	}
	if t.Name == "bvec" {
		w := getBitWidth(t)
		if w > 64 {
			return fmt.Sprintf("bvec%d_t%s", w, ptr)
		}
		if w <= 8 {
			return "uint8_t" + ptr
		}
		if w <= 16 {
			return "uint16_t" + ptr
		}
		if w <= 32 {
			return "uint32_t" + ptr
		}
		return "uint64_t" + ptr
	}
	switch t.Name {
	case "int":
		return "int64_t" + ptr
	case "i32":
		return "int32_t" + ptr
	case "i16":
		return "int16_t" + ptr
	case "u8", "bit":
		return "uint8_t" + ptr
	case "void":
		return "void" + ptr
	default:
		return t.Name + ptr
	}
}
