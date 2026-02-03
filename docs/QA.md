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

Total: 23s, 7990ms in sampled functions (34.74%).

| Function         | Flat  | Flat%  | Cum   | Cum%   |
|------------------|-------|--------|-------|--------|
| runtime.cgocall  | 2.39s | 29.91% | 2.39s | 29.91% |
| BitBuffer.Read   | 1.83s | 22.90% | 1.89s | 23.65% |
| decodeCPEEscape  | 1.03s | 12.89% | 2.65s | 33.17% |
| WriteStereo24    | 0.25s | 3.13%  | 0.25s | 3.13%  |

The CGO CoreAudio decoder (`runtime.cgocall`) appears at ~30% since it runs in-process. Among saprobe-only functions, bit reading dominates at 22.90%, followed by the CPE element decoder (12.89% flat, 33.17% cumulative). The synthetic profile is dominated by BitBuffer.Read because white noise maximizes entropy, exercising the bit reader more than the predictor.

#### Memory

Decoder allocations are negligible. All significant allocations come from test infrastructure:

| Source             | Alloc      | Alloc% |
|--------------------|------------|--------|
| io.ReadAll         | 12,599 MB  | 80.07% |
| CGO GoBytes        | 2,448 MB   | 15.56% |
| generateWhiteNoise | 223 MB     | 1.41%  |
| WriteWAV           | 223 MB     | 1.41%  |
| os.ReadFile        | 223 MB     | 1.42%  |

The `alac.Decode` path itself allocates no significant memory. The CGO `GoBytes` allocation (15.56%) is the CoreAudio benchmark copying decoded PCM from C to Go heap. `inuse_space` at exit: 4.6 KB.

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
| Horace Silver — Song for My Father | 7:06 | 40.6 MB | 0.55 | 991ms | 249ms | 4.0x | 957ms | 1.04x | 831ms | 1.2x | Typical jazz combo |
| Art Blakey — Ending With the Theme | 0:29 | 82 KB | 0.017 | 22ms | 33ms | 0.7x | 14ms | 1.6x | 20ms | 1.1x | Near-silence, extreme compressibility |
| Charlie Parker — Estrellita (take 5) | 0:06 | 0.3 MB | 0.254 | 9ms | 29ms | 0.3x | 8ms | 1.1x | 12ms | 0.8x | Short take, sparse |
| Cecil Taylor Unit — [untitled] | 1:09:50 | 341.7 MB | 0.485 | 9.605s | 2.726s | 3.5x | 9.103s | 1.06x | 8.028s | 1.2x | Long free jazz, moderate density |
| Cecil Taylor — Calling It the 8th | 58:10 | 352.6 MB | 0.601 | 7.503s | 1.931s | 3.9x | 7.319s | 1.03x | 6.634s | 1.1x | Dense free piano, low compressibility |
| John Coltrane — Ascension Ed. II | 40:57 | 287.7 MB | 0.696 | 4.806s | 1.164s | 4.1x | 4.686s | 1.03x | 4.553s | 1.06x | Dense ensemble, near-incompressible |

On short files (<30s), saprobe is faster than ffmpeg (0.3-0.7x) due to zero process-spawn overhead, but slower than CGO CoreAudio (1.1-1.6x).
On longer files, ffmpeg pulls ahead (3.5-4.1x faster) due to SIMD-optimized C.
Saprobe is within ~1.03-1.06x of CoreAudio (CGO, in-process) on long files, and ~1.06-1.2x vs alacconvert. The gap narrowed significantly after adding a specialized predictor for order 6, which handles 70-85% of packets in real music files.

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

Total: 349.58s, 195.48s in sampled functions (55.92%).

| Function         | Flat    | Flat%  | Cum     | Cum%   |
|------------------|---------|--------|---------|--------|
| runtime.cgocall  | 89.54s  | 45.81% | 89.54s  | 45.81% |
| unpcBlock6       | 41.40s  | 21.18% | 54.28s  | 27.77% |
| DynDecomp        | 26.48s  | 13.55% | 30.68s  | 15.69% |
| signOfInt        | 11.20s  | 5.73%  | 11.74s  | 6.01%  |
| unpcBlockGeneral | 7.63s   | 3.90%  | 8.15s   | 4.17%  |

**John Coltrane — Ascension Ed. II (40:57, 287.7 MB)**

Total: 185.29s, 99.38s in sampled functions (53.63%).

| Function         | Flat    | Flat%  | Cum     | Cum%   |
|------------------|---------|--------|---------|--------|
| runtime.cgocall  | 45.32s  | 45.60% | 45.32s  | 45.60% |
| unpcBlock6       | 17.07s  | 17.18% | 20.77s  | 20.90% |
| DynDecomp        | 15.31s  | 15.41% | 17.32s  | 17.43% |
| unpcBlockGeneral | 5.32s   | 5.35%  | 5.75s   | 5.79%  |
| signOfInt        | 3.72s   | 3.74%  | 3.88s   | 3.90%  |
| unpcBlock4       | 2.22s   | 2.23%  | 2.93s   | 2.95%  |

The CGO CoreAudio decoder (`runtime.cgocall`) dominates at ~46% since it runs in-process. The new `unpcBlock6` specialization handles 70-85% of packets in real music and appears at 17-21% flat. `dynGet32Bit` has been manually inlined into `DynDecomp`, which now shows 13-15% flat (previously split across `DynDecomp` + `dynGet32Bit`). The generic predictor path `unpcBlockGeneral` dropped from 29-33% to 3.9-5.4%, now handling only the residual order-5 and order-7+ packets.

Compared to the synthetic profile (white noise) where `BitBuffer.Read` dominates at 23%, real music shifts the bottleneck to the linear predictor — random noise maximizes entropy and bit-reading overhead, while structured audio exercises the predictor reconstruction loop more heavily.

#### Memory (Real Files)

Decoder allocations remain zero. The CGO `GoBytes` allocation is the CoreAudio benchmark copying decoded PCM from C to Go heap:

| File | io.ReadAll | CGO GoBytes | WriteWAV | os.ReadFile | inuse_space |
|------|-----------|-------------|----------|-------------|-------------|
| Cecil Taylor Unit (341.7 MB) | 51.28 GB (85.56%) | 7.57 GB (12.63%) | 0.69 GB | 0.33 GB | 3.5 MB |
| Coltrane Ascension (287.7 MB) | 26.25 GB (83.56%) | 4.44 GB (14.13%) | 0.40 GB | 0.28 GB | 4.1 MB |

The `inuse_space` profile shows only 3.5-4.1 MB of live heap at program end — runtime goroutine stacks. The decoder holds no persistent allocations between packets.

## Mass testing

Comparative decoding is being done on a set of 7434 MP4A/ALAC files.

No failure or discrepancy has been found.