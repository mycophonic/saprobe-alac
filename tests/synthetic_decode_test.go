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
	"encoding/binary"
	"os"
	"testing"

	"github.com/mycophonic/saprobe-alac"
)

// generateWhiteNoise creates random PCM data.
func generateWhiteNoise(sampleRate, bitDepth, channels, durationSec int) []byte {
	numSamples := sampleRate * durationSec * channels

	var bytesPerSample int

	switch bitDepth {
	case 4, 8:
		bytesPerSample = 1
	case 12, 16:
		bytesPerSample = 2
	case 20, 24:
		bytesPerSample = 3
	case 32:
		bytesPerSample = 4
	default:
		bytesPerSample = bitDepth / 8
	}

	buf := make([]byte, numSamples*bytesPerSample)

	// Use a simple PRNG for reproducibility.
	seed := uint64(0x12345678)

	for i := range numSamples {
		// xorshift64
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17

		offset := i * bytesPerSample

		switch bitDepth {
		case 4:
			// Scale to signed 4-bit range (-7..+7), stored sign-extended in 1 byte.
			buf[offset] = byte(int8((seed % 14) - 7))
		case 8:
			// Scale to signed 8-bit range (-120..+120), leave some headroom.
			buf[offset] = byte(int8((seed % 240) - 120))
		case 12:
			// Scale to signed 12-bit range (-2000..+2000), stored in 2 bytes LE.
			val := int16((seed % 4000) - 2000)
			binary.LittleEndian.PutUint16(buf[offset:], uint16(val))
		case 16:
			// Scale to 16-bit range, leave some headroom.
			val := int16((seed % 60000) - 30000)
			binary.LittleEndian.PutUint16(buf[offset:], uint16(val))
		case 20:
			// Scale to signed 20-bit range (-500000..+500000), stored in 3 bytes LE.
			val := int32((seed % 1000000) - 500000)
			buf[offset] = byte(val)
			buf[offset+1] = byte(val >> 8)
			buf[offset+2] = byte(val >> 16)
		case 24:
			// Scale to 24-bit range.
			val := int32((seed % 14000000) - 7000000)
			buf[offset] = byte(val)
			buf[offset+1] = byte(val >> 8)
			buf[offset+2] = byte(val >> 16)
		case 32:
			val := int32((seed % 1800000000) - 900000000)
			binary.LittleEndian.PutUint32(buf[offset:], uint32(val))

		default:
		}
	}

	return buf
}

// compareLosslessSamples requires exact byte match for lossless codecs.
// The label identifies which comparison is being made (e.g. "saprobe vs ffmpeg").
func compareLosslessSamples(t *testing.T, label string, expected, actual []byte, bitDepth, channels int) {
	t.Helper()

	minLen := min(len(expected), len(actual))
	differences := 0
	firstDiff := -1

	for i := range minLen {
		if expected[i] != actual[i] {
			differences++

			if firstDiff == -1 {
				firstDiff = i
			}
		}
	}

	if differences > 0 {
		bytesPerSample := pcmBytesPerSample(bitDepth)
		sampleIndex := firstDiff / bytesPerSample / channels
		t.Errorf("%s: PCM mismatch: %d differing bytes (%.2f%%), first diff at byte %d (sample %d)",
			label, differences, float64(differences)/float64(minLen)*100, firstDiff, sampleIndex)

		showDiffs(t, label, expected, actual, bitDepth, channels, 5)
	}
}

// pcmBytesPerSample returns the number of bytes per sample for a given bit depth.
func pcmBytesPerSample(bitDepth int) int {
	switch bitDepth {
	case 4, 8:
		return 1
	case 12, 16:
		return 2
	case 20, 24:
		return 3
	case 32:
		return 4
	default:
		return bitDepth / 8
	}
}

// showDiffs prints the first N differing samples for debugging.
func showDiffs(t *testing.T, label string, expected, actual []byte, bitDepth, channels, maxDiffs int) {
	t.Helper()

	bytesPerSample := pcmBytesPerSample(bitDepth)
	frameSize := bytesPerSample * channels
	shown := 0

	for i := 0; i < min(len(expected), len(actual))-frameSize && shown < maxDiffs; i += frameSize {
		expectedFrame := expected[i : i+frameSize]
		actualFrame := actual[i : i+frameSize]

		if !bytes.Equal(expectedFrame, actualFrame) {
			sampleIdx := i / frameSize
			t.Logf("%s: sample %d: expected=%v, actual=%v", label, sampleIdx, expectedFrame, actualFrame)

			shown++
		}
	}
}

// decodeSaprobe decodes an encoded file using the saprobe (pure Go) decoder.
func decodeSaprobe(path string) ([]byte, alac.PCMFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, alac.PCMFormat{}, err
	}
	defer f.Close()

	return alac.Decode(f)
}
