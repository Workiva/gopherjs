//go:build js

package bits

import "github.com/gopherjs/gopherjs/js"

type _err string

func (e _err) Error() string {
	return string(e)
}

// RuntimeError implements runtime.Error.
func (e _err) RuntimeError() {
}

var (
	overflowError error = _err("runtime error: integer overflow")
	divideError   error = _err("runtime error: integer divide by zero")
)

//gopherjs:replace
func LeadingZeros32(x uint32) int {
	// See https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Math/clz32
	return js.Global.Get("Math").Call("clz32", x).Int()
}

//gopherjs:replace
func LeadingZeros64(x uint64) int {
	if hi := js.Uint64High(x); hi != 0 {
		return LeadingZeros32(hi)
	}
	return 32 + LeadingZeros32(js.Uint64Low(x))
}

//gopherjs:replace
func TrailingZeros32(x uint32) int {
	// See "ctrz" in https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Math/clz32
	if x == 0 {
		return 32
	}
	return 31 - LeadingZeros32(x&-x)
}

//gopherjs:replace
func TrailingZeros64(x uint64) int {
	lo := js.Uint64Low(x)
	if lo != 0 {
		return 31 - LeadingZeros32(lo&-lo)
	}
	hi := js.Uint64High(x)
	if hi == 0 {
		return 64
	}
	return 63 - LeadingZeros32(hi&-hi)
}

//gopherjs:replace
func OnesCount64(x uint64) int {
	return OnesCount32(js.Uint64High(x)) + OnesCount32(js.Uint64Low(x))
}

//gopherjs:replace
func RotateLeft64(x uint64, k int) uint64 {
	s := uint32(k) & 63
	if s == 0 {
		return x
	}
	xHi := js.Uint64High(x)
	xLo := js.Uint64Low(x)
	if s >= 32 {
		tmp := xLo
		xLo = xHi
		xHi = tmp
		s -= 32
	}
	if s == 0 {
		return js.MakeUint64(float64(xHi), float64(xLo))
	}
	rs := 32 - s
	return js.MakeUint64(float64(xHi<<s|xLo>>rs), float64(xLo<<s|xHi>>rs))
}

//gopherjs:replace
func Reverse64(x uint64) uint64 {
	return js.MakeUint64(
		float64(Reverse32(js.Uint64Low(x))),
		float64(Reverse32(js.Uint64High(x))))
}

//gopherjs:replace
func ReverseBytes64(x uint64) uint64 {
	return js.MakeUint64(
		float64(ReverseBytes32(js.Uint64Low(x))),
		float64(ReverseBytes32(js.Uint64High(x))))
}

//gopherjs:replace
func Len32(x uint32) int {
	return 32 - LeadingZeros32(x)
}

//gopherjs:replace
func Len64(x uint64) int {
	return 64 - LeadingZeros64(x)
}

//gopherjs:replace
func Mul32(x, y uint32) (hi, lo uint32) {
	// Avoid slow 64-bit integers for better performance. Adapted from Mul64().
	const mask16 = 1<<16 - 1
	x0 := x & mask16
	x1 := x >> 16
	y0 := y & mask16
	y1 := y >> 16
	w0 := x0 * y0
	t := x1*y0 + w0>>16
	w1 := t & mask16
	w2 := t >> 16
	w1 += x0 * y1
	hi = x1*y1 + w2 + w1>>16
	lo = x * y
	return
}

//gopherjs:replace
func Mul64(x, y uint64) (uint64, uint64) {
	const mask16 = 1<<16 - 1
	// Decompose x and y into 16-bit parts so each product fits in 32 bits.
	// Compute the 128-bit product using 16-bit column accumulation.
	// This code could be simplified using Mul32 but that will take longer
	// to run since Mul32 would have to decompose into 16-bit parts then
	// repack them into 32-bit numbers for every call, so it is faster
	// to keep the values decomposed as 16-bit numbers.
	xLo := js.Uint64Low(x)
	x0 := xLo & mask16
	x1 := xLo >> 16
	xHi := js.Uint64High(x)
	x2 := xHi & mask16
	x3 := xHi >> 16
	yLo := js.Uint64Low(y)
	y0 := yLo & mask16
	y1 := yLo >> 16
	yHi := js.Uint64High(y)
	y2 := yHi & mask16
	y3 := yHi >> 16
	// Column 0 (bits 0-15): 1 product
	c0 := x0 * y0
	c16 := c0 >> 16
	c0 &= mask16
	// Column 16 (bits 16-31): 2 products
	c16 += x1 * y0
	c32 := c16 >> 16
	c16 &= mask16
	c16 += x0 * y1
	c32 += c16 >> 16
	c16 &= mask16
	// Pack lo.$low (bits 0-31)
	loLo := c16<<16 | c0
	// Column 32 (bits 32-47): 3 products
	// First, split c32 so it's at most 16 bits before adding products
	c48 := c32 >> 16
	c32 &= mask16
	c32 += x2 * y0
	c48 += c32 >> 16
	c32 &= mask16
	c32 += x1 * y1
	c48 += c32 >> 16
	c32 &= mask16
	c32 += x0 * y2
	c48 += c32 >> 16
	c32 &= mask16
	// Column 48 (bits 48-63): 4 products
	c64 := c48 >> 16
	c48 &= mask16
	c48 += x3 * y0
	c64 += c48 >> 16
	c48 &= mask16
	c48 += x2 * y1
	c64 += c48 >> 16
	c48 &= mask16
	c48 += x1 * y2
	c64 += c48 >> 16
	c48 &= mask16
	c48 += x0 * y3
	c64 += c48 >> 16
	c48 &= mask16
	// Pack lo.$high (bits 32-63)
	loHi := c48<<16 | c32
	// Column 64 (bits 64-79): 3 products
	c80 := c64 >> 16
	c64 &= mask16
	c64 += x3 * y1
	c80 += c64 >> 16
	c64 &= mask16
	c64 += x2 * y2
	c80 += c64 >> 16
	c64 &= mask16
	c64 += x1 * y3
	c80 += c64 >> 16
	c64 &= mask16
	// Column 80 (bits 80-95): 2 products
	c96 := c80 >> 16
	c80 &= mask16
	c80 += x3 * y2
	c96 += c80 >> 16
	c80 &= mask16
	c80 += x2 * y3
	c96 += c80 >> 16
	c80 &= mask16
	// Pack hi.$low (bits 64-95)
	hiLo := c80<<16 | c64
	// Column 96 (bits 96-127): 1 product
	c96 += x3 * y3
	// hi.$high is c96 (bits 96-127, no masking needed at the top)
	return js.MakeUint64(float64(c96), float64(hiLo)), js.MakeUint64(float64(loHi), float64(loLo))
}

//gopherjs:replace
func Add32(x, y, carry uint32) (sum, carryOut uint32) {
	// Avoid slow 64-bit integers for better performance. Adapted from Add64().
	sum = x + y + carry
	carryOut = ((x & y) | ((x | y) &^ sum)) >> 31
	return
}

//gopherjs:replace
func Add64(x, y, carry uint64) (sum, carryOut uint64) {
	// Decompose into 32-bit halves and perform the addition as float64,
	// where JS can represent integers up to 2^53 exactly.
	// js.MakeUint64 handles low->high carry propagation automatically.
	hiSum := float64(js.Uint64High(x)) + float64(js.Uint64High(y))
	loSum := float64(js.Uint64Low(x)) + float64(js.Uint64Low(y)) + float64(js.Uint64Low(carry))
	sum = js.MakeUint64(hiSum, loSum)

	// Carry-out = 1 iff (hiSum + low->high carry) >= 2^32.
	if loSum >= 4294967296.0 {
		hiSum++
	}
	if hiSum >= 4294967296.0 {
		carryOut = 1
	}
	return
}

//gopherjs:replace
func Sub64(x, y, borrow uint64) (diff, borrowOut uint64) {
	// Mirror of nativeAdd64. The $Uint64 constructor correctly handles a
	// negative `low` value: Math.floor(negative/2^32) returns -1, which
	// propagates the borrow into the high half automatically.
	hiDiff := float64(js.Uint64High(x)) - float64(js.Uint64High(y))
	loDiff := float64(js.Uint64Low(x)) - float64(js.Uint64Low(y)) - float64(js.Uint64Low(borrow))
	diff = js.MakeUint64(hiDiff, loDiff)

	// Borrow-out = 1 iff the conceptual signed result (hiDiff*2^32 + loDiff)
	// is negative:
	//   hiDiff > 0: result definitely >= 0 (loDiff > -2^32).
	//   hiDiff = 0: result negative iff loDiff < 0.
	//   hiDiff < 0: result definitely < 0.
	if hiDiff < 0 || (hiDiff == 0 && loDiff < 0) {
		borrowOut = 1
	}
	return
}

//gopherjs:replace
func Div32(hi, lo, y uint32) (quo, rem uint32) {
	// Avoid slow 64-bit integers for better performance. Adapted from Div64().
	const (
		two16  = 1 << 16
		mask16 = two16 - 1
	)
	if y == 0 {
		panic(divideError)
	}
	if y <= hi {
		panic(overflowError)
	}

	s := uint(LeadingZeros32(y))
	y <<= s

	yn1 := y >> 16
	yn0 := y & mask16
	un16 := hi<<s | lo>>(32-s)
	un10 := lo << s
	un1 := un10 >> 16
	un0 := un10 & mask16
	q1 := un16 / yn1
	rhat := un16 - q1*yn1

	for q1 >= two16 || q1*yn0 > two16*rhat+un1 {
		q1--
		rhat += yn1
		if rhat >= two16 {
			break
		}
	}

	un21 := un16*two16 + un1 - q1*y
	q0 := un21 / yn1
	rhat = un21 - q0*yn1

	for q0 >= two16 || q0*yn0 > two16*rhat+un0 {
		q0--
		rhat += yn1
		if rhat >= two16 {
			break
		}
	}

	return q1*two16 + q0, (un21*two16 + un0 - q0*y) >> s
}

//gopherjs:replace
func Div64(hi, lo, y uint64) (quo, rem uint64) {
	// This code is similar to the original math/bits.Div64	with all arithmetic
	// operating on 32-bit halves to avoid using uint64 operations that we have
	// to emulate in JS.
	//
	// The original math/bits.Div64 appears to be based on
	// Algorithm D (Division of nonnegative integers) 4.3.1 starting on page 257 in
	// "The Art of Computer Programming" (TAoCP) Vol. 2 by Knuth
	// (a copy can be found at https://github.com/Code42Cate/The-Art-of-Computer-Programming/blob/master/Volume2.pdf)
	yn1 := js.Uint64High(y)
	yn0 := js.Uint64Low(y)
	if yn1 == 0 && yn0 == 0 {
		panic(divideError)
	}
	un32Hi := js.Uint64High(hi)
	un32Lo := js.Uint64Low(hi)
	if yn1 < un32Hi || (yn1 == un32Hi && yn0 <= un32Lo) {
		panic(overflowError)
	}
	un1 := js.Uint64High(lo)
	un0 := js.Uint64Low(lo)

	// If divisor fits in 32 bits, then the y > hi precondition forces
	// un32Hi == 0 and un32Lo < yn0, so neither Div32 below can overflow.
	// (At this point the limb names still hold their pre-shift values.)
	if yn1 == 0 {
		q1, r1 := Div32(un32Lo, un1, yn0)
		q0, r0 := Div32(r1, un0, yn0)
		return js.MakeUint64(float64(q1), float64(q0)),
			js.MakeUint64(0, float64(r0))
	}

	// yn1 != 0 (full 64-bit divisor).
	// Normalize so the divisor's top bit is set; s is in [0, 31]. We update
	// the six limbs in place — each assignment reads the OLD values of the
	// variables on its right since they are not overwritten until subsequent
	// lines. Because hi < y (precondition), hi << s still fits in 64 bits.
	s := uint(LeadingZeros32(yn1))
	rs := 32 - s
	if s != 0 {
		yn1 = yn1<<s | yn0>>rs
		yn0 = yn0 << s
		un32Hi = un32Hi<<s | un32Lo>>rs
		un32Lo = un32Lo<<s | un1>>rs
		un1 = un1<<s | un0>>rs
		un0 = un0 << s
	}

	// --- First quotient digit q1 ≈ (un32Hi:un32Lo) / yn1 ---
	// Precondition un32 < y gives un32Hi <= yn1. When un32Hi == yn1 the
	// true digit is 2^32 or 2^32+1; Knuth's loop implicitly decrements it
	// to 2^32-1 with rhat = un32Lo + yn1. If that sum overflows uint32 the
	// loop's "rhat >= two32" early-exit fires and no further adjustment is
	// possible from 32-bit rhat, so we mark skipAdj.
	var q1, rhat uint32
	var skipAdj bool
	if un32Hi == 0 {
		q1 = un32Lo / yn1
		rhat = un32Lo - q1*yn1
	} else if un32Hi >= yn1 {
		q1 = 0xFFFFFFFF
		sum, carry := Add32(un32Lo, yn1, 0)
		if carry != 0 {
			skipAdj = true
		} else {
			rhat = sum
		}
	} else {
		q1, rhat = Div32(un32Hi, un32Lo, yn1)
	}

	// Track q1 * yn0 incrementally across the correction loop so we avoid
	// re-multiplying every iteration and can reuse the final value for un21.
	qynHi, qynLo := Mul32(q1, yn0)
	if !skipAdj {
		for qynHi > rhat || (qynHi == rhat && qynLo > un1) {
			q1--
			if qynLo < yn0 {
				qynHi--
			}
			qynLo -= yn0
			sum, carry := Add32(rhat, yn1, 0)
			if carry != 0 {
				break
			}
			rhat = sum
		}
	}

	// un21 = (un32:un1) - q1*y, mod 2^64.
	// (un32 << 32) mod 2^64 = (un32Lo : 0), so the top half of un21 is
	// computed from un32Lo (not un32Hi). q1*y mod 2^64 has high 32 bits
	// q1*yn1 + carry(q1*yn0); both intentionally wrap modulo 2^32.
	qyHi := q1*yn1 + qynHi
	un21Lo := un1 - qynLo
	un21Hi := un32Lo - qyHi
	if un1 < qynLo {
		un21Hi--
	}

	// --- Second quotient digit q0 ≈ (un21Hi:un21Lo) / yn1 ---
	var q0 uint32
	skipAdj = false
	if un21Hi >= yn1 {
		q0 = 0xFFFFFFFF
		sum, carry := Add32(un21Lo, yn1, 0)
		if carry != 0 || un21Hi > yn1 {
			skipAdj = true
		} else {
			rhat = sum
		}
	} else {
		q0, rhat = Div32(un21Hi, un21Lo, yn1)
	}

	qynHi, qynLo = Mul32(q0, yn0)
	if !skipAdj {
		for qynHi > rhat || (qynHi == rhat && qynLo > un0) {
			q0--
			if qynLo < yn0 {
				qynHi--
			}
			qynLo -= yn0
			sum, carry := Add32(rhat, yn1, 0)
			if carry != 0 {
				break
			}
			rhat = sum
		}
	}

	// Remainder = (un21:un0) - q0*y, mod 2^64, then >> s to denormalize.
	// qyHi is dead after the un21 computation above, so reuse it here.
	qyHi = q0*yn1 + qynHi
	remLo := un0 - qynLo
	remHi := un21Lo - qyHi
	if un0 < qynLo {
		remHi--
	}
	if s != 0 {
		remLo = remLo>>s | remHi<<rs
		remHi = remHi >> s
	}

	return js.MakeUint64(float64(q1), float64(q0)),
		js.MakeUint64(float64(remHi), float64(remLo))
}

//gopherjs:replace
func Rem32(hi, lo, y uint32) uint32 {
	// We scale down hi so that hi < y, then use Div32 to compute the
	// rem with the guarantee that it won't panic on quotient overflow.
	// Given that
	//   hi ≡ hi%y    (mod y)
	// we have
	//   hi<<64 + lo ≡ (hi%y)<<64 + lo    (mod y)
	_, rem := Div32(hi%y, lo, y)
	return rem
}
