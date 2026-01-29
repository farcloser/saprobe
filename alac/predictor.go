package alac

// Dynamic predictor (inverse linear prediction).
// Ported from dp_dec.c.

// signOfInt returns +1 for positive, -1 for negative, 0 for zero.
func signOfInt(val int32) int32 {
	negiShift := int32(uint32(-val) >> 31)

	return negiShift | (val >> 31)
}

// unpcBlock reverses the linear prediction encoding.
// pc1 contains the entropy-decoded prediction residuals (input).
// out receives the reconstructed sample values (output).
// pc1 and out may be the same slice for in-place operation (numActive==31 mode).
func unpcBlock(pc1, out []int32, num int, coefs []int16, numActive int32, chanBits, denShift uint32) {
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

	if numActive == 31 {
		// Simple first-order delta decode.
		prev := out[0]
		for j := 1; j < num; j++ {
			del := pc1[j] + prev
			prev = (del << chanShift) >> chanShift
			out[j] = prev
		}

		return
	}

	// Warm-up phase: build predictor with growing coefficient set.
	for j := 1; j <= int(numActive); j++ {
		del := pc1[j] + out[j-1]
		out[j] = (del << chanShift) >> chanShift
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
func unpcBlock4(pc1, out []int32, num int, coefs []int16, lim int, chanShift, denShift uint32, denHalf int32) {
	a0 := int32(coefs[0])
	a1 := int32(coefs[1])
	a2 := int32(coefs[2])
	a3 := int32(coefs[3])

	for j := lim; j < num; j++ {
		top := out[j-lim]

		b0 := top - out[j-1]
		b1 := top - out[j-2]
		b2 := top - out[j-3]
		b3 := top - out[j-4]

		sum1 := (denHalf - a0*b0 - a1*b1 - a2*b2 - a3*b3) >> denShift

		del := pc1[j]
		del0 := del
		sg := signOfInt(del)
		del += top + sum1

		out[j] = (del << chanShift) >> chanShift

		if sg > 0 {
			sgn := signOfInt(b3)
			a3 -= int32(int16(sgn))
			del0 -= 1 * ((sgn * b3) >> denShift)
			if del0 <= 0 {
				goto store4
			}

			sgn = signOfInt(b2)
			a2 -= int32(int16(sgn))
			del0 -= 2 * ((sgn * b2) >> denShift)
			if del0 <= 0 {
				goto store4
			}

			sgn = signOfInt(b1)
			a1 -= int32(int16(sgn))
			del0 -= 3 * ((sgn * b1) >> denShift)
			if del0 <= 0 {
				goto store4
			}

			a0 -= int32(int16(signOfInt(b0)))
		} else if sg < 0 {
			sgn := -signOfInt(b3)
			a3 -= int32(int16(sgn))
			del0 -= 1 * ((sgn * b3) >> denShift)
			if del0 >= 0 {
				goto store4
			}

			sgn = -signOfInt(b2)
			a2 -= int32(int16(sgn))
			del0 -= 2 * ((sgn * b2) >> denShift)
			if del0 >= 0 {
				goto store4
			}

			sgn = -signOfInt(b1)
			a1 -= int32(int16(sgn))
			del0 -= 3 * ((sgn * b1) >> denShift)
			if del0 >= 0 {
				goto store4
			}

			a0 += int32(int16(signOfInt(b0)))
		}
	store4:
	}

	coefs[0] = int16(a0)
	coefs[1] = int16(a1)
	coefs[2] = int16(a2)
	coefs[3] = int16(a3)
}

// unpcBlock8 is the optimized predictor for numActive == 8.
func unpcBlock8(pc1, out []int32, num int, coefs []int16, lim int, chanShift, denShift uint32, denHalf int32) {
	a0 := int32(coefs[0])
	a1 := int32(coefs[1])
	a2 := int32(coefs[2])
	a3 := int32(coefs[3])
	a4 := int32(coefs[4])
	a5 := int32(coefs[5])
	a6 := int32(coefs[6])
	a7 := int32(coefs[7])

	for j := lim; j < num; j++ {
		top := out[j-lim]

		b0 := top - out[j-1]
		b1 := top - out[j-2]
		b2 := top - out[j-3]
		b3 := top - out[j-4]
		b4 := top - out[j-5]
		b5 := top - out[j-6]
		b6 := top - out[j-7]
		b7 := top - out[j-8]

		sum1 := (denHalf - a0*b0 - a1*b1 - a2*b2 - a3*b3 - a4*b4 - a5*b5 - a6*b6 - a7*b7) >> denShift

		del := pc1[j]
		del0 := del
		sg := signOfInt(del)
		del += top + sum1

		out[j] = (del << chanShift) >> chanShift

		if sg > 0 {
			sgn := signOfInt(b7)
			a7 -= int32(int16(sgn))
			del0 -= 1 * ((sgn * b7) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(b6)
			a6 -= int32(int16(sgn))
			del0 -= 2 * ((sgn * b6) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(b5)
			a5 -= int32(int16(sgn))
			del0 -= 3 * ((sgn * b5) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(b4)
			a4 -= int32(int16(sgn))
			del0 -= 4 * ((sgn * b4) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(b3)
			a3 -= int32(int16(sgn))
			del0 -= 5 * ((sgn * b3) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(b2)
			a2 -= int32(int16(sgn))
			del0 -= 6 * ((sgn * b2) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			sgn = signOfInt(b1)
			a1 -= int32(int16(sgn))
			del0 -= 7 * ((sgn * b1) >> denShift)
			if del0 <= 0 {
				goto store8
			}

			a0 -= int32(int16(signOfInt(b0)))
		} else if sg < 0 {
			sgn := -signOfInt(b7)
			a7 -= int32(int16(sgn))
			del0 -= 1 * ((sgn * b7) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(b6)
			a6 -= int32(int16(sgn))
			del0 -= 2 * ((sgn * b6) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(b5)
			a5 -= int32(int16(sgn))
			del0 -= 3 * ((sgn * b5) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(b4)
			a4 -= int32(int16(sgn))
			del0 -= 4 * ((sgn * b4) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(b3)
			a3 -= int32(int16(sgn))
			del0 -= 5 * ((sgn * b3) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(b2)
			a2 -= int32(int16(sgn))
			del0 -= 6 * ((sgn * b2) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			sgn = -signOfInt(b1)
			a1 -= int32(int16(sgn))
			del0 -= 7 * ((sgn * b1) >> denShift)
			if del0 >= 0 {
				goto store8
			}

			a0 += int32(int16(signOfInt(b0)))
		}
	store8:
	}

	coefs[0] = int16(a0)
	coefs[1] = int16(a1)
	coefs[2] = int16(a2)
	coefs[3] = int16(a3)
	coefs[4] = int16(a4)
	coefs[5] = int16(a5)
	coefs[6] = int16(a6)
	coefs[7] = int16(a7)
}

// unpcBlockGeneral is the general predictor for any numActive.
func unpcBlockGeneral(pc1, out []int32, num int, coefs []int16, numActive int32, lim int, chanShift, denShift uint32, denHalf int32) {
	for j := lim; j < num; j++ {
		var sum1 int32

		top := out[j-lim]

		for k := int32(0); k < numActive; k++ {
			sum1 += int32(coefs[k]) * (out[j-1-int(k)] - top)
		}

		del := pc1[j]
		del0 := del
		sg := signOfInt(del)
		del += top + ((sum1 + denHalf) >> denShift)
		out[j] = (del << chanShift) >> chanShift

		if sg > 0 {
			for k := numActive - 1; k >= 0; k-- {
				dd := top - out[j-1-int(k)]
				sgn := signOfInt(dd)
				coefs[k] -= int16(sgn)
				del0 -= (numActive - k) * ((sgn * dd) >> int32(denShift))
				if del0 <= 0 {
					break
				}
			}
		} else if sg < 0 {
			for k := numActive - 1; k >= 0; k-- {
				dd := top - out[j-1-int(k)]
				sgn := signOfInt(dd)
				coefs[k] += int16(sgn)
				del0 -= (numActive - k) * ((-sgn * dd) >> int32(denShift))
				if del0 >= 0 {
					break
				}
			}
		}
	}
}
