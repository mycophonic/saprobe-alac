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
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mycophonic/agar/pkg/agar"

	"github.com/mycophonic/saprobe-alac"
)

// TestStreamDecoder exercises NewStreamDecoder + Read with a small buffer,
// verifying incremental reading produces identical output to one-shot Decode.
func TestStreamDecoder(t *testing.T) {
	const (
		sampleRate = 44100
		bitDepth   = 16
		channels   = 2
		durationS  = 1
	)

	// Generate source PCM, encode to M4A via ffmpeg.
	srcPCM := agar.GenerateWhiteNoise(sampleRate, bitDepth, channels, durationS)
	tmpDir := t.TempDir()

	srcPath := filepath.Join(tmpDir, "source.raw")
	if err := os.WriteFile(srcPath, srcPCM, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	encPath := filepath.Join(tmpDir, "encoded.m4a")

	agar.FFmpegEncode(t, agar.FFmpegEncodeOptions{
		Src:        srcPath,
		Dst:        encPath,
		BitDepth:   bitDepth,
		SampleRate: sampleRate,
		Channels:   channels,
		CodecArgs:  []string{"-c:a", "alac", "-sample_fmt", "s16p"},
		InputArgs:  []string{"-channel_layout", "stereo"},
	})

	// One-shot decode as reference.
	refFile, err := os.Open(encPath)
	if err != nil {
		t.Fatalf("open for Decode: %v", err)
	}

	refPCM, refFormat, err := alac.Decode(refFile)
	refFile.Close()

	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// Stream decode with a small buffer (deliberately not aligned to frame boundaries).
	streamFile, err := os.Open(encPath)
	if err != nil {
		t.Fatalf("open for StreamDecoder: %v", err)
	}
	defer streamFile.Close()

	sd, err := alac.NewStreamDecoder(streamFile)
	if err != nil {
		t.Fatalf("NewStreamDecoder: %v", err)
	}

	// Verify Format matches.
	streamFormat := sd.Format()
	if streamFormat != refFormat {
		t.Fatalf("Format mismatch: stream=%+v ref=%+v", streamFormat, refFormat)
	}

	// Read with a small buffer (1000 bytes, not aligned to sample boundaries).
	var got bytes.Buffer
	buf := make([]byte, 1000)

	for {
		n, readErr := sd.Read(buf)
		got.Write(buf[:n])

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}

			t.Fatalf("Read: %v", readErr)
		}
	}

	// Compare.
	if !bytes.Equal(got.Bytes(), refPCM) {
		t.Fatalf("StreamDecoder output differs from Decode: stream=%d bytes, ref=%d bytes", got.Len(), len(refPCM))
	}
}
