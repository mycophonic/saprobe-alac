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
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mycophonic/agar/pkg/agar"
	"github.com/mycophonic/agar/pkg/coreaudio"

	"github.com/mycophonic/saprobe-alac"
	"github.com/mycophonic/saprobe-alac/tests/testutil"
)

// =============================================================================
// Encoder/Decoder Types
// =============================================================================

// encoderType identifies an encoder.
type encoderType int

const (
	encoderFFmpeg encoderType = iota
	encoderCoreAudio
	encoderAlacconvert
	encoderNone // For natural files (already encoded).
)

// decoderType identifies a decoder.
type decoderType int

const (
	decoderSaprobe decoderType = iota
	decoderFFmpeg
	decoderCoreAudio
	decoderAlacconvert
)

func encoderName(enc encoderType) string {
	switch enc {
	case encoderFFmpeg:
		return "ffmpeg"
	case encoderCoreAudio:
		return "coreaudio"
	case encoderAlacconvert:
		return "alacconvert"
	case encoderNone:
		return "natural"
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

// =============================================================================
// Channel Layout Handling
// =============================================================================

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

// coreAudioUsesElementOrder reports whether CoreAudio uses different
// channel ordering than ffmpeg/saprobe-alac for multichannel configurations.
//
// CoreAudio (both encoder and decoder) follows Apple's reference implementation,
// which uses MPEG element order:
//
//	5.1: C, L, R, Ls, Rs, LFE (MPEG element order)
//
// In contrast, ffmpeg and saprobe-alac use SMPTE standard order:
//
//	5.1: L, R, C, LFE, Ls, Rs (SMPTE order)
//
// This difference means CoreAudio-encoded or CoreAudio-decoded output
// cannot be byte-compared against source or ffmpeg/saprobe-alac for multichannel.
func coreAudioUsesElementOrder(channels int) bool {
	return channels > 2
}

// =============================================================================
// Encoder/Decoder Capabilities
// =============================================================================

// coreAudioSupported reports whether CoreAudio's ALAC codec handles
// the given bit depth and channel count combination.
//
// Empirically determined limits (macOS AudioToolbox):
//
//	16-bit: 1-7 channels
//	24-bit: 1-5 channels
//
// Beyond these, CoreAudio's ExtAudioFileRead returns 0 frames
// (OSStatus 0) without error, silently producing empty output.
func coreAudioSupported(bitDepth, channels int) bool {
	switch bitDepth {
	case 16:
		return channels <= 7
	case 24:
		return channels <= 5
	default:
		return channels <= 2
	}
}

// availableEncoders returns which encoders support the given configuration.
// ffmpeg is always available (assumed test dependency).
// CoreAudio requires alac-coreaudio and is limited to supported configurations.
// alacconvert encodes WAV to CAF but only supports mono and stereo.
func availableEncoders(bitDepth, channels int, hasCoreAudio, hasAlacconvert bool) []encoderType {
	encs := []encoderType{encoderFFmpeg}

	if hasCoreAudio && coreAudioSupported(bitDepth, channels) {
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
//	ffmpeg      → M4A: saprobe, ffmpeg, coreaudio (within supported matrix)
//	coreaudio   → M4A: saprobe, ffmpeg, coreaudio
//	natural     → M4A: saprobe, ffmpeg, coreaudio (within supported matrix)
//	alacconvert → CAF: ffmpeg, alacconvert
func decodersForEncoder(enc encoderType, bitDepth, channels int, hasCoreAudio, hasAlacconvert bool) []decoderType {
	switch enc {
	case encoderFFmpeg, encoderCoreAudio, encoderNone:
		decs := []decoderType{decoderSaprobe, decoderFFmpeg}
		if hasCoreAudio && coreAudioSupported(bitDepth, channels) {
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

// =============================================================================
// Unified Verification Logic
// =============================================================================

// conformanceInput describes input for conformance verification.
type conformanceInput struct {
	// EncPath is the path to the encoded ALAC file.
	EncPath string

	// SourcePCM is the original PCM data (nil for natural files).
	SourcePCM []byte

	// Encoder identifies who encoded the file (encoderNone for natural files).
	Encoder encoderType

	// Format describes the audio format.
	BitDepth   int
	SampleRate int
	Channels   int
}

// verifyConformance runs unified conformance verification on an encoded file.
// This is the SINGLE verification path used by both synthetic and natural file tests.
//
// Verification performed:
//  1. Decode with all compatible decoders
//  2. Verify format metadata (saprobe decoder)
//  3. If source PCM provided: bit-for-bit comparison against source
//  4. Cross-decoder comparison: bit-for-bit between all decoder pairs
//
// CoreAudio exception: When CoreAudio is involved (as encoder or decoder)
// in multichannel mode, byte comparison is skipped due to different channel
// ordering conventions. Length verification is still performed.
//
//nolint:cyclop // Test orchestration requires many steps.
func verifyConformance(t *testing.T, input conformanceInput) {
	t.Helper()

	hasCoreAudio := testutil.CoreAudioPath(t) != ""
	hasAlacconvert := testutil.AlacConvertPath(t) != ""
	tmpDir := t.TempDir()

	decoders := decodersForEncoder(input.Encoder, input.BitDepth, input.Channels, hasCoreAudio, hasAlacconvert)

	// CoreAudio uses MPEG element order for multichannel.
	// Skip byte comparison when CoreAudio is involved.
	coreAudioSkipByteCompare := coreAudioUsesElementOrder(input.Channels)
	skipSourceCompare := coreAudioSkipByteCompare && input.Encoder == encoderCoreAudio

	// Decode with every compatible decoder.
	decoded := make(map[string][]byte, len(decoders))
	decoderTypes := make(map[string]decoderType, len(decoders))

	for _, dec := range decoders {
		decName := decoderName(dec)
		pcm, format := runDecode(t, dec, input.EncPath, tmpDir, input.BitDepth, input.Channels)

		// Verify format metadata (saprobe decoder only — others return nil format).
		if dec == decoderSaprobe && format != nil {
			if format.SampleRate != input.SampleRate {
				t.Errorf("sample rate: got %d, want %d", format.SampleRate, input.SampleRate)
			}

			if format.BitDepth != input.BitDepth {
				t.Errorf("bit depth: got %d, want %d", format.BitDepth, input.BitDepth)
			}

			if format.Channels != input.Channels {
				t.Errorf("channels: got %d, want %d", format.Channels, input.Channels)
			}
		}

		// Compare decoded PCM vs original source (if provided).
		if input.SourcePCM != nil {
			skipDecoderByteCompare := coreAudioSkipByteCompare && dec == decoderCoreAudio

			if len(input.SourcePCM) != len(pcm) {
				t.Errorf("decode(%s) vs source length mismatch: source=%d, decoded=%d",
					decName, len(input.SourcePCM), len(pcm))
			} else if !skipSourceCompare && !skipDecoderByteCompare {
				label := fmt.Sprintf("decode(%s) vs source", decName)
				agar.CompareLosslessSamples(t, label, input.SourcePCM, pcm, input.BitDepth, input.Channels)
			}
		}

		decoded[decName] = pcm
		decoderTypes[decName] = dec
	}

	// Cross-compare all decoder outputs against each other (bit-for-bit).
	decoderNames := make([]string, 0, len(decoded))
	for name := range decoded {
		decoderNames = append(decoderNames, name)
	}

	for idx := range decoderNames {
		for jdx := idx + 1; jdx < len(decoderNames); jdx++ {
			nameA := decoderNames[idx]
			nameB := decoderNames[jdx]

			// Skip byte comparison if either decoder is CoreAudio in multichannel mode.
			if coreAudioSkipByteCompare &&
				(decoderTypes[nameA] == decoderCoreAudio || decoderTypes[nameB] == decoderCoreAudio) {
				// Still verify length.
				if len(decoded[nameA]) != len(decoded[nameB]) {
					t.Errorf("decode(%s) vs decode(%s) length mismatch: %d vs %d",
						nameA, nameB, len(decoded[nameA]), len(decoded[nameB]))
				}

				continue
			}

			label := fmt.Sprintf("decode(%s) vs decode(%s)", nameA, nameB)

			if len(decoded[nameA]) != len(decoded[nameB]) {
				t.Errorf("%s length mismatch: %s=%d, %s=%d",
					label, nameA, len(decoded[nameA]), nameB, len(decoded[nameB]))

				continue
			}

			agar.CompareLosslessSamples(t, label, decoded[nameA], decoded[nameB], input.BitDepth, input.Channels)
		}
	}

	// Verify saprobe seek functionality.
	// Use saprobe's full decode output as reference.
	if saprobePCM, ok := decoded["saprobe"]; ok {
		verifySeek(t, input.EncPath, saprobePCM, input.BitDepth, input.Channels)
	}
}

// verifySeek tests that seeking produces correct PCM at various positions.
// It compares decoded bytes after seek against the corresponding offset in reference PCM.
func verifySeek(t *testing.T, encPath string, referencePCM []byte, bitDepth, channels int) {
	t.Helper()

	// Open file for seeking test.
	fileData, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("read file for seek test: %v", err)
	}

	dec, err := alac.NewDecoder(bytes.NewReader(fileData))
	if err != nil {
		t.Fatalf("create decoder for seek test: %v", err)
	}

	duration := dec.Duration()
	if duration == 0 {
		return // Empty file, nothing to test.
	}

	// Test seek at various positions: 0%, 25%, 50%, 75%.
	seekPositions := []float64{0, 0.25, 0.50, 0.75}

	// Bytes per sample frame (all channels).
	bytesPerFrame := (bitDepth / 8) * channels

	// Read buffer - enough for several frames.
	readSize := bytesPerFrame * 1024
	buf := make([]byte, readSize)

	for _, pct := range seekPositions {
		seekTime := time.Duration(float64(duration) * pct)

		actualTime, seekErr := dec.Seek(seekTime)
		if seekErr != nil {
			t.Errorf("seek to %.0f%%: %v", pct*100, seekErr)

			continue
		}

		// Calculate expected byte offset in reference PCM.
		// actualTime is packet-aligned, so use it for offset calculation.
		// Use rounding to avoid floating-point precision errors.
		sampleRate := dec.Format().SampleRate
		frameOffset := int(math.Round(actualTime.Seconds() * float64(sampleRate)))
		byteOffset := frameOffset * bytesPerFrame

		if byteOffset >= len(referencePCM) {
			continue // Seeked past end, nothing to verify.
		}

		// Read some data from seeked position.
		toRead := min(readSize, len(referencePCM)-byteOffset)
		nread, readErr := dec.Read(buf[:toRead])

		if readErr != nil && nread == 0 {
			t.Errorf("seek to %.0f%% read: %v", pct*100, readErr)

			continue
		}

		// Compare against reference.
		expected := referencePCM[byteOffset : byteOffset+nread]

		if !bytes.Equal(buf[:nread], expected) {
			t.Errorf("seek to %.0f%% (time=%v, offset=%d): decoded bytes don't match reference",
				pct*100, actualTime, byteOffset)

			// Find first difference for debugging.
			for idx := range nread {
				if buf[idx] != expected[idx] {
					t.Errorf("  first diff at byte %d: got 0x%02x, want 0x%02x",
						idx, buf[idx], expected[idx])

					break
				}
			}
		}
	}
}

// =============================================================================
// Encoding Helpers
// =============================================================================

// runEncode encodes source PCM with the specified encoder and returns the encoded file path.
func runEncode(
	t *testing.T,
	enc encoderType,
	srcPCM []byte,
	tmpDir string,
	bitDepth, sampleRate, channels int,
) string {
	t.Helper()

	//nolint:exhaustive
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
		encPath := filepath.Join(tmpDir, "encoded.m4a")

		testutil.CoreAudioEncode(t, srcPCM, coreaudio.Format{
			SampleRate: sampleRate,
			BitDepth:   bitDepth,
			Channels:   channels,
		}, encPath)

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

		pcm, _, err := coreaudio.NewCGO().Decode(encoded)
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

// =============================================================================
// Synthetic Test Matrix
// =============================================================================

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

// TestConformance tests all bit depth × encoder × sample rate × channel combinations.
// Each encoder produces an encoded file which is decoded by all compatible decoders.
// Decoded outputs are compared against source PCM and cross-compared against
// each other to verify lossless round-trip correctness.
//
// Uses the unified verifyConformance logic.
func TestConformance(t *testing.T) {
	t.Parallel()

	hasCoreAudio := testutil.CoreAudioPath(t) != ""
	hasAlacconvert := testutil.AlacConvertPath(t) != ""

	for _, bitDepth := range bitDepths {
		for _, sampleRate := range sampleRates {
			for _, channels := range channelCounts {
				encoders := availableEncoders(bitDepth, channels, hasCoreAudio, hasAlacconvert)

				for _, enc := range encoders {
					name := fmt.Sprintf("%dbit/%s/%dHz_%dch",
						bitDepth, encoderName(enc), sampleRate, channels)

					t.Run(name, func(t *testing.T) {
						t.Parallel()

						tmpDir := t.TempDir()

						// Generate source PCM (white noise, 1 second).
						srcPCM := agar.GenerateWhiteNoise(sampleRate, bitDepth, channels, 1)

						// Encode.
						encPath := runEncode(t, enc, srcPCM, tmpDir, bitDepth, sampleRate, channels)

						// Verify using unified conformance logic.
						verifyConformance(t, conformanceInput{
							EncPath:    encPath,
							SourcePCM:  srcPCM,
							Encoder:    enc,
							BitDepth:   bitDepth,
							SampleRate: sampleRate,
							Channels:   channels,
						})
					})
				}
			}
		}
	}
}

// =============================================================================
// Natural File Tests
// =============================================================================

// TestConformanceNatural tests conformance on natural (real) ALAC files.
// Set CONFORMANCE_NATURAL_DIR to a directory containing M4A files to run.
//
// Verification is identical to synthetic tests:
// - Bit-for-bit comparison between saprobe and ffmpeg decoder outputs
// - CoreAudio comparison where supported (mono/stereo only for byte comparison)
//
// No source PCM comparison (original uncompressed audio not available).
func TestConformanceNatural(t *testing.T) {
	t.Parallel()

	naturalDir := os.Getenv("CONFORMANCE_NATURAL_DIR")
	if naturalDir == "" {
		t.Skip("set CONFORMANCE_NATURAL_DIR to run natural file conformance tests")
	}

	// Discover M4A files in the directory.
	entries, err := os.ReadDir(naturalDir)
	if err != nil {
		t.Fatalf("read natural dir: %v", err)
	}

	var files []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) == ".m4a" {
			files = append(files, filepath.Join(naturalDir, name))
		}
	}

	if len(files) == 0 {
		t.Skip("no M4A files found in CONFORMANCE_NATURAL_DIR")
	}

	for _, filePath := range files {
		t.Run(filepath.Base(filePath), func(t *testing.T) {
			t.Parallel()

			// Probe format using saprobe decoder.
			f, err := os.Open(filePath)
			if err != nil {
				t.Fatalf("open file: %v", err)
			}

			dec, err := alac.NewDecoder(f)
			f.Close()

			if err != nil {
				t.Fatalf("probe format: %v", err)
			}

			format := dec.Format()

			// Verify using unified conformance logic.
			// No source PCM — we don't have the original uncompressed audio.
			verifyConformance(t, conformanceInput{
				EncPath:    filePath,
				SourcePCM:  nil, // No source comparison for natural files.
				Encoder:    encoderNone,
				BitDepth:   format.BitDepth,
				SampleRate: format.SampleRate,
				Channels:   format.Channels,
			})
		})
	}
}
