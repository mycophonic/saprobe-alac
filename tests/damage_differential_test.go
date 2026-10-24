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
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mycophonic/agar/pkg/agar"

	alac "github.com/mycophonic/saprobe-alac"
	alacint "github.com/mycophonic/saprobe-alac/internal/alac"
	mp4int "github.com/mycophonic/saprobe-alac/internal/mp4"
)

// Random damage against ffmpeg's decoder. Streams ffmpeg encoded from tonal
// and noise PCM are damaged in the ways files get damaged (bytes flipped,
// spans truncated, dropped, doubled, overwritten, zeroed, the moov box and
// the packets hit on purpose) and decoded by this decoder and by ffmpeg.
// Whatever happens, this decoder must neither panic nor hang, and every
// refusal must be one of the package's errors; where both decode, the PCM is compared and a disagreement is
// kept for review with its input, never imposed: two decoders parting
// on a damaged packet convicts neither (ffmpeg applies the first-order
// pass only for prediction type 15, ALACDecoder.cpp for any non-zero
// modeU). The other outcomes are counted and logged. DAMAGE_CASES sets the
// number of cases (default 200).

type damageOutcome struct {
	pcm []byte
	err error
}

// decodeWithDeadline decodes in a goroutine so a hang is a failure, not a
// stuck test. A panic in the goroutine still ends the test binary, which is
// the failure it should be.
func decodeWithDeadline(data []byte, deadline time.Duration) (damageOutcome, bool) {
	done := make(chan damageOutcome, 1)

	go func() {
		dec, err := alac.NewDecoder(bytes.NewReader(data))
		if err != nil {
			done <- damageOutcome{nil, err}

			return
		}

		pcm, err := io.ReadAll(dec)
		done <- damageOutcome{pcm, err}
	}()

	select {
	case out := <-done:
		return out, true
	case <-time.After(deadline):
		return damageOutcome{}, false
	}
}

// isSentinel reports whether err is or wraps one of the package's errors.
func isSentinel(err error) bool {
	for _, s := range []error{
		alac.ErrConfig, alac.ErrNoTrack, alac.ErrDecode,
		alacint.ErrInvalidCookie, alacint.ErrUnsupportedVersion, alacint.ErrUnsupportedElement,
		alacint.ErrInvalidHeader, alacint.ErrInvalidShift, alacint.ErrBitstreamOverrun,
		alacint.ErrSampleOverrun, alacint.ErrBitDepth, alacint.ErrChannels, alacint.ErrFrameLength,
		alacint.ErrSampleRate, alacint.ErrMissingChannels, alacint.ErrElementLayout,
		mp4int.ErrNoALACTrack, mp4int.ErrInvalidEntry, mp4int.ErrInvalidBoxSize, mp4int.ErrNoChunkOffset,
		mp4int.ErrInvalidChunkOffset, mp4int.ErrNoStsc, mp4int.ErrInvalidStsc, mp4int.ErrNoStsz,
		mp4int.ErrInvalidStsz, mp4int.ErrTableTooLarge,
	} {
		if errors.Is(err, s) {
			return true
		}
	}

	return false
}

// topLevelBox finds a top-level box by type and returns its offset and size.
func topLevelBox(data []byte, typ string) (offset, size int, found bool) {
	pos := 0
	for pos+8 <= len(data) {
		size := int(binary.BigEndian.Uint32(data[pos:]))
		if size < 8 || pos+size > len(data) {
			return 0, 0, false
		}

		if string(data[pos+4:pos+8]) == typ {
			return pos, size, true
		}

		pos += size
	}

	return 0, 0, false
}

// damage returns a damaged copy of data and the name of what was done.
func damage(rng *rand.Rand, data []byte) ([]byte, string) {
	out := bytes.Clone(data)
	if len(out) < 16 {
		return out, "untouched"
	}

	span := func() (int, int) {
		start := rng.IntN(len(out))
		length := 1 + rng.IntN(min(len(out)-start, 1+rng.IntN(4096)))

		return start, length
	}

	switch rng.IntN(9) {
	case 0:
		n := 1 + rng.IntN(8)
		for range n {
			out[rng.IntN(len(out))] ^= byte(1 << rng.IntN(8))
		}

		return out, fmt.Sprintf("%d bits flipped", n)
	case 1:
		at := rng.IntN(len(out))

		return out[:at], fmt.Sprintf("truncated at %d of %d", at, len(out))
	case 2:
		start, length := span()

		return append(out[:start], out[start+length:]...), fmt.Sprintf("%d bytes dropped at %d", length, start)
	case 3:
		start, length := span()
		dup := append([]byte{}, out[start:start+length]...)

		return append(
				out[:start+length],
				append(dup, out[start+length:]...)...,
			), fmt.Sprintf(
				"%d bytes doubled at %d",
				length,
				start,
			)
	case 4:
		start, length := span()
		for i := start; i < start+length; i++ {
			out[i] = byte(rng.IntN(256))
		}

		return out, fmt.Sprintf("%d bytes overwritten at %d", length, start)
	case 5:
		start, length := span()
		for i := start; i < start+length; i++ {
			out[i] = 0
		}

		return out, fmt.Sprintf("%d bytes zeroed at %d", length, start)
	case 6:
		// The moov box: the sample tables and the cookie.
		off, size, ok := topLevelBox(out, "moov")
		if !ok {
			return out, "untouched"
		}

		at := off + rng.IntN(size)
		out[at] ^= byte(1 << rng.IntN(8))

		return out, fmt.Sprintf("a moov bit flipped at %d", at)
	case 7:
		// A moov field overwritten: a 32-bit value somewhere in the tables.
		off, size, ok := topLevelBox(out, "moov")
		if !ok || size < 12 {
			return out, "untouched"
		}

		at := off + 8 + rng.IntN(size-12)
		values := []uint32{0, 1, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF, uint32(len(out)), uint32(rng.IntN(1 << 20))}
		binary.BigEndian.PutUint32(out[at:], values[rng.IntN(len(values))])

		return out, fmt.Sprintf("a moov word overwritten at %d", at)
	default:
		// A packet: the first bytes of mdat's payload, where element
		// headers sit.
		off, size, ok := topLevelBox(out, "mdat")
		if !ok || size < 16 {
			return out, "untouched"
		}

		at := off + 8 + rng.IntN(min(size-8, 64))
		out[at] ^= byte(1 << rng.IntN(8))

		return out, fmt.Sprintf("a packet bit flipped at %d", at)
	}
}

// damageSource is one corpus stream and the PCM it was encoded from.
type damageSource struct {
	data     []byte
	bitDepth int
	channels int
}

// damageCorpus encodes tonal and noise streams through ffmpeg at 8 kHz.
func damageCorpus(t *testing.T) map[string]damageSource {
	t.Helper()

	const sampleRate = 8000

	dir := t.TempDir()
	corpus := map[string]damageSource{}

	encode := func(name string, pcm []byte, bitDepth, channels int) {
		src := filepath.Join(dir, name+".raw")
		if err := os.WriteFile(src, pcm, 0o600); err != nil {
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

		dst := filepath.Join(dir, name+".m4a")
		agar.FFmpegEncode(t, agar.FFmpegEncodeOptions{
			Src: src, Dst: dst, BitDepth: bitDepth, SampleRate: sampleRate, Channels: channels,
			CodecArgs: []string{"-c:a", "alac", "-sample_fmt", sampleFmt},
			InputArgs: inputArgs,
		})

		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}

		corpus[name] = damageSource{data, bitDepth, channels}
	}

	for _, bitDepth := range []int{16, 24} {
		for _, channels := range []int{1, 2, 6, 8} {
			encode(
				fmt.Sprintf("tone-%dbit-%dch", bitDepth, channels),
				tonalPCM(sampleRate, bitDepth, channels),
				bitDepth,
				channels,
			)
		}
	}

	encode("noise-16bit-2ch", agar.GenerateWhiteNoise(sampleRate, 16, 2, 1), 16, 2)

	return corpus
}

// ffmpegDecodeRaw decodes path to raw PCM, returning the output, ffmpeg's
// stderr and whether it exited cleanly.
func ffmpegDecodeRaw(t *testing.T, ffmpegBin, path string, bitDepth, channels int) ([]byte, string, bool) {
	t.Helper()

	args := []string{
		"-nostdin",
		"-v",
		"error",
		"-i",
		path,
		"-f",
		agar.RawPCMFormat(bitDepth),
		"-ac",
		strconv.Itoa(channels),
		"-acodec",
		agar.RawPCMCodec(bitDepth),
	}
	if layout := channelLayout(channels); layout != "" {
		args = append(args, "-channel_layout", layout)
	}

	args = append(args, "-")

	cmd := exec.CommandContext(t.Context(), ffmpegBin, args...)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	return stdout.Bytes(), strings.TrimSpace(stderr.String()), err == nil && stderr.Len() == 0
}

func TestRandomDamageAgainstFFmpeg(t *testing.T) {
	t.Parallel()

	ffmpegBin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on the PATH")
	}

	cases := 200
	if s := os.Getenv("DAMAGE_CASES"); s != "" {
		cases, _ = strconv.Atoi(s)
	}

	corpus := damageCorpus(t)

	names := make([]string, 0, len(corpus))
	for name := range corpus {
		names = append(names, name)
	}

	sort.Strings(names)

	rng := rand.New(rand.NewPCG(20261023, uint64(cases)))
	dir := t.TempDir()

	var same, differ, refOnly, oursOnly, bothRefuse int

	for i := range cases {
		name := names[rng.IntN(len(names))]
		source := corpus[name]
		data, what := damage(rng, source.data)
		label := fmt.Sprintf("case %d (%s, %s)", i, name, what)

		ours, finished := decodeWithDeadline(data, 20*time.Second)
		if !finished {
			t.Fatalf("%s: the decoder did not finish in 20 s; input at %s", label, saveCase(t, i, data))
		}

		if ours.err != nil && !isSentinel(ours.err) {
			t.Fatalf("%s: refused outside the package's errors: %v; input at %s", label, ours.err, saveCase(t, i, data))
		}

		path := filepath.Join(dir, "case.m4a")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}

		reference, refStderr, refClean := ffmpegDecodeRaw(t, ffmpegBin, path, source.bitDepth, source.channels)

		switch {
		case ours.err == nil && len(reference) > 0:
			if bytes.Equal(ours.pcm, reference) {
				same++

				continue
			}

			differ++

			frameBytes := source.channels * source.bitDepth / 8
			at := 0

			for at < len(ours.pcm) && at < len(reference) && ours.pcm[at] == reference[at] {
				at++
			}

			// Two decoders disagreeing on a damaged packet convicts neither:
			// ffmpeg applies the first-order pass only for prediction type 15
			// where ALACDecoder.cpp applies it for any non-zero modeU, so a
			// flipped modeU bit alone parts them. The case is kept for review.
			t.Logf(
				"%s: %d bytes against ffmpeg's %d, first difference at sample %d; ffmpeg clean: %v, said %q; input at %s",
				label,
				len(ours.pcm),
				len(reference),
				at/frameBytes,
				refClean,
				refStderr,
				saveCase(t, i, data),
			)
		case ours.err == nil:
			oursOnly++

			t.Logf("%s: decoded %d bytes where ffmpeg decodes nothing (%q)", label, len(ours.pcm), refStderr)
		case len(reference) > 0:
			refOnly++

			t.Logf("%s: refused (%v) where ffmpeg decodes %d bytes (%q)", label, ours.err, len(reference), refStderr)
		default:
			bothRefuse++
		}
	}

	t.Logf("%d cases: same PCM %d, PCM differs %d, only ffmpeg decodes %d, only this decoder %d, both refuse %d",
		cases, same, differ, refOnly, oursOnly, bothRefuse)
}

// saveCase keeps a failing input in the system's temporary directory, past
// the test's own, for the report.
func saveCase(t *testing.T, i int, data []byte) string {
	t.Helper()

	// DAMAGE_KEEP names a directory that outlives the test; without it the
	// input lives as long as the test's own directory.
	dir := os.Getenv("DAMAGE_KEEP")
	if dir == "" {
		dir = t.TempDir()
	}

	path := filepath.Join(dir, fmt.Sprintf("alac-damage-%d.m4a", i))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}
