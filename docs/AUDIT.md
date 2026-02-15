# Audit: saprobe-alac

Full 360 review covering documentation, code, API, tests, dependencies, and build infrastructure.

## Findings Summary

| # | Category | Finding | Severity |
|---|----------|---------|----------|
| 1 | Docs | Orphaned profile PNGs in docs/ | Low |
| 2 | Code | Defensive panics in decoder.go for impossible states after validation | Info |

---

## 1. Orphaned profile PNGs in docs/

Two PNG files exist in `docs/`:
- `docs/decode_cpu.png` (211 KB)
- `docs/decode_alloc.png` (138 KB)

No markdown file in saprobe-alac references them. They were likely associated with a document that has since been removed or moved.

**Action:** Either reference them from QA.md or OPTIM.md where appropriate, or delete them.

---

## 2. Defensive panics in decoder.go

`decoder.go:222` and `decoder.go:364` contain panics for unsupported bit depths in `decodeSCE` and `decodeCPE`. These are unreachable after `NewPacketDecoder` validation (line 60) rejects invalid bit depths. The panics are defensive against impossible states.

`internal/alac/format.go:32` also panics in `BytesPerSample` for unsupported depths — also internal and should never fire.

Not a bug — defensive programming against invariant violations.

**Action:** None required. Consider converting to errors if panic-free is a goal.

---

## Clean Areas (no issues)

### Public API

The API surface is clean and consistent:

| Function | Purpose | Status |
|----------|---------|--------|
| `NewDecoder` | Create M4A streaming decoder | Correct |
| `Decoder.Read` | io.Reader for decoded PCM | Correct |
| `Decoder.Format` | Return PCM format metadata | Correct |
| `ParseMagicCookie` | Parse ALAC magic cookie from MP4 | Correct |
| `NewPacketDecoder` | Create packet-level decoder | Correct |
| `PacketDecoder.DecodePacket` | Decode single ALAC packet | Correct |
| `PacketDecoder.Format` | Return PCM format metadata | Correct |

API is streaming-only. No one-shot convenience function.

### Dead code

No dead code found. Every unexported function in the library, internal/alac, and internal/mp4 packages is reachable. Verified:
- All decoder methods (decodeSCE, decodeCPE, decodeSCECompressed, decodeSCEEscape, decodeCPECompressed, decodeCPEEscape, skipFIL, skipDSE) called from dispatch
- All predictor paths (unpcBlock4/5/6/8, unpcBlockGeneral) called from switch
- All MP4 helpers (extractCookie, buildSampleTable, readChunkOffsets, readStsc, readStsz, lookupSamplesPerChunk) called from FindALACTrack
- All golomb helpers (lead, lg3a, read32bit, getStreamBits, dynGet, dynGet32Bit) called from DynDecomp

### Error handling

Error wrapping follows a consistent pattern: internal sentinel errors wrapped with public sentinels via `fmt.Errorf("%w: %w", PublicErr, internalErr)`. All three public sentinels (`ErrConfig`, `ErrNoTrack`, `ErrDecode`) provide meaningful context.

### Package structure

Clean separation:
- Root package: public API (config, decode, decoder, format, errors)
- `internal/alac`: codec implementation (bitbuffer, golomb, matrix, predictor)
- `internal/mp4`: container parsing
- `tests/`: separate module for integration tests and test utilities

No misplaced code.

### Bounds safety

BitBuffer uses 4-byte zero padding to allow safe over-reads in hot paths. Predictor functions use BCE (bounds check elimination) patterns with reslicing. Matrix output functions use bounded sub-slices. All correct.

### Resource management

Buffers allocated once per Decoder and reused across packets. BitBuffer backing storage grows but never shrinks (intentional — amortized allocation). No leaks.

### Build infrastructure

Makefile, common.mk, .golangci.yml, .gitignore, hack/bench.sh all functional and well-structured. CGO_ENABLED=1 correctly limited to test targets only. Makefile overrides common.mk test/lint-mod targets to include the tests submodule.

### README

All claims accurate. Support matrix, API surface, dependency list, and documentation links all verified. Research docs (DECODERS.md, ENCODERS.md) exist at the referenced paths.

### Research docs

DECODERS.md and ENCODERS.md are accurate reference material covering the ALAC decoder and encoder landscape.

### No security issues

No security vulnerabilities found.

---

## Testutil status after cleanup

| File | Status | Purpose |
|------|--------|---------|
| alacconvert.go | Clean | AlacConvert runner, WAV I/O, AlacConvertPath, BenchDecodeAlacconvert |
| coreaudio.go | Clean | CoreAudio runner, CoreAudioPath |
| doc.go | Clean | Package doc |
| shared_ffmpeg.go | Deleted | Was thin agar wrapper; inlined into benchmark_test.go |
| shared.go | Deleted | Was pure agar delegation |
| util.go | Deleted | Was single-function agar wrapper |

No stale imports, no dead code, no references to deleted files.
