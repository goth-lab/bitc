# CONCEPTS.md: How AI Works in BIT-C (In Plain English)

> [Читать на русском](ru/CONCEPTS.md)

Traditional machine learning relies heavily on floating-point numbers (`float32`). That is slow, power-hungry, and demands expensive GPUs. **BIT-C** returns to the native language of the CPU: **everything is a zero or a one**.

Here is a plain-English breakdown of the core concepts used throughout BIT-C:

---

### 1. Neuron Weights = A Simple Stencil
* **Academic term:** *"Synaptic weights of a neural network layer."*
* **What it really is:** A plain bitmask (`bvec<64>`), such as `0b10110010...`. Imagine a paper stencil with punched holes: when laid over an incoming signal, if the holes line up, the neuron "recognizes" the pattern.

### 2. `match` & `count` Intrinsics = 1-Cycle Stencil Comparison
* **Academic term:** *"Hamming similarity computation via bitwise XNOR and POPCNT."*
* **What it really is:** Modern CPUs execute two hardware instructions in a single clock cycle:
  * `match(a, b)` — overlays two bit-vectors and outputs `1` wherever bits **agree** (both `0` or both `1`).
  * `count(v)` — instantly counts the total number of `1`-bits. The higher the count, the closer the input matches stored memory.

### 3. Gradient-Free Learning (`nudge`) = "Nudge Bits Toward the Target"
* **Academic term:** *"Stochastic error-driven bitwise weight updates."*
* **What it really is:** Instead of calculus and backpropagation, we generate a sparse random mask (`rand_mask`) allowing only ~10–15% of bits to change:
  * If the neuron **should have fired**, we copy bits from the training input at those masked positions (pulling the stencil closer to the example).
  * If the neuron **fired by mistake**, we invert bits at those positions (pushing the stencil away from the false pattern).
  After 50–100 gentle nudges, the stencil converges into a generalized prototype.

### 4. Thermometer Code (`thermometer64`) = A Ruler of Zeros and Ones
* **Academic term:** *"Unary thermometer quantization of scalar values."*
* **What it really is:** Standard binary numbers behave poorly in neural matching: `7` (`0111`) and `8` (`1000`) differ in all 4 bits despite being adjacent! Encoding numbers like mercury rising in a thermometer (`3` -> `00000111`, `4` -> `00001111`) preserves locality so 15°C and 17°C share almost identical bitmasks.

### 5. Orthogonal Hypervectors = Unique Fingerprints for Concepts
* **Academic term:** *"Quasi-orthogonal vectors in high-dimensional binary space."*
* **What it really is:** Encoding "Cat" as `1` and "Dog" as `2` makes their bits overlap into noise. Instead, each symbol receives a random 64-bit or 256-bit fingerprint where ones and zeros are uniformly scattered. Any two random hypervectors are guaranteed to be nearly orthogonal and can be safely bound together with `XOR` (`^`).
