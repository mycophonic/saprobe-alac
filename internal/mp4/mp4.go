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

//nolint:gosec // Integer conversions are bounded by MP4 atom sizes.
package mp4

import (
	"encoding/binary"
	"fmt"
	"io"

	gomp4 "github.com/abema/go-mp4"
)

// SampleInfo holds the byte offset and size of a single encoded ALAC packet
// within the MP4 container.
type SampleInfo struct {
	Offset uint64
	Size   uint32
}

// FindALACTrack walks the MP4 box tree to locate the first track containing
// an ALAC sample entry. It returns the magic cookie and a flat sample table.
func FindALACTrack(reader io.ReadSeeker) ([]byte, []SampleInfo, error) {
	stbls, err := gomp4.ExtractBox(reader, nil, gomp4.BoxPath{
		gomp4.BoxTypeMoov(), gomp4.BoxTypeTrak(), gomp4.BoxTypeMdia(),
		gomp4.BoxTypeMinf(), gomp4.BoxTypeStbl(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("reading container structure: %w", err)
	}

	for _, stbl := range stbls {
		cookie, err := extractCookie(reader, stbl)
		if err != nil {
			continue // not an ALAC track
		}

		samples, err := buildSampleTable(reader, stbl)
		if err != nil {
			return nil, nil, fmt.Errorf("building sample table: %w", err)
		}

		return cookie, samples, nil
	}

	return nil, nil, ErrNoALACTrack
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
// 'frma'+'alac' atoms which ParseConfig handles).
func extractCookie(reader io.ReadSeeker, stbl *gomp4.BoxInfo) ([]byte, error) {
	stsds, err := gomp4.ExtractBox(reader, stbl, gomp4.BoxPath{gomp4.BoxTypeStsd()})
	if err != nil || len(stsds) == 0 {
		return nil, ErrNoALACTrack
	}

	stsd := stsds[0]
	payloadSize := int(stsd.Size - stsd.HeaderSize)
	data := make([]byte, payloadSize)

	if _, err := reader.Seek(int64(stsd.Offset+stsd.HeaderSize), io.SeekStart); err != nil {
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
		if entrySize < sampleEntryHeaderSize+sampleEntryBaseSize || pos+entrySize > len(data) {
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

// buildSampleTable constructs a flat list of sample offsets and sizes from
// the stco/co64, stsc, and stsz boxes within the given stbl box.
func buildSampleTable(reader io.ReadSeeker, stbl *gomp4.BoxInfo) ([]SampleInfo, error) {
	chunkOffsets, err := readChunkOffsets(reader, stbl)
	if err != nil {
		return nil, err
	}

	stscEntries, err := readStsc(reader, stbl)
	if err != nil {
		return nil, err
	}

	entrySizes, constantSize, sampleCount, err := readStsz(reader, stbl)
	if err != nil {
		return nil, err
	}

	samples := make([]SampleInfo, 0, sampleCount)
	sampleIdx := 0

	for chunkIdx := range chunkOffsets {
		spc := lookupSamplesPerChunk(stscEntries, uint32(chunkIdx+1)) // stsc uses 1-based chunk numbers
		offset := chunkOffsets[chunkIdx]

		for s := uint32(0); s < spc && sampleIdx < int(sampleCount); s++ {
			var size uint32
			if constantSize != 0 {
				size = constantSize
			} else {
				size = entrySizes[sampleIdx]
			}

			samples = append(samples, SampleInfo{Offset: offset, Size: size})
			offset += uint64(size)
			sampleIdx++
		}
	}

	return samples, nil
}

func readChunkOffsets(reader io.ReadSeeker, stbl *gomp4.BoxInfo) ([]uint64, error) {
	// Try 32-bit stco first.
	if boxes, err := gomp4.ExtractBoxWithPayload(reader, stbl,
		gomp4.BoxPath{gomp4.BoxTypeStco()}); err == nil && len(boxes) > 0 {
		if stco, ok := boxes[0].Payload.(*gomp4.Stco); ok {
			offsets := make([]uint64, len(stco.ChunkOffset))
			for i, off := range stco.ChunkOffset {
				offsets[i] = uint64(off)
			}

			return offsets, nil
		}
	}

	// Fall back to 64-bit co64.
	boxes, err := gomp4.ExtractBoxWithPayload(reader, stbl, gomp4.BoxPath{gomp4.BoxTypeCo64()})
	if err != nil || len(boxes) == 0 {
		return nil, ErrNoChunkOffset
	}

	co64, ok := boxes[0].Payload.(*gomp4.Co64)
	if !ok {
		return nil, ErrInvalidCo64
	}

	return co64.ChunkOffset, nil
}

func readStsc(reader io.ReadSeeker, stbl *gomp4.BoxInfo) ([]gomp4.StscEntry, error) {
	boxes, err := gomp4.ExtractBoxWithPayload(reader, stbl, gomp4.BoxPath{gomp4.BoxTypeStsc()})
	if err != nil || len(boxes) == 0 {
		return nil, ErrNoStsc
	}

	stsc, ok := boxes[0].Payload.(*gomp4.Stsc)
	if !ok {
		return nil, ErrInvalidStsc
	}

	return stsc.Entries, nil
}

//revive:disable:function-result-limit,confusing-results
func readStsz(reader io.ReadSeeker, stbl *gomp4.BoxInfo) ([]uint32, uint32, uint32, error) {
	boxes, err := gomp4.ExtractBoxWithPayload(reader, stbl, gomp4.BoxPath{gomp4.BoxTypeStsz()})
	if err != nil || len(boxes) == 0 {
		return nil, 0, 0, ErrNoStsz
	}

	stsz, ok := boxes[0].Payload.(*gomp4.Stsz)
	if !ok {
		return nil, 0, 0, ErrInvalidStsz
	}

	return stsz.EntrySize, stsz.SampleSize, stsz.SampleCount, nil
}

// lookupSamplesPerChunk finds the samples-per-chunk count for a 1-based
// chunk number from the stsc run-length table.
func lookupSamplesPerChunk(entries []gomp4.StscEntry, chunkNumber uint32) uint32 {
	var spc uint32

	for _, e := range entries {
		if e.FirstChunk > chunkNumber {
			break
		}

		spc = e.SamplesPerChunk
	}

	return spc
}
