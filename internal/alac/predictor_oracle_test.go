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
	"slices"
	"testing"

	alacint "github.com/mycophonic/saprobe-alac/internal/alac"
)

// wrap folds v into chanBits bits with sign, as the predictor's output is.
func wrap(v int32, chanBits uint32) int32 {
	shift := 32 - chanBits

	return (v << shift) >> shift
}

// What dp_dec.c defines without its adaptive coefficients, which no encoder
// here reaches: order 0 copies the residuals; order 31 is the running sum
// folded to the channel width; any order's first samples, up to the order,
// are that running sum too; nothing is written for no samples; and the
// predictor in place over its input equals the predictor into a fresh
// buffer, for every order, as the decoder calls it.
func TestUnpcBlockAgainstItsDefinition(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(9, 9))

	for seed := range 200 {
		num := 1 + rng.IntN(300)
		chanBits := uint32([]int{16, 20, 24, 32}[rng.IntN(4)])
		pc1 := make([]int32, num)

		for i := range pc1 {
			pc1[i] = int32(rng.IntN(1<<16) - 1<<15)
			if rng.IntN(16) == 0 {
				pc1[i] = int32(rng.Uint32())
			}
		}

		// Order 0: a copy.
		out := make([]int32, num)
		alacint.UnpcBlock(pc1, out, num, nil, 0, chanBits, 0)

		if !slices.Equal(out, pc1) {
			t.Fatalf("seed %d: order 0 is not a copy", seed)
		}

		// Order 31: the running sum, folded.
		alacint.UnpcBlock(pc1, out, num, nil, alacint.NumActiveDelta, chanBits, 0)

		want := make([]int32, num)
		want[0] = pc1[0]

		for i := 1; i < num; i++ {
			want[i] = wrap(pc1[i]+want[i-1], chanBits)
		}

		if !slices.Equal(out, want) {
			t.Fatalf("seed %d: order 31 is not the folded running sum (%d bits)", seed, chanBits)
		}

		// Every order: the warm-up is the running sum; in place equals
		// into a fresh buffer.
		for _, order := range []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 16, 30, 32} {
			coefs := make([]int16, order)
			for i := range coefs {
				coefs[i] = int16(rng.IntN(1<<16) - 1<<15)
			}

			denShift := uint32(rng.IntN(16))

			fresh := make([]int32, num)
			alacint.UnpcBlock(pc1, fresh, num, slices.Clone(coefs), order, chanBits, denShift)

			for i := 1; i <= int(order) && i < num; i++ {
				if fresh[i] != want[i] {
					t.Fatalf(
						"seed %d order %d: warm-up sample %d is %d, the running sum says %d",
						seed,
						order,
						i,
						fresh[i],
						want[i],
					)
				}
			}

			inPlace := slices.Clone(pc1)
			alacint.UnpcBlock(inPlace, inPlace, num, slices.Clone(coefs), order, chanBits, denShift)

			if !slices.Equal(inPlace, fresh) {
				t.Fatalf("seed %d order %d: in place differs from into a fresh buffer", seed, order)
			}
		}
	}

	// No samples: nothing written.
	sentinel := []int32{7, 7, 7}
	alacint.UnpcBlock([]int32{1, 2, 3}, sentinel, 0, []int16{1}, 1, 16, 0)

	if !slices.Equal(sentinel, []int32{7, 7, 7}) {
		t.Fatal("no samples wrote something")
	}
}
