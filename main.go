// Copyright (c) 2026 Vladimir Borisenko (Goth) & AI Gemini.
// Licensed under the MIT License. See LICENSE file in the project root.
//
// File: main.go
// Description: BIT-C compiler CLI driver supporting single-file execution and batch test suites.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// MARK: - Compiler Driver Entry Point

// main parses CLI arguments and dispatches either a single program build/run
// or a single-file / directory-wide unit test run.
func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: bitc <source_file.bc> | bitc test <file.bc | dir/> | bitc check <file.bc>")
		return
	}

	if os.Args[1] == "test" {
		if len(os.Args) < 3 {
			fmt.Println("Usage: bitc test <file.bc | dir/>")
			return
		}
		runTestTarget(os.Args[2])
		return
	}

	if os.Args[1] == "check" {
		if len(os.Args) < 3 {
			fmt.Println("Usage: bitc check <source_file.bc>")
			return
		}
		checkFile(os.Args[2])
		return
	}

	processFile(os.Args[1], false)
}

// runTestTarget executes either a single .bc test file or all '*_test.bc' files inside a directory.
func runTestTarget(target string) {
	info, err := os.Stat(target)
	if err != nil {
		fmt.Printf("[BIT-C] Test target error: %v\n", err)
		os.Exit(1)
	}

	if !info.IsDir() {
		processFile(target, true)
		return
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		fmt.Printf("[BIT-C] Failed to read test directory '%s': %v\n", target, err)
		os.Exit(1)
	}

	var testFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.bc") {
			testFiles = append(testFiles, filepath.Join(target, entry.Name()))
		}
	}
	sort.Strings(testFiles)

	if len(testFiles) == 0 {
		fmt.Printf("[BIT-C] No '*_test.bc' files found in '%s'\n", target)
		return
	}

	fmt.Printf("[BIT-C SUITE] Running %d test suite(s) in '%s'...\n\n", len(testFiles), target)
	for _, tf := range testFiles {
		processFile(tf, true)
		fmt.Println()
	}
	fmt.Printf("[BIT-C SUITE] All %d test suite(s) passed!\n", len(testFiles))
}

// MARK: - Compilation Pipeline and Test Harness Generation

// processFile runs loading, semantic monomorphization, C code emission, and Clang execution.
func processFile(inputFile string, isTestMode bool) {
	prog, err := LoadProgramWithImports(inputFile)
	if err != nil {
		fmt.Printf("[BIT-C] Load Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[BIT-C] Loaded '%s' (%d structs, %d functions)\n",
		inputFile, len(prog.Structs), len(prog.Funcs))

	ctx, err := AnalyzeProgram(prog, isTestMode)
	if err != nil {
		fmt.Printf("[BIT-C] Semantic Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[BIT-C] Monomorphization & type check passed (%d specialized structs, %d functions)\n",
		len(ctx.MonoStructs), len(ctx.MonoFuncs))

	cCode := GenerateCCode(ctx)
	if isTestMode {
		cCode += buildTestHarnessMain(ctx.TestFuncs)
	}

	compileAndRun(inputFile, cCode, isTestMode)
}

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

// checkFile runs fast lexical, syntactic, and semantic checks without invoking Clang,
// formatting diagnostics in standard file:line:col format for IDE integration.
func checkFile(inputFile string) {
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("%v", r)
			line := "1"
			if strings.HasPrefix(msg, "[Line ") {
				if end := strings.Index(msg, "] "); end > 6 {
					line = msg[6:end]
					msg = msg[end+2:]
				}
			}
			fmt.Printf("%s:%s:1: error: %s\n", inputFile, line, msg)
			os.Exit(1)
		}
	}()

	prog, err := LoadProgramWithImports(inputFile)
	if err != nil {
		fmt.Printf("%s:1:1: error: %v\n", inputFile, err)
		os.Exit(1)
	}
	if _, err := AnalyzeProgram(prog, false); err != nil {
		msg := err.Error()
		line := "1"
		if idx := strings.Index(msg, "[Line "); idx != -1 {
			sub := msg[idx+6:]
			if end := strings.Index(sub, "] "); end > 0 {
				line = sub[:end]
				msg = sub[end+2:]
			}
		}
		fmt.Printf("%s:%s:1: error: %s\n", inputFile, line, msg)
		os.Exit(1)
	}
	fmt.Printf("[BIT-C CHECK] OK: %s\n", inputFile)
}
