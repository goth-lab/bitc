# BIT-C Compiler — Architecture & Context Snapshot (v1.0 -> v1.1)

* **Язык реализации компилятора:** Go (`main.go`, `lexer.go`, `ast.go`, `parser.go`, `sema.go`, `codegen.go`).
* **Целевой бэкенд (Target):** C99/C11 (`clang -O3`, выравнивание SIMD `__attribute__((aligned(32)))`, `__builtin_popcountll`, OpenMP).
* **Целевая ниша:** Edge & Embedded AI (обработка видео, звука, сенсорики и гибридных AI-агентов на микрокомпьютерах и контроллерах с ограниченными ресурсами).
* **Парадигма вычислений:** Hyperdimensional Computing (HDC) + Binary/Bitwise Neural Networks (`bvec<N>` любой ширины) + детерминированная системная логика без GC.
* **Языковые правила и стиль кода:**
  * Исходный код компилятора (`*.go`), стандартной библиотеки (`std/*.bc`), примеров (`examples/*.bc`), сообщения компилятора и C-код: **строго на английском языке**.
  * Проектная документация (`CONTEXT_SNAPSHOT.md`, `specs/SPECIFICATION.md`): на русском языке до этапа релиза.
  * Ограничение в чате: каждый блок кода в ответе AI — **не более 75 строк**.
  * Разметка файлов: секции разделяются метками `// MARK: - <Section Title>`.
  * Документирование Go (`*.go`): стандартный GoDoc + подробные строчные комментарии (`//`) для неочевидной логики и переменных.
  * Документирование BIT-C (`*.bc`): Doxygen-блоки (`/// @brief`, `@param`, `@return`, `@note`).


## 2. Текущая структура проекта и архитектура ядра (BIT-C v1.0)
* **Файлы компилятора:**
  * `main.go` — CLI (`build`, `run`), сборка модулей (`import`), запуск `clang`.
  * `lexer.go` — токенизация (`fn`, `struct`, `import`, `extern`, `parallel`, `for`, `in`, `step`, `if`, `else`, `return`, `match`, `with`, `count`, `nudge`, `bvec`, `slice`, `int`, `f32`, `u8`, `u64`, `bool`, `void`).
  * `ast.go` — узлы AST (`Program`, `ImportDecl`, `StructDecl`, `FuncDecl` с поддержкой ресивера `(self: *T)`, `ExternFuncDecl`, `VarDecl`, `AssignStmt`, `ForStmt`, `ParallelForStmt`, `IfStmt`, `ReturnStmt`, `MatchExpr`, `CountExpr`, `NudgeStmt`, `IndexExpr`, `MemberExpr`, `CallExpr`).
  * `parser.go` — рекурсивный спуск, парсинг шаблонов `Name<A, B>` и типов `bvec<N>`, `slice<T>`, `*T`.
  * `sema.go` — проверка типов, резолвинг импортов (`import "../std/..."`), мономорфизация шаблонных структур и методов (`Dense<256, 8>` -> `Dense_256_8`).
  * `codegen.go` — генерация C-кода:
    * `bvec<N>` для `N <= 64` транслируется в `uint64_t`.
    * `bvec<N>` для `N > 64` транслируется в выровненную структуру `typedef struct { uint64_t w[K]; } __attribute__((aligned(32))) bvecN_t;`.
    * Побитовые операции (`^`, `&`, `|`, `~`, `<<`, `>>`), `match` (XNOR) и `count` (`__builtin_popcountll`) развернуты в статические циклы по словам `w[k]`.
* **Текущая `std/` (требует рефакторинга и расширения по `SPEC.md`):**
  * `std/sys.bc` — `extern fn` обертки над C-рантаймом (`rand_mask`, `now_ns`, `save_raw`, `load_raw`).
  * `std/encode.bc` — базовая генерация случайных гипервекторов (`rand_mask256`, `rand_mask1024`) и термометрическое кодирование (`thermometer64`).
  * `std/learn.bc` — шаблонные структуры `Dense<InBits, OutBits>` и `ContextCell<Vocab>`.

## 3. Выявленные баги и технические ограничения ядра (Техдолг)
1. **Побитовая индексация широких `bvec<N>` (`N > 64`):** В `codegen.go` чтение/запись отдельного бита (`vec[i]` и `vec[i] = bit`) реализованы только для `w <= 64`. Для `w > 64` нужно генерировать обращение к `(vec.w[i >> 6] >> (i & 63)) & 1` (и соответствующую битовую маску при записи).
2. **Генерация `#pragma omp parallel for`:** В `codegen.go` директива `#pragma omp parallel for` выводится в буфер *после* закрывающей скобки цикла `for`, а не непосредственно перед заголовком цикла.
3. **Нет встроенного тестирования:** Отсутствуют конструкция `test "name" { ... }`, модуль `std/test.bc` и команда `./bitc test`.
4. **Нет детерминированных аллокаторов и буферов:** Отсутствуют `ArenaAllocator`, `RingBuffer<T, N>` для потокового аудио/видео на MCU.
5. **Ограниченная обработка ошибок и C-интероп:** Для модулей `std.vision`, `std.audio`, `std.infer` и `std.schema` нужны типизированные ошибки (или коды возврата / структуры `Result`), работа с массивами/срезами и расширение `extern` C-рантайма.

## 4. Дорожная карта (Актуальный план работ)
* **Этап 1 (Завершен):** Аудит компилятора и утверждение полной спецификации `SPEC.md` (Уровни 0–4: `std.test`, `std.mem`, `std.tensor`, `std.stream`, `std.io`, `std.vision`, `std.audio`, `std.infer`, `std.post`, `std.schema`, `std.context`, `std.tools`, `std.reasoning`, `std.flow`).
* **Этап 2.0 (Следующий шаг):** Исправление 2 критических багов в `codegen.go` (индексация битов при `N > 64` и позиция `#pragma omp parallel for`) + внедрение базового фреймворка тестирования (`test "..."`, `std/test.bc`, `./bitc test`).
* **Этап 2.1:** Реализация Уровня 1 (`std/mem.bc` — `RingBuffer`, `Arena`; `std/tensor.bc`; `std/stream.bc`; `std/io.bc`) с 100% покрытием тестами.
* **Этап 2.2:** Реализация Уровня 4 (`std/schema.bc`, `std/flow.bc`, `std/context.bc`, `std/tools.bc`, `std/reasoning.bc`) + доработка компилятора под обнаруженные узкие места.
* **Этап 3:** Реализация Уровней 2 и 3 (`std/vision.bc`, `std/audio.bc`, `std/infer.bc`, `std/post.bc`) и сборка 3 эталонных примеров (Edge Vision, Edge Audio, Hybrid AI Agent).
* **Этап 4:** Релиз на GitHub (`v0.1-alpha`) для привлечения сообщества.

## Code Style & Documentation Standards (Approved)

1. **Language:** All code, comments, commit messages, compiler diagnostics, and generated C code must be written in **English**.
2. **Chat Output Limit:** Keep every code block strictly under **75 lines** (split larger changes into sequential logical blocks).
3. **Section Markers (`MARK`):** Divide every source file (`.go` and `.bc`) into logical sections using `// MARK: - <Section Title>`.
4. **Go Compiler Code (`*.go`) — Idiomatic GoDoc + Detailed Inline Comments:**
   * Document every struct, interface, and function with a standard GoDoc comment (`// FuncName does...`), explicitly describing all parameters, return values, and edge cases.
   * Add detailed inline `//` comments explaining non-trivial AST transformations, memory layout math, and significant local variables.
5. **BIT-C Standard Library & Examples (`*.bc`) — Doxygen Style:**
   * Document all public structs, methods, and functions using `///` doc-blocks with `@brief`, `@param`, `@return`, and `@note` (for memory/allocation guarantees).
