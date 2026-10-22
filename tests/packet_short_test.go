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

// Packets shorter than their elements, and ones the writers cannot take.
// GHSA-g66f-c97f-q276: each used to panic or to decode silently; each must
// come back as ErrDecode. The entropy decoder's own cases are in
// bitstream_overrun_test.go.

// shortStereoPacket is the advisory's packet: a CPE whose first channel
// escapes one sample, after which the zero-run count is read past the end.
//
//nolint:gochecknoglobals // a test vector, read-only
var shortStereoPacket = []byte{0x20, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x01, 0xFF}

// A frame shorter than the predictor's coefficient count.
func TestDecodePacket_ShortStereoShortFrame(t *testing.T) {
	t.Parallel()

	for _, frameLength := range []uint32{1, 16, 30} {
		config := boundsConfig(16, 2)
		config.FrameLength = frameLength

		decodeMustFail(t, config, shortStereoPacket)
	}
}

func TestDecodePacket_Empty(t *testing.T) {
	t.Parallel()

	decodeMustFail(t, boundsConfig(16, 2), nil)
}

// A stereo packet that ends before its second channel used to come back as
// silence with no error.
func TestDecodePacket_EndBeforeChannels(t *testing.T) {
	t.Parallel()

	decodeMustFail(t, boundsConfig(16, 2), []byte{0xE0}) // END first
}

func TestDecodePacket_SCEInStereo(t *testing.T) {
	t.Parallel()

	var w bitWriter

	w.elementHeader(0, true, 0, 1) // SCE, partial frame, escaped
	w.write(1, 32)                 // one sample
	w.write(1, 16)                 // the sample
	w.write(7, 3)                  // END

	dec, err := alac.NewPacketDecoder(boundsConfig(16, 2))
	if err != nil {
		t.Fatalf("NewPacketDecoder: %v", err)
	}

	_, err = dec.DecodePacket(w.buf)
	if !errors.Is(err, alac.ErrDecode) {
		t.Fatalf("expected ErrDecode for one channel of two, got: %v", err)
	}
}

// A channel pair element where the layout puts the pair on the last channel
// of three: found by FuzzDecodePacket, it wrote past the output. The frame
// is one sample, the cookie's whole frame, so the pair's second channel is
// the write past the end and not a write inside a longer frame's buffer.
func TestDecodePacket_CPEOnLastChannel(t *testing.T) {
	t.Parallel()

	var w bitWriter

	w.elementHeader(1, false, 0, 1) // CPE first, where a 3-channel stream carries its centre
	w.write(0, 64)                  // the pair, 32 bits each

	config := boundsConfig(32, 3)
	config.FrameLength = 1

	decodeMustFail(t, config, w.buf)
}

// A compressed 16-bit pair declaring a shifted byte a sample: the 16-bit
// writer takes none, and the frame decoded to its residuals alone, (0, 0)
// for shifted bytes 0x12 and 0x34. No encoder writes one.
func TestDecodePacket_ShiftAt16Bits(t *testing.T) {
	t.Parallel()

	decodeMustFail(t, boundsConfig(16, 2), []byte{
		0x20, 0x00, 0x14, 0x00, 0x00, 0x00, 0x02, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x24, 0x68, 0x70,
	})
}

// The same through a single channel element, under a mono cookie, so the
// missing-channels check cannot answer for the shift's.
func TestDecodePacket_ShiftAt16BitsSCE(t *testing.T) {
	t.Parallel()

	var w bitWriter

	w.elementHeader(0, true, 1, 0) // SCE, partial frame, one shifted byte, compressed
	w.write(1, 32)                 // one sample
	w.write(0, 6*8)                // mixBits, mixRes, the predictor's two header bytes, no coefficients

	decodeMustFail(t, boundsConfig(16, 1), w.buf)
}
