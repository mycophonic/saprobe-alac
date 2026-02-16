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

package alac

import (
	"encoding/binary"
	"fmt"

	alacint "github.com/mycophonic/saprobe-alac/internal/alac"
)

// PacketConfig holds ALAC decoder configuration parsed from the magic cookie.
type PacketConfig struct {
	FrameLength   uint32
	BitDepth      uint8
	NumChannels   uint8
	PB            uint8
	MB            uint8
	KB            uint8
	MaxRun        uint16
	MaxFrameBytes uint32
	AvgBitRate    uint32
	SampleRate    uint32
}

const (
	configSize     = 24 // ALACSpecificConfig binary size.
	atomHeaderSize = 12 // MPEG4 atom header: size (4) + type (4) + payload (4).
)

// ParseMagicCookie reads an ALACSpecificConfig from a magic cookie byte slice.
// Handles legacy wrappers ('frma' and 'alac' atoms).
func ParseMagicCookie(cookie []byte) (PacketConfig, error) {
	data := cookie

	// Skip 'frma' atom if present: [size:4][type:'frma'][format:'alac']
	if len(data) >= atomHeaderSize && data[4] == 'f' && data[5] == 'r' && data[6] == 'm' && data[7] == 'a' {
		data = data[atomHeaderSize:]
	}

	// Skip 'alac' atom header if present: [size:4][type:'alac'][version:4]
	if len(data) >= atomHeaderSize && data[4] == 'a' && data[5] == 'l' && data[6] == 'a' && data[7] == 'c' {
		data = data[atomHeaderSize:]
	}

	if len(data) < configSize {
		return PacketConfig{}, fmt.Errorf("%w: %w", ErrConfig, alacint.ErrInvalidCookie)
	}

	compatibleVersion := data[4]
	if compatibleVersion > 0 {
		return PacketConfig{}, fmt.Errorf("%w: %w: %d", ErrConfig, alacint.ErrUnsupportedVersion, compatibleVersion)
	}

	return PacketConfig{
		FrameLength:   binary.BigEndian.Uint32(data[0:4]),
		BitDepth:      data[5],
		PB:            data[6],
		MB:            data[7],
		KB:            data[8],
		NumChannels:   data[9],
		MaxRun:        binary.BigEndian.Uint16(data[10:12]),
		MaxFrameBytes: binary.BigEndian.Uint32(data[12:16]),
		AvgBitRate:    binary.BigEndian.Uint32(data[16:20]),
		SampleRate:    binary.BigEndian.Uint32(data[20:24]),
	}, nil
}
