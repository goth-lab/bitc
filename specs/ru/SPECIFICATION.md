# Спецификация стандартной библиотеки BIT-C (`SPECIFICATION.md` — v0.1.0-alpha)

> [Read in English](../SPECIFICATION.md)

## 1. Архитектурный манифест (100% Integer & Bitwise)

1. **Никаких `float` и GPU:** Типы с плавающей точкой (`f32`, `f64`) строго исключены. Масштабирование, пороги, геометрия IoU и частота сигнала реализованы на целочисленной арифметике с фиксированной точкой **`Q16.16`** (`>> 16`) и битовых векторах `bvec<N>`.
2. **Детерминированная память (без `malloc`):** Все структуры `std/` используют статические буферы фиксированной ёмкости (`Cap`, `MaxRules`, `MaxTurns`) или невладеющие `zero-copy` представления (`TensorView1D`, `GrayImageView`, `AudioFrame16`).
3. **Обработка ошибок без исключений:** Функции возвращают явные целочисленные статусы (`1`/`0` или отрицательные коды ошибок вроде `io_err_mode()`) либо заполняют состояние `SchemaValidator`.
4. **Doxygen и 100% покрытие тестами:** Все 16 модулей `std/*.bc` снабжены комментариями `///` и покрыты модульными тестами в `tests/*_test.bc`.

---

## 2. Реализованные модули (Уровни 0–4)

* **Уровень 0 (Качество):** `std/test.bc` (`assert_true`, `assert_eq`, `assert_neq`, `assert_popcount<N>`).
* **Уровень 1 (Системный фундамент):** `std/sys.bc` (`rand_mask`, `rand_mask256`, `now_ns`, `print_bench`, `save_raw`, `load_raw`), `std/mem.bc` (`RingBuffer<T, Cap>`, `ByteArena<Cap>`), `std/tensor.bc` (`TensorView1D<T>`, `TensorView2D<T>`, `quantize_i32_to_i8`, `binarize_i8_to_bvec64`), `std/stream.bc` (`Channel<T, Cap>` с политиками переполнения), `std/io.bc` (`ByteStream<Cap>`, `GpioPin`).
* **Уровень 2 (Сенсорика):** `std/encode.bc` (`thermometer64`), `std/vision.bc` (`GrayImageView.crop`, `rgb888_to_gray`, `encode_grid_16x16_bvec256`), `std/audio.bc` (`AudioFrame16`, `audio_mean_abs`, `audio_vad`, `audio_zcr_q16`, `encode_audio_64`).
* **Уровень 3 (Инференс и фильтрация):** `std/learn.bc` (`nudge<N>`), `std/post.bc` (`BBox`, `bbox_iou_q16`, `DebounceFilter`).
* **Уровень 4 (Когнитивный слой и агенты):** `std/flow.bc` (`StateMachine<MaxRules>`, `IntentRouter256<MaxRoutes>`), `std/schema.bc` (`SchemaValidator`, `parse_int_ascii`), `std/context.bc` (`ContextWindow<MaxTurns>`), `std/tools.bc` (`ToolRegistry<MaxTools>`), `std/reasoning.bc` (`vote_majority_int`, `bundle_bvec256`).

---

## 3. Матрица готовности (`v0.1.0-alpha`)

Все 16 модулей стандартной библиотеки имеют статус **`[Done]`** и проходят проверку через `./bitc test tests/`.
