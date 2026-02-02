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
package alac

import (
	"fmt"
	"io"

	alacint "github.com/mycophonic/saprobe-alac/internal/alac"
	mp4int "github.com/mycophonic/saprobe-alac/internal/mp4"
)

// StreamDecoder streams decoded PCM from an ALAC M4A/MP4 source.
// The MP4 container (sample table, config) is parsed upfront; packets are
// decoded on demand via Read.
type StreamDecoder struct {
	reader    io.ReadSeeker
	dec       *Decoder
	samples   []mp4int.SampleInfo
	sampleIdx int
	packetBuf []byte

	// Per-packet PCM buffer, drained by Read.
	buf    []byte
	bufOff int
	eof    bool
}

// NewStreamDecoder opens an M4A/MP4 stream containing ALAC audio and returns
// a streaming decoder. The container structure is parsed immediately; PCM data
// is decoded packet-by-packet on demand via Read.
//
//nolint:varnamelen // rs is idiomatic for io.ReadSeeker
func NewStreamDecoder(rs io.ReadSeeker) (*StreamDecoder, error) {
	cookie, samples, err := mp4int.FindALACTrack(rs)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoTrack, err)
	}

	config, err := ParseConfig(cookie)
	if err != nil {
		return nil, fmt.Errorf("parsing ALAC config: %w", err)
	}

	dec, err := NewDecoder(config)
	if err != nil {
		return nil, err
	}

	bps := alacint.BytesPerSample(config.BitDepth)
	frameBytes := int(config.FrameLength) * int(config.NumChannels) * bps

	return &StreamDecoder{
		reader:  rs,
		dec:     dec,
		samples: samples,
		buf:     make([]byte, 0, frameBytes),
	}, nil
}

// Format returns the PCM output format.
func (s *StreamDecoder) Format() PCMFormat { return s.dec.Format() }

// Read reads decoded PCM bytes from the ALAC stream.
func (s *StreamDecoder) Read(p []byte) (int, error) { //nolint:varnamelen // p is idiomatic for io.Reader.Read
	total := 0

	for len(p) > 0 {
		// Drain buffered packet data.
		if s.bufOff < len(s.buf) {
			n := copy(p, s.buf[s.bufOff:])
			s.bufOff += n
			total += n
			p = p[n:]

			continue
		}

		if s.eof {
			if total > 0 {
				return total, nil
			}

			return 0, io.EOF
		}

		if s.sampleIdx >= len(s.samples) {
			s.eof = true

			if total > 0 {
				return total, nil
			}

			return 0, io.EOF
		}

		// Decode next packet.
		sample := s.samples[s.sampleIdx]

		if int(sample.Size) > len(s.packetBuf) {
			s.packetBuf = make([]byte, sample.Size)
		}

		packet := s.packetBuf[:sample.Size]

		if _, err := s.reader.Seek(int64(sample.Offset), io.SeekStart); err != nil {
			return total, fmt.Errorf("seeking to sample %d at offset %d: %w", s.sampleIdx, sample.Offset, err)
		}

		if _, err := io.ReadFull(s.reader, packet); err != nil {
			return total, fmt.Errorf("reading sample %d: %w", s.sampleIdx, err)
		}

		// Ensure buf has capacity for a full frame.
		s.buf = s.buf[:cap(s.buf)]

		n, err := s.dec.decodePacketInto(packet, s.buf)
		if err != nil {
			return total, fmt.Errorf("decoding packet %d: %w", s.sampleIdx, err)
		}

		s.buf = s.buf[:n]
		s.bufOff = 0
		s.sampleIdx++
	}

	return total, nil
}

// Decode reads an M4A/MP4 stream and decodes the first ALAC audio track
// to interleaved little-endian signed PCM bytes.
func Decode(reader io.ReadSeeker) ([]byte, PCMFormat, error) {
	dec, err := NewStreamDecoder(reader)
	if err != nil {
		return nil, PCMFormat{}, err
	}

	pcm, err := io.ReadAll(dec)
	if err != nil {
		return nil, PCMFormat{}, fmt.Errorf("decoding alac: %w", err)
	}

	return pcm, dec.Format(), nil
}
