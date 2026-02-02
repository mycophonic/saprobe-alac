# Testing ALAC

## Prerequisites

Build optional reference tools (macOS only):

```bash
make alac-coreaudio   # CoreAudio ALAC encoder/decoder → bin/alac-coreaudio
make alacconvert      # Apple reference ALAC converter → bin/alacconvert
```

## Conformance (`TestConformance`)

Round-trip encode/decode tests across all supported bit depth, sample rate, channel count, and encoder/decoder combinations. Subtests are named `{bitDepth}bit/{encoder}/{sampleRate}Hz_{channels}ch`.

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

Each encoder produces an encoded file which is decoded by all compatible decoders. Decoded output is compared against the original source PCM and cross-compared across decoders.

**Encoder/decoder compatibility:**

| Encoder     | Container | Decoders                           | Channels |
|-------------|-----------|------------------------------------|----------|
| ffmpeg      | M4A       | saprobe, ffmpeg, coreaudio         | 1-8      |
| coreaudio   | M4A       | saprobe, ffmpeg, coreaudio         | 1-8      |
| alacconvert | CAF       | ffmpeg, alacconvert                | 1-2      |

With all tools available: 2 bit depths x 11 sample rates x (8ch ffmpeg + 8ch coreaudio + 2ch alacconvert) = 396 subtests.
With ffmpeg only: 2 x 11 x 8 = 176 subtests.

**Verification:**

- Mono and stereo: bit-for-bit PCM match against source, plus cross-decoder comparison.
- Multichannel (3-8ch): output length verified only. Byte comparison is skipped because encoders (ffmpeg, CoreAudio) apply channel layout remapping that reorders channels relative to the raw interleaved input.

**Reference tools:**

| Tool        | Role            | Bit Depths     | Channels | Container |
|-------------|-----------------|----------------|----------|-----------|
| ffmpeg      | Encode + Decode | 16, 24         | 1-8      | M4A       |
| alacconvert | Encode + Decode | 16, 24         | 1-2      | CAF       |
| coreaudio   | Encode + Decode | 16, 20, 24, 32 | 1-8      | M4A       |

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

#### CPU Profile

```bash
make test-unit-profile
```

Total: 22.19s, 7.81s in sampled functions (35.20%).

| Function         | Flat  | Flat%  | Cum   | Cum%   |
|------------------|-------|--------|-------|--------|
| runtime.cgocall  | 2.31s | 29.58% | 2.31s | 29.58% |
| BitBuffer.Read   | 1.51s | 19.33% | 1.57s | 20.10% |
| decodeCPEEscape  | 1.03s | 13.19% | 2.65s | 33.93% |
| WriteStereo24    | 0.35s | 4.48%  | 0.35s | 4.48%  |
| WriteStereo16    | 0.05s | 0.64%  | 0.05s | 0.64%  |

The CGO CoreAudio decoder (`runtime.cgocall`) now appears at 29.58% since it runs in-process. Among saprobe-only functions, bit reading dominates at 19.33%, followed by the CPE element decoder (13.19% flat, 33.93% cumulative).

#### Memory

Decoder allocations are negligible. All significant allocations come from test infrastructure:

| Source             | Alloc      | Alloc% |
|--------------------|------------|--------|
| io.ReadAll         | 12,599 MB  | 80.07% |
| CGO GoBytes        | 2,448 MB   | 15.56% |
| generateWhiteNoise | 223 MB     | 1.41%  |
| WriteWAV           | 223 MB     | 1.41%  |
| os.ReadFile        | 223 MB     | 1.42%  |

The `alac.Decode` path itself allocates no significant memory. The CGO `GoBytes` allocation (15.56%) is the CoreAudio benchmark copying decoded PCM from C to Go heap. `inuse_space` at exit: 3 MB (runtime goroutine stacks only).

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
| Horace Silver — Song for My Father | 7:06 | 40.6 MB | 0.55 | 1.152s | 245ms | 4.7x | 947ms | 1.2x | 818ms | 1.4x | Typical jazz combo |
| Art Blakey — Ending With the Theme | 0:29 | 82 KB | 0.017 | 21ms | 32ms | 0.7x | 14ms | 1.5x | 21ms | 1.0x | Near-silence, extreme compressibility |
| Charlie Parker — Estrellita (take 5) | 0:06 | 0.3 MB | 0.254 | 10ms | 28ms | 0.4x | 8ms | 1.3x | 12ms | 0.8x | Short take, sparse |
| Cecil Taylor Unit — [untitled] | 1:09:50 | 341.7 MB | 0.485 | 11.03s | 2.24s | 4.9x | 9.07s | 1.2x | 8.04s | 1.4x | Long free jazz, moderate density |
| Cecil Taylor — Calling It the 8th | 58:10 | 352.6 MB | 0.601 | 9.01s | 2.03s | 4.4x | 7.28s | 1.2x | 6.54s | 1.4x | Dense free piano, low compressibility |
| John Coltrane — Ascension Ed. II | 40:57 | 287.7 MB | 0.696 | 5.70s | 1.12s | 5.1x | 4.64s | 1.2x | 4.60s | 1.2x | Dense ensemble, near-incompressible |

On short files (<30s), saprobe is faster than ffmpeg (0.4-0.7x) due to zero process-spawn overhead, but slower than CGO CoreAudio (1.3-1.5x).
On longer files, ffmpeg pulls ahead (4.4-5.1x faster) due to SIMD-optimized C.
Saprobe is consistently ~1.2x vs CoreAudio (CGO, in-process) and ~1.2-1.4x vs alacconvert.

Full paths:

```
/Volumes/Anisotope/gill/jazz.lossless/Silver, Horace, Quintet, The/1964 - Song for My Father/[1999-~Song for M~CD-Blue Note-84185-724349900226]/01-10 Song for My Father.m4a
/Volumes/Anisotope/gill/jazz.lossless/Blakey, Art & Jazz Messengers, The/2006-05-13 - Au Club St Germain 1958/[]/2-2/06-06 - Ending With the Theme.m4a
/Volumes/Anisotope/gill/jazz.lossless/Parker, Charlie/1988-09-19 - Bird_ The Complete Charlie Parker on Verve/[1988-09-19-CD-Verve-837 141-2-042283714120]/disc 7-10/12-19 Estrellita (take 5) (Wednesday January 23, 1952 - Charlie Parker Quintet).m4a
/Volumes/Anisotope/gill/jazz.lossless/Cecil Taylor Unit, The/1988 - Live in Bologna/[1988-CD-Leo Records-CD LR 100]/01-01 - [untitled].m4a
/Volumes/Anisotope/gill/jazz.lossless/Taylor, Cecil/2006 - The Eighth/[2006-CD-HatHut Records-hatOLOGY 622-752156062226]/01-02 Calling It the 8th.m4a
/Volumes/Anisotope/gill/jazz.lossless/Coltrane, John/1965 - Ascension/[2009-CD-impulse!-B0012402-02-602517920248]/01-02 - Ascension_ Edition II.m4a
```

#### CPU Profile (Real Files)

```bash
hack/bench.sh TestBenchmarkDecodeFile '/path/to/file.m4a'
```

Profiled against two contrasting tracks: Cecil Taylor Unit — [untitled] (1:09:50, moderate density, compression ratio 0.485) and John Coltrane — Ascension Ed. II (40:57, dense ensemble, compression ratio 0.696).

**Cecil Taylor Unit — [untitled] (1:09:50, 341.7 MB)**

Total: 355.64s, 212.29s in sampled functions (59.69%).

| Function         | Flat    | Flat%  | Cum     | Cum%   |
|------------------|---------|--------|---------|--------|
| runtime.cgocall  | 88.94s  | 41.90% | 88.94s  | 41.90% |
| unpcBlockGeneral | 70.99s  | 33.44% | 77.26s  | 36.39% |
| dynGet32Bit      | 14.86s  | 7.00%  | 16.61s  | 7.82%  |
| DynDecomp        | 14.12s  | 6.65%  | 31.00s  | 14.60% |
| signOfInt        | 3.80s   | 1.79%  | 3.80s   | 1.79%  |
| WriteStereo16    | 1.81s   | 0.85%  | 1.84s   | 0.87%  |
| read32bit        | 1.25s   | 0.59%  | 1.52s   | 0.72%  |
| unpcBlock4       | 1.02s   | 0.48%  | 1.50s   | 0.71%  |

**John Coltrane — Ascension Ed. II (40:57, 287.7 MB)**

Total: 190.15s, 110.50s in sampled functions (58.11%).

| Function         | Flat    | Flat%  | Cum     | Cum%   |
|------------------|---------|--------|---------|--------|
| runtime.cgocall  | 45.68s  | 41.34% | 45.68s  | 41.34% |
| unpcBlockGeneral | 32.43s  | 29.35% | 35.32s  | 31.96% |
| DynDecomp        | 8.57s   | 7.76%  | 17.21s  | 15.57% |
| dynGet32Bit      | 7.27s   | 6.58%  | 8.48s   | 7.67%  |
| signOfInt        | 2.23s   | 2.02%  | 2.24s   | 2.03%  |
| unpcBlock4       | 1.89s   | 1.71%  | 2.82s   | 2.55%  |
| WriteStereo16    | 1.08s   | 0.98%  | 1.08s   | 0.98%  |
| read32bit        | 0.89s   | 0.81%  | 1.07s   | 0.97%  |

The CGO CoreAudio decoder (`runtime.cgocall`) now dominates at ~41% since it runs in-process rather than via shell-out. Excluding CGO, the profile is consistent across both tracks: the linear predictor (`unpcBlockGeneral`) at 29-33%, followed by entropy decoding (`DynDecomp` + `dynGet32Bit`, 14-15% combined flat).

Compared to the synthetic profile (white noise) where `BitBuffer.Read` dominated at 19%, real music shifts the bottleneck to the linear predictor's reconstruction loop — random noise maximizes bit-reading overhead while structured audio exercises the predictor more heavily.

#### Memory (Real Files)

Decoder allocations remain zero. The CGO `GoBytes` allocation is the CoreAudio benchmark copying decoded PCM from C to Go heap:

| File | io.ReadAll | CGO GoBytes | WriteWAV | os.ReadFile | inuse_space |
|------|-----------|-------------|----------|-------------|-------------|
| Cecil Taylor Unit (341.7 MB) | 51.3 GB (80%) | 9.0 GB (14%) | 0.69 GB | 0.33 GB | 3.5 MB |
| Coltrane Ascension (287.7 MB) | 26.3 GB (84%) | 4.4 GB (14%) | 0.40 GB | 0.28 GB | 3.5 MB |

The `inuse_space` profile shows only 3.5 MB of live heap at program end — runtime goroutine stacks. The decoder holds no persistent allocations between packets.

## Mass testing

Comparative decoding is being done on a set of 7434 MP4A/ALAC files.

No failure or discrepancy has been found.