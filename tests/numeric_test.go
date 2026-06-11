package tests

import (
	"cmp"
	"fmt"
	"math"
	"math/bits"
	"math/rand"
	"runtime"
	"testing"
	"testing/quick"

	"github.com/gopherjs/gopherjs/js"
)

// naiveMul64 performs 64-bit multiplication without using the multiplication
// operation and can be used to test correctness of the compiler's multiplication
// implementation.
func naiveMul64(x, y uint64) uint64 {
	var z uint64 = 0
	for i := 0; i < 64; i++ {
		mask := uint64(1) << i
		if y&mask > 0 {
			z += x << i
		}
	}
	return z
}

func TestMul64(t *testing.T) {
	cfg := &quick.Config{
		MaxCountScale: 10000,
		Rand:          rand.New(rand.NewSource(0x5EED)), // Fixed seed for reproducibility.
	}
	if testing.Short() {
		cfg.MaxCountScale = 1000
	}

	t.Run("unsigned", func(t *testing.T) {
		err := quick.CheckEqual(
			func(x, y uint64) uint64 { return x * y },
			naiveMul64,
			cfg)
		if err != nil {
			t.Error(err)
		}
	})
	t.Run("signed", func(t *testing.T) {
		// GopherJS represents 64-bit signed integers in a two-complement form,
		// so bitwise multiplication looks identical for signed and unsigned integers
		// and we can reuse naiveMul64() as a reference implementation for both with
		// appropriate type conversions.
		err := quick.CheckEqual(
			func(x, y int64) int64 { return x * y },
			func(x, y int64) int64 { return int64(naiveMul64(uint64(x), uint64(y))) },
			cfg)
		if err != nil {
			t.Error(err)
		}
	})
}

func BenchmarkMul64(b *testing.B) {
	// Prepare a randomized set of multipliers to make sure the benchmark doesn't
	// get too specific for a single value. The trade-off is that the cost of
	// loading from an array gets mixed into the result, but it is good enough for
	// relative comparisons.
	r := rand.New(rand.NewSource(0x5EED))
	const size = 1024
	xU := [size]uint64{}
	yU := [size]uint64{}
	xS := [size]int64{}
	yS := [size]int64{}
	for i := 0; i < size; i++ {
		xU[i] = r.Uint64()
		yU[i] = r.Uint64()
		xS[i] = r.Int63() | (r.Int63n(2) << 63)
		yS[i] = r.Int63() | (r.Int63n(2) << 63)
	}

	b.Run("noop", func(b *testing.B) {
		// This benchmark allows to gauge the cost of array load operations without
		// the multiplications.
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(yU[i%size])
			runtime.KeepAlive(xU[i%size])
		}
	})
	b.Run("unsigned", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			z := xU[i%size] * yU[i%size]
			runtime.KeepAlive(z)
		}
	})
	b.Run("signed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			z := xS[i%size] * yS[i%size]
			runtime.KeepAlive(z)
		}
	})
}

func TestIssue733(t *testing.T) {
	if runtime.GOOS != "js" {
		t.Skip("test uses GopherJS-specific features")
	}

	t.Run("sign", func(t *testing.T) {
		f := float64(-1)
		i := uint32(f)
		underlying := js.InternalObject(i).Float() // Get the raw JS number behind i.
		if want := float64(4294967295); underlying != want {
			t.Errorf("Got: uint32(float64(%v)) = %v. Want: %v.", f, underlying, want)
		}
	})
	t.Run("truncation", func(t *testing.T) {
		f := float64(300)
		i := uint8(f)
		underlying := js.InternalObject(i).Float() // Get the raw JS number behind i.
		if want := float64(44); underlying != want {
			t.Errorf("Got: uint32(float64(%v)) = %v. Want: %v.", f, underlying, want)
		}
	})
}

// Test_32BitEnvironment tests that GopherJS behaves correctly
// as a 32-bit environment for integers. To simulate a 32 bit environment
// we have to use `$imul` instead of `*` to get the correct result.
func Test_32BitEnvironment(t *testing.T) {
	if bits.UintSize != 32 {
		t.Skip(`test is only relevant for 32-bit environment`)
	}

	tests := []struct {
		x, y, exp uint64
	}{
		{
			x:   65535,      // x = 2^16 - 1
			y:   65535,      // same as x
			exp: 4294836225, // x² works since it doesn't overflow 32 bits.
		},
		{
			x:   134217729, // x = 2^27 + 1, x < 2^32 and x > sqrt(2^53), so x² overflows 53 bits.
			y:   134217729, // same as x
			exp: 268435457, // x² mod 2^32 = (2^27 + 1)² mod 2^32 = (2^54 + 2^28 + 1) mod 2^32 = 2^28 + 1
			// In pure JS, `x * x >>> 0`, would result in 268,435,456 because it lost the least significant bit
			// prior to being truncated, where in a real 32 bit environment, it would be 268,435,457 since
			// the rollover removed the most significant bit and doesn't affect the least significant bit.
		},
		{
			x:   4294967295, // x = 2^32 - 1 another case where x² overflows 53 bits causing a loss of precision.
			y:   4294967295, // same as x
			exp: 1,          // x² mod 2^32 = (2^32 - 1)² mod 2^32 = (2^64 - 2^33 + 1) mod 2^32 = 1
			// In pure JS, `x * x >>> 0`, would result in 0 because it lost the least significant bits.
		},
		{
			x:   4294967295, // x = 2^32 - 1
			y:   3221225473, // y = 2^31 + 2^30 + 1
			exp: 1073741823, // 2^32 - 1.
			// In pure JS, `x * y >>> 0`, would result in 1,073,741,824.
		},
		{
			x:   4294967295, // x = 2^32 - 1
			y:   134217729,  // y = 2^27 + 1
			exp: 4160749567, // In pure JS, `x * y >>> 0`, would result in 4,160,749,568.
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf(`#%d/uint32`, i), func(t *testing.T) {
			x, y, exp := uint32(test.x), uint32(test.y), uint32(test.exp)
			if got := x * y; got != exp {
				t.Errorf("got: %d\nwant: %d.", got, exp)
			}
		})

		t.Run(fmt.Sprintf(`#%d/uintptr`, i), func(t *testing.T) {
			x, y, exp := uintptr(test.x), uintptr(test.y), uintptr(test.exp)
			if got := x * y; got != exp {
				t.Errorf("got: %d\nwant: %d.", got, exp)
			}
		})

		t.Run(fmt.Sprintf(`#%d/uint`, i), func(t *testing.T) {
			x, y, exp := uint(test.x), uint(test.y), uint(test.exp)
			if got := x * y; got != exp {
				t.Errorf("got: %d\nwant: %d.", got, exp)
			}
		})

		t.Run(fmt.Sprintf(`#%d/int32`, i), func(t *testing.T) {
			x, y, exp := int32(test.x), int32(test.y), int32(test.exp)
			if got := x * y; got != exp {
				t.Errorf("got: %d\nwant: %d.", got, exp)
			}
		})

		t.Run(fmt.Sprintf(`#%d/int`, i), func(t *testing.T) {
			x, y, exp := int(test.x), int(test.y), int(test.exp)
			if got := x * y; got != exp {
				t.Errorf("got: %d\nwant: %d.", got, exp)
			}
		})
	}
}

// checkMinMax2 is a helper for Test_MinMax that checks the builtin min
// and max methods. The x value must be less than y.
func checkMinMax2[T cmp.Ordered](t *testing.T, x, y T) {
	t.Helper()
	check := func(a, b T) {
		if got, want := min(a, b), x; got != want {
			t.Errorf("min[%T](%v, %v): got: %v, want: %v", want, a, b, got, want)
		}
		if got, want := max(a, b), y; got != want {
			t.Errorf("max[%T](%v, %v): got: %v, want: %v", want, a, b, got, want)
		}
	}
	check(x, y)
	check(y, x)
}

// checkMinMax4 is a helper for Test_MinMax that checks the builtin min
// and max methods. The builtin min and max are not actually veriadic,
// so cannot be tested via `min(first, rest...)`, but they do allow 1 or more
// arguments, so this one checks 4 arguments. v1 must be the actual min,
// and v4 must be the actual max.
func checkMinMax4[T cmp.Ordered](t *testing.T, v1, v2, v3, v4 T) {
	t.Helper()
	check := func(a, b, c, d T) {
		if got, want := min(a, b, c, d), v1; got != want {
			t.Errorf("min[%T](%v, %v, %v, %v): got: %v, want: %v", want, a, b, c, d, got, want)
		}
		if got, want := max(a, b, c, d), v4; got != want {
			t.Errorf("max[%T](%v, %v, %v, %v): got: %v, want: %v", want, a, b, c, d, got, want)
		}
	}
	check(v1, v2, v3, v4)
	check(v1, v2, v4, v3)
	check(v1, v4, v2, v3)
	check(v1, v4, v3, v2)
	check(v2, v1, v3, v4)
	check(v2, v1, v4, v3)
	check(v2, v4, v1, v3)
	check(v2, v4, v3, v1)
	check(v3, v1, v2, v4)
	check(v3, v1, v4, v2)
	check(v3, v4, v1, v2)
	check(v3, v4, v2, v1)
	check(v4, v1, v2, v3)
	check(v4, v1, v3, v2)
	check(v4, v3, v1, v2)
	check(v4, v3, v2, v1)
}

// checkMinMax1 is a helper for Test_MinMax that checks the builtin min
// and max methods. This checks the edge case with 1 argument.
func checkMinMax1[T cmp.Ordered](t *testing.T, x T) {
	t.Helper()
	if got := min(x); got != x {
		t.Errorf("min[%T](%v): got: %v, want: %v", x, x, got, x)
	}
	if got := max(x); got != x {
		t.Errorf("max[%T](%v): got: %v, want: %v", x, x, got, x)
	}
}

func Test_MinMax(t *testing.T) {
	checkMinMax2(t, 0, 1)     // int
	checkMinMax2(t, -1, 0)    // int
	checkMinMax2(t, 12, 42)   // int
	checkMinMax2(t, -42, -12) // int
	checkMinMax2[int8](t, -9, 13)
	checkMinMax2[int16](t, 0, 23)
	checkMinMax2[int32](t, -87, 1234)
	checkMinMax2[int64](t, -0xDEAD_BEEF, 0x7FFF_FFFF_FFFF_FFFF)
	checkMinMax2[uint8](t, 9, 13)
	checkMinMax2[uint16](t, 0, 23)
	checkMinMax2[uint32](t, 87, 1234)
	checkMinMax2[uint64](t, 0xDEAD_BEEF, 0x7FFF_FFFF_FFFF_FFFF)
	checkMinMax2[uintptr](t, 12345, 54321)
	checkMinMax2[float32](t, 1.41421356237, 3.14159265359)
	checkMinMax2(t, -3.14159265359, 1.41421356237) // float64
	checkMinMax2(t, ``, `a`)                       // string
	checkMinMax2(t, `a`, `z`)                      // string
	checkMinMax2(t, `a`, `aa`)                     // string
	checkMinMax2(t, `banana`, `cat`)               // string
	checkMinMax2(t, `Dog`, `dog`)                  // string

	checkMinMax4(t, -4, -3, -2, -1) // int
	checkMinMax4(t, 1, 2, 3, 4)     // int
	checkMinMax4[int64](t, -4, -3, -2, -1)
	checkMinMax4[int64](t, 1, 2, 3, 4)
	checkMinMax4[uint64](t, 1, 2, 3, 4)
	checkMinMax4(t, `apple`, `banana`, `carrot`, `durian`) // string

	checkMinMax1(t, 1) // int
	checkMinMax1[int64](t, -19)
	checkMinMax1[uint64](t, 244)
	checkMinMax1(t, 2.3)    // float64
	checkMinMax1(t, `Ludo`) // string

	// Note that math.Min and math.Max act differently for NaN than max and min,
	// see [https://github.com/golang/go/issues/60616]
	// Fortunelty the builtin max and min act like JS's Math.max and Math.min,
	// see [https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Math/min]
	// If any argument is NaN, then NaN will be returned.
	if got := min(42, math.NaN(), -81); !math.IsNaN(got) {
		t.Errorf("min(..NaN..): got: %v, want: %v", got, math.NaN())
	}
	if got := max(42, math.NaN(), -81); !math.IsNaN(got) {
		t.Errorf("max(..NaN..): got: %v, want: %v", got, math.NaN())
	}
}

// =============================================================================
// Experimental native bit-twiddling helpers.
//
// The following nativeX functions are alternative implementations of selected
// math/bits functions, written to leverage JavaScript-specific properties:
//   - JS numbers can hold values larger than 2^32 exactly (up to 2^53), so an
//     addition of two uint32 values does not overflow; the carry can be read
//     off the high bits of the sum without a separate bit formula.
//   - GopherJS represents uint64 as a JS object with $high and $low fields,
//     each a uint32. Decomposing uint64 ops into two 32-bit halves can avoid
//     the cost of full uint64 arithmetic.
//   - V8 / SpiderMonkey expose Math.clz32 as a fast count-leading-zeros
//     primitive, often a single machine instruction.
//
// These are kept in test code so we can benchmark them against the standard
// math/bits implementations before promoting any to native overrides in
// compiler/natives. They are JS-only.
// =============================================================================

func nativeLeadingZeros32(x uint32) int {
	return js.Global.Get("Math").Call("clz32", x).Int()
}

func nativeTrailingZeros32(x uint32) int {
	if x == 0 {
		return 32
	}
	return 31 - js.Global.Get("Math").Call("clz32", x&-x).Int()
}

func nativeLen32(x uint32) int {
	return 32 - js.Global.Get("Math").Call("clz32", x).Int()
}

func nativeOnesCount32(x uint32) int {
	x = x - ((x >> 1) & 0x55555555)
	x = (x & 0x33333333) + ((x >> 2) & 0x33333333)
	x = (x + (x >> 4)) & 0x0F0F0F0F
	return int((x * 0x01010101) >> 24)
}

func nativeAdd32(x, y, carry uint32) (sum, carryOut uint32) {
	total := float64(x) + float64(y) + float64(carry)
	sum = uint32(total)
	if total >= 4294967296.0 {
		carryOut = 1
	}
	return
}

func nativeSub32(x, y, borrow uint32) (diff, borrowOut uint32) {
	total := float64(x) - float64(y) - float64(borrow)
	diff = uint32(total)
	if total < 0 {
		borrowOut = 1
	}
	return
}

func nativeMul32(x, y uint32) (hi, lo uint32) {
	xf := float64(x)
	yLo := float64(y & 0xFFFF)
	yHi := float64(y >> 16)

	// Both products are exact since each factor is < 2^16 on the y side and
	// < 2^32 on the x side, so the product is < 2^48 (fits in float64 mantissa).
	lo48 := xf * yLo
	hi48 := xf * yHi

	// Decompose hi48 into the bottom 16 bits (which contribute to bits 16-31
	// of the full product) and the remaining 32 bits (bits 32-63).
	hi48Div16 := math.Floor(hi48 / 65536.0)
	hi48Mod16 := hi48 - hi48Div16*65536.0

	// mid is lo48 plus the contribution of the bottom 16 bits of hi48
	// shifted left by 16. Still ≤ 2^48 + 2^32, so exact in float64.
	mid := lo48 + hi48Mod16*65536.0
	lo = uint32(mid)
	hi = uint32(math.Floor(mid/4294967296.0) + hi48Div16)
	return
}

func nativeLeadingZeros64(x uint64) int {
	hi := js.InternalObject(x).Get("$high").Float()
	if hi != 0 {
		return js.Global.Get("Math").Call("clz32", hi).Int()
	}
	lo := js.InternalObject(x).Get("$low").Float()
	return 32 + js.Global.Get("Math").Call("clz32", lo).Int()
}

func nativeTrailingZeros64(x uint64) int {
	lo := uint32(js.InternalObject(x).Get("$low").Float())
	if lo != 0 {
		return 31 - js.Global.Get("Math").Call("clz32", lo&-lo).Int()
	}
	hi := uint32(js.InternalObject(x).Get("$high").Float())
	if hi == 0 {
		return 64
	}
	return 32 + 31 - js.Global.Get("Math").Call("clz32", hi&-hi).Int()
}

func nativeLen64(x uint64) int {
	return 64 - nativeLeadingZeros64(x)
}

func nativeOnesCount64(x uint64) int {
	hi := uint32(js.InternalObject(x).Get("$high").Float())
	lo := uint32(js.InternalObject(x).Get("$low").Float())
	return nativeOnesCount32(hi) + nativeOnesCount32(lo)
}

func nativeAdd64(x, y, carry uint64) (sum, carryOut uint64) {
	xHi := js.InternalObject(x).Get("$high").Float()
	xLo := js.InternalObject(x).Get("$low").Float()
	yHi := js.InternalObject(y).Get("$high").Float()
	yLo := js.InternalObject(y).Get("$low").Float()
	cLo := js.InternalObject(carry).Get("$low").Float()

	lowSum := xLo + yLo + cLo // ≤ 2^33 - 1, exact in float64
	sumLo := uint32(lowSum)
	lowCarry := math.Floor(lowSum / 4294967296.0) // 0 or 1

	highSum := xHi + yHi + lowCarry // ≤ 2^33 - 1, exact in float64
	sumHi := uint32(highSum)
	highCarry := uint32(math.Floor(highSum / 4294967296.0)) // 0 or 1

	sum = uint64(sumHi)<<32 | uint64(sumLo)
	carryOut = uint64(highCarry)
	return
}

func nativeSub64(x, y, borrow uint64) (diff, borrowOut uint64) {
	xHi := js.InternalObject(x).Get("$high").Float()
	xLo := js.InternalObject(x).Get("$low").Float()
	yHi := js.InternalObject(y).Get("$high").Float()
	yLo := js.InternalObject(y).Get("$low").Float()
	bLo := js.InternalObject(borrow).Get("$low").Float()

	lowDiff := xLo - yLo - bLo // range: -(2^32) to 2^32-1
	diffLo := uint32(lowDiff)
	var lowBorrow float64
	if lowDiff < 0 {
		lowBorrow = 1
	}

	highDiff := xHi - yHi - lowBorrow
	diffHi := uint32(highDiff)
	var highBorrow uint32
	if highDiff < 0 {
		highBorrow = 1
	}

	diff = uint64(diffHi)<<32 | uint64(diffLo)
	borrowOut = uint64(highBorrow)
	return
}

// =============================================================================
// Test inputs and TestNativeBits to verify each nativeX matches bits.X.
// =============================================================================

const (
	nativeBitsBenchSize = 1024
	nativeBitsBenchSeed = 0xC0FFEE
)

type nativeBitsInputs struct {
	xs32, ys32, cs32 []uint32
	xs64, ys64, cs64 []uint64
}

func generateNativeBitsInputs() *nativeBitsInputs {
	r := rand.New(rand.NewSource(nativeBitsBenchSeed))
	in := &nativeBitsInputs{
		xs32: make([]uint32, nativeBitsBenchSize),
		ys32: make([]uint32, nativeBitsBenchSize),
		cs32: make([]uint32, nativeBitsBenchSize),
		xs64: make([]uint64, nativeBitsBenchSize),
		ys64: make([]uint64, nativeBitsBenchSize),
		cs64: make([]uint64, nativeBitsBenchSize),
	}
	for i := 0; i < nativeBitsBenchSize; i++ {
		in.xs32[i] = r.Uint32()
		in.ys32[i] = r.Uint32()
		in.cs32[i] = r.Uint32() & 1 // carry/borrow must be 0 or 1
		in.xs64[i] = r.Uint64()
		in.ys64[i] = r.Uint64()
		in.cs64[i] = r.Uint64() & 1
	}
	return in
}

func TestNative_Bits_Bits(t *testing.T) {
	if runtime.GOOS != "js" {
		t.Skip("native bit functions use JS-specific features")
	}

	in := generateNativeBitsInputs()

	for i := 0; i < nativeBitsBenchSize; i++ {
		x32 := in.xs32[i]
		y32 := in.ys32[i]
		c32 := in.cs32[i]
		x64 := in.xs64[i]
		y64 := in.ys64[i]
		c64 := in.cs64[i]

		if got, want := nativeLeadingZeros32(x32), bits.LeadingZeros32(x32); got != want {
			t.Errorf("nativeLeadingZeros32(0x%08x) = %d, want %d", x32, got, want)
		}
		if got, want := nativeTrailingZeros32(x32), bits.TrailingZeros32(x32); got != want {
			t.Errorf("nativeTrailingZeros32(0x%08x) = %d, want %d", x32, got, want)
		}
		if got, want := nativeLen32(x32), bits.Len32(x32); got != want {
			t.Errorf("nativeLen32(0x%08x) = %d, want %d", x32, got, want)
		}
		if got, want := nativeOnesCount32(x32), bits.OnesCount32(x32); got != want {
			t.Errorf("nativeOnesCount32(0x%08x) = %d, want %d", x32, got, want)
		}

		wantSum, wantCarry := bits.Add32(x32, y32, c32)
		gotSum, gotCarry := nativeAdd32(x32, y32, c32)
		if gotSum != wantSum || gotCarry != wantCarry {
			t.Errorf("nativeAdd32(0x%08x, 0x%08x, %d) = (0x%08x, %d), want (0x%08x, %d)",
				x32, y32, c32, gotSum, gotCarry, wantSum, wantCarry)
		}
		wantDiff, wantBorrow := bits.Sub32(x32, y32, c32)
		gotDiff, gotBorrow := nativeSub32(x32, y32, c32)
		if gotDiff != wantDiff || gotBorrow != wantBorrow {
			t.Errorf("nativeSub32(0x%08x, 0x%08x, %d) = (0x%08x, %d), want (0x%08x, %d)",
				x32, y32, c32, gotDiff, gotBorrow, wantDiff, wantBorrow)
		}
		wantHi32, wantLo32 := bits.Mul32(x32, y32)
		gotHi32, gotLo32 := nativeMul32(x32, y32)
		if gotHi32 != wantHi32 || gotLo32 != wantLo32 {
			t.Errorf("nativeMul32(0x%08x, 0x%08x) = (0x%08x, 0x%08x), want (0x%08x, 0x%08x)",
				x32, y32, gotHi32, gotLo32, wantHi32, wantLo32)
		}

		if got, want := nativeLeadingZeros64(x64), bits.LeadingZeros64(x64); got != want {
			t.Errorf("nativeLeadingZeros64(0x%016x) = %d, want %d", x64, got, want)
		}
		if got, want := nativeTrailingZeros64(x64), bits.TrailingZeros64(x64); got != want {
			t.Errorf("nativeTrailingZeros64(0x%016x) = %d, want %d", x64, got, want)
		}
		if got, want := nativeLen64(x64), bits.Len64(x64); got != want {
			t.Errorf("nativeLen64(0x%016x) = %d, want %d", x64, got, want)
		}
		if got, want := nativeOnesCount64(x64), bits.OnesCount64(x64); got != want {
			t.Errorf("nativeOnesCount64(0x%016x) = %d, want %d", x64, got, want)
		}

		wantSum64, wantCarry64 := bits.Add64(x64, y64, c64)
		gotSum64, gotCarry64 := nativeAdd64(x64, y64, c64)
		if gotSum64 != wantSum64 || gotCarry64 != wantCarry64 {
			t.Errorf("nativeAdd64(0x%016x, 0x%016x, %d) = (0x%016x, %d), want (0x%016x, %d)",
				x64, y64, c64, gotSum64, gotCarry64, wantSum64, wantCarry64)
		}
		wantDiff64, wantBorrow64 := bits.Sub64(x64, y64, c64)
		gotDiff64, gotBorrow64 := nativeSub64(x64, y64, c64)
		if gotDiff64 != wantDiff64 || gotBorrow64 != wantBorrow64 {
			t.Errorf("nativeSub64(0x%016x, 0x%016x, %d) = (0x%016x, %d), want (0x%016x, %d)",
				x64, y64, c64, gotDiff64, gotBorrow64, wantDiff64, wantBorrow64)
		}
	}
}

// =============================================================================
// Benchmarks: each one compares bits.X to nativeX over the same shared inputs.
// =============================================================================

func Benchmark_Bits_LeadingZeros32(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(bits.LeadingZeros32(in.xs32[i%nativeBitsBenchSize]))
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(nativeLeadingZeros32(in.xs32[i%nativeBitsBenchSize]))
		}
	})
}

func Benchmark_Bits_TrailingZeros32(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(bits.TrailingZeros32(in.xs32[i%nativeBitsBenchSize]))
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(nativeTrailingZeros32(in.xs32[i%nativeBitsBenchSize]))
		}
	})
}

func Benchmark_Bits_Len32(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(bits.Len32(in.xs32[i%nativeBitsBenchSize]))
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(nativeLen32(in.xs32[i%nativeBitsBenchSize]))
		}
	})
}

func Benchmark_Bits_OnesCount32(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(bits.OnesCount32(in.xs32[i%nativeBitsBenchSize]))
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(nativeOnesCount32(in.xs32[i%nativeBitsBenchSize]))
		}
	})
}

func Benchmark_Bits_Add32(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			j := i % nativeBitsBenchSize
			s, c := bits.Add32(in.xs32[j], in.ys32[j], in.cs32[j])
			runtime.KeepAlive(s)
			runtime.KeepAlive(c)
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			j := i % nativeBitsBenchSize
			s, c := nativeAdd32(in.xs32[j], in.ys32[j], in.cs32[j])
			runtime.KeepAlive(s)
			runtime.KeepAlive(c)
		}
	})
}

func Benchmark_Bits_Sub32(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			j := i % nativeBitsBenchSize
			d, br := bits.Sub32(in.xs32[j], in.ys32[j], in.cs32[j])
			runtime.KeepAlive(d)
			runtime.KeepAlive(br)
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			j := i % nativeBitsBenchSize
			d, br := nativeSub32(in.xs32[j], in.ys32[j], in.cs32[j])
			runtime.KeepAlive(d)
			runtime.KeepAlive(br)
		}
	})
}

func Benchmark_Bits_Mul32(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			j := i % nativeBitsBenchSize
			h, l := bits.Mul32(in.xs32[j], in.ys32[j])
			runtime.KeepAlive(h)
			runtime.KeepAlive(l)
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			j := i % nativeBitsBenchSize
			h, l := nativeMul32(in.xs32[j], in.ys32[j])
			runtime.KeepAlive(h)
			runtime.KeepAlive(l)
		}
	})
}

func Benchmark_Bits_LeadingZeros64(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(bits.LeadingZeros64(in.xs64[i%nativeBitsBenchSize]))
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(nativeLeadingZeros64(in.xs64[i%nativeBitsBenchSize]))
		}
	})
}

func Benchmark_Bits_TrailingZeros64(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(bits.TrailingZeros64(in.xs64[i%nativeBitsBenchSize]))
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(nativeTrailingZeros64(in.xs64[i%nativeBitsBenchSize]))
		}
	})
}

func Benchmark_Bits_Len64(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(bits.Len64(in.xs64[i%nativeBitsBenchSize]))
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(nativeLen64(in.xs64[i%nativeBitsBenchSize]))
		}
	})
}

func Benchmark_Bits_OnesCount64(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(bits.OnesCount64(in.xs64[i%nativeBitsBenchSize]))
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			runtime.KeepAlive(nativeOnesCount64(in.xs64[i%nativeBitsBenchSize]))
		}
	})
}

func Benchmark_Bits_Add64(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			j := i % nativeBitsBenchSize
			s, c := bits.Add64(in.xs64[j], in.ys64[j], in.cs64[j])
			runtime.KeepAlive(s)
			runtime.KeepAlive(c)
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			j := i % nativeBitsBenchSize
			s, c := nativeAdd64(in.xs64[j], in.ys64[j], in.cs64[j])
			runtime.KeepAlive(s)
			runtime.KeepAlive(c)
		}
	})
}

func Benchmark_Bits_Sub64(b *testing.B) {
	in := generateNativeBitsInputs()
	b.Run("bits", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			j := i % nativeBitsBenchSize
			d, br := bits.Sub64(in.xs64[j], in.ys64[j], in.cs64[j])
			runtime.KeepAlive(d)
			runtime.KeepAlive(br)
		}
	})
	b.Run("native", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			j := i % nativeBitsBenchSize
			d, br := nativeSub64(in.xs64[j], in.ys64[j], in.cs64[j])
			runtime.KeepAlive(d)
			runtime.KeepAlive(br)
		}
	})
}
