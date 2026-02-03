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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mycophonic/agar/pkg/agar"

	"github.com/mycophonic/saprobe-alac"
	"github.com/mycophonic/saprobe-alac/tests/testutil"
)

// encoderType identifies an encoder.
type encoderType int

const (
	encoderFFmpeg encoderType = iota
	encoderCoreAudio
	encoderAlacconvert
)

// decoderType identifies a decoder.
type decoderType int

const (
	decoderSaprobe decoderType = iota
	decoderFFmpeg
	decoderCoreAudio
	decoderAlacconvert
)

// Bit depths testable via ffmpeg and CoreAudio encoders (only s16p and s32p supported).
// The spec also defines 20-bit and 32-bit, but no available encoder produces them.
//
//nolint:gochecknoglobals
var bitDepths = []int{16, 24}

// sampleRates covers the full range of commonly used sample rates.
//
//nolint:gochecknoglobals
var sampleRates = []int{
	8000, 11025, 16000, 22050, 32000, 44100, 48000, 88200, 96000, 176400, 192000,
}

// channelCounts covers all supported channel counts (1 through 8).
//
//nolint:gochecknoglobals
var channelCounts = []int{1, 2, 3, 4, 5, 6, 7, 8}

// channelLayout returns the ffmpeg channel layout name for the given count.
// Without explicit layout, ffmpeg may guess wrong
// (e.g., 3ch → "2.1" instead of "3.0") and silently downmix.
func channelLayout(channels int) string {
	switch channels {
	case 1:
		return "mono"
	case 2:
		return "stereo"
	case 3:
		return "3.0"
	case 4:
		return "4.0"
	case 5:
		return "5.0"
	case 6:
		return "5.1"
	case 7:
		return "6.1(back)"
	case 8:
		return "7.1(wide)"
	default:
		return ""
	}
}

// multichannelRemapped reports whether multichannel has different
// channel ordering between raw PCM input and encoded/decoded output.
// Encoders (ffmpeg, CoreAudio) apply channel layout mapping for >2ch,
// which reorders channels compared to the raw interleaved PCM input.
// Different tools may use different conventions, so we skip byte-level
// comparisons for multichannel and verify output length only.
func multichannelRemapped(channels int) bool {
	return channels > 2
}

func encoderName(enc encoderType) string {
	switch enc {
	case encoderFFmpeg:
		return "ffmpeg"
	case encoderCoreAudio:
		return "coreaudio"
	case encoderAlacconvert:
		return "alacconvert"
	default:
		return "unknown"
	}
}

func decoderName(dec decoderType) string {
	switch dec {
	case decoderSaprobe:
		return "saprobe"
	case decoderFFmpeg:
		return "ffmpeg"
	case decoderCoreAudio:
		return "coreaudio"
	case decoderAlacconvert:
		return "alacconvert"
	default:
		return "unknown"
	}
}

// availableEncoders returns which encoders support the given channel count.
// ffmpeg is always available (assumed test dependency).
// CoreAudio requires alac-coreaudio to be built.
// alacconvert encodes WAV to CAF but only supports mono and stereo.
//

func availableEncoders(channels int, hasCoreAudio, hasAlacconvert bool) []encoderType {
	encs := []encoderType{encoderFFmpeg}

	if hasCoreAudio {
		encs = append(encs, encoderCoreAudio)
	}

	if hasAlacconvert && channels <= 2 {
		encs = append(encs, encoderAlacconvert)
	}

	return encs
}

// decodersForEncoder returns the decoders compatible with an encoder's output container.
//
// Container compatibility:
//
//	ffmpeg      → M4A: saprobe, ffmpeg, coreaudio
//	coreaudio   → M4A: saprobe, ffmpeg, coreaudio
//	alacconvert → CAF: ffmpeg, alacconvert

func decodersForEncoder(enc encoderType, hasCoreAudio, hasAlacconvert bool) []decoderType {
	switch enc {
	case encoderFFmpeg, encoderCoreAudio:
		decs := []decoderType{decoderSaprobe, decoderFFmpeg}
		if hasCoreAudio {
			decs = append(decs, decoderCoreAudio)
		}

		return decs
	case encoderAlacconvert:
		decs := []decoderType{decoderFFmpeg}
		if hasAlacconvert {
			decs = append(decs, decoderAlacconvert)
		}

		return decs
	default:
		return nil
	}
}

// TestConformance tests all bit depth × encoder × sample rate × channel combinations.
// Each encoder produces an encoded file which is decoded by all compatible decoders.
// Decoded outputs are compared against source PCM (≤2ch) and cross-compared against
// each other to verify lossless round-trip correctness.
func TestConformance(t *testing.T) {
	t.Parallel()

	hasCoreAudio := testutil.CoreAudioPath(t) != ""
	hasAlacconvert := testutil.AlacConvertPath(t) != ""

	for _, bitDepth := range bitDepths {
		for _, sampleRate := range sampleRates {
			for _, channels := range channelCounts {
				encoders := availableEncoders(channels, hasCoreAudio, hasAlacconvert)

				for _, enc := range encoders {
					name := fmt.Sprintf("%dbit/%s/%dHz_%dch",
						bitDepth, encoderName(enc), sampleRate, channels)

					t.Run(name, func(t *testing.T) {
						t.Parallel()

						decoders := decodersForEncoder(enc, hasCoreAudio, hasAlacconvert)

						runConformanceTest(t, enc, decoders,
							bitDepth, sampleRate, channels)
					})
				}
			}
		}
	}
}

//nolint:cyclop // Test orchestration requires many steps.
func runConformanceTest(
	t *testing.T,
	enc encoderType,
	decoders []decoderType,
	bitDepth, sampleRate, channels int,
) {
	t.Helper()

	tmpDir := t.TempDir()

	// Generate source PCM (white noise, 1 second).
	srcPCM := agar.GenerateWhiteNoise(sampleRate, bitDepth, channels, 1)

	// Encode.
	encPath := runEncode(t, enc, srcPCM, tmpDir, bitDepth, sampleRate, channels)

	skipSourceCompare := multichannelRemapped(channels)

	// Decode with every compatible decoder.
	decoded := make(map[string][]byte, len(decoders))

	for _, dec := range decoders {
		decName := decoderName(dec)
		pcm, format := runDecode(t, dec, encPath, tmpDir, bitDepth, channels)

		// Verify format metadata (saprobe decoder only — others return nil format).
		if dec == decoderSaprobe && format != nil {
			if format.SampleRate != sampleRate {
				t.Errorf("sample rate: got %d, want %d", format.SampleRate, sampleRate)
			}

			if format.BitDepth != bitDepth {
				t.Errorf("bit depth: got %d, want %d", format.BitDepth, bitDepth)
			}

			if format.Channels != channels {
				t.Errorf("channels: got %d, want %d", format.Channels, channels)
			}
		}

		// Compare decoded PCM vs original source.
		if !skipSourceCompare {
			label := fmt.Sprintf("decode(%s) vs source", decName)

			if len(srcPCM) != len(pcm) {
				t.Errorf("%s length mismatch: source=%d, decoded=%d", label, len(srcPCM), len(pcm))
			}

			agar.CompareLosslessSamples(t, label, srcPCM, pcm, bitDepth, channels)
		}

		decoded[decName] = pcm
	}

	// Multichannel: verify output length only (channel remapping prevents byte comparison).
	if skipSourceCompare {
		for decName, pcm := range decoded {
			if len(srcPCM) != len(pcm) {
				t.Errorf("decode(%s) length mismatch: expected=%d, got=%d", decName, len(srcPCM), len(pcm))
			}
		}

		return
	}

	// Cross-compare all decoder outputs against each other.
	decoderNames := make([]string, 0, len(decoded))
	for name := range decoded {
		decoderNames = append(decoderNames, name)
	}

	for idx := range decoderNames {
		for jdx := idx + 1; jdx < len(decoderNames); jdx++ {
			nameA := decoderNames[idx]
			nameB := decoderNames[jdx]
			label := fmt.Sprintf("decode(%s) vs decode(%s)", nameA, nameB)

			if len(decoded[nameA]) != len(decoded[nameB]) {
				t.Errorf("%s length mismatch: %s=%d, %s=%d",
					label, nameA, len(decoded[nameA]), nameB, len(decoded[nameB]))
			}

			agar.CompareLosslessSamples(t, label, decoded[nameA], decoded[nameB], bitDepth, channels)
		}
	}
}

// runEncode encodes source PCM with the specified encoder and returns the encoded file path.
func runEncode(
	t *testing.T,
	enc encoderType,
	srcPCM []byte,
	tmpDir string,
	bitDepth, sampleRate, channels int,
) string {
	t.Helper()

	switch enc {
	case encoderFFmpeg:
		srcPath := filepath.Join(tmpDir, "source.raw")
		if err := os.WriteFile(srcPath, srcPCM, 0o600); err != nil {
			t.Fatalf("write source: %v", err)
		}

		encPath := filepath.Join(tmpDir, "encoded.m4a")

		sampleFmt := "s16p"
		if bitDepth == 24 {
			sampleFmt = "s32p"
		}

		var inputArgs []string
		if layout := channelLayout(channels); layout != "" {
			inputArgs = []string{"-channel_layout", layout}
		}

		agar.FFmpegEncode(t, agar.FFmpegEncodeOptions{
			Src:        srcPath,
			Dst:        encPath,
			BitDepth:   bitDepth,
			SampleRate: sampleRate,
			Channels:   channels,
			CodecArgs:  []string{"-c:a", "alac", "-sample_fmt", sampleFmt},
			InputArgs:  inputArgs,
		})

		return encPath

	case encoderCoreAudio:
		wavPath := filepath.Join(tmpDir, "source.wav")
		testutil.WriteWAV(t, wavPath, srcPCM, bitDepth, sampleRate, channels)

		encPath := filepath.Join(tmpDir, "encoded.m4a")

		testutil.CoreAudio(t, testutil.CoreAudioOptions{
			Args: []string{"encode", wavPath, encPath},
		})

		return encPath

	case encoderAlacconvert:
		wavPath := filepath.Join(tmpDir, "source.wav")
		testutil.WriteWAV(t, wavPath, srcPCM, bitDepth, sampleRate, channels)

		encPath := filepath.Join(tmpDir, "encoded.caf")

		testutil.AlacConvert(t, testutil.AlacConvertOptions{
			Args: []string{wavPath, encPath},
		})

		return encPath

	default:
		t.Fatalf("unknown encoder type: %d", enc)

		return ""
	}
}

// runDecode decodes an encoded file with the specified decoder.
// Returns raw PCM bytes and (for saprobe only) the decoded format metadata.
func runDecode(
	t *testing.T,
	dec decoderType,
	encPath, tmpDir string,
	bitDepth, channels int,
) ([]byte, *alac.PCMFormat) {
	t.Helper()

	switch dec {
	case decoderSaprobe:
		pcm, format, err := decodeSaprobe(encPath)
		if err != nil {
			t.Fatalf("saprobe decode: %v", err)
		}

		return pcm, &format

	case decoderFFmpeg:
		var args []string
		if layout := channelLayout(channels); layout != "" {
			args = []string{"-channel_layout", layout}
		}

		return agar.FFmpegDecode(t, agar.FFmpegDecodeOptions{
			Src:      encPath,
			BitDepth: bitDepth,
			Channels: channels,
			Args:     args,
		}), nil

	case decoderCoreAudio:
		encoded, err := os.ReadFile(encPath)
		if err != nil {
			t.Fatalf("read encoded file: %v", err)
		}

		pcm, err := agar.CoreAudioDecode(encoded)
		if err != nil {
			t.Fatalf("coreaudio decode: %v", err)
		}

		return pcm, nil

	case decoderAlacconvert:
		wavPath := filepath.Join(tmpDir, "alacconvert_decoded.wav")

		testutil.AlacConvert(t, testutil.AlacConvertOptions{
			Args: []string{encPath, wavPath},
		})

		return testutil.ReadWAVPCMData(t, wavPath), nil

	default:
		t.Fatalf("unknown decoder type: %d", dec)

		return nil, nil
	}
}
