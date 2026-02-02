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

import "encoding/binary"

// Matrix unmix and output byte formatting.
// Ported from matrix_dec.c.
//
// All output is interleaved little-endian signed PCM.

// --- Stereo unmix (channel pair) ---

// WriteStereo16 unmixes and writes 16-bit stereo PCM.
func WriteStereo16(out []byte, mixU, mixV []int32, chanIdx, numChan, numSamples int, mixBits, mixRes int32) {
	stride := numChan * 2
	pos := chanIdx * 2

	if mixRes != 0 {
		for idx := range numSamples {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]

			dst := out[pos : pos+4 : pos+4]
			dst[0] = byte(left)
			dst[1] = byte(left >> 8)
			dst[2] = byte(right)
			dst[3] = byte(right >> 8)
			pos += stride
		}
	} else {
		for idx := range numSamples {
			l := mixU[idx]
			r := mixV[idx]
			dst := out[pos : pos+4 : pos+4]
			dst[0] = byte(l)
			dst[1] = byte(l >> 8)
			dst[2] = byte(r)
			dst[3] = byte(r >> 8)
			pos += stride
		}
	}
}

// WriteStereo20 unmixes and writes 20-bit stereo PCM.
func WriteStereo20(out []byte, mixU, mixV []int32, chanIdx, numChan, numSamples int, mixBits, mixRes int32) {
	stride := numChan * 3
	pos := chanIdx * 3

	if mixRes != 0 {
		for idx := range numSamples {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]
			left <<= 4
			right <<= 4

			dst := out[pos : pos+6 : pos+6]
			dst[0] = byte(left)
			dst[1] = byte(left >> 8)
			dst[2] = byte(left >> 16)
			dst[3] = byte(right)
			dst[4] = byte(right >> 8)
			dst[5] = byte(right >> 16)
			pos += stride
		}
	} else {
		for idx := range numSamples {
			dst := out[pos : pos+6 : pos+6]
			val := mixU[idx] << 4
			dst[0] = byte(val)
			dst[1] = byte(val >> 8)
			dst[2] = byte(val >> 16)

			val = mixV[idx] << 4
			dst[3] = byte(val)
			dst[4] = byte(val >> 8)
			dst[5] = byte(val >> 16)
			pos += stride
		}
	}
}

// WriteStereo24 unmixes and writes 24-bit stereo PCM.
//
//revive:disable-next-line:argument-limit
func WriteStereo24(out []byte, mixU, mixV []int32, chanIdx, numChan, numSamples int,
	mixBits, mixRes int32, shiftBuf []uint16, bytesShifted int,
) {
	stride := numChan * 3
	pos := chanIdx * 3
	shift := bytesShifted * 8

	if mixRes != 0 {
		for idx := range numSamples {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			dst := out[pos : pos+6 : pos+6]
			dst[0] = byte(left)
			dst[1] = byte(left >> 8)
			dst[2] = byte(left >> 16)
			dst[3] = byte(right)
			dst[4] = byte(right >> 8)
			dst[5] = byte(right >> 16)
			pos += stride
		}
	} else {
		for idx := range numSamples {
			left := mixU[idx]
			right := mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			dst := out[pos : pos+6 : pos+6]
			dst[0] = byte(left)
			dst[1] = byte(left >> 8)
			dst[2] = byte(left >> 16)
			dst[3] = byte(right)
			dst[4] = byte(right >> 8)
			dst[5] = byte(right >> 16)
			pos += stride
		}
	}
}

// WriteStereo32 unmixes and writes 32-bit stereo PCM.
//
//revive:disable-next-line:argument-limit
func WriteStereo32(out []byte, mixU, mixV []int32, chanIdx, numChan, numSamples int,
	mixBits, mixRes int32, shiftBuf []uint16, bytesShifted int,
) {
	stride := numChan * 4
	pos := chanIdx * 4
	shift := bytesShifted * 8

	if mixRes != 0 {
		for idx := range numSamples {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			binary.LittleEndian.PutUint32(out[pos:], uint32(left))
			binary.LittleEndian.PutUint32(out[pos+4:], uint32(right))
			pos += stride
		}
	} else {
		for idx := range numSamples {
			left := mixU[idx]
			right := mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			binary.LittleEndian.PutUint32(out[pos:], uint32(left))
			binary.LittleEndian.PutUint32(out[pos+4:], uint32(right))
			pos += stride
		}
	}
}

// --- Mono output (single channel) ---

// WriteMono16 writes 16-bit mono PCM.
func WriteMono16(out []byte, mixU []int32, chanIdx, numChan, numSamples int) {
	stride := numChan * 2
	pos := chanIdx * 2

	for idx := range numSamples {
		binary.LittleEndian.PutUint16(out[pos:], uint16(int16(mixU[idx])))
		pos += stride
	}
}

// WriteMono20 writes 20-bit mono PCM.
func WriteMono20(out []byte, mixU []int32, chanIdx, numChan, numSamples int) {
	stride := numChan * 3
	pos := chanIdx * 3

	for idx := range numSamples {
		val := mixU[idx] << 4
		dst := out[pos : pos+3 : pos+3]
		dst[0] = byte(val)
		dst[1] = byte(val >> 8)
		dst[2] = byte(val >> 16)
		pos += stride
	}
}

// WriteMono24 writes 24-bit mono PCM.
func WriteMono24(out []byte, mixU []int32, chanIdx, numChan, numSamples int, shiftBuf []uint16, bytesShifted int) {
	stride := numChan * 3
	pos := chanIdx * 3
	shift := bytesShifted * 8

	for idx := range numSamples {
		val := mixU[idx]
		if bytesShifted != 0 {
			val = (val << shift) | int32(shiftBuf[idx])
		}

		dst := out[pos : pos+3 : pos+3]
		dst[0] = byte(val)
		dst[1] = byte(val >> 8)
		dst[2] = byte(val >> 16)
		pos += stride
	}
}

// WriteMono32 writes 32-bit mono PCM.
func WriteMono32(out []byte, mixU []int32, chanIdx, numChan, numSamples int, shiftBuf []uint16, bytesShifted int) {
	stride := numChan * 4
	pos := chanIdx * 4
	shift := bytesShifted * 8

	for idx := range numSamples {
		val := mixU[idx]
		if bytesShifted != 0 {
			val = (val << shift) | int32(shiftBuf[idx])
		}

		binary.LittleEndian.PutUint32(out[pos:], uint32(val))
		pos += stride
	}
}
