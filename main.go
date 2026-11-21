/// @file main.go
/// @brief Главный управляющий модуль (Driver) компилятора BIT-C v1.0.
/// @details Координирует загрузку файлов, импорт библиотек, семантический анализ,
/// генерацию C-кода и вызов системного компилятора Clang.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// MARK: Блок 1 - Основной конвейер компиляции и запуска

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run . <source_file.bc>")
		return
	}

	inputFile := os.Args[1]

	// 1. Загрузка исходного файла и всех подключенных библиотек (import)
	prog, err := LoadProgramWithImports(inputFile)
	if err != nil {
		fmt.Printf("[BIT-C] Load Error: %v\n", err)
		return
	}
	fmt.Printf("[BIT-C] Loaded '%s' (%d structs, %d functions)\n",
		inputFile, len(prog.Structs), len(prog.Funcs))

	// 2. Семантический анализ и мономорфизация шаблонов
	ctx, err := AnalyzeProgram(prog)
	if err != nil {
		fmt.Printf("[BIT-C] Semantic Error: %v\n", err)
		return
	}
	fmt.Printf("[BIT-C] Monomorphization & type check passed (%d specialized structs, %d functions)\n",
		len(ctx.MonoStructs), len(ctx.MonoFuncs))

	// 3. Генерация низкоуровневого C-кода
	cCode := GenerateCCode(ctx)

	buildDir := "build"
	_ = os.MkdirAll(buildDir, 0755)
	baseName := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
	cFile := filepath.Join(buildDir, baseName+".c")
	binFile := filepath.Join(buildDir, baseName)

	if err := os.WriteFile(cFile, []byte(cCode), 0644); err != nil {
		fmt.Println("[BIT-C] Write error:", err)
		return
	}

	// 4. Компиляция в нативный бинарник под текущую архитектуру CPU
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
		return
	}
	fmt.Printf("[BIT-C] Native binary compiled: %s\n", binFile)

	// 5. Выполнение программы
	fmt.Println("[BIT-C] Execution output:")
	out, err := exec.Command("./" + binFile).CombinedOutput()
	if err != nil {
		fmt.Println("[BIT-C] Run error:", err)
		return
	}
	fmt.Print(string(out))
}
