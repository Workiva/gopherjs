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
func Len32(x uint32) int {
	return 32 - LeadingZeros32(x)
}

//gopherjs:replace
func OnesCount32(x uint32) int {
	x = x - ((x >> 1) & 0x55555555)
	x = (x & 0x33333333) + ((x >> 2) & 0x33333333)
	x = (x + (x >> 4)) & 0x0F0F0F0F
	return int((x * 0x01010101) >> 24)
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
func Add32(x, y, carry uint32) (sum, carryOut uint32) {
	// Avoid slow 64-bit integers for better performance. Adapted from Add64().
	sum = x + y + carry
	carryOut = ((x & y) | ((x | y) &^ sum)) >> 31
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

//gopherjs:replace
func Len64(x uint64) int {
	return 64 - LeadingZeros64(x)
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
func Add64(x, y, carry uint64) (sum, carryOut uint64) {
	// Decompose into 32-bit halves and perform the addition as float64,
	// where JS can represent integers up to 2^53 exactly. js.MakeUint64
	// handles low->high carry propagation automatically.
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
