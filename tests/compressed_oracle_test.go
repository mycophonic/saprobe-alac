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
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/mycophonic/agar/pkg/agar"

	alac "github.com/mycophonic/saprobe-alac"
)

// Compressed streams against their source. White noise, the conformance
// matrix's signal, is stored escape-coded by every ALAC encoder, so only a
// compressible signal reaches the predictor, the adaptive Golomb decoder
// and the shifted low bytes of 24-bit content. Tones with low-order
// activity are encoded with ffmpeg at the depths it writes, 16 and 24 bits,
// one to eight channels, decoded by the stream decoder and held to the
// source byte for byte; the encoded size is held below the PCM's, so a
// corpus the encoder gave up compressing fails for that reason, loudly.

// tonalPCM is one second of per-channel tones with the low bits busy, as
// interleaved little-endian signed PCM.
func tonalPCM(sampleRate, bitDepth, channels int) []byte {
	bytesPer := bitDepth / 8
	full := float64(int64(1)<<(bitDepth-1) - 1)
	out := make([]byte, 0, sampleRate*channels*bytesPer)

	for i := range sampleRate {
		for c := range channels {
			phase := 2 * math.Pi * float64(i) * (220 * float64(c+1)) / float64(sampleRate)
			v := int64(0.6*full*math.Sin(phase)) + int64(i%251) - 125

			for b := range bytesPer {
				out = append(out, byte(v>>(8*b)))
			}
		}
	}

	return out
}

// compressedCase encodes source through ffmpeg with encoderArgs added,
// holds the encoded size below the PCM's, decodes the stream back to the
// source byte for byte, and checks two seeks inside it.
func compressedCase(t *testing.T, bitDepth, channels int, source []byte, encoderArgs ...string) {
	t.Helper()

	const sampleRate = 44100

	dir := t.TempDir()
	src := filepath.Join(dir, "source.raw")

	if err := os.WriteFile(src, source, 0o600); err != nil {
		t.Fatal(err)
	}

	sampleFmt := "s16p"
	if bitDepth == 24 {
		sampleFmt = "s32p"
	}

	var inputArgs []string
	if layout := channelLayout(channels); layout != "" {
		inputArgs = []string{"-channel_layout", layout}
	}

	enc := filepath.Join(dir, "encoded.m4a")
	agar.FFmpegEncode(t, agar.FFmpegEncodeOptions{
		Src: src, Dst: enc, BitDepth: bitDepth, SampleRate: sampleRate, Channels: channels,
		CodecArgs: append([]string{"-c:a", "alac", "-sample_fmt", sampleFmt}, encoderArgs...),
		InputArgs: inputArgs,
	})

	info, err := os.Stat(enc)
	if err != nil {
		t.Fatal(err)
	}

	if info.Size() >= int64(len(source)) {
		t.Fatalf(
			"the encoder did not compress: %d bytes for %d of PCM; the stream would be escape-coded",
			info.Size(),
			len(source),
		)
	}

	file, err := os.Open(enc)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = file.Close() }()

	dec, err := alac.NewDecoder(file)
	if err != nil {
		t.Fatal(err)
	}

	got, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !bytes.Equal(got, source) {
		at := 0
		for at < len(got) && at < len(source) && got[at] == source[at] {
			at++
		}

		t.Fatalf("%d bytes decoded for %d of source, first difference at byte %d (sample %d)",
			len(got), len(source), at, at/(channels*bitDepth/8))
	}

	// Seek inside the compressed stream: the landing is a packet
	// boundary at or before the target, Position agrees, and
	// reading on from there is the source from that boundary.
	perSecond := sampleRate * channels * bitDepth / 8
	total := dec.Duration()

	for _, target := range []time.Duration{total / 3, 2 * total / 3} {
		landed, err := dec.Seek(target)
		if err != nil {
			t.Fatalf("Seek(%v): %v", target, err)
		}

		if landed > target || target-landed > time.Second || landed != dec.Position() {
			t.Fatalf("Seek(%v) landed %v, Position %v", target, landed, dec.Position())
		}

		offset := int(landed.Seconds()*float64(perSecond) + 0.5)
		if offset%(channels*bitDepth/8) != 0 || offset > len(source) {
			t.Fatalf("Seek(%v) landed at byte %d of %d", target, offset, len(source))
		}

		rest, err := io.ReadAll(dec)
		if err != nil {
			t.Fatalf("after Seek(%v): %v", target, err)
		}

		if !bytes.Equal(rest, source[offset:]) {
			t.Fatalf(
				"after Seek(%v) at byte %d: %d bytes, want %d, or the bytes differ",
				target,
				offset,
				len(rest),
				len(source)-offset,
			)
		}
	}
}

func TestCompressedStreamsAreTheirSource(t *testing.T) {
	t.Parallel()

	for _, bitDepth := range []int{16, 24} {
		for channels := 1; channels <= 8; channels++ {
			t.Run(fmt.Sprintf("%dbit/%dch", bitDepth, channels), func(t *testing.T) {
				t.Parallel()
				compressedCase(t, bitDepth, channels, tonalPCM(44100, bitDepth, channels))
			})
		}
	}
}

// correlatedPCM is stereo where the right channel follows the left closely,
// which the encoder decorrelates with a non-zero mixing residual.
func correlatedPCM(sampleRate, bitDepth int) []byte {
	bytesPer := bitDepth / 8
	full := float64(int64(1)<<(bitDepth-1) - 1)
	out := make([]byte, 0, sampleRate*2*bytesPer)

	for i := range sampleRate {
		phase := 2 * math.Pi * float64(i) * 330 / float64(sampleRate)
		left := int64(0.6 * full * math.Sin(phase))
		right := left*4/5 + int64(i%17) - 8

		for _, v := range []int64{left, right} {
			for b := range bytesPer {
				out = append(out, byte(v>>(8*b)))
			}
		}
	}

	return out
}

// impulsePCM is a quiet tone with full-scale impulses every 997 samples:
// residuals the Golomb coder cannot fit in its parameter and must escape.
func impulsePCM(sampleRate, bitDepth, channels int) []byte {
	bytesPer := bitDepth / 8
	maxV := int64(1)<<(bitDepth-1) - 1
	out := make([]byte, 0, sampleRate*channels*bytesPer)

	for i := range sampleRate {
		for c := range channels {
			phase := 2 * math.Pi * float64(i) * (110 * float64(c+1)) / float64(sampleRate)
			v := int64(0.02 * float64(maxV) * math.Sin(phase))

			switch (i + 37*c) % 997 {
			case 0:
				v = maxV
			case 1:
				v = -maxV - 1
			}

			for b := range bytesPer {
				out = append(out, byte(v>>(8*b)))
			}
		}
	}

	return out
}

// Every predictor order the encoder can be made to use, from 1 to 30, so
// the decoder's unrolled paths for 4, 5, 6 and 8 and its general path all
// run; and the two signals the tone alone does not reach: a correlated
// stereo pair, and impulses that escape the Golomb coder.
func TestCompressedStreamsAcrossOrdersAndSignals(t *testing.T) {
	t.Parallel()

	for _, bitDepth := range []int{16, 24} {
		for _, order := range []int{1, 2, 3, 4, 5, 6, 7, 8, 12, 30} {
			for _, channels := range []int{1, 2} {
				t.Run(fmt.Sprintf("%dbit/%dch/order%d", bitDepth, channels, order), func(t *testing.T) {
					t.Parallel()

					n := strconv.Itoa(order)
					compressedCase(t, bitDepth, channels, tonalPCM(44100, bitDepth, channels),
						"-min_prediction_order", n, "-max_prediction_order", n)
				})
			}
		}

		t.Run(fmt.Sprintf("%dbit/correlated stereo", bitDepth), func(t *testing.T) {
			t.Parallel()
			compressedCase(t, bitDepth, 2, correlatedPCM(44100, bitDepth))
		})

		for _, channels := range []int{1, 2} {
			t.Run(fmt.Sprintf("%dbit/%dch/impulses", bitDepth, channels), func(t *testing.T) {
				t.Parallel()
				compressedCase(t, bitDepth, channels, impulsePCM(44100, bitDepth, channels))
			})
		}
	}
}
