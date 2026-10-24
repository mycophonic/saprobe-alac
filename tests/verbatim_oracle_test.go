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
	"fmt"
	"math/rand/v2"
	"testing"

	alac "github.com/mycophonic/saprobe-alac"
)

// An escape-coded element is its samples, written as they are: the one
// ALAC packet whose decoded output is known without any decoder, at every
// bit depth, 20 and 32 included, which no encoder produces. The packets
// here are built from ALACDecoder.cpp's layout (element tag, instance tag,
// 12 unused bits, the header nibble with partialFrame, bytesShifted and
// escapeFlag, a 32-bit sample count when partial, then the samples at
// bitDepth bits each, pairs interleaved in a CPE, an END element and the
// byte alignment), and the decoder is held to the PCM the samples spell:
// little-endian signed, 20-bit left-aligned in three bytes, the channels in
// SMPTE order from the bitstream's MPEG order.

// layoutElements is how a channel count is laid out as elements, Apple's
// kALACDefaultChannelLayouts: SCE and CPE in MPEG order, the LFE an SCE.
func layoutElements(channels int) []int {
	switch channels {
	case 1:
		return []int{1}
	case 2:
		return []int{2}
	case 3:
		return []int{1, 2}
	case 4:
		return []int{1, 2, 1}
	case 5:
		return []int{1, 2, 2}
	case 6:
		return []int{1, 2, 2, 1}
	case 7:
		return []int{1, 2, 2, 1, 1}
	default:
		return []int{1, 2, 2, 2, 1}
	}
}

// smpteOrder maps the bitstream channel index to the output position, as
// the README and ffmpeg's ff_alac_channel_layout_offsets state it.
func smpteOrder(channels int) []int {
	return [][]int{
		{0}, {0, 1}, {2, 0, 1}, {2, 0, 1, 3}, {2, 0, 1, 3, 4}, {2, 0, 1, 4, 5, 3}, {2, 0, 1, 4, 5, 6, 3}, {2, 6, 7, 0, 1, 4, 5, 3},
	}[channels-1]
}

// escapePacket writes one packet of samples[channel][i] as escape-coded
// elements. partial writes the sample count into every element; a full
// frame leaves it to the configuration.
func escapePacket(bitDepth uint8, samples [][]int32, partial bool) []byte {
	var w bitWriter

	depth := uint(bitDepth)
	count := len(samples[0])
	channel := 0

	for _, width := range layoutElements(len(samples)) {
		tag := uint64(0)
		if width == 2 {
			tag = 1
		}

		w.elementHeader(tag, partial, 0, 1)

		if partial {
			w.write(uint64(count), 32)
		}

		for i := range count {
			for c := channel; c < channel+width; c++ {
				w.write(uint64(uint32(samples[c][i])), depth)
			}
		}

		channel += width
	}

	w.write(7, 3) // END

	return w.buf
}

// expectPCM is the interleaved little-endian PCM the samples spell.
func expectPCM(bitDepth uint8, samples [][]int32) []byte {
	channels := len(samples)
	count := len(samples[0])
	order := smpteOrder(channels)
	bytesPer := map[uint8]int{16: 2, 20: 3, 24: 3, 32: 4}[bitDepth]
	out := make([]byte, count*channels*bytesPer)

	for i := range count {
		for c := range channels {
			v := samples[c][i]
			if bitDepth == 20 {
				v <<= 4
			}

			off := (i*channels + order[c]) * bytesPer

			switch bytesPer {
			case 2:
				binary.LittleEndian.PutUint16(out[off:], uint16(v))
			case 3:
				out[off], out[off+1], out[off+2] = byte(v), byte(v>>8), byte(v>>16)
			default:
				binary.LittleEndian.PutUint32(out[off:], uint32(v))
			}
		}
	}

	return out
}

// sampleShapes are the values a channel carries: the extremes of the depth,
// zero and its neighbours, and a pseudo-random spread.
func sampleShapes(rng *rand.Rand, bitDepth uint8, count, channel int) []int32 {
	maxV := int32(1)<<(bitDepth-1) - 1
	minV := -maxV - 1
	out := make([]int32, count)

	for i := range out {
		switch (i + channel) % 7 {
		case 0:
			out[i] = maxV
		case 1:
			out[i] = minV
		case 2:
			out[i] = 0
		case 3:
			out[i] = -1
		case 4:
			out[i] = 1
		case 5:
			out[i] = maxV - int32(channel)
		default:
			out[i] = int32(rng.Int64N(int64(maxV)-int64(minV)+1) + int64(minV))
		}
	}

	return out
}

func verbatimConfig(bitDepth, channels uint8, frameLength uint32) alac.PacketConfig {
	return alac.PacketConfig{
		FrameLength: frameLength,
		BitDepth:    bitDepth,
		NumChannels: channels,
		PB:          40,
		MB:          10,
		KB:          14,
		MaxRun:      255,
		SampleRate:  48000,
	}
}

func TestEscapePacketsAreTheirSamples(t *testing.T) {
	t.Parallel()

	for _, bitDepth := range []uint8{16, 20, 24, 32} {
		for channels := 1; channels <= 8; channels++ {
			t.Run(fmt.Sprintf("%dbit/%dch", bitDepth, channels), func(t *testing.T) {
				t.Parallel()

				rng := rand.New(rand.NewPCG(uint64(bitDepth), uint64(channels)))

				const frameLength = 64

				dec, err := alac.NewPacketDecoder(verbatimConfig(bitDepth, uint8(channels), frameLength))
				if err != nil {
					t.Fatal(err)
				}

				// A full frame, then partial frames of every length down to
				// one sample, through the same decoder: its state from a
				// longer frame must not reach a shorter one.
				for _, count := range []int{frameLength, 63, 33, 17, 8, 3, 2, 1} {
					samples := make([][]int32, channels)
					for c := range samples {
						samples[c] = sampleShapes(rng, bitDepth, count, c)
					}

					packet := escapePacket(bitDepth, samples, count != frameLength)

					got, err := dec.DecodePacket(packet)
					if err != nil {
						t.Fatalf("%d samples: %v", count, err)
					}

					if want := expectPCM(bitDepth, samples); !bytes.Equal(got, want) {
						t.Fatalf(
							"%d samples: the decoder's PCM is not the samples'\n got %x\nwant %x",
							count,
							firstN(got, 48),
							firstN(want, 48),
						)
					}
				}
			})
		}
	}
}

// The same packet twice through one decoder, and through a fresh one, is the
// same PCM: nothing of a packet survives into the next.
func TestDecodePacketHasNoMemory(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(7, 11))

	for _, bitDepth := range []uint8{16, 24, 32} {
		config := verbatimConfig(bitDepth, 2, 256)

		reused, err := alac.NewPacketDecoder(config)
		if err != nil {
			t.Fatal(err)
		}

		var packets [][]byte

		for _, count := range []int{256, 100, 256, 1, 256} {
			samples := [][]int32{sampleShapes(rng, bitDepth, count, 0), sampleShapes(rng, bitDepth, count, 1)}
			packets = append(packets, escapePacket(bitDepth, samples, count != 256))
		}

		for i, packet := range packets {
			fresh, freshErr := alac.NewPacketDecoder(config)
			if freshErr != nil {
				t.Fatal(freshErr)
			}

			want, wantErr := fresh.DecodePacket(packet)
			if wantErr != nil {
				t.Fatalf("%d-bit packet %d, fresh: %v", bitDepth, i, wantErr)
			}

			got, gotErr := reused.DecodePacket(packet)
			if gotErr != nil {
				t.Fatalf("%d-bit packet %d, reused: %v", bitDepth, i, gotErr)
			}

			if !bytes.Equal(got, want) {
				t.Fatalf("%d-bit packet %d: a reused decoder differs from a fresh one", bitDepth, i)
			}

			again, againErr := reused.DecodePacket(packet)
			if againErr != nil || !bytes.Equal(again, want) {
				t.Fatalf("%d-bit packet %d: the second decode differs: %v", bitDepth, i, againErr)
			}
		}
	}
}

// The sample count an element declares is bounded by the frame length and
// is one count for the packet: a second element declaring another is not a
// packet, and the output is sized by the count, not the configuration.
func TestEscapePacketCounts(t *testing.T) {
	t.Parallel()

	config := verbatimConfig(16, 2, 32)

	dec, err := alac.NewPacketDecoder(config)
	if err != nil {
		t.Fatal(err)
	}

	// Zero samples: a legal partial frame of nothing.
	var w bitWriter

	w.elementHeader(1, true, 0, 1)
	w.write(0, 32)
	w.write(7, 3)

	if got, zeroErr := dec.DecodePacket(w.buf); zeroErr != nil || len(got) != 0 {
		t.Fatalf("zero samples: %d bytes, %v", len(got), zeroErr)
	}

	// One above the frame length is refused.
	w = bitWriter{}
	w.elementHeader(1, true, 0, 1)
	w.write(33, 32)
	w.write(0, 33*2*16)
	w.write(7, 3)

	if _, overErr := dec.DecodePacket(w.buf); overErr == nil {
		t.Fatal("33 samples in a 32-sample frame were decoded")
	}

	// A full frame without END: the element loop stops at the channel count.
	samples := [][]int32{
		sampleShapes(rand.New(rand.NewPCG(1, 2)), 16, 32, 0),
		sampleShapes(rand.New(rand.NewPCG(3, 4)), 16, 32, 1),
	}
	packet := escapePacket(16, samples, false)
	packet = packet[:len(packet)-1] // drop the END byte

	got, err := dec.DecodePacket(packet)
	if err != nil {
		t.Fatalf("a full frame without END: %v", err)
	}

	if want := expectPCM(16, samples); !bytes.Equal(got, want) {
		t.Fatal("a full frame without END decodes to other samples")
	}
}

func firstN(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}

	return b
}
