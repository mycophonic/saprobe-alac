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
	"time"

	"github.com/mycophonic/agar/pkg/agar"

	"github.com/mycophonic/saprobe-alac"
	"github.com/mycophonic/saprobe-alac/tests/testutil"
)

// Audio formats to benchmark.
//
//nolint:gochecknoglobals
var benchFormats = []testutil.BenchFormat{
	{Name: "CD 44.1kHz/16bit", SampleRate: 44100, BitDepth: 16, Channels: 2},
	{Name: "HiRes 96kHz/24bit", SampleRate: 96000, BitDepth: 24, Channels: 2},
}

// Audio durations to benchmark: short captures decoder overhead,
// long measures sustained throughput.
//
//nolint:gochecknoglobals
var benchDurations = []time.Duration{
	10 * time.Second,
	5 * time.Minute,
}

// TestBenchmarkDecode benchmarks decoding across all available decoders.
// Each format is tested at multiple audio durations (short and long) to measure
// both decoder overhead and sustained throughput.
// All synthetic files are encoded with CoreAudio (requires alac-coreaudio).
//
//nolint:paralleltest // Benchmark must run sequentially for accurate timing.
func TestBenchmarkDecode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark in short mode")
	}

	if testutil.CoreAudioPath(t) == "" {
		t.Skip("alac-coreaudio required for benchmarks: run 'make alac-coreaudio'")
	}

	opts := testutil.BenchOptions{}.WithDefaults()
	tmpDir := t.TempDir()
	hasAlacconvert := testutil.AlacConvertPath(t) != ""

	var results []testutil.BenchResult

	for _, dur := range benchDurations {
		durationSec := int(dur.Seconds())

		for _, bf := range benchFormats {
			// Tag the format name with duration for results display.
			bf.Name = fmt.Sprintf("%s %ds", bf.Name, durationSec)

			t.Logf("=== %s ===", bf.Name)

			srcPCM := agar.GenerateWhiteNoise(bf.SampleRate, bf.BitDepth, bf.Channels, durationSec)

			// Write WAV for encoding.
			wavPath := filepath.Join(tmpDir, fmt.Sprintf("src_%d_%d_%ds.wav", bf.SampleRate, bf.BitDepth, durationSec))
			testutil.WriteWAV(t, wavPath, srcPCM, bf.BitDepth, bf.SampleRate, bf.Channels)

			t.Logf("  PCM size: %.1f MB (%d bytes)", float64(len(srcPCM))/(1024*1024), len(srcPCM))

			// Encode to M4A via CoreAudio.
			m4aPath := filepath.Join(tmpDir, fmt.Sprintf("enc_%d_%d_%ds.m4a", bf.SampleRate, bf.BitDepth, durationSec))

			testutil.CoreAudio(t, testutil.CoreAudioOptions{
				Args: []string{"encode", wavPath, m4aPath},
			})

			m4aInfo, err := os.Stat(m4aPath)
			if err != nil {
				t.Fatalf("stat: %v", err)
			}

			t.Logf("  M4A size: %.1f MB (%d bytes)", float64(m4aInfo.Size())/(1024*1024), m4aInfo.Size())

			// Benchmark saprobe decode.
			encoded, err := os.ReadFile(m4aPath)
			if err != nil {
				t.Fatalf("read encoded: %v", err)
			}

			durations := make([]time.Duration, opts.Iterations)

			for iter := range opts.Iterations {
				start := time.Now()

				_, _, decErr := alac.Decode(bytes.NewReader(encoded))
				if decErr != nil {
					t.Fatalf("decode: %v", decErr)
				}

				durations[iter] = time.Since(start)
			}

			results = append(results, testutil.ComputeResult(bf, "saprobe", "decode", durations, len(encoded)))

			// Benchmark ffmpeg decode.
			results = append(results, testutil.BenchDecodeFFmpeg(t, bf, opts, m4aPath))

			// Benchmark CoreAudio decode (CGO, in-process).
			results = append(results, agar.BenchDecodeCoreAudio(t, bf, opts, encoded))

			// Benchmark alacconvert decode (from CAF).
			if hasAlacconvert {
				cafPath := filepath.Join(
					tmpDir,
					fmt.Sprintf("enc_%d_%d_%ds.caf", bf.SampleRate, bf.BitDepth, durationSec),
				)

				testutil.AlacConvert(t, testutil.AlacConvertOptions{
					Args: []string{wavPath, cafPath},
				})

				results = append(results, testutil.BenchDecodeAlacconvert(t, bf, opts, cafPath))
			}
		}
	}

	testutil.PrintResults(t, opts, results)
}

// TestBenchmarkDecodeFile benchmarks decoding a real M4A file.
// Set BENCH_FILE to an M4A file path to run.
//
//nolint:paralleltest // Benchmark must run sequentially for accurate timing.
func TestBenchmarkDecodeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark in short mode")
	}

	filePath := os.Getenv("BENCH_FILE")
	if filePath == "" {
		t.Skip("set BENCH_FILE to run this benchmark")
	}

	opts := testutil.BenchOptions{}.WithDefaults()

	encoded, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	t.Logf("File: %s (%.1f MB)", filePath, float64(len(encoded))/(1024*1024))

	tmpDir := t.TempDir()

	// Probe format with initial decode.
	_, pcmFormat, err := alac.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("probe decode: %v", err)
	}

	bf := testutil.BenchFormat{
		Name:       filepath.Base(filePath),
		SampleRate: pcmFormat.SampleRate,
		BitDepth:   pcmFormat.BitDepth,
		Channels:   pcmFormat.Channels,
	}

	var results []testutil.BenchResult

	// Benchmark saprobe decode.
	durations := make([]time.Duration, opts.Iterations)

	for iter := range opts.Iterations {
		start := time.Now()

		_, _, decErr := alac.Decode(bytes.NewReader(encoded))
		if decErr != nil {
			t.Fatalf("decode: %v", decErr)
		}

		durations[iter] = time.Since(start)
	}

	results = append(results, testutil.ComputeResult(bf, "saprobe", "decode", durations, len(encoded)))

	// Write M4A to temp for tool-based decoders.
	m4aPath := filepath.Join(tmpDir, "input.m4a")
	if err := os.WriteFile(m4aPath, encoded, 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	// Benchmark ffmpeg decode.
	results = append(results, testutil.BenchDecodeFFmpeg(t, bf, opts, m4aPath))

	// Benchmark CoreAudio decode (CGO, in-process).
	results = append(results, agar.BenchDecodeCoreAudio(t, bf, opts, encoded))

	// Benchmark alacconvert decode (from CAF).
	// Setup: saprobe decode → WAV → alacconvert encode → CAF.
	if testutil.AlacConvertPath(t) != "" {
		pcm, _, decErr := alac.Decode(bytes.NewReader(encoded))
		if decErr != nil {
			t.Fatalf("decode for CAF setup: %v", decErr)
		}

		wavPath := filepath.Join(tmpDir, "input.wav")
		testutil.WriteWAV(t, wavPath, pcm, pcmFormat.BitDepth, pcmFormat.SampleRate, pcmFormat.Channels)

		cafPath := filepath.Join(tmpDir, "input.caf")

		testutil.AlacConvert(t, testutil.AlacConvertOptions{
			Args: []string{wavPath, cafPath},
		})

		results = append(results, testutil.BenchDecodeAlacconvert(t, bf, opts, cafPath))
	}

	testutil.PrintResults(t, opts, results)
}
