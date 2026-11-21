# BIT-C Standard Library Specification (`SPEC.md` — Part 1)

## 0. Архитектурные соглашения языка для `stdlib`

Чтобы вся стандартная библиотека (от работы с регистрами MCU до когнитивных агентов) была консистентной, фиксируем базовые правила, которые должен поддерживать компилятор BIT-C:

1. **Обработка ошибок (`Result<T, E>`):**
   * В `stdlib` **нет исключений (exceptions)** — они непредсказуемы по памяти и времени выполнения на микроконтроллерах.
   * Любая операция, которая может завершиться неудачей (аллокация, парсинг JSON, инференс, I/O), возвращает `Result<T, Error>`.
   * Оператор проброса ошибки: `?` (аналог Rust/Zig).
2. **Явное управление памятью (Explicit Allocators):**
   * Ни одна функция `stdlib` не делает скрытых глобальных аллокаций (`malloc`).
   * Все структуры, требующие динамической или пуловой памяти, принимают интерфейс `Allocator` (например, `ArenaAllocator`, `PoolAllocator`, `StackAllocator`).
3. **Срезы и владение (`Slice<T>` / `View<T>`):**
   * Данные передаются через невладеющие представления (`View<u8>`, `TensorView`), что обеспечивает `zero-copy` при прогоне кадров с камеры или аудиопотока через конвейер.
4. **Модульность и неймспейсы:**
   * Импорт осуществляется через `use std.<module>::{...};`.

---

## 1. Встроенный фреймворк тестирования (`std.test`) — [Priority: CRITICAL]

Тестирование встроено в синтаксис языка и CLI компилятора (`bitc test`). На микроконтроллерах тесты могут запускаться как на хост-машине (x86/ARM64), так и через QEMU / hardware-in-the-loop.

### Ключевые конструкции языка и примитивы:
* Блок `test "описание теста" { ... }` — игнорируется при обычной сборке (`bitc build`), компилируется только при `bitc test`.
* Макросы/функции проверок:
  * `std.test.expect(cond: bool)` — базовая проверка истинности.
  * `std.test.expect_eq(actual: T, expected: T)` — проверка равенства с выводом diff при падении.
  * `std.test.expect_err(res: Result<T, E>, expected_err: E)` — проверка ожидаемой ошибки.
  * `std.test.expect_no_leaks(allocator: *TrackingAllocator)` — проверка отсутствия утечек памяти (критично для `std.mem` и `std.tensor`).
  * `std.test.mock<T>(...)` — примитив для подмены аппаратного I/O (камеры, UART) или ответов LLM в юнит-тестах.

### Пример использования:
```bitc
use std.test;
use std.mem::RingBuffer;

test "RingBuffer переполнение и чтение" {
    var buf: RingBuffer<u8, 4> = RingBuffer.init();
    try test.expect(buf.push(10).is_ok());
    try test.expect(buf.push(20).is_ok());
    try test.expect_eq(buf.pop(), Some(10));
}
```

---

## 2. Уровень 1: Системный фундамент (`Core & Resources`)

### 2.1. `std.mem` — Детерминированная память
Набор аллокаторов и структур без сборщика мусора для работы в условиях жесткого лимита RAM.

* **Типы:**
  * `Allocator` — базовый интерфейс (`alloc(size, align)`, `free(ptr)`).
  * `FixedBufferAllocator` — выделение памяти поверх статического массива на стеке или в `.bss`.
  * `ArenaAllocator` — быстрая аллокация с очисткой всего региона разом (идеально для одного кадра видео или одного шага рассуждения агента).
  * `RingBuffer<T, const N: usize>` — потокобезопасный (lock-free SPSC) кольцевой буфер фиксированного размера для аудио-сэмплов и пакетов.
  * `Pool<T, const N: usize>` — пул объектов фиксированного размера (без фрагментации).

### 2.2. `std.tensor` — Легковесные тензоры и квантование
Абстракция над многомерными массивами с поддержкой типов данных, используемых в Edge AI.

* **Типы данных (`DType`):** `f32`, `f16`, `bf16`, `i8`, `u8`, `i4` (упакованные по 2 значения в байт).
* **Структуры:**
  * `Shape` — статический массив размерностей (до 4D/5D без динамических аллокаций).
  * `Tensor<T>` — владеющий тензор (привязан к `Allocator`).
  * `TensorView<T>` — невладеющий срез (`slice`, `reshape`, `transpose`, `permute` выполняются за $O(1)$ через изменение `strides` без копирования данных).
* **Операции:** `quantize(src: TensorView<f32>, scale: f32, zero_point: i8) -> Tensor<i8>`, `dequantize(...)`, `dot(...)`, `softmax(...)`.

### 2.3. `std.stream` — Потоковые конвейеры (Pipelines)
Управление асинхронными потоками данных с контролем переполнения (backpressure).

* **Примитивы:**
  * `Channel<T, const CAP: usize>` — типизированный канал передачи данных между этапами конвейера.
  * `BackpressurePolicy` — стратегия при переполнении: `Block` (ждать), `DropOldest` (выкинуть старый кадр камеры — критично для real-time), `DropNewest` (отбросить новый), `Error`.
  * `Pipeline` — композиция узлов обработки (`Source -> Transform -> Sink`) с поддержкой тайм-аутов (`timeout_ms`) и graceful shutdown.

### 2.4. `std.io` — Аппаратный и системный ввод-вывод
Унифицированные потоки байтов (`Reader` / `Writer`), работающие одинаково для файлов, сокетов и микроконтроллерной периферии.

* **Интерфейсы:**
  * `Reader` (`read(buf: Slice<u8>) -> Result<usize, IoError>`)
  * `Writer` (`write(data: View<u8>) -> Result<usize, IoError>`)
* **Реализации:** `File`, `TcpStream`, `SerialPort` (UART), `I2cBus`, `SpiBus`, `GpioPin` (Digital/PWM).

# BIT-C Standard Library Specification (`SPEC.md` — Part 2)

## 3. Уровень 2: Сенсорика и предобработка (`Sensors & Preprocessing`)

Модули этого уровня избавляют разработчика от необходимости тащить тяжелые C++/Python-зависимости (`OpenCV`, `Librosa`) на микрокомпьютеры. Все операции поддерживают работу поверх предварительно выделенных буферов (`ArenaAllocator` / `FixedBufferAllocator`).

### 3.1. `std.vision` — Компьютерное зрение для Edge-устройств
* **Типы:**
  * `PixelFormat`: `RGB888`, `BGR888`, `GRAY8`, `YUV420`, `NV12`.
  * `ImageView`: невладеющая обертка над буфером кадра (`width`, `height`, `stride`, `format`, `data: View<u8>`).
  * `Rect`: `{ x: u32, y: u32, w: u32, h: u32 }`.
* **Примитивы:**
  * `Camera.open(device_id: u8, config: CamConfig) -> Result<Camera, VisionError>` (обертка над V4L2 / libcamera / DCMI).
  * `convert(src: ImageView, dst: &mut ImageBuffer, target_fmt: PixelFormat) -> Result<(), VisionError>` — аппаратно-ускоряемая конвертация цветов.
  * `resize(src: ImageView, dst: &mut ImageBuffer, method: InterpMethod) -> Result<(), VisionError>` (`Nearest`, `Bilinear`).
  * `crop(src: ImageView, roi: Rect) -> Result<ImageView, VisionError>` — **zero-copy** вырезка области интереса за $O(1)$.
  * `to_tensor(src: ImageView, dst: &mut Tensor<f32>, norm: NormParams) -> Result<(), VisionError>` — нормализация пикселей (`mean`, `std`, `HWC -> CHW`) под вход нейросети.

### 3.2. `std.audio` — Обработка звука и сигналов
* **Типы:**
  * `AudioFormat`: `{ sample_rate: u32, channels: u8, bit_depth: u8 }`.
  * `AudioFrame<T>`: срез сэмплов (`View<i16>` или `View<f32>`).
  * `WindowType`: `Hann`, `Hamming`, `Blackman`.
* **Примитивы:**
  * `Mic.open(device_id: u8, fmt: AudioFormat) -> Result<Mic, AudioError>` (ALSA / I2S).
  * `apply_window(frame: &mut [f32], win: WindowType)` — наложение оконной функции на месте (in-place).
  * `fft_real(input: View<f32>, output_mag: &mut [f32]) -> Result<(), AudioError>` — быстрое преобразование Фурье для вещественного сигнала.
  * `mel_spectrogram(audio: View<f32>, config: MelConfig, out_tensor: &mut Tensor<f32>) -> Result<(), AudioError>` — извлечение признаков для моделей распознавания речи и звуков.
  * `vad_energy(frame: View<i16>, threshold_db: f32) -> bool` — легковесный детектор голосовой активности (Voice Activity Detection), чтобы не запускать тяжелую модель в тишине.

---

## 4. Уровень 3: Локальный инференс и постобработка (`Local Inference`)

### 4.1. `std.infer` — Универсальный движок запуска моделей
Единый контракт для запуска квантованных и полноразмерных моделей на CPU, NPU или микроконтроллерах без переписывания прикладного кода.

* **Типы:**
  * `Backend`: `CpuSimd`, `TFLiteMicro`, `OnnxRuntime`, `LlamaCpp`, `NpuDelegate`.
  * `SessionConfig`: `{ backend: Backend, threads: u8, arena_size_kb: usize }`.
* **Примитивы:**
  * `Model.load(bytes: View<u8>, cfg: SessionConfig, alloc: &Allocator) -> Result<Model, InferError>` — загрузка модели из файла или Flash-памяти (XIP).
  * `Session.run(inputs: View<TensorView>, outputs: &mut [Tensor]) -> Result<(), InferError>` — детерминированный прогон инференса без скрытых аллокаций.

### 4.2. `std.post` — Постобработка и фильтрация событий
Инструменты, превращающие сырые тензоры нейросети в надежные инженерные события (особенно важно для робототехники и промышленных датчиков).

* **Примитивы:**
  * `nms(boxes: View<BBox>, scores: View<f32>, iou_thresh: f32, out: &mut Vec<BBox>)` — Non-Maximum Suppression для детекторов объектов.
  * `argmax(logits: View<f32>) -> (usize, f32)` — индекс и уверенность топового класса.
  * `Tracker` — легковесный трекер объектов между кадрами (IoU / Kalman-фильтр) с присвоением стабильного `track_id`.
  * `Debounce<T>` / `Hysteresis` — фильтры дребезга и ложных срабатываний (например, срабатывание тревоги только если объект подтвержден в 4 из 5 последних кадров).

---

## 5. Уровень 4: Когнитивный слой и агенты (`Cognitive & Agents`)

Примитивы для работы с LLM/SLM (как локальными, так и облачными), структурирования ответов и безопасного управления внешним миром (API, базы данных или исполнительные механизмы робота).

### 5.1. `std.schema` — Строгая типизация вывода AI
Аналог Pydantic / Instructor, встроенный в систему типов языка.

* **Примитивы:**
  * Атрибут `#[derive(Schema)]` для структур BIT-C: автоматически генерирует JSON-Schema / GBNF-грамматику для constrained decoding (гарантирует, что модель физически не сможет выдать невалидный JSON).
  * `parse_structured<T: Schema>(raw_text: View<u8>, alloc: &Allocator) -> Result<T, SchemaError>` — парсинг и валидация ограничений (`#[range(0, 100)]`, `#[non_empty]`).

### 5.2. `std.context` — Управление памятью и окном контекста
* **Примитивы:**
  * `ContextWindow`: кольцевой или приоритетный буфер сообщений с жестким бюджетом токенов (`max_tokens`).
  * `Role`: `System`, `User`, `Assistant`, `Tool`.
  * `ContextWindow.push(msg: Message, strategy: EvictionStrategy)` — стратегии вытеснения при нехватке памяти (`DropOldest`, `KeepSystemAndRecent`, `SummarizeCallback`).

### 5.3. `std.tools` — Вызов внешних инструментов и аппаратных актуаторов
Безопасный мост между решением модели и реальными действиями (HTTP-запрос или поворот сервопривода).

* **Типы:**
  * `ToolSpec`: описание имени, параметров (`Schema`) и прав доступа (`ReadOnly`, `Actuator`, `Network`).
  * `ToolRegistry`: статический реестр разрешенных функций.
* **Примитивы:**
  * `ToolRegistry.dispatch(call: ToolCall, ctx: &mut ExecContext) -> Result<ToolOutput, ToolError>` — валидация аргументов по схеме перед вызовом реальной функции.

### 5.4. `std.reasoning` — Паттерны рассуждений и самопроверки
* **Примитивы:**
  * `verify_loop<T: Schema>(llm: &LlmClient, prompt: View<u8>, validator: fn(&T) -> Result<(), Error>, max_retries: u8) -> Result<T, ReasonError>` — цикл генерации с автоматической передачей текста ошибки валидации обратно модели для самоисправления.
  * `vote<T: Eq>(candidates: View<T>) -> Option<T>` — консенсус (Self-Consistency) по нескольким прогонам.

### 5.5. `std.flow` — Оркестрация и конечные автоматы (FSM / DAG)
* **Примитивы:**
  * `StateMachine<State, Event>` — детерминированный конечный автомат (идеально для роботов и умных устройств: `Idle -> Listening -> Thinking -> Acting -> SafeStop`).
  * `route(intent: View<u8>, routes: View<Route>) -> RouteId` — семантическая или детерминированная маршрутизация задач между легкой локальной моделью и тяжелой облачной.

---

## 6. Сводная дорожная карта реализации (`STATUS_MATRIX`)

| Модуль | Уровень | Приоритет | Статус | Целевой этап |
| :--- | :--- | :--- | :--- | :--- |
| `std.test` | 0. Quality | **P0 (Critical)** | `[Planned]` | Этап 2.1 (Фундамент) |
| `std.mem` | 1. Core | **P0 (Critical)** | `[Planned]` | Этап 2.1 (Фундамент) |
| `std.tensor` | 1. Core | **P0 (Critical)** | `[Planned]` | Этап 2.2 (Фундамент) |
| `std.stream` | 1. Core | **P1 (High)** | `[Planned]` | Этап 2.3 (Фундамент) |
| `std.io` | 1. Core | **P1 (High)** | `[Planned]` | Этап 2.3 (Фундамент) |
| `std.schema` | 4. Cognitive | **P1 (High)** | `[Planned]` | Этап 2.4 (AI Core) |
| `std.flow` | 4. Cognitive | **P1 (High)** | `[Planned]` | Этап 2.4 (AI Core) |
| `std.vision` | 2. Sensors | **P2 (Medium)** | `[Planned]` | Этап 3.1 (Edge Vision) |
| `std.audio` | 2. Sensors | **P2 (Medium)** | `[Planned]` | Этап 3.2 (Edge Audio) |
| `std.infer` | 3. Inference | **P2 (Medium)** | `[Planned]` | Этап 3.1 (Edge Vision) |
| `std.post` | 3. Inference | **P2 (Medium)** | `[Planned]` | Этап 3.1 (Edge Vision) |
| `std.context` | 4. Cognitive | **P2 (Medium)** | `[Planned]` | Этап 3.3 (Hybrid Agent) |
| `std.tools` | 4. Cognitive | **P2 (Medium)** | `[Planned]` | Этап 3.3 (Hybrid Agent) |
| `std.reasoning`| 4. Cognitive | **P3 (Normal)** | `[Planned]` | Этап 3.3 (Hybrid Agent) |
