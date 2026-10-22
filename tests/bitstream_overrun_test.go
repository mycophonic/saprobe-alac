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
	"bytes"
	"testing"

	alac "github.com/mycophonic/saprobe-alac"
)

// Packets whose last code runs past the end of the packet, written by the
// 1-bit bridge (acoseac/1-bit-bridge, ops/upstream) with its report of
// GHSA-g66f-c97f-q276 and carried here as sent. The bit buffer
// pads a packet with four zero bytes so that a code can be read near the end;
// each of these either panicked, reading past the padding as well, or was
// decoded from the padding without an error. Each must come back as ErrDecode.

// compressedPair writes a compressed channel pair of numSamples frames, with
// mixBits, mixRes and both predictors' header bytes zero (no coefficients),
// and then the entropy-coded bits given as '0' and '1'.
func compressedPair(numSamples uint64, bits string) []byte {
	var w bitWriter

	w.elementHeader(1, true, 0, 0)
	w.write(numSamples, 32)
	w.write(0, 6*8)

	for _, b := range bits {
		w.write(uint64(b-'0'), 1)
	}

	return w.buf
}

// U's first residual is an escape whose value lies in the padding, and the
// zero run after it read four bytes from past the padding: a panic.
func TestDecodePacket_EscapeThenZeroRunPastEnd(t *testing.T) {
	t.Parallel()

	packet := []byte{0x20, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x01, 0xFF}

	decodeMustFail(t, boundsConfig(16, 2), packet)
	decodeMustFail(t, boundsConfig(24, 2), packet)
}

// The same packet as a 32-bit pair with no shifted byte: the escape's value is
// 33 bits, and getStreamBits read its fifth byte from past the padding.
func TestDecodePacket_EscapeOf33BitsPastEnd(t *testing.T) {
	t.Parallel()

	decodeMustFail(t, boundsConfig(32, 2), []byte{0x20, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x01, 0xFF})
}

// V's escape needs 17 value bits where the packet has 7.
func TestDecodePacket_EscapeValuePastEnd(t *testing.T) {
	t.Parallel()

	decodeMustFail(t, boundsConfig(16, 2), compressedPair(1, "0"+"111111111"+"0000000"))
}

// V's code is eight ones that end the packet: its stop bit is the padding's.
func TestDecodePacket_StopBitPastEnd(t *testing.T) {
	t.Parallel()

	decodeMustFail(t, boundsConfig(16, 2), compressedPair(1, "0"+"11111111"))
}

// V's zero run (1 0 000, a run of 15) has three of its five bits past the end.
func TestDecodePacket_ZeroRunPastEnd(t *testing.T) {
	t.Parallel()

	decodeMustFail(t, boundsConfig(16, 2), compressedPair(16, "0"+"10000"+"0"+"10"))
}

// A last code that ends exactly at the end of the packet decodes: V's seven
// ones and a stop bit are a residual of 7, the sample -4.
func TestDecodePacket_CodeEndingAtTheEnd(t *testing.T) {
	t.Parallel()

	dec, err := alac.NewPacketDecoder(boundsConfig(16, 2))
	if err != nil {
		t.Fatalf("NewPacketDecoder: %v", err)
	}

	pcm, err := dec.DecodePacket(compressedPair(1, "0"+"11111110"))
	if err != nil {
		t.Fatalf("DecodePacket: %v", err)
	}

	if want := []byte{0x00, 0x00, 0xfc, 0xff}; !bytes.Equal(pcm, want) {
		t.Fatalf("decoded % x, want % x", pcm, want)
	}
}
