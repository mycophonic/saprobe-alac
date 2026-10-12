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
//
// A read at or past the end of the data (an overrun, which PastEnd reports)
// returns zero bits and does not advance: the 4-byte padding covers a read
// that starts inside the data and crosses its end, not one that starts beyond it.
func (b *BitBuffer) Read(numBits uint8) uint32 {
	if b.Pos >= b.Size {
		return 0
	}

	// BCE: sub-slice with constant length 3; element indices 0,1,2 are provably in-bounds.
	// Safety: 4-byte padding guarantees Pos+2 < len(Buf) for any position inside the data.
	w := b.Buf[b.Pos : b.Pos+3 : b.Pos+3]
	// Load 3 bytes starting at current position (24 bits available).
	returnBits := uint32(w[0])<<16 | uint32(w[1])<<8 | uint32(w[2])
	returnBits = (returnBits << b.BitIdx) & 0x00FFFFFF //nolint:mnd // the 24-bit window the three loaded bytes fill
	returnBits >>= 24 - uint32(numBits)

	b.BitIdx += uint32(numBits)
	b.Pos += int(b.BitIdx >> 3)
	b.BitIdx &= 7

	return returnBits
}

// ReadSmall reads up to 8 bits.
// Equivalent to BitBufferReadSmall. Past the end it behaves as Read does.
func (b *BitBuffer) ReadSmall(numBits uint8) uint8 {
	if b.Pos >= b.Size {
		return 0
	}

	// BCE: sub-slice with constant length 2; element indices 0,1 are provably in-bounds.
	w := b.Buf[b.Pos : b.Pos+2 : b.Pos+2]
	returnBits := uint16(w[0])<<8 | uint16(w[1])
	returnBits <<= b.BitIdx
	returnBits >>= 16 - uint16(numBits)

	b.BitIdx += uint32(numBits)
	b.Pos += int(b.BitIdx >> 3)
	b.BitIdx &= 7

	return uint8(returnBits) // #nosec G115 -- numBits is 3, 4 or 8 at every call: 8 bits remain after the shift
}

// ReadOne reads a single bit. Past the end it behaves as Read does.
func (b *BitBuffer) ReadOne() uint8 {
	if b.Pos >= b.Size {
		return 0
	}

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

// BitsLeft returns the number of data bits from the read position to the end,
// negative once the position has been advanced past the end.
func (b *BitBuffer) BitsLeft() int64 {
	return int64(b.Size-b.Pos)*8 - int64(b.BitIdx)
}

// Copy returns a snapshot of the current BitBuffer state.
// The copy shares the underlying data but has independent position tracking.
func (b *BitBuffer) Copy() BitBuffer {
	return *b
}
