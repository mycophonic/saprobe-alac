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
	"errors"
	"testing"

	alac "github.com/mycophonic/saprobe-alac"
)

// Cookies the decoder cannot work with: each used to panic on the first
// packet, or in Duration and Position; each must be refused as ErrConfig.
func TestNewPacketDecoder_RefusesConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		shape func(*alac.PacketConfig)
	}{
		{"zero channels", func(c *alac.PacketConfig) { c.NumChannels = 0 }},
		{"nine channels", func(c *alac.PacketConfig) { c.NumChannels = 9 }},
		{"zero frame length", func(c *alac.PacketConfig) { c.FrameLength = 0 }},
		{"frame length above the bound", func(c *alac.PacketConfig) { c.FrameLength = 65537 }},
		{"zero sample rate", func(c *alac.PacketConfig) { c.SampleRate = 0 }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := boundsConfig(16, 2)
			tc.shape(&config)

			_, err := alac.NewPacketDecoder(config)
			if !errors.Is(err, alac.ErrConfig) {
				t.Fatalf("expected ErrConfig, got: %v", err)
			}
		})
	}
}

func TestNewPacketDecoder_AcceptsBounds(t *testing.T) {
	t.Parallel()

	for _, channels := range []uint8{1, 8} {
		for _, frameLength := range []uint32{1, 65536} {
			config := boundsConfig(16, channels)
			config.FrameLength = frameLength

			if _, err := alac.NewPacketDecoder(config); err != nil {
				t.Fatalf("channels %d, frame length %d: %v", channels, frameLength, err)
			}
		}
	}
}
