# Testing ALAC

## Prerequisites

Build optional reference tools (macOS only):

```bash
make alac-coreaudio   # CoreAudio ALAC encoder/decoder → tests/bin/alac-coreaudio
make alacconvert      # Apple reference ALAC converter → tests/bin/alacconvert
```

## Conformance Testing

Round-trip encode/decode tests across all supported configurations. Both synthetic and natural files use the same unified verification logic (`verifyConformance`).

### Synthetic (`TestConformance`)

Tests all bit depth × encoder × sample rate × channel combinations using generated white noise. Subtests are named `{bitDepth}bit/{encoder}/{sampleRate}Hz_{channels}ch`.

```bash
# Full suite (all tools)
make test

# Conformance only
go test ./tests/ -run TestConformance -count=1 -v

# Single subtest
go test ./tests/ -run TestConformance/16bit/ffmpeg/44100Hz_2ch -count=1 -v
```

**Test matrix:**

- **Bit depths:** 16, 24
- **Sample rates:** 8000, 11025, 16000, 22050, 32000, 44100, 48000, 88200, 96000, 176400, 192000
- **Channels:** 1-8

With all tools available: 11 sample rates × (16b: 8ch ffmpeg + 7ch coreaudio + 2ch alacconvert) + (24b: 8ch ffmpeg + 5ch coreaudio + 2ch alacconvert) = 352 subtests.
With ffmpeg only: 2 × 11 × 8 = 176 subtests.

### Natural Files (`TestConformanceNatural`)

Tests real ALAC files from a directory. Set `CONFORMANCE_NATURAL_DIR` to run.

```bash
CONFORMANCE_NATURAL_DIR=/path/to/m4a/files go test ./tests/ -run TestConformanceNatural -count=1 -v
```

Discovers all `.m4a` files in the directory, probes format via saprobe, and runs unified verification. No source comparison (original uncompressed audio unavailable).

### Encoder/Decoder Compatibility

| Encoder     | Container | Decoders                           | Channels (16-bit) | Channels (24-bit) |
|-------------|-----------|------------------------------------|--------------------|--------------------|
| ffmpeg      | M4A       | saprobe, ffmpeg, coreaudio         | 1-8                | 1-8                |
| coreaudio   | M4A       | saprobe, ffmpeg, coreaudio         | 1-7                | 1-5                |
| alacconvert | CAF       | ffmpeg, alacconvert                | 1-2                | 1-2                |
| natural     | M4A       | saprobe, ffmpeg, coreaudio         | (as probed)        | (as probed)        |

CoreAudio's ALAC codec silently returns 0 frames (OSStatus 0) without error for channel counts beyond these limits.

### Verification

**Unified verification (all tests):**

1. Decode with all compatible decoders
2. Verify format metadata (saprobe decoder)
3. If source PCM provided: bit-for-bit comparison against source
4. Cross-decoder comparison: bit-for-bit between all decoder pairs

**Channel ordering and bit-for-bit comparison:**

ALAC bitstreams use MPEG element order (e.g., 5.1: C, L, R, Ls, Rs, LFE). Saprobe and ffmpeg both remap to SMPTE standard order (L, R, C, LFE, Ls, Rs) during decoding.

- **ffmpeg encoder + saprobe/ffmpeg decoders:** Full bit-for-bit comparison (all channels).
- **CoreAudio encoder or decoder:** Skipped for multichannel (3-8ch). CoreAudio follows Apple's reference implementation which uses MPEG element order without remapping. Length verification still performed.

### Reference Tools

| Tool        | Role            | Bit Depths     | Channels         | Container | Channel Order |
|-------------|-----------------|----------------|------------------|-----------|---------------|
| ffmpeg      | Encode + Decode | 16, 24         | 1-8              | M4A       | SMPTE         |
| alacconvert | Encode + Decode | 16, 24         | 1-2              | CAF       | SMPTE         |
| coreaudio   | Encode + Decode | 16, 20, 24, 32 | 1-7 (16b), 1-5 (24b) | M4A  | MPEG element  |

CoreAudio and alacconvert are optional; tests skip gracefully when unavailable.

## Example Decoder (`TestExampleDecoder`)

Integration test for the `alac-example-decoder` binary (`cmd/alac-example-decoder`).

```bash
go test ./tests/ -run TestExampleDecoder -count=1 -v
```

Builds the binary from source, generates 1 second of synthetic 16-bit/44.1kHz stereo PCM, encodes to M4A via CoreAudio, then decodes with both CoreAudio (reference) and the example binary.

Verifies:

- PCM output mode (`-format pcm`): byte-for-byte match against CoreAudio reference.
- WAV output mode (`-format wav`): correct size (44-byte header + PCM data) and PCM content match.

Requires `alac-coreaudio`; skips otherwise.

## Benchmarks

### Synthetic (`TestBenchmarkDecode`)

Benchmarks decoding across all available decoders using synthetic white noise encoded via CoreAudio. Requires `alac-coreaudio`; skips otherwise. Skipped in `-short` mode.

```bash
go test ./tests/ -run TestBenchmarkDecode -count=1 -v
```

- **Formats:** CD (44.1kHz/16bit stereo), HiRes (96kHz/24bit stereo)
- **Durations:** 10 seconds and 5 minutes per format
- **Decoders:** saprobe, ffmpeg, coreaudio, alacconvert (last two if available)
- **Iterations:** 10 per configuration
- **Statistics:** median, mean, stddev, min, max

| Format | saprobe | ffmpeg | coreaudio | alacconvert |
|--------|---------|--------|-----------|-------------|
| CD 16bit 10s | 4ms | 34ms | 2ms | 9ms |
| HiRes 24bit 10s | 12ms | 36ms | 9ms | 23ms |
| CD 16bit 300s | 114ms | 89ms | 60ms | 182ms |
| HiRes 24bit 300s | 346ms | 152ms | 237ms | 622ms |

On short files (<30s), saprobe is faster than ffmpeg due to zero process-spawn overhead.
On longer files, ffmpeg pulls ahead due to SIMD-optimized C.

#### CPU Profile (`TestProfileDecode`)

Profiling uses `TestProfileDecode` (saprobe-only) to avoid polluting pprof with CGO/ffmpeg/alacconvert noise. Comparative timing uses `TestBenchmarkDecode` (all decoders).

```bash
hack/bench.sh TestProfileDecode
```

Total: 9.07s, 4.68s in sampled functions (51.61%).

| Function         | Flat  | Flat%  | Cum   | Cum%   |
|------------------|-------|--------|-------|--------|
| BitBuffer.Read   | 2.33s | 49.79% | 2.41s | 51.50% |
| decodeCPEEscape  | 1.13s | 24.15% | 3.59s | 76.71% |
| WriteStereo24    | 0.30s | 6.41%  | 0.30s | 6.41%  |
| WriteStereo16    | 0.08s | 1.71%  | 0.09s | 1.92%  |

Bit reading dominates at 50%, followed by the CPE escape decoder (24% flat, 77% cumulative). The synthetic profile is dominated by BitBuffer.Read because white noise maximizes entropy, exercising the bit reader more than the predictor. Specialized predictors (`unpcBlock4/5/6/8`) do not appear in the top functions because entropy decoding overwhelms the predictor on random noise.

#### Memory

Decoder allocations are zero. The saprobe profile uses a streaming `Read` loop with a pre-allocated buffer, producing no allocations. All significant allocations come from test setup (encoding source data):

| Source             | Alloc      | Alloc% |
|--------------------|------------|--------|
| os.ReadFile        | 223 MB     | 32.74% |
| GenerateWhiteNoise | 223 MB     | 32.72% |
| WriteWAV           | 223 MB     | 32.72% |
| NewDecoder   | 5.7 MB     | 0.83%  |

The `Decoder.Read` path itself allocates no memory. `inuse_space` at exit: 2.0 KB (runtime goroutine stacks only).

### Real Files (`TestBenchmarkDecodeFile`)

Benchmarks decoding natural M4A files. 10 iterations, same decoder set and statistics as synthetic benchmarks. For alacconvert, a CAF is prepared via intermediate WAV conversion. Skipped in `-short` mode.

```bash
BENCH_FILE='/path/to/file.m4a' go test ./tests/ -run TestBenchmarkDecodeFile -count=1 -v
```

Reference files selected for variety in duration and compressibility.
All 44.1kHz/16bit stereo.
Compression ratio = encoded size / raw PCM bitrate.
Decode times are median over 10 iterations.
Ratio columns show saprobe time relative to each reference tool (>1x = saprobe slower, <1x = saprobe faster).

| File | Duration | Size | Comp. | saprobe | ffmpeg | vs ffmpeg | coreaudio | vs coreaudio | alacconvert | vs alacconvert | Character |
|------|----------|------|-------|---------|--------|-----------|-----------|--------------|-------------|----------------|-----------|
| Cecil Taylor Unit — [untitled] | 1:09:50 | 341.7 MB | 0.485 | 11.37s | 3.78s | 3.0x | 11.72s | 0.97x | 10.23s | 1.11x | Long free jazz, moderate density |
| John Coltrane — Ascension Ed. II | 40:57 | 287.7 MB | 0.696 | 5.61s | 1.87s | 3.0x | 7.04s | 0.80x | 5.87s | 0.96x | Dense ensemble, near-incompressible |

On longer files, ffmpeg pulls ahead (3.0x faster) due to SIMD-optimized C.
Saprobe is faster than CoreAudio (CGO, in-process) on long files, measuring 0.80-0.97x. Against alacconvert (Apple reference C), saprobe is at parity or faster on dense material (0.96-1.11x). Specialized predictors for orders 4, 5, 6, and 8 cover ~95% of packets in real music files, leaving only residual orders for the generic path.

Full paths:

```
/Volumes/Anisotope/gill/jazz.lossless/Cecil Taylor Unit, The/1988 - Live in Bologna/[1988-CD-Leo Records-CD LR 100]/01-01 - [untitled].m4a
/Volumes/Anisotope/gill/jazz.lossless/Coltrane, John/1965 - Ascension/[2009-CD-impulse!-B0012402-02-602517920248]/01-02 - Ascension_ Edition II.m4a
```

#### CPU Profile (Real Files, `TestProfileDecodeFile`)

```bash
hack/bench.sh TestProfileDecodeFile '/path/to/file.m4a'
```

Profiled against two contrasting tracks: Cecil Taylor Unit — [untitled] (1:09:50, moderate density, compression ratio 0.485) and John Coltrane — Ascension Ed. II (40:57, dense ensemble, compression ratio 0.696).

**Cecil Taylor Unit — [untitled] (1:09:50, 341.7 MB)**

Total: 134.62s, 100.87s in sampled functions (74.93%).

| Function         | Flat    | Flat%  | Cum     | Cum%   |
|------------------|---------|--------|---------|--------|
| unpcBlock6       | 42.80s  | 42.43% | 56.39s  | 55.90% |
| DynDecomp        | 28.14s  | 27.90% | 32.18s  | 31.90% |
| signOfInt        | 12.55s  | 12.44% | 13.11s  | 13.00% |
| unpcBlock5       | 4.43s   | 4.39%  | 5.90s   | 5.85%  |
| read32bit        | 2.88s   | 2.86%  | 3.06s   | 3.03%  |
| WriteStereo16    | 1.63s   | 1.62%  | 1.64s   | 1.63%  |
| unpcBlock4       | 1.11s   | 1.10%  | 1.44s   | 1.43%  |

**John Coltrane — Ascension Ed. II (40:57, 287.7 MB)**

Total: 75.44s, 49.59s in sampled functions (65.73%).

| Function         | Flat    | Flat%  | Cum     | Cum%   |
|------------------|---------|--------|---------|--------|
| unpcBlock6       | 17.66s  | 35.61% | 21.20s  | 42.75% |
| DynDecomp        | 15.62s  | 31.50% | 18.11s  | 36.52% |
| signOfInt        | 3.64s   | 7.34%  | 3.86s   | 7.78%  |
| unpcBlock5       | 2.86s   | 5.77%  | 3.54s   | 7.14%  |
| unpcBlock4       | 2.32s   | 4.68%  | 3.07s   | 6.19%  |
| read32bit        | 1.77s   | 3.57%  | 1.90s   | 3.83%  |
| WriteStereo16    | 1.24s   | 2.50%  | 1.26s   | 2.54%  |

The `unpcBlock6` specialization handles the majority of packets in real music and dominates at 36-42% flat. `DynDecomp` (with `dynGet32Bit` manually inlined) shows 28-32% flat. The generic predictor path `unpcBlockGeneral` does not appear in the top functions — specialized predictors for orders 4, 5, 6, and 8 handle effectively all packets.

Compared to the synthetic profile (white noise) where `BitBuffer.Read` dominates at 50%, real music shifts the bottleneck to the linear predictor — random noise maximizes entropy and bit-reading overhead, while structured audio exercises the predictor reconstruction loop more heavily.

#### Memory (Real Files)

Decoder allocations remain zero during steady-state decoding. The only allocations come from one-time setup and test infrastructure:

| Source | Cecil Taylor (341.7 MB) | Coltrane (287.7 MB) |
|--------|------------------------|---------------------|
| os.ReadFile (test infra) | 341.7 MB (94.02%) | 287.7 MB (93.74%) |
| buildSampleTable (init) | 8.3 MB (2.29%) | 5.8 MB (1.90%) |
| Decoder.Read | 2.5 MB (0.69%) | 3.5 MB (1.15%) |
| BitBuffer.Reset (init) | — | 2.0 MB (0.66%) |
| readStsz (init) | 4.8 MB (1.31%) | 1.7 MB (0.54%) |

The `Decoder.Read` allocation is from `BitBuffer.Reset` growing its backing buffer on the first packets and from `packetBuf` growth in the `Read` loop. These stabilize after the first few packets. `inuse_space` at exit: 1.5-2.0 KB (runtime goroutine stacks only).

## Mass testing

Comparative decoding is being done in Saprobe on a set of 8409 MP4A/ALAC files.

No failure or discrepancy has been found.
