# Saprobe ALAC

Pure Go ALAC streaming decoder, ported from Apple's open-source C implementation (Apache 2.0, 2011) with
performance optimizations.

No encoder.

An example cli decoder is provided.
Note that file open in this example is crude and inefficient.

For a proper full-blown library and cli including buffered file IO, see [Saprobe](https://github.com/mycophonic/saprobe).

## Support

- **Bit depths:** 16, 20, 24, 32 (20 and 32 are implemented but untestable -- no available encoder produces them)
- **Channels:** 1-8 (mono through 7.1 surround)
- **Sample rates:** any valid uint32; tested at 8000-192000 Hz (11 rates)
- **Container:** M4A/MP4
- **Output:** interleaved little-endian signed PCM

| Bit Depth | Bytes/Sample | Notes                             |
|-----------|--------------|-----------------------------------|
| 16        | 2            | Signed LE                         |
| 20        | 3            | Left-aligned in 24-bit, signed LE |
| 24        | 3            | Signed LE, optional shift buffer  |
| 32        | 4            | Signed LE, optional shift buffer  |

### Not supported

- CCE / PCE element types (returns error)
- Encoding
- CAF container parsing

## API

```go
func ParseConfig(cookie []byte) (Config, error)

func NewDecoder(config Config) (*Decoder, error)
func (d *Decoder) DecodePacket(packet []byte) ([]byte, error)
func (d *Decoder) Format() PCMFormat

func NewStreamDecoder(rs io.ReadSeeker) (*StreamDecoder, error)
func (s *StreamDecoder) Read(p []byte) (int, error)
func (s *StreamDecoder) Format() PCMFormat

func Decode(reader io.ReadSeeker) ([]byte, PCMFormat, error)
```

## Performance

Performance is around 1.3x slower that Apple open-source C implementation and CoreAudio decoder across the board.

Given Go vs. C, this is respectable.

Additional efforts are underway to leverage SIMD, bound checks elimination, and better inlining to squeeze out a little
bit more juice out of it. See [optimization](./docs/OPTIM.md).

Comparison with ffmpeg is more crushing, which is expected as well given the highly optimized nature of ffmpeg.

## Dependencies

MP4 box parsing uses github.com/abema/go-mp4

Other dependencies (agar) are purely for test tooling.

## Detailed documentation

* [decoders landscape](./docs/research/DECODERS.md)
* [encoders landscape](./docs/research/ENCODERS.md)
* [implementation details](./docs/IMPLEMENTATION.md)
* [tests and benchmarks](./docs/QA.md)