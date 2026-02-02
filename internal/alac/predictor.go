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

// Dynamic predictor (inverse linear prediction).
// Ported from dp_dec.c.

const (
	// MaxCoefs is the maximum predictor order (5-bit field).
	MaxCoefs = 32

	// NumActiveDelta triggers first-order delta decode mode.
	NumActiveDelta = 31

	// UnusedHeaderBits is the unused header field size in SCE/CPE (spec-defined).
	UnusedHeaderBits = 12
)

// signOfInt returns +1 for positive, -1 for negative, 0 for zero.
func signOfInt(val int32) int32 {
	negiShift := int32(uint32(-val) >> 31)

	return negiShift | (val >> 31) //revive:disable-line:add-constant
}

// UnpcBlock reverses the linear prediction encoding.
// pc1 contains the entropy-decoded prediction residuals (input).
// out receives the reconstructed sample values (output).
// pc1 and out may be the same slice for in-place operation (numActive==31 mode).
func UnpcBlock(pc1, out []int32, num int, coefs []int16, numActive int32, chanBits, denShift uint32) {
	chanShift := uint32(32) - chanBits

	var denHalf int32
	if denShift > 0 {
		denHalf = 1 << (denShift - 1)
	}

	out[0] = pc1[0]

	if numActive == 0 {
		if num > 1 && &pc1[0] != &out[0] {
			copy(out[1:num], pc1[1:num])
		}

		return
	}

	if numActive == NumActiveDelta {
		// Simple first-order delta decode.
		prev := out[0]
		for idx := 1; idx < num; idx++ {
			del := pc1[idx] + prev
			prev = (del << chanShift) >> chanShift
			out[idx] = prev
		}

		return
	}

	// Warm-up phase: build predictor with growing coefficient set.
	for idx := 1; idx <= int(numActive); idx++ {
		del := pc1[idx] + out[idx-1]
		out[idx] = (del << chanShift) >> chanShift
	}

	lim := int(numActive) + 1

	switch numActive {
	case 4:
		unpcBlock4(pc1, out, num, coefs, lim, chanShift, denShift, denHalf)
	case 8:
		unpcBlock8(pc1, out, num, coefs, lim, chanShift, denShift, denHalf)
	default:
		unpcBlockGeneral(pc1, out, num, coefs, numActive, lim, chanShift, denShift, denHalf)
	}
}

// unpcBlock4 is the optimized predictor for numActive == 4.
//
//revive:disable:argument-limit
func unpcBlock4(pc1, out []int32, num int, coefs []int16, lim int, chanShift, denShift uint32, denHalf int32) {
	// BCE: reslice to exact length so compiler knows bounds for forward and backward indexing.
	_ = coefs[3]
	pc1 = pc1[:num:num]
	out = out[:num:num]

	coef0 := int32(coefs[0])
	coef1 := int32(coefs[1])
	coef2 := int32(coefs[2])
	coef3 := int32(coefs[3])

	for idx := lim; idx < num; idx++ {
		top := out[idx-lim]

		diff0 := top - out[idx-1]
		diff1 := top - out[idx-2]
		diff2 := top - out[idx-3]
		diff3 := top - out[idx-4]

		sum1 := (denHalf - coef0*diff0 - coef1*diff1 - coef2*diff2 - coef3*diff3) >> denShift

		del := pc1[idx]
		del0 := del
		sign := signOfInt(del)
		del += top + sum1

		out[idx] = (del << chanShift) >> chanShift

		if sign > 0 {
			sgn := signOfInt(diff3)
			coef3 -= int32(int16(sgn))

			del0 -= 1 * ((sgn * diff3) >> denShift)
			if del0 <= 0 {
				goto store4
			}

			sgn = signOfInt(diff2)
			coef2 -= int32(int16(sgn))

			del0 -= 2 * ((sgn * diff2) >> denShift)
			if del0 <= 0 {
				goto store4
			}

			sgn = signOfInt(diff1)
			coef1 -= int32(int16(sgn))

			del0 -= 3 * ((sgn * diff1) >> denShift)
			if del0 <= 0 {
				goto store4
			}

			coef0 -= int32(int16(signOfInt(diff0)))
		} else if sign < 0 {
			sgn := -signOfInt(diff3)
			coef3 -= int32(int16(sgn))

			del0 -= 1 * ((sgn * diff3) >> denShift)
			if del0 >= 0 {
				goto store4
			}

			sgn = -signOfInt(diff2)
			coef2 -= int32(int16(sgn))

			del0 -= 2 * ((sgn * diff2) >> denShift)
			if del0 >= 0 {
				goto store4
			}

			sgn = -signOfInt(diff1)
			coef1 -= int32(int16(sgn))

			del0 -= 3 * ((sgn * diff1) >> denShift)
			if del0 >= 0 {
				goto store4
			}

			coef0 += int32(int16(signOfInt(diff0)))
		}

	store4:
	}

	coefs[0] = int16(coef0)
	coefs[1] = int16(coef1)
	coefs[2] = int16(coef2)
	coefs[3] = int16(coef3)
}

// unpcBlock8 is the optimized predictor for numActive == 8.
//
//revive:disable:cognitive-complexity // deal with it!
func unpcBlock8(pc1, out []int32, num int, coefs []int16, lim int, chanShift, denShift uint32, denHalf int32) {
	// BCE: reslice to exact length so compiler knows bounds for forward and backward indexing.
	_ = coefs[7]
	pc1 = pc1[:num:num]
	out = out[:num:num]

	coef0 := int32(coefs[0])
	coef1 := int32(coefs[1])
	coef2 := int32(coefs[2])
	coef3 := int32(coefs[3])
	coef4 := int32(coefs[4])
	coef5 := int32(coefs[5])
	coef6 := int32(coefs[6])
	coef7 := int32(coefs[7])

	for idx := lim; idx < num; idx++ {
		top := out[idx-lim]

		diff0 := top - out[idx-1]
		diff1 := top - out[idx-2]
		diff2 := top - out[idx-3]
		diff3 := top - out[idx-4]
		diff4 := top - out[idx-5]
		diff5 := top - out[idx-6]
		diff6 := top - out[idx-7]
		diff7 := top - out[idx-8]

		sum1 := (denHalf - coef0*diff0 - coef1*diff1 - coef2*diff2 - coef3*diff3 - coef4*diff4 - coef5*diff5 - coef6*diff6 - coef7*diff7) >> denShift

		del := pc1[idx]
		del0 := del
		sign := signOfInt(del)
		del += top + sum1

		out[idx] = (del << chanShift) >> chanShift

		if sign > 0 {
			sgn := signOfInt(diff7)
			coef7 -= int32(int16(sgn))

			del0 -= 1 * ((sgn * diff7) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(diff6)
			coef6 -= int32(int16(sgn))

			del0 -= 2 * ((sgn * diff6) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(diff5)
			coef5 -= int32(int16(sgn))

			del0 -= 3 * ((sgn * diff5) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(diff4)
			coef4 -= int32(int16(sgn))

			del0 -= 4 * ((sgn * diff4) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(diff3)
			coef3 -= int32(int16(sgn))

			del0 -= 5 * ((sgn * diff3) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(diff2)
			coef2 -= int32(int16(sgn))

			del0 -= 6 * ((sgn * diff2) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(diff1)
			coef1 -= int32(int16(sgn))

			del0 -= 7 * ((sgn * diff1) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			coef0 -= int32(int16(signOfInt(diff0)))
		} else if sign < 0 {
			sgn := -signOfInt(diff7)
			coef7 -= int32(int16(sgn))

			del0 -= 1 * ((sgn * diff7) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(diff6)
			coef6 -= int32(int16(sgn))

			del0 -= 2 * ((sgn * diff6) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(diff5)
			coef5 -= int32(int16(sgn))

			del0 -= 3 * ((sgn * diff5) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(diff4)
			coef4 -= int32(int16(sgn))

			del0 -= 4 * ((sgn * diff4) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(diff3)
			coef3 -= int32(int16(sgn))

			del0 -= 5 * ((sgn * diff3) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(diff2)
			coef2 -= int32(int16(sgn))

			del0 -= 6 * ((sgn * diff2) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(diff1)
			coef1 -= int32(int16(sgn))

			del0 -= 7 * ((sgn * diff1) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			coef0 += int32(int16(signOfInt(diff0)))
		}

	store8:
	}

	coefs[0] = int16(coef0)
	coefs[1] = int16(coef1)
	coefs[2] = int16(coef2)
	coefs[3] = int16(coef3)
	coefs[4] = int16(coef4)
	coefs[5] = int16(coef5)
	coefs[6] = int16(coef6)
	coefs[7] = int16(coef7)
}

// unpcBlockGeneral is the general predictor for any numActive.
//
//revive:disable:cognitive-complexity
func unpcBlockGeneral(
	pc1, out []int32,
	num int,
	coefs []int16,
	numActive int32,
	lim int,
	chanShift, denShift uint32,
	denHalf int32,
) {
	activeCount := int(numActive)
	coefsNA := coefs[:activeCount:activeCount] // BCE: compiler knows len and cap

	// BCE: reslice to exact length so compiler knows bounds.
	pc1 = pc1[:num:num]
	out = out[:num:num]

	for idx := lim; idx < num; idx++ {
		// Sub-slice the history window [idx-lim .. idx-1] for bounds check elimination.
		// hist[0] = out[idx-lim] = top, hist[activeCount] = out[idx-1] (since lim = activeCount+1).
		hist := out[idx-lim : idx : idx] // length = lim = activeCount+1; cap=idx for BCE

		top := hist[0]

		// Prediction sum: dot product of coefs and (history - top).
		var sum1 int32

		for k := range activeCount {
			sum1 += int32(coefsNA[k]) * (hist[activeCount-k] - top)
		}

		del := pc1[idx]
		del0 := del
		sign := signOfInt(del)
		del += top + ((sum1 + denHalf) >> denShift)
		out[idx] = (del << chanShift) >> chanShift

		// Coefficient adaptation.
		if sign > 0 {
			for k := activeCount - 1; k >= 0; k-- {
				dd := top - hist[activeCount-k]
				sgn := signOfInt(dd)
				coefsNA[k] -= int16(sgn)

				del0 -= int32(activeCount-k) * ((sgn * dd) >> int32(denShift))
				if del0 <= 0 {
					break
				}
			}
		} else if sign < 0 {
			for k := activeCount - 1; k >= 0; k-- {
				dd := top - hist[activeCount-k]
				sgn := signOfInt(dd)
				coefsNA[k] += int16(sgn)

				del0 -= int32(activeCount-k) * ((-sgn * dd) >> int32(denShift))
				if del0 >= 0 {
					break
				}
			}
		}
	}
}
