# Optimization Notes

## Current Performance

Saprobe is ~1.2x vs CoreAudio and ~1.3-1.4x vs alacconvert (Apple's reference C) on real files. FFmpeg is 3.7-4.5x faster due to SIMD-optimized C. On short files (<30s), saprobe is competitive or faster than all external tools due to zero process-spawn overhead.

Memory: the decoder allocates zero significant memory. All allocations in benchmarks come from test infrastructure.

See [saprobe-flac](https://github.com/mycophonic/saprobe-flac) for extra testing and bench tooling.

## Profile Summary

Two distinct regimes depending on input:

| Hotspot | Synthetic (white noise) | Real Music | Function |
|---|---|---|---|
| Linear predictor | -- | **51-57%** | `unpcBlockGeneral` |
| Entropy decode | -- | **23-26%** | `DynDecomp` + `dynGet32Bit` |
| Bit reading | **25%** | ~1.5% | `BitBuffer.Read` |
| Output write | 6% | ~1.5% | `WriteStereo16/24` |

Real music is the optimization target. White noise is a pathological case that maximizes bit-reading overhead due to incompressible data.

**Important: the real-file profiles are stale.** They were collected before `unpcBlock8` was added. At the time, `numActive=8` traffic hit `unpcBlockGeneral` (the general-purpose path). Now it dispatches to the specialized `unpcBlock8`. Re-profiling is needed before any optimization work.

## Predictor Order (`numActive`)

The Apple reference encoder (`ALACEncoder.cpp`) uses `kMinUV=4`, `kMaxUV=8`, step 4 -- it only tests predictor orders 4 and 8. FFmpeg defaults to orders 4-6.

The switch in `UnpcBlock` dispatches:
- `numActive=4` -> `unpcBlock4` (fully unrolled, 249 ARM64 instructions)
- `numActive=8` -> `unpcBlock8` (fully unrolled, 501 ARM64 instructions)
- everything else -> `unpcBlockGeneral` (loop-based, 181 instructions)

Apple-encoded files (the vast majority of real ALAC) will only ever hit `unpcBlock4` and `unpcBlock8`.

## Investigation Findings

### 1. Bounds Check Elimination (BCE) Failures

The Go compiler fails to eliminate bounds checks in every hot loop. Verified with `go build -gcflags='-d=ssa/check_bce/debug=1'`.

**`unpcBlock4`** -- 7 bounds checks per sample in the main loop:
- `predictor.go:87` -- `out[idx-lim]` (top)
- `predictor.go:89-92` -- `out[idx-1]` through `out[idx-4]` (history reads)
- `predictor.go:96` -- `pc1[idx]`
- `predictor.go:101` -- `out[idx]` (store)

Plus 4 one-time checks at function start for `coefs[0..3]`.

**`unpcBlock8`** -- same pattern but 8 history reads, so 11 bounds checks per sample.

**`unpcBlockGeneral`** -- per-sample checks at lines 349 (hist slice), 351 (hist[0]), 360 (pc1[idx]), 364 (out[idx]). The inner dot product loop at line 357 has a bounds check **per coefficient per sample**. The adaptation loops at 369/380 add more.

**`DynDecomp`** (golomb.go) -- bounds checks at line 200 (`predCoefs[count]`), line 226 (`predCoefs[count]` in zero-run loop), and in every `read32bit` call (lines 66/99/135).

Each bounds check is a CMP+branch pair per sample. With 7-11 checks x ~4096 samples/packet x thousands of packets per file, this is likely **3-8% of total decode time**.

### 2. `signOfInt` -- Already Optimal

Generates fully branchless ARM64:
```
NEG R0, R1
MOVW R1, R1
SBFX $31, R0, $1, R2
ORR R1>>31, R2, R0
```

Inlined at every call site (cost 16, budget 80). No improvement possible.

### 3. Inlining

**Inlined** (good): `signOfInt`, `read32bit`, `lead`, `lg3a`, `BitBuffer.Read`, `BitBuffer.ReadSmall`, `BitBuffer.Advance`.

**Not inlined** (exceeds cost budget of 80):
- `dynGet32Bit` -- cost 177. Called per sample in `DynDecomp`. Full function call overhead (register save/restore) on the 12-13% entropy decode path.
- `dynGet` -- cost 119. Called per zero-run in `DynDecomp`.
- `getStreamBits` -- cost 101. Called from `dynGet32Bit` on escape codes.
- `DynDecomp` -- cost 437.

### 4. SIMD Applicability

Primordium has `DotFloat32` (ARM64 NEON + AMD64 SSE assembly) and `MatVecMul64x32`. Neither operates on `int16 x int32` which is what the ALAC predictor needs.

However, since `numActive` is always 4 or 8 for Apple-encoded files, the predictor dot product is only 4 or 8 multiply-adds -- too short for SIMD vector lanes to amortize setup cost. The specialized `unpcBlock4`/`unpcBlock8` with full register unrolling are already close to what SIMD would achieve for these lengths.

SIMD would only help `unpcBlockGeneral` for hypothetical large predictor orders (>8), which no known encoder produces.

## Prioritized Actions

### P0: Re-profile

The existing profile data is stale (collected before `unpcBlock8`). Re-run with real files to establish the current bottleneck distribution before any optimization work.

```bash
hack/bench.sh TestBenchmarkDecodeFile '/path/to/file.m4a'
```

### P1: Eliminate Bounds Checks in Predictor

Target: `unpcBlock4` and `unpcBlock8` (the only paths hit by real files).

Strategy: add upfront length assertions before the main loop so the compiler can prove all indexed accesses are in-bounds:

```go
// Example for unpcBlock4:
// The loop runs idx in [lim, num). lim=5.
// Accesses: out[idx-5] through out[idx], pc1[idx].
// Worst case idx=num-1, so out[num-1] and pc1[num-1] must be valid.
_ = out[num-1]
_ = pc1[num-1]
```

For history reads (`out[idx-1]` through `out[idx-4]`), the loop starts at `idx=lim=5`, so `idx-4 >= 1` is always true -- but the compiler can't prove this from the loop bounds alone. A single `_ = out[num-1]` assertion combined with the loop condition `idx < num` should allow the compiler to eliminate forward-indexed checks. The backward-indexed checks (`idx-1` through `idx-4/8`) may need restructured indexing.

Expected gain: **3-8% overall decode time**.

### P2: Inline `dynGet32Bit` into `DynDecomp`

The function exceeds the inline cost budget (177 vs 80). It's called once per sample in the entropy decode loop. Manual inlining eliminates per-sample function call overhead.

Options:
1. Copy the function body directly into `DynDecomp` at the call site.
2. Restructure to reduce cost below 80 (split into smaller helpers).

Expected gain: **1-3% overall** (eliminates call overhead on the 12-13% entropy path).

### P3: Eliminate Bounds Checks in `DynDecomp`

`predCoefs[count]` at golomb.go:200 and 226 has per-sample bounds checks. An upfront `_ = predCoefs[numSamples-1]` assertion would eliminate them.

`read32bit` calls at lines 66/99/135 each trigger a bounds check on the byte slice. These are harder to eliminate since the bit position advances unpredictably.

Expected gain: **1-2% overall**.

### P4: WriteStereo16/24 (Low Priority)

At 1.5% of real-file decode time, even a 2x speedup saves <1%. The strided write pattern with per-sample bounds checks could benefit from upfront assertions, but the return is marginal.

SIMD packing (ARM64 `ST2` for channel interleaving) would be clean but the gain doesn't justify the complexity at current profile weight.

### Not Worth Pursuing

- **`signOfInt` optimization** -- already branchless, already inlined.
- **`BitBuffer.Read` optimization** -- only dominates on incompressible synthetic data, not real music.
- **SIMD for predictor dot product** -- vector lengths (4 or 8) are too short to benefit from SIMD lanes. The unrolled scalar specializations are already near-optimal for these sizes.
- **Entropy decoding parallelism** -- Golomb-Rice is inherently serial (each codeword's length depends on its value, which determines the next codeword's start position).
