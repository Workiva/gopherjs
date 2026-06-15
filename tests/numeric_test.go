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

func Benchmark_Native_MathBits(b *testing.B) {
	if runtime.GOOS != "js" {
		b.Skip("native bit functions use JS-specific features")
	}

	const (
		inputSize = 1024
		randSeed  = 0xC0FFEE
		// set trial count and randomize to take multiple samples and randomize
		// the order of the tests to help reduce burn-in error and noise.
		trialCount = 1
		randomize  = false
	)

	type mathBitsBenchArg struct {
		x8            uint8
		x16           uint16
		x32, y32, c32 uint32
		x64, y64, c64 uint64
		k             int
	}

	r := rand.New(rand.NewSource(randSeed))
	ins := make([]*mathBitsBenchArg, inputSize)
	for i := 0; i < inputSize; i++ {
		x32 := r.Uint32()
		y32 := r.Uint32()
		c32 := r.Uint32() & 1 // carry or borrow must be 0 or 1
		x64 := r.Uint64()
		y64 := r.Uint64()
		c64 := r.Uint64() & 1
		k := int(r.Uint32())

		ins[i] = &mathBitsBenchArg{
			x8: uint8(x32), x16: uint16(x32),
			x32: x32, y32: y32, c32: c32,
			x64: x64, y64: y64, c64: c64, k: k,
		}
	}

	type testFn struct {
		name string
		fn   func(*mathBitsBenchArg)
	}

	tests := []testFn{
		// // --- LeadingZeros ---
		// {name: `LeadingZeros8`, fn: func(arg *mathBitsBenchArg) { _ = bits.LeadingZeros8(arg.x8) }},
		// {name: `LeadingZeros16`, fn: func(arg *mathBitsBenchArg) { _ = bits.LeadingZeros16(arg.x16) }},
		// {name: `LeadingZeros32`, fn: func(arg *mathBitsBenchArg) { _ = bits.LeadingZeros32(arg.x32) }},
		// {name: "LeadingZeros64", fn: func(arg *mathBitsBenchArg) { _ = bits.LeadingZeros64(arg.x64) }},
		// // --- TrailingZeros ---
		// {name: `TrailingZeros8`, fn: func(arg *mathBitsBenchArg) { _ = bits.TrailingZeros8(arg.x8) }},
		// {name: `TrailingZeros16`, fn: func(arg *mathBitsBenchArg) { _ = bits.TrailingZeros16(arg.x16) }},
		// {name: `TrailingZeros32`, fn: func(arg *mathBitsBenchArg) { _ = bits.TrailingZeros32(arg.x32) }},
		// {name: "TrailingZeros64", fn: func(arg *mathBitsBenchArg) { _ = bits.TrailingZeros64(arg.x64) }},
		// // --- OnesCount ---
		// {name: `OnesCount8`, fn: func(arg *mathBitsBenchArg) { _ = bits.OnesCount8(arg.x8) }},
		// {name: `OnesCount16`, fn: func(arg *mathBitsBenchArg) { _ = bits.OnesCount16(arg.x16) }},
		// {name: `OnesCount32`, fn: func(arg *mathBitsBenchArg) { _ = bits.OnesCount32(arg.x32) }},
		// {name: "OnesCount64", fn: func(arg *mathBitsBenchArg) { _ = bits.OnesCount64(arg.x64) }},
		// --- RotateLeft ---
		{name: "RotateLeft8", fn: func(arg *mathBitsBenchArg) { _ = bits.RotateLeft8(arg.x8, arg.k) }},
		{name: "RotateLeft16", fn: func(arg *mathBitsBenchArg) { _ = bits.RotateLeft16(arg.x16, arg.k) }},
		{name: "RotateLeft32", fn: func(arg *mathBitsBenchArg) { _ = bits.RotateLeft32(arg.x32, arg.k) }},
		{name: "RotateLeft64", fn: func(arg *mathBitsBenchArg) { _ = bits.RotateLeft64(arg.x64, arg.k) }},
		// --- Reverse ---
		{name: "Reverse8", fn: func(arg *mathBitsBenchArg) { _ = bits.Reverse8(arg.x8) }},
		{name: "Reverse16", fn: func(arg *mathBitsBenchArg) { _ = bits.Reverse16(arg.x16) }},
		{name: "Reverse32", fn: func(arg *mathBitsBenchArg) { _ = bits.Reverse32(arg.x32) }},
		{name: "Reverse64", fn: func(arg *mathBitsBenchArg) { _ = bits.Reverse64(arg.x64) }},
		// --- ReverseBytes ---
		{name: "ReverseBytes16", fn: func(arg *mathBitsBenchArg) { _ = bits.ReverseBytes16(arg.x16) }},
		{name: "ReverseBytes32", fn: func(arg *mathBitsBenchArg) { _ = bits.ReverseBytes32(arg.x32) }},
		{name: "ReverseBytes64", fn: func(arg *mathBitsBenchArg) { _ = bits.ReverseBytes64(arg.x64) }},
		// // --- Len ---
		// {name: `Len8`, fn: func(arg *mathBitsBenchArg) { _ = bits.Len8(arg.x8) }},
		// {name: `Len16`, fn: func(arg *mathBitsBenchArg) { _ = bits.Len16(arg.x16) }},
		// {name: `Len32`, fn: func(arg *mathBitsBenchArg) { _ = bits.Len32(arg.x32) }},
		// {name: "Len64", fn: func(arg *mathBitsBenchArg) { _ = bits.Len64(arg.x64) }},
		// // --- Add with carry ---
		// {name: "Add32", fn: func(arg *mathBitsBenchArg) { _, _ = bits.Add32(arg.x32, arg.y32, arg.c32) }},
		// {name: "Add64", fn: func(arg *mathBitsBenchArg) { _, _ = bits.Add64(arg.x64, arg.y64, arg.c64) }},
		// // --- Subtract with borrow ---
		// {name: "Sub32", fn: func(arg *mathBitsBenchArg) { _, _ = bits.Sub32(arg.x32, arg.y32, arg.c32) }},
		// {name: "Sub64", fn: func(arg *mathBitsBenchArg) { _, _ = bits.Sub64(arg.x64, arg.y64, arg.c64) }},
		// // --- Full-width multiply ---
		// {name: "Mul32", fn: func(arg *mathBitsBenchArg) { _, _ = bits.Mul32(arg.x32, arg.y32) }},
		// {name: "Mul64", fn: func(arg *mathBitsBenchArg) { _, _ = bits.Mul64(arg.x64, arg.y64) }},
		// // Full-width divide not benchmarked
	}

	trials := make([]testFn, 0, trialCount*len(tests))
	for i := 0; i < trialCount; i++ {
		trials = append(trials, tests...)
	}
	if randomize {
		r.Shuffle(len(trials), func(i, j int) {
			trials[i], trials[j] = trials[j], trials[i]
		})
	}

	for _, tt := range trials {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				tt.fn(ins[i%inputSize])
			}
		})
	}
}
