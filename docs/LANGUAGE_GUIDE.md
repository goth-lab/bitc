# BIT-C Language Guide (`LANGUAGE_GUIDE.md`)

> [Читать на русском](ru/LANGUAGE_GUIDE.md)

## 1. Why BIT-C? (The "AI Lego" Philosophy)

Modern AI stacks have grown into heavy monoliths requiring gigabytes of dependencies, floating-point units (`float`), and expensive GPUs. **BIT-C** restores speed, determinism, and architectural clarity:

* **Zero `float`, Zero GPU:** Every computation uses integer types (`int`, `i32`, `i16`, `i8`, `u8`), `Q16.16` fixed-point math, and wide bit-vectors `bvec<N>`.
* **Two-Tier Modular Architecture:**
  * *Systems Tier:* Direct control over static memory arenas, ring buffers, and SIMD alignment for building reusable primitives.
  * *Application Tier:* Assemble smart cameras, audio triggers, and safe autonomous agents from `std/` building blocks in 20–30 lines of readable code.
* **Native C Compilation:** Emits 32-byte aligned C99/C11 code compiled via `clang -O3` with hardware `POPCNT` / NEON / AVX instructions.

---

## 2. Primitive Types and Static Arrays

Variables are explicitly typed using `name: Type = value;`:

```bitc
count: int = 42;          // 64-bit signed integer (int64_t)
sample: i16 = -1200;      // 16-bit audio PCM sample (int16_t)
weight: i8 = 127;         // Quantized neural weight (int8_t)
pixel: u8 = 255;          // Unsigned byte / pixel (uint8_t)
flag: bit = 1;            // Single bit (0 or 1)
buf: u8[64];              // Stack-allocated static array (no malloc)
```

---

## 3. Hypervectors `bvec<N>`, `match`, and `count`

In **Hyperdimensional Computing (HDC)**, concepts and sensor frames are encoded into wide binary vectors `bvec<N>` (from `bvec<4>` to `bvec<2048>+`):

* **Bit-Level Indexing:** Read or mutate individual bits directly (`v[idx] = 1; b: bit = v[idx];`) for any bit-width `N`.
* **`match(a, b)` (Bitwise XNOR):** Outputs `1` at every bit position where `a` and `b` agree, and `0` where they differ.
* **`count(v)` (Hardware POPCNT):** Counts the number of set `1`-bits in a single CPU instruction.

```bitc
sim_bits: int = count(match(sensor_hv, reference_hv));
if sim_bits > 200 {
    // Matched more than 200 out of 256 bits!
}
```

## 4. Structs, Receiver Methods, and Generics

BIT-C supports Go-style pointer receiver methods and compile-time template monomorphization with zero runtime overhead:

```bitc
struct Counter<MaxVal> {
    val: int;
}

fn (self: *Counter<MaxVal>) inc() -> int {
    if self.val < MaxVal {
        self.val = self.val + 1;
    }
    return self.val;
}
```

---

## 5. Bounded Loops and OpenMP Parallelism

Range loops use half-open intervals (`0..10` iterates `0` through `9`):

```bitc
for i in 0..64 step 4 {
    acc = acc + buf[i];
}

// Multi-core execution via OpenMP
parallel for row in 0..256 {
    process_row(row);
}
```

---

## 6. Built-In Unit Testing (`test` and `bitc test`)

Unit tests live natively in `.bc` files and are stripped completely from production builds:

```bitc
import "../std/test.bc";

test "bitwise similarity sanity check" {
    a: bvec<64> = 0xFF;
    b: bvec<64> = 0xFF;
    assert_eq(count(match(a, b)), 64, 1);
}
```

Run a single test file or an entire test directory:
```bash
./bitc test tests/
```
