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

//nolint:gosec // Integer conversions match Apple reference C implementation's fixed-width arithmetic.
package alac

// BitBuffer provides bit-level reading from a byte buffer.
// Ported from ALACBitUtilities.c.
//
// The buffer is padded with 4 zero bytes to allow safe reads near the end
// without bounds checking in hot paths.
type BitBuffer struct {
	Buf    []byte // padded data (original + 4 zero bytes)
	Pos    int    // current byte position within Buf
	BitIdx uint32 // 0-7, bit offset within current byte
	Size   int    // original (unpadded) byte size
}

const bitBufferPadding = 4

// Reset reuses the BitBuffer's backing storage, growing it only if needed.
// This avoids a fresh allocation per packet.
func (b *BitBuffer) Reset(data []byte) {
	needed := len(data) + bitBufferPadding
	if cap(b.Buf) < needed {
		b.Buf = make([]byte, needed)
	} else {
		b.Buf = b.Buf[:needed]
	}

	copy(b.Buf, data)
	// Zero the padding bytes (copy doesn't touch them if data is shorter).
	clear(b.Buf[len(data):])

	b.Pos = 0
	b.BitIdx = 0
	b.Size = len(data)
}

// Read reads up to 16 bits and returns them right-aligned.
// Equivalent to BitBufferRead in the Apple implementation.
func (b *BitBuffer) Read(numBits uint8) uint32 {
	// Load 3 bytes starting at current position (24 bits available).
	returnBits := uint32(b.Buf[b.Pos])<<16 | uint32(b.Buf[b.Pos+1])<<8 | uint32(b.Buf[b.Pos+2])
	returnBits = (returnBits << b.BitIdx) & 0x00FFFFFF //revive:disable-line:add-constant
	returnBits >>= 24 - uint32(numBits)

	b.BitIdx += uint32(numBits)
	b.Pos += int(b.BitIdx >> 3)
	b.BitIdx &= 7

	return returnBits
}

// ReadSmall reads up to 8 bits.
// Equivalent to BitBufferReadSmall.
func (b *BitBuffer) ReadSmall(numBits uint8) uint8 {
	returnBits := uint16(b.Buf[b.Pos])<<8 | uint16(b.Buf[b.Pos+1])
	returnBits <<= b.BitIdx
	returnBits >>= 16 - uint16(numBits)

	b.BitIdx += uint32(numBits)
	b.Pos += int(b.BitIdx >> 3)
	b.BitIdx &= 7

	return uint8(returnBits)
}

// ReadOne reads a single bit.
func (b *BitBuffer) ReadOne() uint8 {
	returnBit := (b.Buf[b.Pos] >> (7 - b.BitIdx)) & 1

	b.BitIdx++
	b.Pos += int(b.BitIdx >> 3)
	b.BitIdx &= 7

	return returnBit
}

// Advance skips forward by numBits bits.
func (b *BitBuffer) Advance(numBits uint32) {
	b.BitIdx += numBits
	b.Pos += int(b.BitIdx >> 3)
	b.BitIdx &= 7
}

// ByteAlign advances to the next byte boundary (if not already aligned).
func (b *BitBuffer) ByteAlign() {
	if b.BitIdx == 0 {
		return
	}

	b.Advance(8 - b.BitIdx)
}

// PastEnd returns true if the read position is at or past the original data end.
func (b *BitBuffer) PastEnd() bool {
	return b.Pos >= b.Size
}

// Copy returns a snapshot of the current BitBuffer state.
// The copy shares the underlying data but has independent position tracking.
func (b *BitBuffer) Copy() BitBuffer {
	return *b
}
