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
	"testing"

	alac "github.com/mycophonic/saprobe-alac"
)

// FuzzDecodePacket feeds arbitrary packets to every bit depth and channel
// count. A packet may be refused; it may never panic. The seeds are the
// packets of GHSA-g66f-c97f-q276.
func FuzzDecodePacket(f *testing.F) {
	f.Add(uint8(16), uint8(2), shortStereoPacket)
	f.Add(uint8(32), uint8(2), shortStereoPacket)
	f.Add(uint8(16), uint8(2), []byte{0xE0})
	f.Add(uint8(16), uint8(2), []byte{0x20})
	f.Add(uint8(24), uint8(1), []byte{0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x08})
	f.Add(uint8(20), uint8(8), []byte{0x20, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x01, 0xFF})

	f.Fuzz(func(t *testing.T, bitDepth, channels uint8, packet []byte) {
		depths := []uint8{16, 20, 24, 32}
		config := boundsConfig(depths[int(bitDepth)%len(depths)], channels%8+1)

		dec, err := alac.NewPacketDecoder(config)
		if err != nil {
			t.Fatalf("NewPacketDecoder: %v", err)
		}

		_, _ = dec.DecodePacket(packet)
	})
}
