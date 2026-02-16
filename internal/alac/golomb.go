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

import (
	"encoding/binary"
	"math/bits"
)

// Adaptive Golomb-Rice entropy decoder.
// Ported from ag_dec.c and aglib.h.

const (
	qbShift       = 9
	quantBits     = 1 << qbShift // 512
	mmulShift     = 2
	mdenShift     = qbShift - mmulShift - 1 // 6
	moff          = 1 << (mdenShift - 2)    // 16
	bitoff        = 24
	maxPrefix16   = 9
	maxPrefix32   = 9
	maxDatatype16 = 16
	nMaxMeanClamp = 0xffff
	nMeanClampVal = 0xffff
	maxZeroRun    = 65535 // Maximum zero-run length before resetting zmode.
)

// AGParams holds the adaptive Golomb-Rice codec state.
type AGParams struct {
	MB, MB0 uint32
	PB      uint32
	KB      uint32
	WB      uint32
	QB      uint32
	FW, SW  uint32
	MaxRun  uint32
}

// SetAGParams initialises the adaptive Golomb-Rice parameters.
func SetAGParams(params *AGParams, meanBase, partBound, kBase, frameWin, sampleWin, maxrun uint32) {
	params.MB = meanBase
	params.MB0 = meanBase
	params.PB = partBound
	params.KB = kBase
	params.WB = (1 << kBase) - 1
	params.QB = quantBits - partBound
	params.FW = frameWin
	params.SW = sampleWin
	params.MaxRun = maxrun
}

// lead returns the number of leading zeros in a 32-bit value.
// Equivalent to the Apple lead() function.
func lead(m int32) int32 {
	return int32(bits.LeadingZeros32(uint32(m)))
}

// lg3a computes floor(log2(x+3)).
func lg3a(x int32) int32 {
	return 31 - lead(x+3) //revive:disable-line:add-constant
}

// read32bit reads 4 bytes big-endian from a byte slice at the given offset.
// binary.BigEndian.Uint32 is intrinsified by the Go compiler as a single
// load + byte-swap instruction, replacing 4 individual byte loads.
func read32bit(buf []byte, offset int) uint32 {
	return binary.BigEndian.Uint32(buf[offset:])
}

// getStreamBits reads up to 32 bits from an arbitrary bit position in a byte buffer.
func getStreamBits(input []byte, bitOffset, numBits uint32) uint32 {
	byteOffset := bitOffset / 8
	load1 := read32bit(input, int(byteOffset))

	if numBits+(bitOffset&7) > 32 {
		// Need bits from a 5th byte.
		result := load1 << (bitOffset & 7)
		load2 := uint32(input[byteOffset+4])
		load2shift := 8 - (numBits + (bitOffset & 7) - 32)
		load2 >>= load2shift
		result >>= 32 - numBits
		result |= load2

		return result
	}

	result := load1 >> (32 - numBits - (bitOffset & 7))
	if numBits < 32 {
		result &= (1 << numBits) - 1
	}

	return result
}

// dynGet decodes one Golomb-coded value (16-bit variant used for zero-run counts).
// Returns (decoded value, updated bit position).
func dynGet(input []byte, bitPos, golombM, golombK uint32) (result, newBitPos uint32) {
	tempBits := bitPos

	streamLong := read32bit(input, int(tempBits>>3))
	streamLong <<= tempBits & 7

	// Count leading ones (= leading zeros in complement).
	pre := uint32(lead(int32(^streamLong)))

	if pre >= maxPrefix16 {
		pre = maxPrefix16
		tempBits += pre
		streamLong <<= pre
		result = streamLong >> (32 - maxDatatype16)
		tempBits += maxDatatype16

		return result, tempBits
	}

	tempBits += pre + 1
	streamLong <<= pre + 1
	val := streamLong >> (32 - golombK)
	tempBits += golombK

	if val < 2 {
		result = pre * golombM
		tempBits--
	} else {
		result = pre*golombM + val - 1
	}

	return result, tempBits
}

// DynDecomp performs adaptive Golomb-Rice entropy decoding of a sample block.
// Writes decoded prediction residuals into predCoefs.
func DynDecomp(params *AGParams, bitBuf *BitBuffer, predCoefs []int32, numSamples, maxSize int) error {
	input := bitBuf.Buf[bitBuf.Pos:]
	startPos := bitBuf.BitIdx
	maxPos := uint32(bitBuf.Size-bitBuf.Pos) * 8
	bitPos := startPos

	// BCE: reslice so compiler knows len(predCoefs) == numSamples.
	predCoefs = predCoefs[:numSamples:numSamples]

	meanAccum := params.MB0
	zmode := int32(0)
	count := 0

	pbLocal := params.PB
	kbLocal := params.KB
	wbLocal := params.WB

	var residual uint32

	for count < numSamples {
		if bitPos >= maxPos {
			return ErrBitstreamOverrun
		}

		m := meanAccum >> qbShift                //nolint:varnamelen // standard Golomb-Rice parameter name
		k := min(lg3a(int32(m)), int32(kbLocal)) //nolint:varnamelen // standard Golomb-Rice parameter name

		m = (1 << uint32(k)) - 1

		// Inlined dynGet32Bit: eliminates per-sample function call overhead (~7% of decode time).
		{
			streamLong := read32bit(input, int(bitPos>>3))
			streamLong <<= bitPos & 7

			residual = uint32(lead(int32(^streamLong)))

			if residual >= maxPrefix32 {
				residual = getStreamBits(input, bitPos+maxPrefix32, uint32(maxSize))
				bitPos += maxPrefix32 + uint32(maxSize)
			} else {
				bitPos += residual + 1

				if k != 1 {
					streamLong <<= residual + 1
					v := streamLong >> (32 - uint32(k))

					if v >= 2 {
						residual = residual*m + v - 1
						bitPos += uint32(k)
					} else {
						residual *= m
						bitPos += uint32(k) - 1
					}
				}
			}
		}

		// Decode sign from LSB.
		ndecode := residual + uint32(zmode)
		multiplier := -int32(ndecode & 1)
		multiplier |= 1
		del := int32((ndecode+1)>>1) * multiplier

		predCoefs[count] = del
		count++

		// Update mean.
		meanAccum = pbLocal*(residual+uint32(zmode)) + meanAccum - ((pbLocal * meanAccum) >> qbShift)
		if residual > nMaxMeanClamp {
			meanAccum = nMeanClampVal
		}

		zmode = 0

		// Check for zero run mode.
		if (meanAccum<<mmulShift) < quantBits && count < numSamples {
			zmode = 1

			k32 := max(lead(int32(meanAccum))-bitoff+int32((meanAccum+moff)>>mdenShift), 0)

			mz := ((uint32(1) << uint32(k32)) - 1) & wbLocal

			residual, bitPos = dynGet(input, bitPos, mz, uint32(k32))

			if count+int(residual) > numSamples {
				return ErrSampleOverrun
			}

			// BCE: single slice check replaces per-element bounds checks.
			end := count + int(residual)
			clear(predCoefs[count:end])
			count = end

			if residual >= maxZeroRun {
				zmode = 0
			}

			meanAccum = 0
		}
	}

	bitsConsumed := bitPos - startPos
	bitBuf.Advance(bitsConsumed)

	return nil
}
