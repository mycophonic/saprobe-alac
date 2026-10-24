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
	"testing"
	"time"

	alac "github.com/mycophonic/saprobe-alac"
)

// The stream decoder's contract as the README states it: Read is an
// io.Reader over the whole PCM whatever the buffer, Seek lands on a packet
// boundary and the stream reads on from there, past the end is the end,
// negative is the start, Position is where the next Read starts and
// Duration the whole; every one of them held against one full decode of
// the same file.

// wholeDecode is the file's PCM in one read, the reference the rest is
// held to.
func wholeDecode(t *testing.T, data []byte) ([]byte, alac.PCMFormat) {
	t.Helper()

	dec, err := alac.NewDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	pcm, err := io.ReadAll(dec)
	if err != nil {
		t.Fatal(err)
	}

	return pcm, dec.Format()
}

func TestReadIsTheSameWhateverTheBuffer(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)
	want, format := wholeDecode(t, data)
	frame := format.Channels * format.BitDepth / 8

	for _, size := range []int{1, 2, 3, frame - 1, frame, frame + 1, 1000, 4096*frame - 1, 4096 * frame, 1 << 20} {
		dec, err := alac.NewDecoder(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}

		var got bytes.Buffer

		buf := make([]byte, size)

		for {
			n, err := dec.Read(buf)
			got.Write(buf[:n])

			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				t.Fatalf("buffer %d: %v", size, err)
			}
		}

		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("buffer %d: %d bytes, want %d, or the bytes differ", size, got.Len(), len(want))
		}

		// EOF is sticky, and a zero-length read asks for nothing.
		if n, err := dec.Read(buf); n != 0 || !errors.Is(err, io.EOF) {
			t.Fatalf("buffer %d: after EOF, %d bytes and %v", size, n, err)
		}

		if n, err := dec.Read(nil); n != 0 || err != nil {
			t.Fatalf("buffer %d: a zero-length read gave %d, %v", size, n, err)
		}
	}
}

func TestSeekLandsOnAPacketAndReadsOn(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)
	want, format := wholeDecode(t, data)
	frame := format.Channels * format.BitDepth / 8
	// The test file is one second at 44100 Hz in 4096-sample packets.
	packet := 4096 * frame
	perSecond := format.SampleRate * frame

	dec, err := alac.NewDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	total := dec.Duration()
	if want := time.Duration(
		len(want),
	) * time.Second / time.Duration(
		perSecond,
	); total < want ||
		total > want+time.Second {
		// Duration counts whole packets, so it may overshoot by less than one.
		t.Fatalf("Duration %v, the PCM is %v", total, want)
	}

	targets := []time.Duration{
		0, time.Millisecond, 92*time.Millisecond + 879*time.Microsecond, 100 * time.Millisecond, 500 * time.Millisecond,
		total - time.Nanosecond, total, total + time.Second, -time.Second,
	}

	for _, target := range targets {
		landed, seekErr := dec.Seek(target)
		if seekErr != nil {
			t.Fatalf("Seek(%v): %v", target, seekErr)
		}

		if landed != dec.Position() {
			t.Fatalf("Seek(%v) landed %v, Position says %v", target, landed, dec.Position())
		}

		// The landing is a packet boundary at or before the target, within
		// one packet of it, clamped to the stream; the stream's end is the
		// packet grid's, past the PCM by what the last packet does not fill
		// (Duration counts whole packets).
		offset := int(landed.Seconds()*float64(perSecond) + 0.5)
		gridEnd := (len(want) + packet - 1) / packet * packet

		if offset%packet != 0 || offset > gridEnd {
			t.Fatalf("Seek(%v) landed at byte %d: not a packet boundary within the stream", target, offset)
		}

		offset = min(offset, len(want))

		clamped := max(min(target, total), 0)
		if landed > clamped ||
			clamped-landed >= time.Duration(packet)*time.Second/time.Duration(perSecond)+time.Microsecond {
			t.Fatalf("Seek(%v) landed %v, not within one packet before it", target, landed)
		}

		rest, readErr := io.ReadAll(dec)
		if readErr != nil {
			t.Fatalf("after Seek(%v): %v", target, readErr)
		}

		if !bytes.Equal(rest, want[offset:]) {
			t.Fatalf(
				"after Seek(%v) at byte %d: %d bytes, want %d, or the bytes differ",
				target,
				offset,
				len(rest),
				len(want)-offset,
			)
		}

		if dec.Position() != total && offset != len(want) {
			t.Fatalf("after reading to the end from %v: Position %v, Duration %v", landed, dec.Position(), total)
		}
	}

	// Seeking back after the end reads the stream again.
	if _, seekErr := dec.Seek(0); seekErr != nil {
		t.Fatal(seekErr)
	}

	again, err := io.ReadAll(dec)
	if err != nil || !bytes.Equal(again, want) {
		t.Fatalf("after Seek(0) from the end: %v, %d bytes", err, len(again))
	}
}

func TestPositionAdvancesByPackets(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)
	_, format := wholeDecode(t, data)
	frame := format.Channels * format.BitDepth / 8
	packet := 4096 * frame

	dec, err := alac.NewDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	if dec.Position() != 0 {
		t.Fatalf("Position before any read: %v", dec.Position())
	}

	// One byte read decodes one packet: Position is past it.
	if _, err := dec.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}

	// Position is frames × 1e9 / rate in integers, so each expectation is
	// computed the same way rather than scaled from one packet.
	at := func(packets int) time.Duration {
		return time.Duration(int64(packets) * 4096 * int64(time.Second) / int64(format.SampleRate))
	}
	onePacket := at(1)

	if got := dec.Position(); got != onePacket {
		t.Fatalf("Position after one byte: %v, want one packet %v", got, onePacket)
	}

	// Reading the rest of that packet does not move it; the next byte does.
	if _, err := io.ReadFull(dec, make([]byte, packet-1)); err != nil {
		t.Fatal(err)
	}

	if got := dec.Position(); got != onePacket {
		t.Fatalf("Position after the packet: %v, want %v", got, onePacket)
	}

	if _, err := dec.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}

	if got := dec.Position(); got != at(2) {
		t.Fatalf("Position into the second packet: %v, want %v", got, at(2))
	}
}

// A reader that fails under the decoder once armed: the failure comes
// back, wrapped, never a panic or silence.
type failingReader struct {
	*bytes.Reader

	armed bool
}

var errMedium = errors.New("the medium went away")

func (r *failingReader) Read(p []byte) (int, error) {
	if r.armed {
		return 0, errMedium
	}

	return r.Reader.Read(p)
}

func TestReadReportsTheSourcesFailure(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)
	source := &failingReader{Reader: bytes.NewReader(data)}

	dec, err := alac.NewDecoder(source)
	if err != nil {
		t.Fatal(err)
	}

	// The first packet decodes; then the source fails under the second.
	if _, err := dec.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}

	source.armed = true

	if _, err := io.ReadAll(dec); !errors.Is(err, errMedium) {
		t.Fatalf("the source's failure is not reported: %v", err)
	}
}
