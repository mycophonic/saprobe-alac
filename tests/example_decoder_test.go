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
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mycophonic/agar/pkg/agar"
	"github.com/mycophonic/agar/pkg/coreaudio"

	"github.com/mycophonic/saprobe-alac/tests/testutil"
)

// TestExampleDecoder verifies the alac-example-decoder binary produces
// output identical to the CoreAudio reference decoder.
func TestExampleDecoder(t *testing.T) {
	t.Parallel()

	if testutil.CoreAudioPath(t) == "" {
		t.Skip("alac-coreaudio not available")
	}

	tmpDir := t.TempDir()

	// Build the example decoder binary.
	decoderBin := filepath.Join(tmpDir, "alac-example-decoder")

	build := exec.CommandContext(context.Background(), "go", "build", "-o", decoderBin, "./cmd/alac-example-decoder")
	build.Dir = ".."

	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build alac-example-decoder: %v\n%s", err, out)
	}

	// Generate synthetic PCM (16-bit, 44100 Hz, stereo, 1 second).
	const (
		bitDepth   = 16
		sampleRate = 44100
		channels   = 2
	)

	srcPCM := agar.GenerateWhiteNoise(sampleRate, bitDepth, channels, 1)

	// Encode to M4A via CoreAudio binary.
	m4aPath := filepath.Join(tmpDir, "encoded.m4a")

	testutil.CoreAudioEncode(t, srcPCM, coreaudio.Format{
		SampleRate: sampleRate,
		BitDepth:   bitDepth,
		Channels:   channels,
	}, m4aPath)

	// Decode with CoreAudio as reference (CGO, in-process).
	m4aData, err := os.ReadFile(m4aPath)
	if err != nil {
		t.Fatalf("read encoded: %v", err)
	}

	refPCM, _, err := coreaudio.NewCGO().Decode(m4aData)
	if err != nil {
		t.Fatalf("coreaudio decode: %v", err)
	}

	// Decode with example decoder in PCM mode.
	var exStdout, exStderr bytes.Buffer

	decCmd := exec.CommandContext(context.Background(), decoderBin, "-format", "pcm", m4aPath)
	decCmd.Stdout = &exStdout
	decCmd.Stderr = &exStderr

	if err := decCmd.Run(); err != nil {
		t.Fatalf("example decoder: %v\n%s", err, exStderr.String())
	}

	exPCM := exStdout.Bytes()

	// Compare lengths.
	if len(refPCM) != len(exPCM) {
		t.Fatalf("PCM length mismatch: coreaudio=%d, example-decoder=%d", len(refPCM), len(exPCM))
	}

	// Compare sample data.
	agar.CompareLosslessSamples(t, "example-decoder vs coreaudio", refPCM, exPCM, bitDepth, channels)

	// Also verify WAV output mode produces valid output with correct size.
	var wavStdout, wavStderr bytes.Buffer

	wavCmd := exec.CommandContext(context.Background(), decoderBin, "-format", "wav", m4aPath)
	wavCmd.Stdout = &wavStdout
	wavCmd.Stderr = &wavStderr

	if err := wavCmd.Run(); err != nil {
		t.Fatalf("example decoder (wav): %v\n%s", err, wavStderr.String())
	}

	// WAV output should be 44-byte header + PCM data.
	expectedWAVSize := 44 + len(refPCM)
	if wavStdout.Len() != expectedWAVSize {
		t.Errorf("WAV output size: got %d, want %d", wavStdout.Len(), expectedWAVSize)
	}

	// Extract PCM from WAV output and compare.
	wavOutPath := filepath.Join(tmpDir, "output.wav")

	if err := os.WriteFile(wavOutPath, wavStdout.Bytes(), 0o600); err != nil {
		t.Fatalf("write WAV output: %v", err)
	}

	wavPCM := testutil.ReadWAVPCMData(t, wavOutPath)

	agar.CompareLosslessSamples(t, "example-decoder(wav) vs coreaudio", refPCM, wavPCM, bitDepth, channels)
}
