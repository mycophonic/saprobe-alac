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
	"os"
	"path/filepath"
	"testing"

	"github.com/mycophonic/agar/pkg/agar"
	"github.com/mycophonic/agar/pkg/coreaudio"

	alac "github.com/mycophonic/saprobe-alac"
	"github.com/mycophonic/saprobe-alac/tests/testutil"
)

// TestProfileDecode runs saprobe-only decoding for clean pprof profiling.
// Use with: hack/bench.sh TestProfileDecode
//
//nolint:paralleltest // Profile must run sequentially for accurate sampling.
func TestProfileDecode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping profile in short mode")
	}

	if testutil.CoreAudioPath(t) == "" {
		t.Skip("alac-coreaudio required to encode test data: run 'make alac-coreaudio'")
	}

	opts := agar.BenchOptions{}.WithDefaults()
	tmpDir := t.TempDir()

	var results []agar.BenchResult

	for _, dur := range benchDurations {
		durationSec := int(dur.Seconds())

		for _, bf := range benchFormats {
			bf.Name = fmt.Sprintf("%s %ds", bf.Name, durationSec)
			t.Logf("=== %s ===", bf.Name)

			srcPCM := agar.GenerateWhiteNoise(bf.SampleRate, bf.BitDepth, bf.Channels, durationSec)

			m4aPath := filepath.Join(tmpDir, fmt.Sprintf("enc_%d_%d_%ds.m4a", bf.SampleRate, bf.BitDepth, durationSec))

			testutil.CoreAudioEncode(t, srcPCM, coreaudio.Format{
				SampleRate: bf.SampleRate,
				BitDepth:   bf.BitDepth,
				Channels:   bf.Channels,
			}, m4aPath)

			encoded, err := os.ReadFile(m4aPath)
			if err != nil {
				t.Fatalf("read encoded: %v", err)
			}

			t.Logf("  M4A size: %.1f MB (%d bytes)", float64(len(encoded))/(1024*1024), len(encoded))

			results = append(results, benchDecodeSaprobe(t, bf, opts, encoded))
		}
	}

	agar.PrintResults(t, opts, results)
}

// TestProfileDecodeFile runs saprobe-only decoding of a real file for clean pprof profiling.
// Set BENCH_FILE to an M4A file path.
// Use with: hack/bench.sh TestProfileDecodeFile /path/to/file.m4a
//
//nolint:paralleltest // Profile must run sequentially for accurate sampling.
func TestProfileDecodeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping profile in short mode")
	}

	filePath := os.Getenv("BENCH_FILE")
	if filePath == "" {
		t.Skip("set BENCH_FILE to run this profile")
	}

	opts := agar.BenchOptions{}.WithDefaults()

	encoded, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	t.Logf("File: %s (%.1f MB)", filePath, float64(len(encoded))/(1024*1024))

	probeDec, probeErr := alac.NewDecoder(bytes.NewReader(encoded))
	if probeErr != nil {
		t.Fatalf("probe decode: %v", probeErr)
	}

	pcmFormat := probeDec.Format()

	bf := agar.BenchFormat{
		Name:       filepath.Base(filePath),
		SampleRate: pcmFormat.SampleRate,
		BitDepth:   pcmFormat.BitDepth,
		Channels:   pcmFormat.Channels,
	}

	results := []agar.BenchResult{
		benchDecodeSaprobe(t, bf, opts, encoded),
	}

	agar.PrintResults(t, opts, results)
}
