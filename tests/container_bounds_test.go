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
	"testing"
	"time"

	alac "github.com/mycophonic/saprobe-alac"
	"github.com/mycophonic/saprobe-alac/internal/mp4"
)

// Containers whose tables claim more than the file holds. Each used to size
// an allocation or a walk from the table's own count; each must come back as
// ErrNoTrack wrapping the parser's own refusal, at once. The refusal is the
// oracle: an allocation Go hands out untouched is fast too, and then the read
// fails with an EOF of its own.

// mp4Box frames payloads as an ISO base media box.
func mp4Box(typ string, payloads ...[]byte) []byte {
	size := 8
	for _, p := range payloads {
		size += len(p)
	}

	out := make([]byte, 8, size)
	binary.BigEndian.PutUint32(out, uint32(size))
	copy(out[4:], typ)

	for _, p := range payloads {
		out = append(out, p...)
	}

	return out
}

// fullBox is mp4Box with the version and flags of a full box.
func fullBox(typ string, payloads ...[]byte) []byte {
	return mp4Box(typ, append([][]byte{{0, 0, 0, 0}}, payloads...)...)
}

func be32(vals ...uint32) []byte {
	out := make([]byte, 0, 4*len(vals))
	for _, v := range vals {
		out = binary.BigEndian.AppendUint32(out, v)
	}

	return out
}

// alacStsd is a sample description table with one 'alac' entry carrying a
// stereo 16-bit cookie.
func alacStsd(entryCount uint32, entries ...[]byte) []byte {
	return fullBox("stsd", append([][]byte{be32(entryCount)}, entries...)...)
}

func alacEntry() []byte {
	cookie := make([]byte, 24)
	binary.BigEndian.PutUint32(cookie[0:], 4096) // frameLength
	cookie[5] = 16                               // bitDepth
	cookie[6] = 40                               // pb
	cookie[7] = 10                               // mb
	cookie[8] = 14                               // kb
	cookie[9] = 2                                // channels
	binary.BigEndian.PutUint16(cookie[10:], 255) // maxRun
	binary.BigEndian.PutUint32(cookie[20:], 44100)

	base := make([]byte, 28) // reserved, dataRefIdx, version 0, the audio fields

	return mp4Box("alac", base, cookie)
}

// container wraps a stbl in moov/trak/mdia/minf.
func container(stbl ...[]byte) []byte {
	return mp4Box("moov", mp4Box("trak", mp4Box("mdia", mp4Box("minf", mp4Box("stbl", stbl...)))))
}

func openMustFail(t *testing.T, file []byte, refusal error) {
	t.Helper()

	start := time.Now()

	_, err := alac.NewDecoder(bytes.NewReader(file))
	if !errors.Is(err, alac.ErrNoTrack) || !errors.Is(err, refusal) {
		t.Fatalf("expected ErrNoTrack and %v, got: %v", refusal, err)
	}

	if took := time.Since(start); took > time.Second {
		t.Fatalf("refused, but in %v", took)
	}

	t.Logf("refused: %v", err)
}

// A chunk offset table claiming 2^31-1 entries in a box of 4 bytes.
func TestNewDecoder_StcoCountPastBox(t *testing.T) {
	t.Parallel()

	openMustFail(t, container(
		alacStsd(1, alacEntry()),
		fullBox("stco", be32(0x7FFFFFFF, 0)),
		fullBox("stsc", be32(1, 1, 1, 1)),
		fullBox("stsz", be32(0, 1, 100)),
	), mp4.ErrTableTooLarge)
}

// A chunk offset table whose box claims a size past the file, so the count
// is within the box's word: the box is cut to the file, and the table is
// then larger than it.
func TestNewDecoder_StcoBoxSizePastFile(t *testing.T) {
	t.Parallel()

	stco := fullBox("stco", be32(0x3FFFFFF0, 0))
	binary.BigEndian.PutUint32(stco, 0xFFFFFFF0)

	openMustFail(t, container(
		alacStsd(1, alacEntry()),
		stco,
		fullBox("stsc", be32(1, 1, 1, 1)),
		fullBox("stsz", be32(0, 1, 100)),
	), mp4.ErrTableTooLarge)
}

// A sample-to-chunk table claiming 2^31-1 entries.
func TestNewDecoder_StscCountPastBox(t *testing.T) {
	t.Parallel()

	openMustFail(t, container(
		alacStsd(1, alacEntry()),
		fullBox("stco", be32(1, 0)),
		fullBox("stsc", be32(0x7FFFFFFF, 1, 1, 1)),
		fullBox("stsz", be32(0, 1, 100)),
	), mp4.ErrTableTooLarge)
}

// A sample size table claiming 2^31-1 per-sample sizes.
func TestNewDecoder_StszCountPastBox(t *testing.T) {
	t.Parallel()

	openMustFail(t, container(
		alacStsd(1, alacEntry()),
		fullBox("stco", be32(1, 0)),
		fullBox("stsc", be32(1, 1, 1, 1)),
		fullBox("stsz", be32(0, 0x7FFFFFFF, 100)),
	), mp4.ErrTableTooLarge)
}

// A constant sample size with a count no file could hold: no per-sample
// entries bound it, so the file's size must.
func TestNewDecoder_ConstantSizeCountPastFile(t *testing.T) {
	t.Parallel()

	openMustFail(t, container(
		alacStsd(1, alacEntry()),
		fullBox("stco", be32(1, 0)),
		fullBox("stsc", be32(1, 1, 0xFFFFFFFF, 1)),
		fullBox("stsz", be32(1, 0xFFFFFFFF)),
	), mp4.ErrInvalidStsz)
}

// A sample description table of 2^32-1 entries, the first of size 0, which
// held the walk in place for every one of them: the second's time is the
// only observable difference.
func TestNewDecoder_StsdZeroSizedEntries(t *testing.T) {
	t.Parallel()

	openMustFail(t, container(
		alacStsd(0xFFFFFFFF, be32(0, 0)),
		fullBox("stco", be32(1, 0)),
		fullBox("stsc", be32(1, 1, 1, 1)),
		fullBox("stsz", be32(0, 1, 100)),
	), mp4.ErrNoALACTrack)
}

// The same tables, well formed, open: the bounds refuse only what the file
// cannot hold.
func TestNewDecoder_BoundedTablesOpen(t *testing.T) {
	t.Parallel()

	file := container(
		alacStsd(1, alacEntry()),
		fullBox("stco", be32(1, 0)),
		fullBox("stsc", be32(1, 1, 1, 1)),
		fullBox("stsz", be32(0, 1, 100)),
	)

	if _, err := alac.NewDecoder(bytes.NewReader(file)); err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
}

// A sample description box whose header starts in the last 4 bytes of its
// parent: read whole from the file beyond the parent, then cut to the parent
// to a size below its own header, a negative payload, and an allocation from
// it. The 4 bytes are trailing bytes, not a child, and the track has no
// sample description. Found by a fuzz target over NewDecoder.
func TestNewDecoder_ChildHeaderPastParent(t *testing.T) {
	t.Parallel()

	openMustFail(t,
		[]byte("0000moov0000trak\x00\x00\x00\x1c0000000000000000000000000000mdia\x00"+
			"\x00\x00 0000000000000000000000000000\x00\x00"+
			"\x00-000000000000000000000000000000000000000000000minf\x00\x00\x00"+
			"\x10000000000000\x00\x00\x00\fstbl0000stsd"),
		mp4.ErrNoALACTrack)
}

// A sample description box whose 16-byte header (a large size) begins in the
// last 8 bytes of its parent: the cut to the parent leaves less than the
// header, which is refused as an invalid box size and ends the search, where
// the earlier version read the box whole and allocated from a negative length.
func TestNewDecoder_ChildLargeHeaderPastParent(t *testing.T) {
	t.Parallel()

	stbl := []byte("\x00\x00\x00\x10stbl\x00\x00\x00\x01stsd")

	tail := append([]byte("\x00\x00\x00\x00\x00\x00\x00\x40"), make([]byte, 16)...)

	openMustFail(t,
		mp4Box("moov", mp4Box("trak", mp4Box("mdia", mp4Box("minf", append(stbl, tail...))))),
		mp4.ErrInvalidBoxSize)
}
