# BIT-C Compiler — Architecture & Context Snapshot (v1.1)

## 1. Философия проекта и правила разработки
* **Миссия BIT-C:** Компилируемый язык (транслируется в C99/C11 с SIMD/OpenMP) как сверхбыстрая, энергоэффективная альтернатива дорогим вычислениям на GPU и FPU — от микроконтроллеров и умных устройств до серверного инференса и гибридных AI-агентов.
* **Фундаментальный закон вычислений:** **100% Integer & Bitwise Computing (No Float, No GPU).** Только целочисленная арифметика (`int`, `i32`, `i16`, `i8`, `u8`), фиксированная точка (`Q16.16`) и бинарные гипервекторы (`bvec<N>` любой ширины).
* **Парадигма:** Hyperdimensional Computing (HDC) + Binary/Bitwise Neural Networks + детерминированная системная логика без GC и скрытых `malloc`.
* **Авторство и лицензия:** `Copyright (c) 2026 Vladimir Borisenko (Goth) & AI Gemini` (MIT License) в шапке каждого исходного файла.
* **Стандарты кода и документации:**
  * Весь код (`*.go`, `*.bc`), комментарии, ошибки компилятора и C-код: **строго на английском языке**.
  * Проектная документация (`CONTEXT_SNAPSHOT.md`, `specs/SPECIFICATION.md`, `README.md`): на русском языке до этапа релиза.
  * Ограничение в чате: каждый блок кода от AI — **строго до 75 строк** (осторожно с квадратными скобками в Markdown).
  * Разметка секций: `// MARK: - <Section Title>` во всех файлах.
  * Go (`*.go`): стандартный GoDoc + подробные строчные комментарии (`//`).
  * BIT-C (`*.bc`): Doxygen-блоки (`/// @brief`, `@param`, `@return`, `@note`).

## 2. Актуальная архитектура и статус репозитория
* **Ядро компилятора (переписано и очищено в v1.1):**
  * `main.go` — CLI (`./bitc <file.bc>` и `./bitc test <file.bc>`), сборка через `clang -O3` с флагами нативного CPU (`-mcpu=native` / `-march=native`).
  * `lexer.go` — токенизация, включая `TOK_TEST` (`test`).
  * `ast.go` — узлы AST с поддержкой шаблонов, ресиверов `(self: *T)`, `parallel for`, `match`, `count`.
  * `parser.go` — рекурсивный спуск + парсер Пратта, неразрушающий предпросмотр `isGenericCallAhead()` для `<` vs `<T>(`, снижение `test "name" { ... }` в `__bitc_test_<N>`.
  * `sema.go` — резолвинг `import`, полная мономорфизация типов и констант в структурах/функциях (`substituteType`), поддержка тестового режима `AnalyzeProgram(prog, isTestMode)`.
  * `codegen.go` — C-бэкенд: `bvec<N>` (`<=64` -> `uintN_t`, `>64` -> 32-байтно выровненный `bvecN_t`), побитовые чтение/запись для любой ширины `N`, `#pragma omp parallel for` перед циклом, маппинг `i8` (без `float`), хук `bitc_assert`.
* **Стандартная библиотека (`std/`) и тесты (`tests/`):**
  * `[Done]` `std/test.bc` (`assert_true`, `assert_eq`, `assert_neq`, `assert_popcount<N>`).
  * `[Done]` `std/sys.bc` (`rand_mask`, `now_ns`, `save_raw`, `load_raw`, `rand_mask256`) + `tests/sys_test.bc`.
  * `[Done]` `std/encode.bc` (`thermometer64`) + `tests/encode_test.bc`.
  * `[Done]` `std/learn.bc` (`nudge<N>`) + `tests/learn_test.bc`.
  * `[Done]` `std/mem.bc` (`RingBuffer<T, Cap>`, `ByteArena<Cap>`) + `tests/mem_test.bc`.
  * `[In Progress]` `std/tensor.bc` (`TensorView1D<T>`, `TensorView2D<T>`, `quantize_i32_to_i8` Q16.16, `binarize_i8_to_bvec64`) + `tests/tensor_test.bc`.

## 3. Дорожная карта (Roadmap)
* **Этап 1 (Завершен):** Аудит ядра и утверждение спецификации `specs/SPECIFICATION.md`.
* **Этап 2.0 (Завершен):** Рефакторинг компилятора, фикс широких `bvec<N>` и OpenMP, внедрение `./bitc test`, `std/test.bc` и вынос тестов в `tests/`.
* **Этап 2.1 (Текущий — Уровень 1 Core):**
  * [x] `std/mem.bc` + `tests/mem_test.bc`
  * [~] `std/tensor.bc` + `tests/tensor_test.bc` (дописать проверку сборки + закоммитить)
  * [ ] `std/stream.bc` (потоковые каналы `Channel<T, Cap>` с backpressure) + тесты
  * [ ] `std/io.bc` (унифицированные интерфейсы `Reader`/`Writer`, файлы, UART/I2C/GPIO) + тесты
* **Этап 2.2 (Уровень 4 — Когнитивный слой и агенты):**
  * [ ] `std/schema.bc` (валидация и парсинг структурированного вывода без исключений)
  * [ ] `std/flow.bc` (конечные автоматы `StateMachine` и маршрутизация)
  * [ ] `std/context.bc`, `std/tools.bc`, `std/reasoning.bc` (память, реестр инструментов, циклы самопроверки)
* **Этап 3 (Уровни 2 и 3 — Сенсорика и локальный инференс):**
  * [ ] `std/vision.bc`, `std/audio.bc`, `std/infer.bc`, `std/post.bc` (целочисленные БПФ/Mel, NMS, трекер, антидребезг)
  * [ ] 3 эталонных проекта в `examples/` (Edge Vision, Edge Audio, Hybrid AI Agent) + пользовательская документация в `docs/`.
* **Этап 4:** Публикация `v0.1-alpha` на GitHub с файлом `LICENSE` (MIT).

## 4. Точка возобновления следующей сессии
* **Сделано:** Написаны `std/tensor.bc` и `tests/tensor_test.bc`, добавлен `i8` в `codegen.go`.
* **Следующий шаг:** Прогнать `./bitc test tests/tensor_test.bc`, закоммитить `std/tensor.bc`, и приступить к **`std/stream.bc`** и **`std/io.bc`**.
