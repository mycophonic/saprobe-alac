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

package tests_test

import (
	"errors"
	"testing"

	alac "github.com/mycophonic/saprobe-alac"
)

// Packets whose headers promise more than the packet holds. Each one used to
// panic in the decoder; each must come back as ErrDecode.

// bitWriter appends bit fields MSB-first, the way the ALAC bitstream is laid out.
type bitWriter struct {
	buf  []byte
	nbit uint
}

func (w *bitWriter) write(val uint64, bits uint) {
	for i := bits; i > 0; i-- {
		if w.nbit%8 == 0 {
			w.buf = append(w.buf, 0)
		}

		if (val>>(i-1))&1 != 0 {
			w.buf[len(w.buf)-1] |= 1 << (7 - w.nbit%8)
		}

		w.nbit++
	}
}

// elementHeader writes an SCE (tag 0) or CPE (tag 1) element header: the tag,
// the instance tag, the 12 unused bits and the 4-bit header (partialFrame,
// bytesShifted, escapeFlag).
func (w *bitWriter) elementHeader(tag uint64, partial bool, bytesShifted, escape uint64) {
	w.write(tag, 3)
	w.write(0, 4)
	w.write(0, 12)

	var hdr uint64
	if partial {
		hdr |= 1 << 3
	}

	hdr |= bytesShifted << 1
	hdr |= escape
	w.write(hdr, 4)
}

func boundsConfig(bitDepth, channels uint8) alac.PacketConfig {
	return alac.PacketConfig{
		FrameLength: 4096,
		BitDepth:    bitDepth,
		NumChannels: channels,
		PB:          40,
		MB:          10,
		KB:          14,
		MaxRun:      255,
		SampleRate:  44100,
	}
}

func decodeMustFail(t *testing.T, config alac.PacketConfig, packet []byte) {
	t.Helper()

	dec, err := alac.NewPacketDecoder(config)
	if err != nil {
		t.Fatalf("NewPacketDecoder: %v", err)
	}

	_, err = dec.DecodePacket(packet)
	if err == nil {
		t.Fatal("expected an error, got none")
	}

	if !errors.Is(err, alac.ErrDecode) {
		t.Fatalf("expected ErrDecode, got: %v", err)
	}

	t.Logf("refused: %v", err)
}

// A partial-frame count above the frame length: the work buffers hold one frame.
func TestDecodePacket_SampleCountAboveFrameLength(t *testing.T) {
	t.Parallel()

	var w bitWriter

	w.elementHeader(0, true, 0, 1)
	w.write(65536, 32) // numSamples, above FrameLength 4096
	w.write(0, 64*8)   // some sample data, far less than 65536 samples

	decodeMustFail(t, boundsConfig(16, 1), w.buf)
}

// The same through a channel pair element.
func TestDecodePacket_SampleCountAboveFrameLengthCPE(t *testing.T) {
	t.Parallel()

	var w bitWriter

	w.elementHeader(1, true, 0, 1)
	w.write(65536, 32)
	w.write(0, 64*8)

	decodeMustFail(t, boundsConfig(16, 2), w.buf)
}

// An escape-coded (uncompressed) element whose packet ends long before its samples do.
func TestDecodePacket_EscapeSamplesPastEnd(t *testing.T) {
	t.Parallel()

	var w bitWriter

	w.elementHeader(0, false, 0, 1)
	w.write(0, 5*8) // five bytes where 4096 16-bit samples should follow

	decodeMustFail(t, boundsConfig(16, 1), w.buf)
}

// The same through a channel pair element.
func TestDecodePacket_EscapeSamplesPastEndCPE(t *testing.T) {
	t.Parallel()

	var w bitWriter

	w.elementHeader(1, false, 0, 1)
	w.write(0, 5*8)

	decodeMustFail(t, boundsConfig(16, 2), w.buf)
}

// A compressed 24-bit element with one shifted byte per sample: the shift bytes
// alone would run 4096 bytes past a 10-byte packet.
func TestDecodePacket_ShiftBytesPastEnd(t *testing.T) {
	t.Parallel()

	var w bitWriter

	w.elementHeader(0, false, 1, 0)
	w.write(0, 8)   // mixBits
	w.write(0, 8)   // mixRes
	w.write(0, 8)   // modeU and denShiftU
	w.write(0, 8)   // pbFactorU and numU, both zero
	w.write(0, 6*8) // the start of what should be 4096 shift bytes

	decodeMustFail(t, boundsConfig(24, 1), w.buf)
}

// A packet cut inside the element header: the reads past its end yield zeros
// and the overrun is reported, not a slice panic.
func TestDecodePacket_TruncatedHeader(t *testing.T) {
	t.Parallel()

	var w bitWriter

	w.elementHeader(0, false, 0, 0)
	w.write(0, 8) // mixBits; the rest of the header and the coefficients are missing

	decodeMustFail(t, boundsConfig(16, 1), w.buf)
}

// A one-byte packet.
func TestDecodePacket_OneByte(t *testing.T) {
	t.Parallel()

	decodeMustFail(t, boundsConfig(16, 2), []byte{0x20})
}
