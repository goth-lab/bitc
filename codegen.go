package main

import (
	"fmt"
	"sort"
	"strings"
)

// MARK: - CodeGenerator State and Entry Point

// CodeGenerator translates a monomorphized SemaContext AST into aligned C99/C11 code.
type CodeGenerator struct {
	ctx       *SemaContext    // Type-checked AST and monomorphized symbol tables
	sb        strings.Builder // Output buffer accumulating generated C source
	wideSizes map[int]bool    // Set of bvec<N> bit-widths > 64 used in the program
	tempID    int             // Monotonic counter for unique temporary C variables
}

// GenerateCCode compiles the validated SemaContext into a complete C source file string.
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

// collectWideVectors scans all expression types and struct fields to register every
// bvec<N> width exceeding 64 bits so corresponding bvecN_t typedefs can be emitted.
func (g *CodeGenerator) collectWideVectors() {
	for _, t := range g.ctx.ExprTypes {
		if w := getBitWidth(t); w > 64 {
			g.wideSizes[w] = true
		}
	}
	for _, st := range g.ctx.MonoStructs {
		for _, f := range st.Fields {
			if w := getBitWidth(f.Type); w > 64 {
				g.wideSizes[w] = true
			}
		}
	}
}

// MARK: - C Runtime Header and Wide Vector Typedefs

// emitHeader writes standard C includes, inline runtime helpers (PRNG, monotonic clock,
// raw binary persistence), and 32-byte SIMD-aligned typedefs for wide bvec<N> types.
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
static inline void bitc_assert(int64_t cond, int64_t line) {
	if (!cond) {
		fprintf(stderr, "[BIT-C TEST FAIL] Assertion failed at line %lld\n", (long long)line);
		exit(1);
	}
}
`)
	// Emit deterministic sorted typedefs for all wide bvec<N> widths.
	var widths []int
	for w := range g.wideSizes {
		widths = append(widths, w)
	}
	sort.Ints(widths)
	for _, w := range widths {
		words := w / 64
		fmt.Fprintf(&g.sb, "typedef struct __attribute__((aligned(32))) { uint64_t w[%d]; } bvec%d_t;\n", words, w)
	}
	g.sb.WriteString("\n")
}

// MARK: - Structs and Function Declarations

// emitStructs emits C typedef structs for all monomorphized BIT-C structures.
func (g *CodeGenerator) emitStructs() {
	for _, st := range g.ctx.MonoStructs {
		g.sb.WriteString("typedef struct {\n")
		for _, f := range st.Fields {
			fmt.Fprintf(&g.sb, "    %s;\n", g.formatVarDecl(f.Name, f.Type))
		}
		fmt.Fprintf(&g.sb, "} %s;\n\n", st.Name)
	}
}

// emitFuncPrototypes emits forward declarations for all non-extern functions.
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

// emitFuncs emits full C function definitions for all monomorphized BIT-C functions.
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

// buildFnSignature formats a C function header, prepending the receiver pointer for methods.
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

// MARK: - Statement Code Generation

// emitStmts translates a slice of AST statements into indented C statements.
func (g *CodeGenerator) emitStmts(stmts []Stmt, indent string) {
	for _, s := range stmts {
		switch st := s.(type) {
		case *VarDeclStmt:
			decl := g.formatVarDecl(st.Name, st.Type)
			if st.Value != nil {
				valStr := g.emitExpr(st.Value, indent)
				fmt.Fprintf(&g.sb, "%s%s = %s;\n", indent, decl, valStr)
			} else {
				fmt.Fprintf(&g.sb, "%s%s = {0};\n", indent, decl)
			}

		case *AssignStmt:
			// Support single-bit mutation on bvec<N> scalars (both <= 64 and > 64 bits).
			if idxEx, ok := st.Target.(*IndexExpr); ok {
				objType := g.ctx.ExprTypes[idxEx.Obj]
				if objType.ArraySize == "" && objType.Name == "bvec" {
					w := getBitWidth(objType)
					vecStr := g.emitExpr(idxEx.Obj, indent)
					idxStr := g.emitExpr(idxEx.Index, indent)
					valStr := g.emitExpr(st.Value, indent)
					if w <= 64 {
						fmt.Fprintf(&g.sb, "%sif (%s) { %s |= (1ULL << (%s)); } else { %s &= ~(1ULL << (%s)); }\n",
							indent, valStr, vecStr, idxStr, vecStr, idxStr)
					} else {
						// Wide bvecN_t word-indexed bit write: word = idx >> 6, bit = idx & 63.
						tmpIdx := g.nextTemp()
						fmt.Fprintf(&g.sb, "%suint64_t %s = (uint64_t)(%s);\n", indent, tmpIdx, idxStr)
						fmt.Fprintf(&g.sb, "%sif (%s) { (%s).w[%s >> 6] |= (1ULL << (%s & 63)); } else { (%s).w[%s >> 6] &= ~(1ULL << (%s & 63)); }\n",
							indent, valStr, vecStr, tmpIdx, tmpIdx, vecStr, tmpIdx, tmpIdx)
					}
					continue
				}
			}
			targetStr := g.emitExpr(st.Target, indent)
			valStr := g.emitExpr(st.Value, indent)
			fmt.Fprintf(&g.sb, "%s%s = %s;\n", indent, targetStr, valStr)

		case *ExprStmt:
			fmt.Fprintf(&g.sb, "%s%s;\n", indent, g.emitExpr(st.Expr, indent))

		case *IfStmt:
			fmt.Fprintf(&g.sb, "%sif (%s) {\n", indent, g.emitExpr(st.Cond, indent))
			g.emitStmts(st.ThenBody, indent+"    ")
			if len(st.ElseBody) > 0 {
				fmt.Fprintf(&g.sb, "%s} else {\n", indent)
				g.emitStmts(st.ElseBody, indent+"    ")
			}
			fmt.Fprintf(&g.sb, "%s}\n", indent)

		case *ForStmt:
			startStr := g.emitExpr(st.Start, indent)
			endStr := g.emitExpr(st.End, indent)
			stepStr := "1"
			if st.Step != nil {
				stepStr = g.emitExpr(st.Step, indent)
			}
			// Emit OpenMP directive directly above the C loop header.
			if st.IsParallel {
				fmt.Fprintf(&g.sb, "%s#if defined(_OPENMP)\n%s#pragma omp parallel for\n%s#endif\n", indent, indent, indent)
			}
			fmt.Fprintf(&g.sb, "%sfor (int %s = %s; %s < %s; %s += %s) {\n",
				indent, st.Iterator, startStr, st.Iterator, endStr, st.Iterator, stepStr)
			g.emitStmts(st.Body, indent+"    ")
			fmt.Fprintf(&g.sb, "%s}\n", indent)

		case *ReturnStmt:
			if st.Value != nil {
				fmt.Fprintf(&g.sb, "%sreturn %s;\n", indent, g.emitExpr(st.Value, indent))
			} else {
				fmt.Fprintf(&g.sb, "%sreturn;\n", indent)
			}

		case *PrintStmt:
			tmp := g.nextTemp()
			fmt.Fprintf(&g.sb, "%sint64_t %s = (int64_t)(%s);\n", indent, tmp, g.emitExpr(st.Value, indent))
			fmt.Fprintf(&g.sb, "%sprintf(\"[BIT-C] 0x%%llX (dec: %%lld)\\n\", (unsigned long long)%s, (long long)%s);\n",
				indent, tmp, tmp)
		}
	}
}

// MARK: - Expression Code Generation

// emitExpr translates an AST expression into a C expression string, unrolling wide
// SIMD vector operations into temporary local variables when bit-width exceeds 64.
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
		if w := getBitWidth(rType); ex.Op == "~" && w > 64 {
			tmp := g.nextTemp()
			words := w / 64
			fmt.Fprintf(&g.sb, "%sbvec%d_t %s;\n", indent, w, tmp)
			fmt.Fprintf(&g.sb, "%sfor (int _k = 0; _k < %d; _k++) { %s.w[_k] = ~(%s).w[_k]; }\n",
				indent, words, tmp, rStr)
			return tmp
		}
		return fmt.Sprintf("(%s%s)", ex.Op, rStr)

	case *BinaryExpr:
		lType := g.ctx.ExprTypes[ex.Left]
		lStr := g.emitExpr(ex.Left, indent)
		rStr := g.emitExpr(ex.Right, indent)
		if w := getBitWidth(lType); w > 64 && (ex.Op == "^" || ex.Op == "&" || ex.Op == "|") {
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
		// Support single-bit reads on both scalar (<= 64) and wide (> 64) bvec<N>.
		if objType.ArraySize == "" && objType.Name == "bvec" {
			if getBitWidth(objType) <= 64 {
				return fmt.Sprintf("((%s >> (%s)) & 1ULL)", objStr, idxStr)
			}
			tmpIdx := g.nextTemp()
			fmt.Fprintf(&g.sb, "%suint64_t %s = (uint64_t)(%s);\n", indent, tmpIdx, idxStr)
			return fmt.Sprintf("(((%s).w[%s >> 6] >> (%s & 63)) & 1ULL)", objStr, tmpIdx, tmpIdx)
		}
		return fmt.Sprintf("%s[%s]", objStr, idxStr)

	case *CallExpr:
		return g.emitCallExpr(ex, indent)
	}
	return ""
}

// MARK: - Intrinsics, Function Calls, and C Type Mapping

// emitCallExpr generates C code for built-in intrinsics (match, count), receiver
// method calls, and regular functions.
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
			fmt.Fprintf(&g.sb, "%sbvec%d_t %s;\n", indent, w, tmp)
			fmt.Fprintf(&g.sb, "%sfor (int _k = 0; _k < %d; _k++) { %s.w[_k] = ~((%s).w[_k] ^ (%s).w[_k]); }\n",
				indent, words, tmp, aStr, bStr)
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
			fmt.Fprintf(&g.sb, "%sint %s = 0;\n", indent, tmp)
			fmt.Fprintf(&g.sb, "%sfor (int _k = 0; _k < %d; _k++) { %s += __builtin_popcountll((%s).w[_k]); }\n",
				indent, words, tmp, aStr)
			return tmp
		}
		return fmt.Sprintf("__builtin_popcountll((uint64_t)(%s))", aStr)
	}

	var args []string
	// Automatically pass &obj as the first receiver argument for struct method calls.
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

// nextTemp generates a unique sequential temporary variable identifier in C.
func (g *CodeGenerator) nextTemp() string {
	g.tempID++
	return fmt.Sprintf("_tmp_%d", g.tempID)
}

// formatVarDecl formats a C variable or struct field declaration with optional array bounds.
func (g *CodeGenerator) formatVarDecl(name string, t TypeExpr) string {
	base := g.mapCType(t)
	if t.ArraySize != "" {
		return fmt.Sprintf("%s %s[%s]", base, name, t.ArraySize)
	}
	return fmt.Sprintf("%s %s", base, name)
}

// mapCType converts a BIT-C TypeExpr into its corresponding native C type name.
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
	case "i8":
		return "int8_t" + ptr
	default:
		return t.Name + ptr
	}
}
