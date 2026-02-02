/*
   Copyright Mycophonic.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

//nolint:gosec // Integer conversions match Apple reference C implementation's fixed-width arithmetic.
package alac

import (
	"fmt"
	"slices"

	alacint "github.com/mycophonic/saprobe-alac/internal/alac"
)

//nolint:gochecknoglobals
var alacBitDepths = []uint8{
	16,
	20,
	24,
	32,
}

// Element type tags from the ALAC bitstream.
const (
	elemSCE = 0 // Single Channel Element
	elemCPE = 1 // Channel Pair Element
	elemCCE = 2 // Coupling Channel Element (unsupported)
	elemLFE = 3 // LFE Channel Element
	elemDSE = 4 // Data Stream Element
	elemPCE = 5 // Program Config Element (unsupported)
	elemFIL = 6 // Fill Element
	elemEND = 7 // End of Frame
)

// Decoder decodes ALAC audio packets into interleaved LE signed PCM.
type Decoder struct {
	config      Config
	format      PCMFormat
	mixBufferU  []int32
	mixBufferV  []int32
	predictor   []int32
	shiftBuffer []uint16
	bits        alacint.BitBuffer // reusable bit reader (avoids per-packet allocation)
}

// NewDecoder creates a new ALAC decoder from the given configuration.
func NewDecoder(config Config) (*Decoder, error) {
	if !slices.Contains(alacBitDepths, config.BitDepth) {
		return nil, fmt.Errorf("%w: %w: %d", ErrConfig, alacint.ErrBitDepth, config.BitDepth)
	}

	frameLen := int(config.FrameLength)

	return &Decoder{
		config: config,
		format: PCMFormat{
			SampleRate: int(config.SampleRate),
			BitDepth:   int(config.BitDepth),
			Channels:   int(config.NumChannels),
		},
		mixBufferU:  make([]int32, frameLen),
		mixBufferV:  make([]int32, frameLen),
		predictor:   make([]int32, frameLen),
		shiftBuffer: make([]uint16, frameLen*2), // stereo worst case
	}, nil
}

// Format returns the PCM output format.
func (d *Decoder) Format() PCMFormat {
	return d.format
}

// DecodePacket decodes a single ALAC packet into interleaved LE signed PCM bytes.
func (d *Decoder) DecodePacket(packet []byte) ([]byte, error) {
	numChan := int(d.config.NumChannels)
	bps := alacint.BytesPerSample(d.config.BitDepth)
	output := make([]byte, int(d.config.FrameLength)*numChan*bps)

	n, err := d.decodePacketInto(packet, output)
	if err != nil {
		return nil, err
	}

	return output[:n], nil
}

// decodePacketInto decodes a single ALAC packet into the provided output buffer.
// Returns the number of bytes written. The output buffer must be large enough
// to hold one full frame (FrameLength * NumChannels * BytesPerSample).
func (d *Decoder) decodePacketInto(packet, output []byte) (int, error) {
	d.bits.Reset(packet)
	bits := &d.bits
	numSamples := d.config.FrameLength
	numChan := int(d.config.NumChannels)
	bps := alacint.BytesPerSample(d.config.BitDepth)
	chanIdx := 0

	for {
		if bits.PastEnd() {
			return 0, fmt.Errorf("%w: %w", ErrDecode, alacint.ErrBitstreamOverrun)
		}

		tag := bits.ReadSmall(3)

		switch tag {
		case elemSCE, elemLFE:
			ns, err := d.decodeSCE(bits, output, chanIdx, numChan, numSamples)
			if err != nil {
				return 0, fmt.Errorf("%w: SCE/LFE: %w", ErrDecode, err)
			}

			numSamples = ns
			chanIdx++

		case elemCPE:
			if chanIdx+2 > numChan {
				goto done
			}

			ns, err := d.decodeCPE(bits, output, chanIdx, numChan, numSamples)
			if err != nil {
				return 0, fmt.Errorf("%w: CPE: %w", ErrDecode, err)
			}

			numSamples = ns
			chanIdx += 2

		case elemCCE, elemPCE:
			return 0, fmt.Errorf("%w: %w", ErrDecode, alacint.ErrUnsupportedElement)

		case elemDSE:
			if err := d.skipDSE(bits); err != nil {
				return 0, fmt.Errorf("%w: DSE: %w", ErrDecode, err)
			}

		case elemFIL:
			if err := d.skipFIL(bits); err != nil {
				return 0, fmt.Errorf("%w: FIL: %w", ErrDecode, err)
			}

		case elemEND:
			bits.ByteAlign()

			goto done

		default:
		}

		if chanIdx >= numChan {
			break
		}
	}

done:
	return int(numSamples) * numChan * bps, nil
}

// decodeSCE decodes a Single Channel Element (mono) or LFE element.
func (d *Decoder) decodeSCE(
	bits *alacint.BitBuffer, output []byte, chanIdx, numChan int, numSamples uint32,
) (uint32, error) {
	_ = bits.ReadSmall(4) // element instance tag

	// 12 unused header bits (must be 0).
	unusedHeader := bits.Read(alacint.UnusedHeaderBits)
	if unusedHeader != 0 {
		return 0, alacint.ErrInvalidHeader
	}

	headerByte := bits.Read(4)
	partialFrame := headerByte >> 3
	bytesShifted := int((headerByte >> 1) & 0x3)

	if bytesShifted == 3 {
		return 0, alacint.ErrInvalidShift
	}

	escapeFlag := headerByte & 0x1
	chanBits := uint32(d.config.BitDepth) - uint32(bytesShifted)*8

	if partialFrame != 0 {
		numSamples = bits.Read(16) << 16
		numSamples |= bits.Read(16)
	}

	if escapeFlag == 0 {
		if err := d.decodeSCECompressed(bits, chanBits, bytesShifted, int(numSamples)); err != nil {
			return 0, err
		}
	} else {
		d.decodeSCEEscape(bits, chanBits, int(numSamples))

		bytesShifted = 0
	}

	// Write output.
	sampleCount := int(numSamples)

	switch d.config.BitDepth {
	case 16:
		alacint.WriteMono16(output, d.mixBufferU, chanIdx, numChan, sampleCount)
	case 20:
		alacint.WriteMono20(output, d.mixBufferU, chanIdx, numChan, sampleCount)
	case 24:
		alacint.WriteMono24(output, d.mixBufferU, chanIdx, numChan, sampleCount, d.shiftBuffer, bytesShifted)
	case 32:
		alacint.WriteMono32(output, d.mixBufferU, chanIdx, numChan, sampleCount, d.shiftBuffer, bytesShifted)

	default:
		panic(fmt.Sprintf("alac: decodeSCE called with unsupported bit depth %d", d.config.BitDepth))
	}

	return numSamples, nil
}

func (d *Decoder) decodeSCECompressed(bits *alacint.BitBuffer, chanBits uint32, bytesShifted, numSamples int) error {
	_ = bits.Read(8) // mixBits (unused for mono)
	_ = bits.Read(8) // mixRes (unused for mono)

	headerByte := bits.Read(8)
	modeU := headerByte >> 4
	denShiftU := headerByte & 0xf

	headerByte = bits.Read(8)
	pbFactorU := headerByte >> 5
	numU := headerByte & 0x1f

	var coefsU [alacint.MaxCoefs]int16
	for i := range numU {
		coefsU[i] = int16(bits.Read(16))
	}

	// Save shift bits position, skip past them.
	var shiftBits alacint.BitBuffer
	if bytesShifted != 0 {
		shiftBits = bits.Copy()
		bits.Advance(uint32(bytesShifted) * 8 * uint32(numSamples))
	}

	// Entropy decode.
	predBound := uint32(d.config.PB)

	var agP alacint.AGParams
	alacint.SetAGParams(&agP, uint32(d.config.MB), (predBound*pbFactorU)/4, uint32(d.config.KB),
		uint32(numSamples), uint32(numSamples), uint32(d.config.MaxRun))

	if err := alacint.DynDecomp(&agP, bits, d.predictor, numSamples, int(chanBits)); err != nil {
		return fmt.Errorf("entropy decode: %w", err)
	}

	// Predictor.
	if modeU != 0 {
		alacint.UnpcBlock(d.predictor, d.predictor, numSamples, nil, alacint.NumActiveDelta, chanBits, 0)
	}

	alacint.UnpcBlock(d.predictor, d.mixBufferU, numSamples, coefsU[:numU], int32(numU), chanBits, denShiftU)

	// Read shift buffer from saved position.
	if bytesShifted != 0 {
		shift := uint8(bytesShifted * 8)
		for i := range numSamples {
			d.shiftBuffer[i] = uint16(shiftBits.Read(shift))
		}
	}

	return nil
}

func (d *Decoder) decodeSCEEscape(bits *alacint.BitBuffer, chanBits uint32, numSamples int) {
	shift := uint32(32) - chanBits

	if chanBits <= 16 {
		for idx := range numSamples {
			val := int32(bits.Read(uint8(chanBits)))
			val = (val << shift) >> shift
			d.mixBufferU[idx] = val
		}
	} else {
		extraBits := chanBits - 16

		for idx := range numSamples {
			val := int32(bits.Read(16))
			val = (val << 16) >> shift
			d.mixBufferU[idx] = val | int32(bits.Read(uint8(extraBits)))
		}
	}
}

// decodeCPE decodes a Channel Pair Element (stereo).
func (d *Decoder) decodeCPE(
	bits *alacint.BitBuffer,
	output []byte,
	chanIdx, numChan int,
	numSamples uint32,
) (uint32, error) {
	_ = bits.ReadSmall(4) // element instance tag

	unusedHeader := bits.Read(alacint.UnusedHeaderBits)
	if unusedHeader != 0 {
		return 0, alacint.ErrInvalidHeader
	}

	headerByte := bits.Read(4)
	partialFrame := headerByte >> 3
	bytesShifted := int((headerByte >> 1) & 0x3)

	if bytesShifted == 3 {
		return 0, alacint.ErrInvalidShift
	}

	escapeFlag := headerByte & 0x1
	// CPE has +1 bit for decorrelation.
	chanBits := uint32(d.config.BitDepth) - uint32(bytesShifted)*8 + 1

	if partialFrame != 0 {
		numSamples = bits.Read(16) << 16
		numSamples |= bits.Read(16)
	}

	var mixBits, mixRes int32

	if escapeFlag == 0 {
		var err error

		mixBits, mixRes, err = d.decodeCPECompressed(bits, chanBits, bytesShifted, int(numSamples))
		if err != nil {
			return 0, err
		}
	} else {
		chanBits = uint32(d.config.BitDepth) // Reset for escape.
		d.decodeCPEEscape(bits, chanBits, int(numSamples))

		bytesShifted = 0
	}

	// Unmix and write output.
	sampleCount := int(numSamples)

	switch d.config.BitDepth {
	case 16:
		alacint.WriteStereo16(output, d.mixBufferU, d.mixBufferV, chanIdx, numChan, sampleCount, mixBits, mixRes)
	case 20:
		alacint.WriteStereo20(output, d.mixBufferU, d.mixBufferV, chanIdx, numChan, sampleCount, mixBits, mixRes)
	case 24:
		alacint.WriteStereo24(output, d.mixBufferU, d.mixBufferV, chanIdx, numChan, sampleCount,
			mixBits, mixRes, d.shiftBuffer, bytesShifted)
	case 32:
		alacint.WriteStereo32(output, d.mixBufferU, d.mixBufferV, chanIdx, numChan, sampleCount,
			mixBits, mixRes, d.shiftBuffer, bytesShifted)

	default:
		panic(fmt.Sprintf("alac: decodeCPE called with unsupported bit depth %d", d.config.BitDepth))
	}

	return numSamples, nil
}

func (d *Decoder) decodeCPECompressed(
	bits *alacint.BitBuffer,
	chanBits uint32,
	bytesShifted, numSamples int,
) (int32, int32, error) { //revive:disable-line:confusing-results
	mixBits := int32(bits.Read(8))
	mixRes := int32(int8(bits.Read(8)))

	// Read U channel predictor params.
	headerByte := bits.Read(8)
	modeU := headerByte >> 4
	denShiftU := headerByte & 0xf

	headerByte = bits.Read(8)
	pbFactorU := headerByte >> 5
	numU := headerByte & 0x1f

	var coefsU [alacint.MaxCoefs]int16
	for i := range numU {
		coefsU[i] = int16(bits.Read(16))
	}

	// Read V channel predictor params.
	headerByte = bits.Read(8)
	modeV := headerByte >> 4
	denShiftV := headerByte & 0xf

	headerByte = bits.Read(8)
	pbFactorV := headerByte >> 5
	numV := headerByte & 0x1f

	var coefsV [alacint.MaxCoefs]int16
	for i := range numV {
		coefsV[i] = int16(bits.Read(16))
	}

	// Save shift bits position, skip past interleaved shift data.
	var shiftBits alacint.BitBuffer
	if bytesShifted != 0 {
		shiftBits = bits.Copy()
		bits.Advance(uint32(bytesShifted) * 8 * 2 * uint32(numSamples))
	}

	predBound := uint32(d.config.PB)

	var agP alacint.AGParams

	// Decompress and predict U channel.
	alacint.SetAGParams(&agP, uint32(d.config.MB), (predBound*pbFactorU)/4, uint32(d.config.KB),
		uint32(numSamples), uint32(numSamples), uint32(d.config.MaxRun))

	if err := alacint.DynDecomp(&agP, bits, d.predictor, numSamples, int(chanBits)); err != nil {
		return 0, 0, fmt.Errorf("entropy decode U: %w", err)
	}

	if modeU != 0 {
		alacint.UnpcBlock(d.predictor, d.predictor, numSamples, nil, alacint.NumActiveDelta, chanBits, 0)
	}

	alacint.UnpcBlock(d.predictor, d.mixBufferU, numSamples, coefsU[:numU], int32(numU), chanBits, denShiftU)

	// Decompress and predict V channel.
	alacint.SetAGParams(&agP, uint32(d.config.MB), (predBound*pbFactorV)/4, uint32(d.config.KB),
		uint32(numSamples), uint32(numSamples), uint32(d.config.MaxRun))

	if err := alacint.DynDecomp(&agP, bits, d.predictor, numSamples, int(chanBits)); err != nil {
		return 0, 0, fmt.Errorf("entropy decode V: %w", err)
	}

	if modeV != 0 {
		alacint.UnpcBlock(d.predictor, d.predictor, numSamples, nil, alacint.NumActiveDelta, chanBits, 0)
	}

	alacint.UnpcBlock(d.predictor, d.mixBufferV, numSamples, coefsV[:numV], int32(numV), chanBits, denShiftV)

	// Read shift buffer from saved position.
	if bytesShifted != 0 {
		shift := uint8(bytesShifted * 8)
		for i := 0; i < numSamples*2; i += 2 {
			d.shiftBuffer[i+0] = uint16(shiftBits.Read(shift))
			d.shiftBuffer[i+1] = uint16(shiftBits.Read(shift))
		}
	}

	return mixBits, mixRes, nil
}

func (d *Decoder) decodeCPEEscape(bits *alacint.BitBuffer, chanBits uint32, numSamples int) {
	shift := uint32(32) - chanBits

	if chanBits <= 16 {
		for idx := range numSamples {
			val := int32(bits.Read(uint8(chanBits)))
			val = (val << shift) >> shift
			d.mixBufferU[idx] = val

			val = int32(bits.Read(uint8(chanBits)))
			val = (val << shift) >> shift
			d.mixBufferV[idx] = val
		}
	} else {
		extraBits := chanBits - 16

		for idx := range numSamples {
			val := int32(bits.Read(16))
			val = (val << 16) >> shift
			d.mixBufferU[idx] = val | int32(bits.Read(uint8(extraBits)))

			val = int32(bits.Read(16))
			val = (val << 16) >> shift
			d.mixBufferV[idx] = val | int32(bits.Read(uint8(extraBits)))
		}
	}
}

// skipFIL skips a Fill Element.
func (*Decoder) skipFIL(bits *alacint.BitBuffer) error {
	count := int16(bits.ReadSmall(4))
	if count == 15 { //revive:disable-line:add-constant
		count += int16(bits.ReadSmall(8)) - 1
	}

	bits.Advance(uint32(count) * 8)

	if bits.PastEnd() {
		return alacint.ErrBitstreamOverrun
	}

	return nil
}

// skipDSE skips a Data Stream Element.
func (*Decoder) skipDSE(bits *alacint.BitBuffer) error {
	_ = bits.ReadSmall(4) // element instance tag
	dataByteAlignFlag := bits.ReadOne()

	count := uint16(bits.ReadSmall(8))
	if count == 255 { //revive:disable-line:add-constant
		count += uint16(bits.ReadSmall(8))
	}

	if dataByteAlignFlag != 0 {
		bits.ByteAlign()
	}

	bits.Advance(uint32(count) * 8)

	if bits.PastEnd() {
		return alacint.ErrBitstreamOverrun
	}

	return nil
}
