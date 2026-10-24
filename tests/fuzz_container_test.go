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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mycophonic/agar/pkg/agar"

	alac "github.com/mycophonic/saprobe-alac"
)

// FuzzNewDecoder feeds arbitrary bytes to the container parser and the
// stream decoder: NewDecoder, a full read, Duration, a Seek to the middle
// and a read from there. A stream may be refused at any point with one of
// the package's errors; it may never panic or hang, yield PCM that is not
// whole samples of its format, land a Seek past its Duration, or read on
// after a clean end. The seeds are small streams ffmpeg writes, each also
// cut short and with a byte flipped, and the ftyp box alone.
func FuzzNewDecoder(f *testing.F) {
	ffmpegBin, err := exec.LookPath("ffmpeg")
	if err != nil {
		f.Skip("no ffmpeg on the PATH")
	}

	for _, data := range fuzzSeedStreams(f, ffmpegBin) {
		f.Add(data)
		f.Add(data[:len(data)*2/3])

		flipped := bytes.Clone(data)
		flipped[len(flipped)/2] ^= 0x55
		f.Add(flipped)
	}

	f.Add([]byte{0, 0, 0, 0x14, 'f', 't', 'y', 'p', 'M', '4', 'A', ' ', 0, 0, 0, 0, 'M', '4', 'A', ' '})

	f.Fuzz(func(t *testing.T, data []byte) {
		dec, err := alac.NewDecoder(bytes.NewReader(data))
		if err != nil {
			if !isSentinel(err) {
				t.Fatalf("refused outside the package's errors: %v", err)
			}

			return
		}

		format := dec.Format()
		frameBytes := format.Channels * format.BitDepth / 8

		if format.Channels < 1 || format.Channels > 8 || frameBytes == 0 {
			t.Fatalf("format %+v", format)
		}

		duration := dec.Duration()
		if duration < 0 {
			t.Fatalf("Duration %v", duration)
		}

		// A hostile table can declare an enormous stream: one case reads
		// at most this much.
		const limit = 32 << 20

		pcm, err := io.ReadAll(io.LimitReader(dec, limit))
		if err != nil && !isSentinel(err) {
			t.Fatalf("Read failed outside the package's errors: %v", err)
		}

		if len(pcm)%frameBytes != 0 {
			t.Fatalf("%d bytes of PCM are not whole %d-byte samples", len(pcm), frameBytes)
		}

		if err == nil && len(pcm) < limit {
			if n, again := dec.Read(make([]byte, 16)); n != 0 || !errors.Is(again, io.EOF) {
				t.Fatalf("after the end, Read gave %d bytes and %v", n, again)
			}
		}

		landed, err := dec.Seek(duration / 2)
		if err != nil {
			if !isSentinel(err) {
				t.Fatalf("Seek failed outside the package's errors: %v", err)
			}

			return
		}

		if landed < 0 || landed > duration || landed > duration/2 {
			t.Fatalf("Seek(%v) landed %v of %v", duration/2, landed, duration)
		}

		if dec.Position() != landed {
			t.Fatalf("Seek landed %v, Position says %v", landed, dec.Position())
		}

		again, err := io.ReadAll(io.LimitReader(dec, limit))
		if err != nil && !isSentinel(err) {
			t.Fatalf("Read after Seek failed outside the package's errors: %v", err)
		}

		if len(again)%frameBytes != 0 {
			t.Fatalf("%d bytes of PCM after Seek are not whole %d-byte samples", len(again), frameBytes)
		}
	})
}

// fuzzSeedStreams encodes a few small streams through ffmpeg.
func fuzzSeedStreams(f *testing.F, ffmpegBin string) [][]byte {
	f.Helper()

	dir := f.TempDir()

	var seeds [][]byte

	for _, c := range []struct{ bitDepth, channels int }{{16, 1}, {16, 2}, {24, 2}, {16, 6}} {
		const sampleRate = 8000

		pcm := tonalPCM(sampleRate, c.bitDepth, c.channels)[:sampleRate*c.channels*c.bitDepth/8/10] // a tenth of a second
		src := filepath.Join(dir, fmt.Sprintf("%d-%d.raw", c.bitDepth, c.channels))

		if err := os.WriteFile(src, pcm, 0o600); err != nil {
			f.Fatal(err)
		}

		dst := filepath.Join(dir, fmt.Sprintf("%d-%d.m4a", c.bitDepth, c.channels))
		if err := ffmpegEncodeALAC(ffmpegBin, src, dst, c.bitDepth, sampleRate, c.channels); err != nil {
			f.Fatal(err)
		}

		data, err := os.ReadFile(dst)
		if err != nil {
			f.Fatal(err)
		}

		seeds = append(seeds, data)
	}

	return seeds
}

// ffmpegEncodeALAC encodes raw PCM to ALAC in MP4 with the options the
// conformance tests use.
func ffmpegEncodeALAC(ffmpegBin, src, dst string, bitDepth, sampleRate, channels int) error {
	sampleFmt := "s16p"
	if bitDepth == 24 {
		sampleFmt = "s32p"
	}

	args := []string{
		"-nostdin", "-v", "error", "-y", "-f", agar.RawPCMFormat(bitDepth),
		"-ar", strconv.Itoa(sampleRate), "-ac", strconv.Itoa(channels),
	}
	if layout := channelLayout(channels); layout != "" {
		args = append(args, "-channel_layout", layout)
	}

	args = append(args, "-i", src, "-c:a", "alac", "-sample_fmt", sampleFmt, dst)

	if out, err := exec.Command(ffmpegBin, args...).
		CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg %v: %w\n%s", args, err, out)
	}

	return nil
}

// cookieBytes is the 24-byte ALACSpecificConfig: frame length, compatible
// version, bit depth, pb, mb, kb, channels, max run, max frame bytes,
// average bit rate, sample rate.
func cookieBytes(frameLength uint32, bitDepth, channels uint8, sampleRate uint32) []byte {
	out := make([]byte, 24)
	binary.BigEndian.PutUint32(out[0:], frameLength)
	out[4] = 0
	out[5] = bitDepth
	out[6], out[7], out[8] = 40, 10, 14
	out[9] = channels
	binary.BigEndian.PutUint16(out[10:], 255)
	binary.BigEndian.PutUint32(out[12:], 0)
	binary.BigEndian.PutUint32(out[16:], 0)
	binary.BigEndian.PutUint32(out[20:], sampleRate)

	return out
}

// wrapAtom wraps a payload in a 12-byte MPEG-4 atom header of the type.
func wrapAtom(typ string, payload []byte) []byte {
	out := make([]byte, 12, 12+len(payload))
	binary.BigEndian.PutUint32(out, uint32(12+len(payload)))
	copy(out[4:], typ)

	return append(out, payload...)
}

// FuzzParseMagicCookie feeds arbitrary bytes to the cookie parser. A cookie
// may be refused with ErrConfig; one that is accepted either builds a
// packet decoder or is refused there with ErrConfig, and never panics
// either way.
func FuzzParseMagicCookie(f *testing.F) {
	plain := cookieBytes(4096, 16, 2, 44100)
	f.Add(plain)
	f.Add(wrapAtom("alac", plain))
	f.Add(wrapAtom("frma", append([]byte("alac"), wrapAtom("alac", plain)...)))
	f.Add(cookieBytes(4096, 32, 8, 192000))
	f.Add(cookieBytes(0, 16, 2, 44100))
	f.Add(cookieBytes(0xFFFFFFFF, 20, 1, 0))
	f.Add(plain[:23])
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		config, err := alac.ParseMagicCookie(data)
		if err != nil {
			if !errors.Is(err, alac.ErrConfig) {
				t.Fatalf("refused outside ErrConfig: %v", err)
			}

			return
		}

		dec, err := alac.NewPacketDecoder(config)
		if err != nil {
			if !errors.Is(err, alac.ErrConfig) {
				t.Fatalf("NewPacketDecoder refused outside ErrConfig: %v", err)
			}

			return
		}

		// The decoder sizes its buffers from the frame length: a cookie the
		// parser accepts but the decoder would not allocate for is refused
		// above, never built.
		if config.FrameLength == 0 || config.FrameLength > 65536 {
			t.Fatalf("NewPacketDecoder accepted a frame length of %d", config.FrameLength)
		}

		format := dec.Format()
		if format.Channels != int(config.NumChannels) || format.BitDepth != int(config.BitDepth) {
			t.Fatalf("format %+v for config %+v", format, config)
		}
	})
}
