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
	off := chanIdx * 2

	// BCE: reslice so range-based indexing eliminates mixU/mixV bounds checks.
	mixU = mixU[:numSamples:numSamples]
	mixV = mixV[:numSamples:numSamples]

	if mixRes != 0 {
		for idx := range mixU {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]

			dst := out[off : off+4 : off+4]
			dst[0] = byte(left)
			dst[1] = byte(left >> 8)
			dst[2] = byte(right)
			dst[3] = byte(right >> 8)
			off += stride
		}
	} else {
		for idx := range mixU {
			left := mixU[idx]
			right := mixV[idx]

			dst := out[off : off+4 : off+4]
			dst[0] = byte(left)
			dst[1] = byte(left >> 8)
			dst[2] = byte(right)
			dst[3] = byte(right >> 8)
			off += stride
		}
	}
}

// WriteStereo20 unmixes and writes 20-bit stereo PCM.
func WriteStereo20(out []byte, mixU, mixV []int32, chanIdx, numChan, numSamples int, mixBits, mixRes int32) {
	stride := numChan * 3
	off := chanIdx * 3

	mixU = mixU[:numSamples:numSamples]
	mixV = mixV[:numSamples:numSamples]

	if mixRes != 0 {
		for idx := range mixU {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]
			left <<= 4
			right <<= 4

			dst := out[off : off+6 : off+6]
			dst[0] = byte(left)
			dst[1] = byte(left >> 8)
			dst[2] = byte(left >> 16)
			dst[3] = byte(right)
			dst[4] = byte(right >> 8)
			dst[5] = byte(right >> 16)
			off += stride
		}
	} else {
		for idx := range mixU {
			val := mixU[idx] << 4

			dst := out[off : off+6 : off+6]
			dst[0] = byte(val)
			dst[1] = byte(val >> 8)
			dst[2] = byte(val >> 16)

			val = mixV[idx] << 4
			dst[3] = byte(val)
			dst[4] = byte(val >> 8)
			dst[5] = byte(val >> 16)
			off += stride
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
	shift := bytesShifted * 8
	off := chanIdx * 3

	mixU = mixU[:numSamples:numSamples]
	mixV = mixV[:numSamples:numSamples]

	if bytesShifted != 0 {
		shiftBuf = shiftBuf[: numSamples*2 : numSamples*2]
	}

	if mixRes != 0 {
		for idx := range mixU {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			dst := out[off : off+6 : off+6]
			dst[0] = byte(left)
			dst[1] = byte(left >> 8)
			dst[2] = byte(left >> 16)
			dst[3] = byte(right)
			dst[4] = byte(right >> 8)
			dst[5] = byte(right >> 16)
			off += stride
		}
	} else {
		for idx := range mixU {
			left := mixU[idx]
			right := mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			dst := out[off : off+6 : off+6]
			dst[0] = byte(left)
			dst[1] = byte(left >> 8)
			dst[2] = byte(left >> 16)
			dst[3] = byte(right)
			dst[4] = byte(right >> 8)
			dst[5] = byte(right >> 16)
			off += stride
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
	shift := bytesShifted * 8
	off := chanIdx * 4

	mixU = mixU[:numSamples:numSamples]
	mixV = mixV[:numSamples:numSamples]

	if bytesShifted != 0 {
		shiftBuf = shiftBuf[: numSamples*2 : numSamples*2]
	}

	if mixRes != 0 {
		for idx := range mixU {
			left := mixU[idx] + mixV[idx] - ((mixRes * mixV[idx]) >> mixBits)
			right := left - mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			dst := out[off : off+8 : off+8]
			binary.LittleEndian.PutUint32(dst[:4], uint32(left))
			binary.LittleEndian.PutUint32(dst[4:], uint32(right))

			off += stride
		}
	} else {
		for idx := range mixU {
			left := mixU[idx]
			right := mixV[idx]

			if bytesShifted != 0 {
				left = (left << shift) | int32(shiftBuf[idx*2+0])
				right = (right << shift) | int32(shiftBuf[idx*2+1])
			}

			dst := out[off : off+8 : off+8]
			binary.LittleEndian.PutUint32(dst[:4], uint32(left))
			binary.LittleEndian.PutUint32(dst[4:], uint32(right))

			off += stride
		}
	}
}

// --- Mono output (single channel) ---

// WriteMono16 writes 16-bit mono PCM.
func WriteMono16(out []byte, mixU []int32, chanIdx, numChan, numSamples int) {
	stride := numChan * 2
	off := chanIdx * 2

	mixU = mixU[:numSamples:numSamples]

	for idx := range mixU {
		dst := out[off : off+2 : off+2]
		binary.LittleEndian.PutUint16(dst, uint16(int16(mixU[idx])))

		off += stride
	}
}

// WriteMono20 writes 20-bit mono PCM.
func WriteMono20(out []byte, mixU []int32, chanIdx, numChan, numSamples int) {
	stride := numChan * 3
	off := chanIdx * 3

	mixU = mixU[:numSamples:numSamples]

	for idx := range mixU {
		val := mixU[idx] << 4

		dst := out[off : off+3 : off+3]
		dst[0] = byte(val)
		dst[1] = byte(val >> 8)
		dst[2] = byte(val >> 16)
		off += stride
	}
}

// WriteMono24 writes 24-bit mono PCM.
func WriteMono24(out []byte, mixU []int32, chanIdx, numChan, numSamples int, shiftBuf []uint16, bytesShifted int) {
	stride := numChan * 3
	shift := bytesShifted * 8
	off := chanIdx * 3

	mixU = mixU[:numSamples:numSamples]

	if bytesShifted != 0 {
		shiftBuf = shiftBuf[:numSamples:numSamples]
	}

	for idx := range mixU {
		val := mixU[idx]
		if bytesShifted != 0 {
			val = (val << shift) | int32(shiftBuf[idx])
		}

		dst := out[off : off+3 : off+3]
		dst[0] = byte(val)
		dst[1] = byte(val >> 8)
		dst[2] = byte(val >> 16)
		off += stride
	}
}

// WriteMono32 writes 32-bit mono PCM.
func WriteMono32(out []byte, mixU []int32, chanIdx, numChan, numSamples int, shiftBuf []uint16, bytesShifted int) {
	stride := numChan * 4
	shift := bytesShifted * 8
	off := chanIdx * 4

	mixU = mixU[:numSamples:numSamples]

	if bytesShifted != 0 {
		shiftBuf = shiftBuf[:numSamples:numSamples]
	}

	for idx := range mixU {
		val := mixU[idx]
		if bytesShifted != 0 {
			val = (val << shift) | int32(shiftBuf[idx])
		}

		dst := out[off : off+4 : off+4]
		binary.LittleEndian.PutUint32(dst, uint32(val))

		off += stride
	}
}
