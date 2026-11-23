# BIT-C — Specialized Compiled Language for Edge AI & Bitwise Computing

> **Language / Язык:** English | [Русская документация](docs/ru/LANGUAGE_GUIDE.md)

**BIT-C** is a lightweight compiled programming language (transpiling to SIMD-aligned C99/C11 with OpenMP) designed as a fast, energy-efficient alternative to heavy AI frameworks, GPU compute, and floating-point arithmetic (`float`).

## Core Principles

* **100% Integer & Bitwise (No Float, No GPU):** All math relies strictly on integer types (`int`, `i32`, `i16`, `i8`, `u8`), `Q16.16` fixed-point arithmetic, and arbitrary-width bit-vectors `bvec<N>` (`bvec<4>` up to `bvec<2048>+`).
* **Deterministic Memory (No `malloc`, No GC):** Static arenas (`ByteArena`), lock-free style ring buffers (`RingBuffer`), and zero-copy views (`TensorView`, `GrayImageView`) run reliably on both servers and bare-metal microcontrollers.
* **Built-In Testing & IDE Diagnostics:** Native `test "name" { ... }` blocks, batch suite execution (`./bitc test tests/`), and instant editor diagnostics (`./bitc check <file.bc>`).

## Performance Benchmarks (`examples/07_benchmark_hdc.bc`)

Measured on a single Apple Silicon M1 CPU core (`clang -O3 -mcpu=native`, zero floats, zero GPU):

| Benchmark Workload | Iterations | Total Time | Latency | Throughput |
| :--- | :---: | :---: | :---: | :---: |
| **256-bit XNOR + POPCNT** (`bvec<256>`) | 10,000,000 | 18 ms | **1.89 ns/op** | **526.70 M ops/sec** |
| **1024-bit XNOR + POPCNT** (`bvec<1024>`) | 10,000,000 | 47 ms | **4.75 ns/op** | **210.25 M ops/sec** |
| **2-Layer Binary Neural Net** (`256 -> 8 -> 4`) | 1,000,000 | 3 ms | **3.49 ns/op** | **286.45 M ops/sec** |

## Quick Start

```bash
# 1. Build the BIT-C compiler (requires Go and Clang)
go build -o bitc .

# 2. Run all 12 standard library unit test suites
./bitc test tests/

# 3. Run Edge AI, Audio, Agent, and Benchmark examples
./bitc examples/04_edge_vision_guard.bc
./bitc examples/05_edge_audio_wakeword.bc
./bitc examples/06_hybrid_robot_agent.bc
./bitc examples/07_benchmark_hdc.bc
```

## Repository Map

* `main.go`, `lexer.go`, `ast.go`, `parser.go`, `sema.go`, `codegen.go` — BIT-C compiler written in pure Go.
* `std/` — Standard library (16 integer-only, zero-allocation modules):
  * *Core:* `test.bc`, `sys.bc`, `mem.bc`, `tensor.bc`, `stream.bc`, `io.bc`
  * *Sensors & Inference:* `encode.bc`, `vision.bc`, `audio.bc`, `learn.bc`, `post.bc`
  * *Cognitive & Agents:* `flow.bc`, `schema.bc`, `context.bc`, `tools.bc`, `reasoning.bc`
* `tests/` — 12 unit test suites covering the entire standard library.
* `examples/` — 7 runnable showcases from dense memory to smart cameras, wake-word audio, autonomous robot agents, and benchmarks.
* `docs/` — Language guide & stdlib reference (English in `docs/`, Russian in `docs/ru/`).
* `specs/` — Architecture specifications (English in `specs/`, Russian in `specs/ru/`).

## License & Authors

Copyright (c) 2026 Vladimir Borisenko (Goth) & AI Gemini. Released under the **MIT License** (see `LICENSE`).
