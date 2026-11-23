# REFERENCE.md: BIT-C Language Reference (In Plain English)

> [Читать на русском](ru/REFERENCE.md)

If you have written **Go**, **C**, or **TypeScript**, **BIT-C** will feel immediately familiar. Everything compiles into flat C99/C11 structs and single-cycle SIMD instructions.

---

## 1. Data Types

* **Integer Types (Zero `float` Policy):**
  * `int` — 64-bit signed integer (`int64_t`) for counters, indices, and similarity scores.
  * `i32`, `i16`, `i8`, `u8` — compact signed/unsigned integers (32, 16, and 8 bits) for PCM audio, pixels, and quantized tensors.
  * `bit` — single bit (`0` or `1`), returned by comparison operators (`>`, `==`).
* **Bit-Vector Registers (`bvec<N>`):**
  * Core language primitive of arbitrary width (`bvec<4>`, `bvec<64>`, `bvec<256>`, `bvec<1024>+`), 32-byte SIMD-aligned in C.
  ```bitc
  mask: bvec<64> = 0xFF00FF00AA55AA55;
  flag: bit = mask[0]; // Read bit 0
  mask[3] = 1;         // Write 1 to bit 3 (works for any width N)
  ```
* **Static Arrays and Pointers:**
  * `bvec<64>[10]` — stack-allocated fixed array (no `malloc`).
  * `*T` — typed pointer (`&x` address-of, `*p` dereference, `ptr[i]` element indexing).

---

## 2. Structs, Methods, and Nested Generics

Data and behavior are bound via Go-style receiver methods `fn (self: *Type)` with full compile-time monomorphization (including nested templates like `Channel<bvec<64>, 2>`):

```bitc
struct Layer<InBits, OutBits> {
    weights: bvec<InBits>[OutBits];
    threshold: int;
}

fn (l: *Layer<InBits, OutBits>) predict(input: bvec<InBits>) -> bvec<OutBits> {
    out: bvec<OutBits> = 0;
    for i in 0..OutBits {
        out[i] = count(match(input, l.weights[i])) > l.threshold;
    }
    return out;
}
```

---

## 3. Atomic Operations, Control Flow, and Built-In Tests

* **AI Intrinsics:** `match(a, b)` (bitwise XNOR) and `count(v)` (hardware POPCNT).
* **Bitwise Operators:** `&`, `|`, `^`, `~`, `<<`, `>>` across any width `N`.
* **Modules & FFI:** `import "../std/learn.bc";` and `extern fn now_ns() -> int;`.
* **Loops & OpenMP:** `for i in 0..100 { ... }`, `for x in 0..640 step 4 { ... }`, `parallel for n in 0..1024 { ... }`.
* **Built-In Unit Tests:** `test "description" { ... }` (compiled only via `./bitc test`, zero overhead in production builds).
