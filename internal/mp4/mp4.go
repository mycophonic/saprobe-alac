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

package mp4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// SampleInfo holds the byte offset and size of a single encoded ALAC packet
// within the MP4 container.
type SampleInfo struct {
	Offset uint64
	Size   uint32
}

// stscEntry mirrors the ISO 14496-12 sample-to-chunk table entry.
type stscEntry struct {
	FirstChunk      uint32
	SamplesPerChunk uint32
}

// boxInfo holds the position and size of a parsed box.
type boxInfo struct {
	// Offset of the box header start in the file.
	offset int64
	// Total box size including header.
	size int64
	// Header size (8 for normal, 16 for extended).
	headerSize int64
	// Four-character box type code.
	fourCC [4]byte
}

const (
	smallHeaderSize = 8
	largeHeaderSize = 16
	fullBoxSize     = 4 // version(1) + flags(3)
)

// readBoxInfo reads a single box header from the current position.
// Returns io.EOF if there are no more bytes to read.
func readBoxInfo(reader io.ReadSeeker) (boxInfo, error) {
	offset, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return boxInfo{}, fmt.Errorf("seeking current position: %w", err)
	}

	var header [largeHeaderSize]byte

	if _, err := io.ReadFull(reader, header[:smallHeaderSize]); err != nil {
		return boxInfo{}, fmt.Errorf("reading box header: %w", err)
	}

	info := boxInfo{
		offset:     offset,
		headerSize: smallHeaderSize,
		fourCC:     [4]byte{header[4], header[5], header[6], header[7]},
	}

	rawSize := binary.BigEndian.Uint32(header[:4])

	switch rawSize {
	case 0:
		// Box extends to end of file.
		end, seekErr := reader.Seek(0, io.SeekEnd)
		if seekErr != nil {
			return boxInfo{}, fmt.Errorf("seeking to end of file: %w", seekErr)
		}

		info.size = end - offset

		if _, seekErr := reader.Seek(offset+info.headerSize, io.SeekStart); seekErr != nil {
			return boxInfo{}, fmt.Errorf("seeking past box header: %w", seekErr)
		}

	case 1:
		// Extended 64-bit size.
		if _, err := io.ReadFull(reader, header[smallHeaderSize:largeHeaderSize]); err != nil {
			return boxInfo{}, fmt.Errorf("reading extended box header: %w", err)
		}

		largeSize := binary.BigEndian.Uint64(header[smallHeaderSize:largeHeaderSize])
		if largeSize > math.MaxInt64 {
			return boxInfo{}, fmt.Errorf("%w: size %d at offset %d", ErrInvalidBoxSize, largeSize, offset)
		}

		info.headerSize = largeHeaderSize
		info.size = int64(largeSize)

	default:
		info.size = int64(rawSize)
	}

	if info.size < info.headerSize {
		return boxInfo{}, fmt.Errorf("%w: size %d at offset %d", ErrInvalidBoxSize, info.size, offset)
	}

	return info, nil
}

// payloadOffset returns the file offset where this box's payload begins.
func (info *boxInfo) payloadOffset() int64 {
	return info.offset + info.headerSize
}

// seekToPayload seeks to the start of this box's payload.
func (info *boxInfo) seekToPayload(reader io.ReadSeeker) error {
	_, err := reader.Seek(info.payloadOffset(), io.SeekStart)
	if err != nil {
		return fmt.Errorf("seeking to box payload: %w", err)
	}

	return nil
}

// seekToEnd seeks past this box (to the start of the next sibling).
func (info *boxInfo) seekToEnd(reader io.ReadSeeker) error {
	_, err := reader.Seek(info.offset+info.size, io.SeekStart)
	if err != nil {
		return fmt.Errorf("seeking past box: %w", err)
	}

	return nil
}

// payloadSize returns the number of payload bytes (total size minus header).
func (info *boxInfo) payloadSize() int64 {
	return info.size - info.headerSize
}

// iterChildren calls callback for each direct child box within parent's payload.
// callback returns true to stop iteration early.
//
//nolint:gocognit // the box walk: position, header, callback, skip, with EOF ending it; one loop, no state to name
func iterChildren(
	reader io.ReadSeeker,
	parent *boxInfo,
	callback func(child boxInfo) (stop bool, err error),
) error {
	if err := parent.seekToPayload(reader); err != nil {
		return err
	}

	end := parent.offset + parent.size

	for {
		pos, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("seeking current position: %w", err)
		}

		// Fewer bytes than a box header before the parent's end: trailing
		// bytes, not a child.
		if end-pos < smallHeaderSize {
			return nil
		}

		child, err := readBoxInfo(reader)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}

			return err
		}

		// A child ends within its parent, and a top-level box within the
		// file: one that claims more is cut there, so a table's size bounds
		// what its box can hold, and a file cut short still opens.
		if child.offset+child.size > end {
			child.size = end - child.offset

			// The cut must leave the header itself: a size below it would make
			// the payload negative, and a table's allocation with it.
			if child.size < child.headerSize {
				return fmt.Errorf("%w: box %q at %d runs past its parent",
					ErrInvalidBoxSize, child.fourCC[:], child.offset)
			}
		}

		stop, err := callback(child)
		if err != nil {
			return err
		}

		if stop {
			return nil
		}

		if err := child.seekToEnd(reader); err != nil {
			return err
		}
	}
}

// findChild finds the first child box with the given fourCC inside parent.
func findChild(reader io.ReadSeeker, parent *boxInfo, target [4]byte) (boxInfo, bool, error) {
	var found boxInfo

	var matched bool

	err := iterChildren(reader, parent, func(child boxInfo) (bool, error) {
		if child.fourCC == target {
			found = child
			matched = true

			return true, nil
		}

		return false, nil
	})

	return found, matched, err
}

// findDescendant walks a path of fourCCs from parent, descending one level per element.
func findDescendant(reader io.ReadSeeker, parent *boxInfo, path [][4]byte) (boxInfo, bool, error) {
	current := *parent

	for _, target := range path {
		child, found, err := findChild(reader, &current, target)
		if err != nil {
			return boxInfo{}, false, err
		}

		if !found {
			return boxInfo{}, false, nil
		}

		current = child
	}

	return current, true, nil
}

// FindALACTrack walks the MP4 box tree to locate the first track containing
// an ALAC sample entry. It returns the magic cookie and a flat sample table.
func FindALACTrack(reader io.ReadSeeker) ([]byte, []SampleInfo, error) {
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return nil, nil, fmt.Errorf("seeking to start: %w", err)
	}

	// Find the moov box.
	fileEnd, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, nil, fmt.Errorf("seeking to end: %w", err)
	}

	root := boxInfo{offset: 0, size: fileEnd, headerSize: 0}
	fccMoov := [4]byte{'m', 'o', 'o', 'v'}

	moov, found, err := findChild(reader, &root, fccMoov)
	if err != nil {
		return nil, nil, fmt.Errorf("reading container structure: %w", err)
	}

	if !found {
		return nil, nil, ErrNoALACTrack
	}

	// Iterate trak boxes within moov, descend to stbl in each.
	var cookie []byte

	var samples []SampleInfo

	fccTrak := [4]byte{'t', 'r', 'a', 'k'}

	err = iterChildren(reader, &moov, func(trak boxInfo) (bool, error) {
		if trak.fourCC != fccTrak {
			return false, nil
		}

		track, ours, trakErr := alacTrack(reader, &trak, fileEnd)
		if trakErr != nil || !ours {
			return false, trakErr
		}

		cookie = track.cookie
		samples = track.samples

		return true, nil // found it, stop
	})
	if err != nil {
		return nil, nil, err
	}

	if cookie == nil {
		return nil, nil, ErrNoALACTrack
	}

	return cookie, samples, nil
}

const (
	alacFourCC            = "alac"
	sampleEntryHeaderSize = 8  // box header: size(4) + type(4)
	sampleEntryBaseSize   = 28 // standard AudioSampleEntry fields
	sampleEntryV1Extra    = 16 // QuickTime version 1 extra fields
	stsdPayloadHeader     = 8  // version(1) + flags(3) + entryCount(4)
)

// extractCookie reads the stsd box from stbl, finds an 'alac' sample entry,
// and extracts the raw magic cookie (ALACSpecificConfig, possibly wrapped in
// 'frma'+'alac' atoms which ParseMagicCookie handles).
//
//nolint:gocognit // one walk over the stsd entries with the size, type and version checks the format defines
func extractCookie(reader io.ReadSeeker, stbl *boxInfo) ([]byte, error) {
	fccStsd := [4]byte{'s', 't', 's', 'd'}

	stsd, found, err := findChild(reader, stbl, fccStsd)
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, ErrNoALACTrack
	}

	payloadLen := int(stsd.payloadSize())
	data := make([]byte, payloadLen)

	if err := stsd.seekToPayload(reader); err != nil {
		return nil, fmt.Errorf("seeking to stsd payload: %w", err)
	}

	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, fmt.Errorf("reading stsd payload: %w", err)
	}

	if len(data) < stsdPayloadHeader {
		return nil, ErrNoALACTrack
	}

	entryCount := binary.BigEndian.Uint32(data[4:8])
	pos := stsdPayloadHeader

	for range entryCount {
		if pos+sampleEntryHeaderSize > len(data) {
			break
		}

		entrySize := int(binary.BigEndian.Uint32(data[pos : pos+4]))

		// An entry smaller than its header, or past the box, ends the table:
		// a zero size would otherwise hold the walk in place for entryCount.
		if entrySize < sampleEntryHeaderSize || pos+entrySize > len(data) {
			break
		}

		if entrySize < sampleEntryHeaderSize+sampleEntryBaseSize {
			pos += entrySize

			continue
		}

		if string(data[pos+4:pos+8]) != alacFourCC {
			pos += entrySize

			continue
		}

		// Found ALAC sample entry. Determine cookie start from QT version field.
		// Layout after 8-byte box header: reserved(6) + dataRefIdx(2) + version(2) + ...
		// Version is at offset 8 within the payload (i.e., pos + headerSize + 8).
		version := binary.BigEndian.Uint16(data[pos+sampleEntryHeaderSize+8 : pos+sampleEntryHeaderSize+10])

		skip := sampleEntryHeaderSize + sampleEntryBaseSize
		if version == 1 {
			skip += sampleEntryV1Extra
		}

		cookieStart := pos + skip
		cookieEnd := pos + entrySize

		if cookieStart >= cookieEnd {
			return nil, ErrInvalidEntry
		}

		return data[cookieStart:cookieEnd], nil
	}

	return nil, ErrNoALACTrack
}

// alacTrackInfo is what FindALACTrack takes from the trak that is ours.
type alacTrackInfo struct {
	cookie  []byte
	samples []SampleInfo
}

// alacTrack reads a trak's ALAC cookie and sample table. A track without a
// sample table, with no ALAC entry or with a malformed one is not ours and
// the search continues; a box the file cannot hold is the file's fault and
// ends it.
func alacTrack(reader io.ReadSeeker, trak *boxInfo, fileEnd int64) (track *alacTrackInfo, ours bool, err error) {
	fccMdia := [4]byte{'m', 'd', 'i', 'a'}
	fccMinf := [4]byte{'m', 'i', 'n', 'f'}
	fccStbl := [4]byte{'s', 't', 'b', 'l'}

	stbl, found, err := findDescendant(reader, trak, [][4]byte{fccMdia, fccMinf, fccStbl})
	if err != nil || !found {
		return nil, false, err
	}

	cookie, err := extractCookie(reader, &stbl)
	if err != nil {
		if errors.Is(err, ErrNoALACTrack) || errors.Is(err, ErrInvalidEntry) {
			return nil, false, nil
		}

		return nil, false, err
	}

	samples, err := buildSampleTable(reader, &stbl, fileEnd)
	if err != nil {
		return nil, false, fmt.Errorf("building sample table: %w", err)
	}

	return &alacTrackInfo{cookie: cookie, samples: samples}, true, nil
}

// buildSampleTable constructs a flat list of sample offsets and sizes from
// the stco/co64, stsc, and stsz boxes within the given stbl box.
func buildSampleTable(reader io.ReadSeeker, stbl *boxInfo, fileEnd int64) ([]SampleInfo, error) {
	chunkOffsets, err := readChunkOffsets(reader, stbl)
	if err != nil {
		return nil, err
	}

	stscEntries, err := readStsc(reader, stbl)
	if err != nil {
		return nil, err
	}

	entrySizes, constantSize, sampleCount, err := readStsz(reader, stbl, fileEnd)
	if err != nil {
		return nil, err
	}

	samples := make([]SampleInfo, 0, sampleCount)
	sampleIdx := 0

	for chunkIdx := range chunkOffsets {
		samplesInChunk := lookupSamplesPerChunk(stscEntries, uint32(chunkIdx+1)) // stsc uses 1-based chunk numbers
		chunkOffset := chunkOffsets[chunkIdx]

		for iter := uint32(0); iter < samplesInChunk && sampleIdx < int(sampleCount); iter++ {
			// readStsz returns the per-sample sizes, or none and the constant size.
			size := constantSize
			if entrySizes != nil {
				size = entrySizes[sampleIdx]
			}

			// The offset is seeked to as an int64 (co64 is 64-bit, and the sizes accumulate).
			if chunkOffset > math.MaxInt64 {
				return nil, fmt.Errorf("%w: sample %d at %d", ErrInvalidChunkOffset, sampleIdx, chunkOffset)
			}

			samples = append(samples, SampleInfo{Offset: chunkOffset, Size: size})
			chunkOffset += uint64(size)
			sampleIdx++
		}
	}

	return samples, nil
}

func readChunkOffsets(reader io.ReadSeeker, stbl *boxInfo) ([]uint64, error) {
	fccStco := [4]byte{'s', 't', 'c', 'o'}
	fccCo64 := [4]byte{'c', 'o', '6', '4'}

	// Try 32-bit stco first.
	if stco, stcoFound, err := findChild(reader, stbl, fccStco); err == nil && stcoFound {
		return readStco(reader, &stco)
	}

	// Fall back to 64-bit co64.
	co64, found, err := findChild(reader, stbl, fccCo64)
	if err != nil || !found {
		return nil, ErrNoChunkOffset
	}

	return readCo64(reader, &co64)
}

// readStco reads a 32-bit chunk offset box.
// Layout: FullBox(4) + entryCount(4) + entryCount × uint32.
func readStco(reader io.ReadSeeker, box *boxInfo) ([]uint64, error) {
	if err := box.seekToPayload(reader); err != nil {
		return nil, err
	}

	var header [fullBoxSize + 4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoChunkOffset, err)
	}

	count := binary.BigEndian.Uint32(header[fullBoxSize:])
	if err := tableFits(box, len(header), count, 4); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoChunkOffset, err)
	}

	buf := make([]byte, int(count)*4)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoChunkOffset, err)
	}

	offsets := make([]uint64, count)
	for idx := range count {
		offsets[idx] = uint64(binary.BigEndian.Uint32(buf[idx*4:]))
	}

	return offsets, nil
}

// readCo64 reads a 64-bit chunk offset box.
// Layout: FullBox(4) + entryCount(4) + entryCount × uint64.
func readCo64(reader io.ReadSeeker, box *boxInfo) ([]uint64, error) {
	if err := box.seekToPayload(reader); err != nil {
		return nil, err
	}

	var header [fullBoxSize + 4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCo64, err)
	}

	count := binary.BigEndian.Uint32(header[fullBoxSize:])
	if err := tableFits(box, len(header), count, 8); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoChunkOffset, err)
	}

	buf := make([]byte, int(count)*8)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCo64, err)
	}

	offsets := make([]uint64, count)
	for idx := range count {
		offsets[idx] = binary.BigEndian.Uint64(buf[idx*8:])
	}

	return offsets, nil
}

// readStsc reads the sample-to-chunk box.
// Layout: FullBox(4) + entryCount(4) + entryCount × (firstChunk(4) + samplesPerChunk(4) + sampleDescIdx(4)).
func readStsc(reader io.ReadSeeker, stbl *boxInfo) ([]stscEntry, error) {
	fccStsc := [4]byte{'s', 't', 's', 'c'}

	box, found, err := findChild(reader, stbl, fccStsc)
	if err != nil || !found {
		return nil, ErrNoStsc
	}

	if err := box.seekToPayload(reader); err != nil {
		return nil, err
	}

	var header [fullBoxSize + 4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidStsc, err)
	}

	count := binary.BigEndian.Uint32(header[fullBoxSize:])

	const entryBytes = 12 // 3 × uint32

	if err := tableFits(&box, len(header), count, entryBytes); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidStsc, err)
	}

	buf := make([]byte, int(count)*entryBytes)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidStsc, err)
	}

	entries := make([]stscEntry, count)
	for idx := range count {
		off := int(idx) * entryBytes
		entries[idx] = stscEntry{
			FirstChunk:      binary.BigEndian.Uint32(buf[off:]),
			SamplesPerChunk: binary.BigEndian.Uint32(buf[off+4:]),
			// sampleDescriptionIndex at buf[off+8:] is unused.
		}
	}

	return entries, nil
}

// readStsz reads the sample size box.
// Layout: FullBox(4) + sampleSize(4) + sampleCount(4) + [sampleCount × uint32 if sampleSize == 0].
//
//revive:disable:function-result-limit,confusing-results
func readStsz(reader io.ReadSeeker, stbl *boxInfo, fileEnd int64) ([]uint32, uint32, uint32, error) {
	fccStsz := [4]byte{'s', 't', 's', 'z'}

	box, found, err := findChild(reader, stbl, fccStsz)
	if err != nil || !found {
		return nil, 0, 0, ErrNoStsz
	}

	if err := box.seekToPayload(reader); err != nil {
		return nil, 0, 0, fmt.Errorf("seeking to stsz payload: %w", err)
	}

	var header [fullBoxSize + 8]byte // version+flags + sampleSize + sampleCount
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, 0, 0, fmt.Errorf("%w: %w", ErrInvalidStsz, err)
	}

	sampleSize := binary.BigEndian.Uint32(header[fullBoxSize:])
	sampleCount := binary.BigEndian.Uint32(header[fullBoxSize+4:])

	if sampleSize != 0 {
		// Constant size: no per-sample entries, so the count is bounded by
		// the file the samples lie in, not by the box.
		fileBytes := uint64(fileEnd) // #nosec G115 -- a Seek result, 0 or more
		if uint64(sampleCount)*uint64(sampleSize) > fileBytes {
			return nil, 0, 0, fmt.Errorf("%w: %d samples of %d bytes in a %d-byte file",
				ErrInvalidStsz, sampleCount, sampleSize, fileEnd)
		}

		return nil, sampleSize, sampleCount, nil
	}

	if err := tableFits(&box, len(header), sampleCount, 4); err != nil {
		return nil, 0, 0, fmt.Errorf("%w: %w", ErrInvalidStsz, err)
	}

	buf := make([]byte, int(sampleCount)*4)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, 0, 0, fmt.Errorf("%w: %w", ErrInvalidStsz, err)
	}

	sizes := make([]uint32, sampleCount)
	for idx := range sampleCount {
		sizes[idx] = binary.BigEndian.Uint32(buf[idx*4:])
	}

	return sizes, 0, sampleCount, nil
}

// tableFits checks that a table of count entries of entryBytes each lies
// within its box after the header read, before the table is allocated: the
// count is the file's word, the box's size (cut to its parent and the file)
// what the file holds.
func tableFits(box *boxInfo, headerLen int, count uint32, entryBytes int) error {
	room := box.payloadSize() - int64(headerLen)
	if need := int64(count) * int64(entryBytes); need > room {
		return fmt.Errorf("%w: %d entries need %d bytes, the box holds %d",
			ErrTableTooLarge, count, need, room)
	}

	return nil
}

// lookupSamplesPerChunk finds the samples-per-chunk count for a 1-based
// chunk number from the stsc run-length table.
func lookupSamplesPerChunk(entries []stscEntry, chunkNumber uint32) uint32 {
	var samplesPerChunk uint32

	for _, entry := range entries {
		if entry.FirstChunk > chunkNumber {
			break
		}

		samplesPerChunk = entry.SamplesPerChunk
	}

	return samplesPerChunk
}
