# Implementation

## ALAC Specification

Apple released the ALAC codec as open source (Apache 2.0) in 2011. Key parameters from the reference implementation:

| Parameter | Spec Range | Notes |
|-----------|-----------|-------|
| Bit depths | 16, 20, 24, 32 | Stored in ALACSpecificConfig |
| Channels | 1-8 | Mono through 7.1 surround |
| Sample rates | 1-4,294,967,295 Hz | Any uint32; practically 8kHz-192kHz |
| Frame length | Any uint32 | Typically 4096 samples |
| Predictor orders | 0-31 | 5-bit field; 31 = delta mode |
| Element types | SCE, CPE, CCE, LFE, DSE, PCE, FIL, END | CCE and PCE rarely used |
| Shift modes | 0, 1, 2 | LSB stripping for 24/32-bit |
| Container | MP4/M4A, CAF | M4A is standard distribution format |

## Source Map

Port of Apple's reference C sources:

| File         | C Source           | Purpose                                  |
|--------------|--------------------|------------------------------------------|
| config.go    | ALACAudioTypes.h   | Config parsing (ALACSpecificConfig)      |
| bitbuffer.go | ALACBitUtilities.c | Bit-level reader                         |
| golomb.go    | ag_dec.c           | Adaptive Golomb-Rice entropy decoder     |
| predictor.go | dp_dec.c           | Linear predictor (FIR filter)            |
| matrix.go    | matrix_dec.c       | Stereo unmix + PCM output formatting     |
| decoder.go   | ALACDecoder.cpp    | Packet decode, element dispatch          |
| decode.go    | -                  | M4A container parsing, streaming decoder |

## Decoder Pipeline

```
M4A -> MP4 box traversal (moov/trak/mdia/minf/stbl)
  -> ALACSpecificConfig from stsd
  -> Flat sample table from stco/stsc/stsz
  -> Per packet:
      -> Element dispatch (SCE/CPE/LFE/DSE/FIL/END)
      -> Compressed: Golomb-Rice entropy decode -> linear predictor (FIR)
      -> Escape: raw PCM samples
      -> Stereo: matrix unmix (mid/side decorrelation)
      -> Interleaved LE signed PCM output
```

### Predictor paths

| Order | Path               | Notes                            |
|-------|--------------------|----------------------------------|
| 0     | Copy               | No prediction                    |
| 4     | `unpcBlock4`       | Hand-unrolled 4-tap FIR          |
| 8     | `unpcBlock8`       | Hand-unrolled 8-tap FIR          |
| 31    | Delta mode         | First-order delta decode         |
| Other | `unpcBlockGeneral` | General FIR with BCE sub-slicing |

### Optimizations

- **bitBuffer reuse**: reset per-packet instead of allocating
- **decodePacketInto**: write PCM directly into output buffer, no intermediate copies
- **writeStereo16 BCE**: sub-slice pattern for bounds check elimination
- **read32bit intrinsic**: `binary.BigEndian.Uint32` compiles to single load+bswap
- **Register-friendly entropy decoder**: `dynGet32Bit`/`dynGet` return `(result, newBitPos)` instead of `*uint32`, keeping `bitPos` in a register
- **Specialized predictors**: hand-unrolled `unpcBlock4`/`unpcBlock8` with coefficients in local variables
