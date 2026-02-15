# Optimization Notes

## Current Performance

Saprobe is faster than CoreAudio (CGO, in-process) on long files, measuring 0.80-0.97x.
On short files (<30s), saprobe is faster than ffmpeg (0.1-0.3x) due to zero process-spawn overhead, but slower than CGO CoreAudio (1.3-2.0x).
Against alacconvert (Apple reference C, out-of-process), saprobe is at parity or faster on dense material (0.96-1.11x).

FFmpeg is ~3x faster on long files due to SIMD-optimized C.

Memory: the decoder allocates zero significant memory during steady-state decoding. All allocations in benchmarks come from test infrastructure and one-time initialization.

See [QA.md](./QA.md) for full benchmark data and CPU profiles.

## Profile Summary

Two distinct regimes depending on input:

| Hotspot | Synthetic (white noise) | Real Music | Function |
|---|---|---|---|
| Linear predictor | -- | **36-42%** | `unpcBlock6` (dominant), `unpcBlock5` (5-6%), `unpcBlock4` (1-5%) |
| Entropy decode | -- | **28-32%** | `DynDecomp` (with `dynGet32Bit` manually inlined) |
| Sign adaptation | -- | **7-12%** | `signOfInt` |
| Bit reading | **50%** | ~3% | `BitBuffer.Read` |
| Output write | 6% | ~2% | `WriteStereo16/24` |

Real music is the optimization target. White noise is a pathological case that maximizes bit-reading overhead due to incompressible data.

## Predictor Order (`numActive`)

The Apple reference encoder (`ALACEncoder.cpp`) uses `kMinUV=4`, `kMaxUV=8`, step 4 -- it only tests predictor orders 4 and 8. FFmpeg defaults to orders 4-6.

The switch in `UnpcBlock` dispatches:
- `numActive=4` -> `unpcBlock4` (fully unrolled)
- `numActive=5` -> `unpcBlock5` (fully unrolled)
- `numActive=6` -> `unpcBlock6` (fully unrolled)
- `numActive=8` -> `unpcBlock8` (fully unrolled)
- everything else -> `unpcBlockGeneral` (loop-based)

In practice, `unpcBlock6` handles 70-85% of packets in real music (ffmpeg-encoded files dominate this order). `unpcBlock4` and `unpcBlock8` cover Apple-encoded files. The generic path handles only residual order-5 and order-7+ packets.

## Completed Optimizations

### Specialized Predictors (unpcBlock4/5/6/8)

Added hand-unrolled predictor paths for orders 4, 5, 6, and 8. `unpcBlock6` alone moved 70-85% of packets off the generic path, dropping `unpcBlockGeneral` from 51-57% to negligible.

### Manual Inlining of `dynGet32Bit`

`dynGet32Bit` exceeded the Go compiler's inline cost budget (177 vs 80). It was called once per sample in `DynDecomp`. Manually inlined the function body, eliminating per-sample function call overhead.

### Bounds Check Elimination (BCE)

Eliminated bounds checks from all hot inner loops using safe Go patterns:

**Predictor (`unpcBlock4/5/6/8`)**: Window sub-slicing with constant `lim` per function. Instead of `out[idx-4]`, use `w := out[idx-lim:idx:idx]` then `w[lim-4]`. The compiler proves constant indices are within the sub-slice bounds. Reduces 7-11 checks per sample to 1 sub-slice check per iteration.

**BitBuffer.Read/ReadSmall**: Sub-slicing with constant length. `w := b.Buf[b.Pos:b.Pos+3:b.Pos+3]` then index `w[0]`, `w[1]`, `w[2]`. Reduces 3 element checks to 1 sub-slice check.

**DynDecomp zero-run**: Replaced per-element loop with `clear(predCoefs[count:end])`. Single slice check instead of N element checks.

**Matrix output**: Resliced `mixU`/`mixV` at function entry, used `range mixU` for iteration. Offset variable pattern for strided output writes.

Total BCE checks reduced from 188 to 139 across the hot path.

### Go BCE Patterns Learned

**What works:**
- Window sub-slicing: `w := out[idx-const:idx:idx]` with constant offset
- Sub-slicing for multi-byte loads: `w := buf[pos:pos+N:pos+N]`
- Range-over-slice: `for idx := range slice { slice[idx] }` is BCE-free
- `clear()` for bulk zero-fill: 1 check instead of N

**What does NOT work:**
- Bounds hints: `_ = buf[pos+2]` does NOT prove `buf[pos]` is in-bounds
- Element access hints for slice creation: proving element access doesn't transfer to slice creation

## Investigation Findings

### `signOfInt` -- Already Optimal

Generates fully branchless ARM64:
```
NEG R0, R1
MOVW R1, R1
SBFX $31, R0, $1, R2
ORR R1>>31, R2, R0
```

Inlined at every call site (cost 16, budget 80). No improvement possible.

### Inlining

**Inlined** (good): `signOfInt`, `read32bit`, `lead`, `lg3a`, `BitBuffer.Read`, `BitBuffer.ReadSmall`, `BitBuffer.Advance`.

**Not inlined** (exceeds cost budget of 80):
- `dynGet` -- cost 119. Called per zero-run in `DynDecomp`.
- `getStreamBits` -- cost 101. Called from inlined `dynGet32Bit` on escape codes.
- `DynDecomp` -- cost 437.

### SIMD Applicability

Primordium has `DotFloat32` (ARM64 NEON + AMD64 SSE assembly) and `MatVecMul64x32`. Neither operates on `int16 x int32` which is what the ALAC predictor needs.

With specialized predictors covering orders 4-6 and 8, the dot product is only 4-8 multiply-adds -- too short for SIMD vector lanes to amortize setup cost. The hand-unrolled scalar specializations are already close to what SIMD would achieve for these lengths.

SIMD would only help `unpcBlockGeneral` for hypothetical large predictor orders (>8), which no known encoder produces.

## Not Worth Pursuing

- **`signOfInt` optimization** -- already branchless, already inlined.
- **`BitBuffer.Read` optimization** -- only dominates on incompressible synthetic data, not real music.
- **SIMD for predictor dot product** -- vector lengths (4-8) are too short to benefit from SIMD lanes. The unrolled scalar specializations are already near-optimal for these sizes.
- **Entropy decoding parallelism** -- Golomb-Rice is inherently serial (each codeword's length depends on its value, which determines the next codeword's start position).
- **`unsafe` for BCE** -- The remaining bounds checks (1 per BitBuffer call, 1 per output sub-slice) account for <1% of decode time. The safe patterns already eliminated the hot inner-loop checks.
