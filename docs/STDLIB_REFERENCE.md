# BIT-C Standard Library Reference (`STDLIB_REFERENCE.md`)

> [Читать на русском](ru/STDLIB_REFERENCE.md)

The BIT-C standard library provides 16 composable, zero-allocation, integer-only modules organized into five architectural tiers.

## Tier 0: Quality & Unit Testing
* **`std/test.bc`** — Built-in test assertions:
  * `assert_true(cond, step)` — Asserts that `cond` is non-zero.
  * `assert_eq(actual, expected, step)` / `assert_neq(...)` — Strict 64-bit integer equality assertions.
  * `assert_popcount<N>(vec, expected_ones, step)` — Verifies the exact Hamming weight of a `bvec<N>`.

## Tier 1: Core Systems & Resource Management
* **`std/sys.bc`** — Low-level runtime primitives:
  * `rand_mask() -> bvec<64>`, `rand_mask256() -> bvec<256>` — Sparse Xorshift64* random hypervector masks.
  * `now_ns() -> int` — Monotonic nanosecond clock for real-time telemetry.
  * `print_bench(id, iterations, elapsed_ns)` — Integer-only formatted latency (`ns/op`) and throughput (`M ops/sec`) report.
  * `save_raw(slot, ptr, bytes)` / `load_raw(...)` — Raw binary persistence for model weights.
* **`std/mem.bc`** — Deterministic memory without heap fragmentation:
  * `RingBuffer<T, Cap>` — Fixed-capacity FIFO buffer (`push`, `push_overwrite`, `pop`, `len`, `is_full`).
  * `ByteArena<Cap>` — 8-byte aligned bump allocator with $O(1)$ bulk `reset()`.
* **`std/tensor.bc`** — Zero-copy views and fixed-point quantization:
  * `TensorView1D<T>` (`slice`) and `TensorView2D<T>` (`at`) — Non-owning views over contiguous or strided buffers.
  * `quantize_i32_to_i8(src, dst, inv_scale_q16)` — FPU-free Q16.16 quantization into clamped `i8` (`-128..127`).
  * `binarize_i8_to_bvec64(src, threshold)` — Threshold binarization into a 64-bit hypervector.
* **`std/stream.bc`** — Bounded streaming pipelines:
  * `Channel<T, Cap>` — Typed queue (`send`, `recv`, `close`, `pending`) with `stream_policy_reject()`, `stream_policy_drop_oldest()`, and `stream_policy_drop_newest()`.
* **`std/io.bc`** — Serial streams and hardware peripherals:
  * `ByteStream<Cap>` — Byte-oriented FIFO stream (`write_bytes`, `read_bytes`) for UART, sensor frames, and mocks.
  * `GpioPin` — Direction-enforced digital GPIO controller (`init`, `write`, `read`) with toggle telemetry.

## Tier 2: Multimodal Sensors & Preprocessing
* **`std/encode.bc`** — Scalar feature encoders:
  * `thermometer64(val) -> bvec<64>` — Locality-preserving thermometer encoder for sensor readings.
* **`std/vision.bc`** — FPU-free computer vision:
  * `GrayImageView` — Strided 8-bit image view with $O(1)$ zero-copy `crop(x, y, w, h)`.
  * `rgb888_to_gray(rgb, dst)` — Integer ITU-R BT.601 luma conversion.
  * `encode_grid_16x16_bvec256(img, threshold)` — Spatial downsampling and binarization into `bvec<256>`.
* **`std/audio.bc`** — FPU-free digital audio DSP:
  * `AudioFrame16` — Non-owning 16-bit PCM sample slice.
  * `audio_mean_abs(frame)` & `audio_vad(frame, threshold)` — L1 energy and Voice Activity Detection (VAD).
  * `audio_zcr_q16(frame)` — Zero-Crossing Rate in Q16.16 fixed-point.
  * `encode_audio_64(frame)` — Temporal energy envelope encoder into `bvec<64>`.

## Tier 3: On-Device Inference & Post-Processing
* **`std/learn.bc`** — Gradient-free on-device learning:
  * `nudge<N>(&weight, input, wantFire, mask)` — Stochastic bitwise synaptic weight nudging.
* **`std/post.bc`** — Detection filtering and geometry:
  * `BBox` & `bbox_iou_q16(a, b)` — Exact integer Intersection-over-Union in Q16.16 (`0..65536`).
  * `DebounceFilter` — Hysteresis debounce filter (`init`, `update`) eliminating false-positive triggers.

## Tier 4: Cognitive Layer & Autonomous Agents
* **`std/flow.bc`** — Orchestration and semantic routing:
  * `StateMachine<MaxRules>` — Deterministic finite state machine (`add_rule`, `step`, `force_safe_stop`).
  * `IntentRouter256<MaxRoutes>` — XNOR/POPCNT semantic intent router (`register_route`, `route`).
* **`std/schema.bc`** — Data contracts and strict parsing:
  * `SchemaValidator` — Non-throwing field validator (`check_range`, `check_required`, `check_density256`).
  * `parse_int_ascii(buf, len, &out)` — Strict signed integer parser replacing unsafe C `atoi`.
* **`std/context.bc`** — Agent memory management:
  * `ContextWindow<MaxTurns>` — Token-budgeted history (`push`, `recall_best`) that pins `role_system()` prompts against eviction.
* **`std/tools.bc`** — Capability-based actuator security:
  * `ToolRegistry<MaxTools>` — Permission-checked tool registry (`perm_read_only()`, `perm_actuator()`, `perm_network()`).
* **`std/reasoning.bc`** — Self-consistency consensus:
  * `vote_majority_int(candidates, count, &winner)` — Strict majority voting across candidate runs.
  * `bundle_bvec256(vectors, count)` — Bit-wise majority bundling of noisy hypervectors.
