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

func BenchmarkWriteStereo16(b *testing.B) {
	const (
		numSamples = 4096
		numChan    = 2
	)

	out := make([]byte, numSamples*numChan*2)
	mixU := make([]int32, numSamples)
	mixV := make([]int32, numSamples)

	for i := range numSamples {
		mixU[i] = int32(i * 17)
		mixV[i] = int32(i * 13)
	}

	b.Run("mixRes=1", func(b *testing.B) {
		for range b.N {
			alacint.WriteStereo16(out, mixU, mixV, 0, numChan, numSamples, 2, 1)
		}
	})

	b.Run("mixRes=0", func(b *testing.B) {
		for range b.N {
			alacint.WriteStereo16(out, mixU, mixV, 0, numChan, numSamples, 0, 0)
		}
	})
}
