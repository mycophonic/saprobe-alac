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

package alac_test

import (
	"math/rand/v2"
	"testing"

	alacint "github.com/mycophonic/saprobe-alac/internal/alac"
)

// A plain bit reader as the oracle for BitBuffer: random sequences of
// Read (1 to 16 bits, its documented width), ReadSmall, ReadOne, Advance and ByteAlign over random
// bytes give the oracle's values while the data lasts, BitsLeft is the
// oracle's count, and reading on past the end is PastEnd and zeros, never a
// panic.
type plainBits struct {
	data []byte
	pos  int
}

func (p *plainBits) read(n int) uint32 {
	var x uint32

	for range n {
		var b uint32
		if p.pos < len(p.data)*8 {
			b = uint32(p.data[p.pos/8]>>(7-p.pos%8)) & 1
		}

		x = x<<1 | b
		p.pos++
	}

	return x
}

func TestBitBufferMatchesAPlainBitReader(t *testing.T) {
	t.Parallel()

	for seed := range 300 {
		rng := rand.New(rand.NewPCG(uint64(seed), 5))
		data := make([]byte, rng.IntN(100))

		for i := range data {
			data[i] = byte(rng.IntN(256))
		}

		var bb alacint.BitBuffer

		bb.Reset(data)
		oracle := &plainBits{data: data}
		total := len(data) * 8

		for step := 0; oracle.pos+40 <= total; step++ {
			var got, want uint32

			switch rng.IntN(5) {
			case 0:
				n := 1 + rng.IntN(16)
				want = oracle.read(n)
				got = bb.Read(uint8(n))
			case 1:
				n := 1 + rng.IntN(8)
				want = oracle.read(n)
				got = uint32(bb.ReadSmall(uint8(n)))
			case 2:
				want = oracle.read(1)
				got = uint32(bb.ReadOne())
			case 3:
				n := rng.IntN(40)
				oracle.read(n)
				bb.Advance(uint32(n))
			default:
				if r := oracle.pos % 8; r != 0 {
					oracle.read(8 - r)
				}

				bb.ByteAlign()
			}

			if got != want {
				t.Fatalf("seed %d step %d: read %d, the oracle says %d", seed, step, got, want)
			}

			if left := bb.BitsLeft(); left != int64(total-oracle.pos) {
				t.Fatalf("seed %d step %d: BitsLeft %d, the oracle has %d", seed, step, left, total-oracle.pos)
			}

			if bb.PastEnd() {
				t.Fatalf("seed %d step %d: PastEnd with %d bits left", seed, step, total-oracle.pos)
			}
		}

		// On past the end: zeros and PastEnd, whatever is asked.
		bb.Advance(uint32(total))

		for range 4 {
			if v := bb.Read(16); v != 0 {
				t.Fatalf("seed %d: past the end, Read gave %d", seed, v)
			}
		}

		if !bb.PastEnd() {
			t.Fatalf("seed %d: not PastEnd after reading past the end of %d bytes", seed, len(data))
		}
	}
}
