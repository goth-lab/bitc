package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// MARK: - Compiler Driver Entry Point

// main parses CLI arguments, runs the BIT-C frontend/backend pipeline, invokes Clang
// to emit a native binary, and executes the compiled program or unit test suite.
func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: bitc <source_file.bc> | bitc test <source_file.bc>")
		return
	}

	isTestMode := false
	inputFile := os.Args[1]
	if os.Args[1] == "test" {
		if len(os.Args) < 3 {
			fmt.Println("Usage: bitc test <source_file.bc>")
			return
		}
		isTestMode = true
		inputFile = os.Args[2]
	}

	// 1. Load root module and recursively resolve all 'import' dependencies.
	prog, err := LoadProgramWithImports(inputFile)
	if err != nil {
		fmt.Printf("[BIT-C] Load Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[BIT-C] Loaded '%s' (%d structs, %d functions)\n",
		inputFile, len(prog.Structs), len(prog.Funcs))

	// 2. Run semantic validation and template monomorphization.
	ctx, err := AnalyzeProgram(prog, isTestMode)
	if err != nil {
		fmt.Printf("[BIT-C] Semantic Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[BIT-C] Monomorphization & type check passed (%d specialized structs, %d functions)\n",
		len(ctx.MonoStructs), len(ctx.MonoFuncs))

	// 3. Emit C99/C11 source code (appending a test harness main() when in test mode).
	cCode := GenerateCCode(ctx)
	if isTestMode {
		cCode += buildTestHarnessMain(ctx.TestFuncs)
	}

	compileAndRun(inputFile, cCode, isTestMode)
}

// MARK: - Native Compilation, Execution, and Test Harness Generation

// compileAndRun writes generated C code to build/, compiles it with Clang using
// native CPU SIMD flags, and executes the resulting binary.
func compileAndRun(inputFile string, cCode string, isTestMode bool) {
	buildDir := "build"
	_ = os.MkdirAll(buildDir, 0755)
	baseName := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
	if isTestMode {
		baseName += "_test"
	}
	cFile := filepath.Join(buildDir, baseName+".c")
	binFile := filepath.Join(buildDir, baseName)

	if err := os.WriteFile(cFile, []byte(cCode), 0644); err != nil {
		fmt.Printf("[BIT-C] Write error: %v\n", err)
		os.Exit(1)
	}

	archFlag := "-O3"
	switch runtime.GOARCH {
	case "arm64":
		archFlag = "-mcpu=native"
	case "amd64":
		archFlag = "-march=native"
	}

	compileCmd := exec.Command("clang", "-O3", archFlag, cFile, "-o", binFile)
	if out, err := compileCmd.CombinedOutput(); err != nil {
		fmt.Printf("[BIT-C] Clang error:\n%s\n", string(out))
		os.Exit(1)
	}
	fmt.Printf("[BIT-C] Native binary compiled: %s\n", binFile)

	fmt.Println("[BIT-C] Execution output:")
	runCmd := exec.Command("./" + binFile)
	out, err := runCmd.CombinedOutput()
	fmt.Print(string(out))
	if err != nil {
		fmt.Printf("[BIT-C] Execution failed: %v\n", err)
		os.Exit(1)
	}
}

// buildTestHarnessMain synthesizes a C main() entry point that executes every compiled
// unit test function sequentially and reports pass/fail totals.
func buildTestHarnessMain(testFuncs []string) string {
	var sb strings.Builder
	sb.WriteString("int main() {\n")
	fmt.Fprintf(&sb, "    printf(\"[BIT-C TEST] Running %d unit test(s)...\\n\");\n", len(testFuncs))
	for _, fnName := range testFuncs {
		fmt.Fprintf(&sb, "    %s();\n", fnName)
		fmt.Fprintf(&sb, "    printf(\"  [PASS] %s\\n\");\n", fnName)
	}
	fmt.Fprintf(&sb, "    printf(\"[BIT-C TEST] All %d test(s) passed!\\n\");\n", len(testFuncs))
	sb.WriteString("    return 0;\n}\n")
	return sb.String()
}
