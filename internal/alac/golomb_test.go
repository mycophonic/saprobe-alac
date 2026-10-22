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
	"errors"
	"testing"

	alacint "github.com/mycophonic/saprobe-alac/internal/alac"
)

// The entropy decoder's own refusals, held at the unit with one sample: a
// packet's decoder refuses them too, but at the next element's entry or the
// next sample, after this one was decoded from the padding.
func TestDynDecomp_RefusesCodesPastTheEnd(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		packet []byte
	}{
		// Nine ones: an escape whose 16 value bits lie past the packet.
		{"escape value past the end", []byte{0xFF, 0x80}},
		// Eight ones that end the packet: the stop bit is the padding's.
		{"stop bit past the end", []byte{0xFF}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				bits   alacint.BitBuffer
				params alacint.AGParams
				out    [1]int32
			)

			bits.Reset(tc.packet)
			alacint.SetAGParams(&params, 10, 40, 14, 1, 1, 255)

			err := alacint.DynDecomp(&params, &bits, out[:], 1, 16)
			if !errors.Is(err, alacint.ErrBitstreamOverrun) {
				t.Fatalf("expected ErrBitstreamOverrun, got: %v", err)
			}
		})
	}
}

// A code that ends exactly at the packet's end decodes.
func TestDynDecomp_CodeEndingAtTheEnd(t *testing.T) {
	t.Parallel()

	var (
		bits   alacint.BitBuffer
		params alacint.AGParams
		out    [1]int32
	)

	bits.Reset([]byte{0xFE}) // seven ones and a stop bit: residual 7, sample -4
	alacint.SetAGParams(&params, 10, 40, 14, 1, 1, 255)

	if err := alacint.DynDecomp(&params, &bits, out[:], 1, 16); err != nil {
		t.Fatalf("DynDecomp: %v", err)
	}

	if out[0] != -4 {
		t.Fatalf("decoded %d, want -4", out[0])
	}
}
