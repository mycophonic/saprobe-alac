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
	"testing"

	alacint "github.com/mycophonic/saprobe-alac/internal/alac"
)

// A frame shorter than the predictor's coefficient count: the warm-up must
// stop at the frame, not at the coefficient count (GHSA-g66f-c97f-q276).
func TestUnpcBlock_FrameShorterThanOrder(t *testing.T) {
	t.Parallel()

	const order = 30

	coefs := make([]int16, order)
	for i := range coefs {
		coefs[i] = 1
	}

	for _, num := range []int{0, 1, 2, order, order + 1} {
		pc1 := make([]int32, num)
		out := make([]int32, num)

		for i := range pc1 {
			pc1[i] = int32(i + 1)
		}

		alacint.UnpcBlock(pc1, out, num, coefs, order, 16, 9)

		if num > 0 && out[0] != pc1[0] {
			t.Fatalf("num %d: out[0] = %d, want %d", num, out[0], pc1[0])
		}
	}
}
