# BIT-C Compiler — Architecture & Context Snapshot (v1.1)

## 1. Философия проекта и правила разработки
* **Миссия BIT-C:** Компилируемый язык (транслируется в C99/C11 с SIMD/OpenMP) как сверхбыстрая, энергоэффективная альтернатива дорогим вычислениям на GPU и FPU — от микроконтроллеров и умных устройств до серверного инференса и гибридных AI-агентов.
* **Фундаментальный закон вычислений:** **100% Integer & Bitwise Computing (No Float, No GPU).** Только целочисленная арифметика (`int`, `i32`, `i16`, `i8`, `u8`), фиксированная точка (`Q16.16`) и бинарные гипервекторы (`bvec<N>` любой ширины).
* **Парадигма:** Hyperdimensional Computing (HDC) + Binary/Bitwise Neural Networks + детерминированная системная логика без GC и скрытых `malloc`.
* **Авторство и лицензия:** `Copyright (c) 2026 Vladimir Borisenko (Goth) & AI Gemini` (MIT License) в шапке каждого исходного файла.
* **Стандарты кода и документации:**
  * Весь код (`*.go`, `*.bc`), комментарии, ошибки компилятора и C-код: **строго на английском языке**.
  * Проектная документация (`CONTEXT_SNAPSHOT.md`, `specs/SPECIFICATION.md`, `README.md`): на русском языке до этапа релиза.
  * Ограничение в чате: каждый блок кода от AI — **строго до 75 строк** (внимание на квадратные скобки в Markdown).
  * Разметка секций: `// MARK: - <Section Title>` во всех файлах.
  * Go (`*.go`): стандартный GoDoc + подробные строчные комментарии (`//`).
  * BIT-C (`*.bc`): Doxygen-блоки (`/// @brief`, `@param`, `@return`, `@note`).

## 2. Актуальная архитектура компилятора (`v1.1`)
* `main.go` — CLI (`./bitc <file.bc>` и `./bitc test <file.bc>`), сборка через `clang -O3` с флагами нативного CPU (`-mcpu=native` / `-march=native`).
* `lexer.go` — токенизация, включая `TOK_TEST` (`test`) и контекстные идентификаторы (`in`, `step`).
* `ast.go` — узлы AST с поддержкой шаблонов, ресиверов `(self: *T)`, `parallel for`, `match`, `count`.
* `parser.go` — рекурсивный спуск + парсер Пратта, неразрушающий предпросмотр `isGenericCallAhead()`, поддержка вложенных шаблонов (`Channel<bvec<64>, 2>`), снижение `test "name" { ... }` в `__bitc_test_<N>`.
* `sema.go` — резолвинг `import`, полная мономорфизация вложенных типов (`substituteType`, `MonoStructOrder`), ленивая инстанциация соседних методов структуры, проверка типов границ `ForStmt`, вывод типов индексации указателей `*T[i]` vs битов `bvec<N>[i]`.
* `codegen.go` — C-бэкенд: `bvec<N>` (`<=64` -> `uintN_t`, `>64` -> 32-байтно выровненный `bvecN_t`), побитовые чтение/запись для любой ширины `N`, топологический порядок структур `MonoStructOrder`, `#pragma omp parallel for` перед циклом, маппинг `i8` (без `float`), хук `bitc_assert`.

## 3. Реализованная стандартная библиотека (`std/`) и тесты (`tests/`)
Все 13 модулей `stdlib` написаны, документированы и покрыты юнит-тестами:
1. `std/test.bc` — `assert_true`, `assert_eq`, `assert_neq`, `assert_popcount<N>`.
2. `std/sys.bc` (`tests/sys_test.bc`) — `rand_mask`, `now_ns`, `save_raw`, `load_raw`, `rand_mask256`.
3. `std/encode.bc` (`tests/encode_test.bc`) — `thermometer64`.
4. `std/learn.bc` (`tests/learn_test.bc`) — стохастическое on-device обучение `nudge<N>`.
5. `std/mem.bc` (`tests/mem_test.bc`) — `RingBuffer<T, Cap>` (`push`, `push_overwrite`, `pop`), `ByteArena<Cap>`.
6. `std/tensor.bc` (`tests/tensor_test.bc`) — `TensorView1D<T>`, `TensorView2D<T>`, Q16.16 `quantize_i32_to_i8`, `binarize_i8_to_bvec64`.
7. `std/stream.bc` (`tests/stream_test.bc`) — `Channel<T, Cap>` (политики `Reject`, `DropOldest`, `DropNewest`).
8. `std/io.bc` (`tests/io_test.bc`) — `ByteStream<Cap>` (`write_bytes`, `read_bytes`), безопасный `GpioPin`.
9. `std/flow.bc` (`tests/flow_test.bc`) — конечный автомат `StateMachine<MaxRules>`, гипервекторный маршрутизатор `IntentRouter256<MaxRoutes>`.
10. `std/schema.bc` (`tests/schema_test.bc`) — валидатор контрактов `SchemaValidator`, строгий `parse_int_ascii`.
11. `std/context.bc` (`tests/context_test.bc`) — `ContextWindow<MaxTurns>` с бюджетом токенов, защитой `SYSTEM` и HDC-поиском `recall_best`.
12. `std/tools.bc` (`tests/agent_primitives_test.bc`) — реестр прав доступа `ToolRegistry<MaxTools>` (`ReadOnly`, `Actuator`, `Network`).
13. `std/reasoning.bc` (`tests/agent_primitives_test.bc`) — `vote_majority_int`, `bundle_bvec256`.
14. `std/vision.bc`, `std/audio.bc`, `std/post.bc` (`tests/edge_sensors_test.bc`) — `GrayImageView` (`crop`, `rgb888_to_gray`, `encode_grid_16x16_bvec256`), `AudioFrame16` (`audio_mean_abs`, `audio_vad`, `audio_zcr_q16`, `encode_audio_64`), `BBox` (`bbox_iou_q16`), `DebounceFilter`.

## 4. Следующие шаги (Этап 3 и Этап 4)
* **Этап 3 (Прикладные эталоны и документация):**
  * [ ] Добавить поддержку пакетного запуска `./bitc test tests/` (прогон всей папки тестов одной командой).
  * [ ] Написать 3 эталонных проекта в `examples/` (Edge Vision детектор, Edge Audio голосовой триггер, Hybrid AI Agent с `SchemaValidator` + `ToolRegistry` + `StateMachine`).
  * [ ] Подготовить пользовательскую документацию в `docs/` (`LANGUAGE_GUIDE.md`, `STDLIB_REFERENCE.md`) и перевести `README.md` / `SPECIFICATION.md` на английский.
* **Этап 4:** Релиз `v0.1-alpha` на GitHub и привлечение разработчиков.
