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

package testutil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mycophonic/agar/pkg/agar"
	"github.com/mycophonic/primordium/filesystem"
)

const alacConvertBinary = "alacconvert"

// AlacConvertPath returns the path to the alacconvert binary, or an empty
// string if it is not available. Build it with: just alacconvert.
func AlacConvertPath(t *testing.T) string {
	t.Helper()

	path, err := agar.LookFor(alacConvertBinary)
	if err != nil {
		t.Log("alacconvert not found: run 'just alacconvert' to enable Apple reference tests")

		return ""
	}

	return path
}

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

	cmd := exec.CommandContext(t.Context(), bin, opts.Args...) // #nosec G204 -- arguments are test-controlled

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

	data, err := os.ReadFile(path) // #nosec G304 -- the path is the test harness's
	if err != nil {
		t.Fatalf("read WAV %s: %v", path, err)
	}

	const riffFormHeaderSize = 12 // RIFF chunk header + "WAVE" form tag; chunks start after this.

	for i := riffFormHeaderSize; i+8 <= len(data); i++ {
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

// The canonical PCM WAV header: the RIFF chunk header (8), the "WAVE" form
// tag (4), the fmt chunk (24) and the data chunk header (8). The RIFF size
// field counts the bytes after itself: the file minus the RIFF chunk header.
const (
	wavHeaderSize       = 44
	riffChunkHeaderSize = 8
)

// WriteWAV writes a standard PCM WAV file from raw PCM data.
// alacconvert requires WAV input for encoding (it cannot read raw PCM).
func WriteWAV(t *testing.T, path string, pcm []byte, bitDepth, sampleRate, channels int) {
	t.Helper()

	bytesPerSample := bitDepth / 8
	blockAlign := channels * bytesPerSample
	byteRate := sampleRate * blockAlign
	dataSize := len(pcm)

	buf := make([]byte, wavHeaderSize+dataSize)

	// RIFF header.
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(wavHeaderSize-riffChunkHeaderSize+dataSize))
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
	copy(buf[wavHeaderSize:], pcm)

	if err := os.WriteFile(path, buf, filesystem.FilePermissionsPrivate); err != nil {
		t.Fatalf("write WAV %s: %v", path, err)
	}
}

// BenchDecodeAlacconvert benchmarks alacconvert decoding a CAF to WAV.
func BenchDecodeAlacconvert(
	t *testing.T, format agar.BenchFormat, opts agar.BenchOptions, cafPath string,
) agar.BenchResult {
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

	return agar.ComputeResult(format, "alacconvert", "decode", durations, int(info.Size()))
}
