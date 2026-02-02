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

//nolint:gosec // Integer conversions bounded by audio format constraints; file paths from test harness.
package testutil

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const alacConvertBinary = "alacconvert"

// AlacConvertOptions configures an alacconvert invocation.
type AlacConvertOptions struct {
	// Args are passed directly to the alacconvert binary.
	Args []string
	// Stdin is connected to the command's standard input when non-nil.
	Stdin io.Reader
	// Stdout receives the command's standard output when non-nil.
	// When nil, stdout is captured and returned in AlacConvertResult.Stdout.
	Stdout io.Writer
	// Stderr receives the command's standard error when non-nil.
	// When nil, stderr is captured and included in the fatal message on failure.
	Stderr io.Writer
}

// AlacConvertResult holds captured output from an alacconvert invocation.
type AlacConvertResult struct {
	// Stdout contains captured standard output, populated only when
	// AlacConvertOptions.Stdout was nil.
	Stdout []byte
}

// AlacConvert runs alacconvert with the given options.
// It fatals the test if alacconvert cannot be found or the command returns an error.
func AlacConvert(t *testing.T, opts AlacConvertOptions) AlacConvertResult {
	t.Helper()

	bin := AlacConvertPath(t)
	if bin == "" {
		t.Log(alacConvertBinary + ": not found")
		t.FailNow()
	}

	//nolint:gosec // arguments are test-controlled
	cmd := exec.CommandContext(context.Background(), bin, opts.Args...)

	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}

	var stdoutBuf bytes.Buffer

	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = &stdoutBuf
	}

	var stderrBuf bytes.Buffer

	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = &stderrBuf
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("alacconvert: %v\n%s", err, stderrBuf.String())
	}

	return AlacConvertResult{
		Stdout: stdoutBuf.Bytes(),
	}
}

// ReadWAVPCMData reads a WAV file and returns only the raw PCM data.
// It searches for the "data" chunk rather than assuming a fixed header size.
func ReadWAVPCMData(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read WAV %s: %v", path, err)
	}

	const wavHeaderSize = 12 // RIFF header + "WAVE" tag; chunks start after this.

	for i := wavHeaderSize; i+8 <= len(data); i++ {
		if string(data[i:i+4]) == "data" {
			chunkSize := int(data[i+4]) | int(data[i+5])<<8 | int(data[i+6])<<16 | int(data[i+7])<<24
			pcmStart := i + 8

			if pcmStart+chunkSize > len(data) {
				return data[pcmStart:]
			}

			return data[pcmStart : pcmStart+chunkSize]
		}
	}

	t.Fatalf("no data chunk found in WAV file %s", path)

	return nil
}

// WriteWAV writes a standard PCM WAV file from raw PCM data.
// alacconvert requires WAV input for encoding (it cannot read raw PCM).
func WriteWAV(t *testing.T, path string, pcm []byte, bitDepth, sampleRate, channels int) {
	t.Helper()

	bytesPerSample := bitDepth / 8
	blockAlign := channels * bytesPerSample
	byteRate := sampleRate * blockAlign
	dataSize := len(pcm)

	buf := make([]byte, 44+dataSize) //nolint:mnd // WAV header is always 44 bytes.

	// RIFF header.
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataSize))
	copy(buf[8:12], "WAVE")

	// fmt chunk.
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(buf[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(buf[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buf[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(buf[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(buf[34:36], uint16(bitDepth))

	// data chunk.
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))
	copy(buf[44:], pcm)

	const fileMode = 0o600

	if err := os.WriteFile(path, buf, fileMode); err != nil {
		t.Fatalf("write WAV %s: %v", path, err)
	}
}

// BenchDecodeAlacconvert benchmarks alacconvert decoding a CAF to WAV.
func BenchDecodeAlacconvert(
	t *testing.T, format BenchFormat, opts BenchOptions, cafPath string,
) BenchResult {
	t.Helper()

	if AlacConvertPath(t) == "" {
		t.Skip("alacconvert not available")
	}

	opts = opts.WithDefaults()
	tmpDir := t.TempDir()

	durations := make([]time.Duration, opts.Iterations)

	for iter := range opts.Iterations {
		wavOut := filepath.Join(tmpDir, fmt.Sprintf("decoded_%d.wav", iter))

		start := time.Now()

		AlacConvert(t, AlacConvertOptions{
			Args: []string{cafPath, wavOut},
		})

		durations[iter] = time.Since(start)

		_ = os.Remove(wavOut)
	}

	info, err := os.Stat(cafPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	return ComputeResult(format, "alacconvert", "decode", durations, int(info.Size()))
}
