# Saprobe ALAC

A pure Go ALAC decoder, ported from Apple's open-source C implementation (Apache 2.0, 2011).
- streaming / seekable
- fast (faster than CGO+CoreAudio)
- no-dependency
- zero runtime allocation
- minimal BCE
- no unsafe

This is a decoder only.

A _crude_ example cli is provided as well.

For a proper full-blown, higher-level decoder library and cli, see [Saprobe](https://github.com/mycophonic/saprobe).

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

- CCE / PCE element types (returns error) (note that no known encoder ever produce these)
- Encoding
- CAF container parsing

## API

```go
// High-level — M4A/MP4 files
func NewDecoder(rs io.ReadSeeker) (*Decoder, error)
func (d *Decoder) Read(p []byte) (int, error)
func (d *Decoder) Format() PCMFormat
func (d *Decoder) Duration() time.Duration
func (d *Decoder) Position() time.Duration
func (d *Decoder) Seek(t time.Duration) (time.Duration, error)

// Low-level — custom containers, network streams
func ParseMagicCookie(cookie []byte) (PacketConfig, error)
func NewPacketDecoder(config PacketConfig) (*PacketDecoder, error)
func (d *PacketDecoder) DecodePacket(packet []byte) ([]byte, error)
func (d *PacketDecoder) Format() PCMFormat
```

## Performance

saprobe-alac is generally faster than CGO>Apple CoreAudio.

To get there, optimizations have been done with a mixture of targeted inlining and bound checks elimination.
See [optimization](./docs/OPTIM.md).

Comparison with ffmpeg is more crushing, which is expected, given the highly optimized nature of ffmpeg.

It should be noted that the comparison with CoreAudio is not entirely fair (there is a cost associated with crossing GO/C
boundaries). The comparison with Apple open-source alacconvert is more fair to Apple implementation
(although shelling out does also introduce latency on smaller files that has to be accounted for).

Further optimization work would be unlikely to bring in significant returns and would presumably require intense assembly
work...

## Detailed documentation

* [decoders landscape](./docs/research/DECODERS.md)
* [encoders landscape](./docs/research/ENCODERS.md)
* [implementation details](./docs/IMPLEMENTATION.md)
* [tests and benchmarks](./docs/QA.md)