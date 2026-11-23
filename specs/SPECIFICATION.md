# BIT-C Standard Library Specification (`SPECIFICATION.md` — v0.1.0-alpha)

> [Читать на русском](ru/SPECIFICATION.md)

## 1. Architectural Manifesto (100% Integer & Bitwise)

1. **Zero `float`, Zero GPU:** Floating-point types (`f32`, `f64`) are strictly excluded. Scaling, thresholds, IoU geometry, and signal rates use **`Q16.16`** fixed-point integer math (`>> 16`) and wide bit-vectors `bvec<N>`.
2. **Deterministic Memory (Zero `malloc`):** Every `std/` structure uses fixed-capacity stack/static storage (`Cap`, `MaxRules`, `MaxTurns`) or non-owning zero-copy views (`TensorView1D`, `GrayImageView`, `AudioFrame16`).
3. **Non-Throwing Error Contracts:** Functions return explicit integer status codes (`1`/`0` or negative error constants like `io_err_mode()`) or populate `SchemaValidator` state without exceptions.
4. **Native Doxygen & Test Coverage:** All `std/*.bc` modules carry `///` Doxygen documentation and verified unit test suites in `tests/*_test.bc`.

---

## 2. Implemented Module Architecture (Tiers 0–4)

### Tier 0 & Tier 1: Quality, Runtime, Memory, Tensors, Streams & I/O
* **`std/test.bc`:** `assert_true(cond, step)`, `assert_eq(actual, expected, step)`, `assert_neq(actual, unexpected, step)`, `assert_popcount<N>(vec, expected_ones, step)`.
* **`std/sys.bc`:** `rand_mask() -> bvec<64>`, `rand_mask256() -> bvec<256>`, `now_ns() -> int`, `print_bench(id, iters, elapsed_ns)`, `save_raw`, `load_raw`.
* **`std/mem.bc`:** `RingBuffer<T, Cap>` (`push`, `push_overwrite`, `pop`, `len`, `is_full`), `ByteArena<Cap>` (8-byte aligned bump allocator with $O(1)$ `reset()`).
* **`std/tensor.bc`:** `TensorView1D<T>` (`slice`), `TensorView2D<T>` (`at`), Q16.16 `quantize_i32_to_i8`, `binarize_i8_to_bvec64`.
* **`std/stream.bc`:** `Channel<T, Cap>` (`init`, `send`, `recv`, `close`, `pending`) with `stream_policy_reject()`, `stream_policy_drop_oldest()`, `stream_policy_drop_newest()`.
* **`std/io.bc`:** `ByteStream<Cap>` (`write_byte`, `read_byte`, `write_bytes`, `read_bytes`), `GpioPin` (`init`, `write`, `read` with `INPUT`/`OUTPUT` enforcement).

### Tier 2 & Tier 3: Sensors, Preprocessing, On-Device Learning & Post-Processing
* **`std/encode.bc`:** `thermometer64(val) -> bvec<64>` locality-preserving scalar encoder.
* **`std/vision.bc`:** `GrayImageView` (`crop` in $O(1)$), ITU-R BT.601 `rgb888_to_gray`, spatial `encode_grid_16x16_bvec256`.
* **`std/audio.bc`:** `AudioFrame16`, `audio_mean_abs`, `audio_vad`, Q16.16 `audio_zcr_q16`, temporal energy `encode_audio_64`.
* **`std/learn.bc`:** `nudge<N>(&weight, input, wantFire, mask)` stochastic bitwise weight update.
* **`std/post.bc`:** `BBox`, Q16.16 `bbox_iou_q16`, `DebounceFilter` (`init`, `update` hysteresis filter).

### Tier 4: Cognitive Layer & Autonomous Agents
* **`std/flow.bc`:** `StateMachine<MaxRules>` (`init`, `add_rule`, `step`, `force_safe_stop`), `IntentRouter256<MaxRoutes>` (`register_route`, `route`).
* **`std/schema.bc`:** `SchemaValidator` (`check_range`, `check_required`, `check_density256`), strict `parse_int_ascii`.
* **`std/context.bc`:** `ContextWindow<MaxTurns>` (`init`, `push` with token-budget eviction preserving `role_system()`, `recall_best`).
* **`std/tools.bc`:** `ToolRegistry<MaxTools>` (`init`, `register_tool`, `authorize` enforcing `perm_read_only()`, `perm_actuator()`, `perm_network()`).
* **`std/reasoning.bc`:** `vote_majority_int`, `bundle_bvec256` bit-wise majority consensus.

---

## 3. Implementation Status Matrix (`v0.1.0-alpha`)

| Module | Tier | Priority | Status | Unit Test Suite |
| :--- | :--- | :--- | :---: | :--- |
| `std/test.bc` | 0. Quality | P0 | `[Done]` | `tests/*_test.bc` |
| `std/sys.bc` | 1. Core | P0 | `[Done]` | `tests/sys_test.bc` |
| `std/mem.bc` | 1. Core | P0 | `[Done]` | `tests/mem_test.bc` |
| `std/tensor.bc` | 1. Core | P0 | `[Done]` | `tests/tensor_test.bc` |
| `std/stream.bc` | 1. Core | P1 | `[Done]` | `tests/stream_test.bc` |
| `std/io.bc` | 1. Core | P1 | `[Done]` | `tests/io_test.bc` |
| `std/encode.bc` | 2. Sensors | P0 | `[Done]` | `tests/encode_test.bc` |
| `std/vision.bc` | 2. Sensors | P2 | `[Done]` | `tests/edge_sensors_test.bc` |
| `std/audio.bc` | 2. Sensors | P2 | `[Done]` | `tests/edge_sensors_test.bc` |
| `std/learn.bc` | 3. Inference | P0 | `[Done]` | `tests/learn_test.bc` |
| `std/post.bc` | 3. Inference | P2 | `[Done]` | `tests/edge_sensors_test.bc` |
| `std/flow.bc` | 4. Cognitive | P1 | `[Done]` | `tests/flow_test.bc` |
| `std/schema.bc` | 4. Cognitive | P1 | `[Done]` | `tests/schema_test.bc` |
| `std/context.bc` | 4. Cognitive | P2 | `[Done]` | `tests/context_test.bc` |
| `std/tools.bc` | 4. Cognitive | P2 | `[Done]` | `tests/agent_primitives_test.bc` |
| `std/reasoning.bc`| 4. Cognitive | P3 | `[Done]` | `tests/agent_primitives_test.bc` |
