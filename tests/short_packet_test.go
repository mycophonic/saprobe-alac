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

	alac "github.com/mycophonic/saprobe-alac"
)

// A packet the file does not hold is a cut stream: Read must say so with the
// package's error wrapping io.ErrUnexpectedEOF, not a plain io.EOF that
// io.ReadAll takes for the end. The container is a fuzz target's: a sample
// table whose one entry lies past the file.
func TestRead_PacketPastTheEnd(t *testing.T) {
	t.Parallel()

	file := []byte("\x00\x00\x00" +
		"\xc6000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000moov" +
		"\x00\x00" +
		"\x00l000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000trak" +
		"\x00\x00" +
		"\x00x000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000mdia" +
		"\x00\x00\x00 0000000000000000000000000000\x00\x00" +
		"\x00-000000000000000000000000000000000000000000000minf\x00\x00" +
		"\x00 0000000000000000000000000000\x00\x00\x00\f000000000000stbl\x00\x00" +
		"\x00Xstsd00000000\x00\x00" +
		"\x00Halac00000000000000000000000000000000alac0000\x00\x0000\x00 000" +
		"\x0100000000000000\x00\x00\x00\x1800000000000000000000\x00\x00\x00" +
		"\x1cstsc0000\x00\x00\x00\x01\x00\x00\x00\x0100000000\x00\x00\x00" +
		"\x14stsz0000\x00\x00\x02\xff\x00\x00\x00\x010000stco0000\x00\x00\x00" +
		"\x010000")

	dec, err := alac.NewDecoder(bytes.NewReader(file))
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}

	_, err = io.ReadAll(dec)
	if err == nil {
		t.Fatal("a packet past the end read as a clean end")
	}

	if !errors.Is(err, alac.ErrDecode) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected ErrDecode and io.ErrUnexpectedEOF, got: %v", err)
	}
}

// countingReader counts the Reads made through it.
type countingReader struct {
	io.ReadSeeker

	reads int
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.reads++

	return r.ReadSeeker.Read(p)
}

// A per-sample size of 0xFFFFFFFF at offset 8: the packet lies past the
// file, and Read refuses it before any read, so no 4 GiB buffer and no I/O
// precede the refusal.
func TestRead_PacketPastTheEndIsRefusedBeforeReading(t *testing.T) {
	t.Parallel()

	file := container(
		alacStsd(1, alacEntry()),
		fullBox("stco", be32(1, 8)),
		fullBox("stsc", be32(1, 1, 1, 1)),
		fullBox("stsz", be32(0, 1, 0xFFFFFFFF)),
	)

	reader := &countingReader{ReadSeeker: bytes.NewReader(file)}

	dec, err := alac.NewDecoder(reader)
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}

	reader.reads = 0

	_, err = dec.Read(make([]byte, 16))
	if !errors.Is(err, alac.ErrDecode) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected ErrDecode and io.ErrUnexpectedEOF, got: %v", err)
	}

	if reader.reads != 0 {
		t.Fatalf("Read made %d reads before refusing a packet past the end", reader.reads)
	}
}

// lyingReader reports a length beyond what it holds, as a file cut after
// NewDecoder took its end does.
type lyingReader struct {
	io.ReadSeeker

	claimed int64
}

func (r *lyingReader) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekEnd {
		return r.claimed + offset, nil
	}

	return r.ReadSeeker.Seek(offset, whence)
}

// A packet starting exactly where the bytes end, within the length the
// reader claims: the read comes up with nothing, and the plain io.EOF of that
// becomes the package's refusal, not the stream's end.
func TestRead_ShortReadIsNotTheEnd(t *testing.T) {
	t.Parallel()

	build := func(offset uint32) []byte {
		return container(
			alacStsd(1, alacEntry()),
			fullBox("stco", be32(1, offset)),
			fullBox("stsc", be32(1, 1, 1, 1)),
			fullBox("stsz", be32(0, 1, 100)),
		)
	}

	file := build(0)
	file = build(uint32(len(file))) // the one packet starts at the real end

	reader := &lyingReader{ReadSeeker: bytes.NewReader(file), claimed: int64(len(file)) + 4096}

	dec, err := alac.NewDecoder(reader)
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}

	_, err = dec.Read(make([]byte, 16))
	if !errors.Is(err, alac.ErrDecode) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected ErrDecode and io.ErrUnexpectedEOF, got: %v", err)
	}
}
