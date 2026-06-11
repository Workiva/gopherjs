//go:build js

package bigmod

import "math/bits"

// montgomeryMul calculates x = a * b / R mod m, with R = 2^(_W * n) and
// n = len(m.nat.limbs), also known as a Montgomery multiplication.
//
// All inputs should be the same length and already reduced modulo m.
// x will be resized to the size of m and overwritten.
//
//gopherjs:replace
func (x *Nat) montgomeryMul(a *Nat, b *Nat, m *Modulus) *Nat {
	// GOPHERJS: The original has specialized cases for 1024, 1536, and 2048 bit
	// sizes that use addMulVVW1024/1536/2048 which take *uint pointers.
	// Those functions then use unsafe.Slice to convert back to slices.
	// In GopherJS, creating pointers via &slice[i] generates an $indexPtr.
	// We avoid this by always using the no-asm slice-based implementation
	// which calls addMulVVW with slices directly.

	n := len(m.nat.limbs)
	mLimbs := m.nat.limbs[:n]
	aLimbs := a.limbs[:n]
	bLimbs := b.limbs[:n]

	T := make([]uint, n*2)

	// This loop implements Word-by-Word Montgomery Multiplication, as
	// described in Algorithm 4 (Fig. 3) of "Efficient Software
	// Implementations of Modular Exponentiation" by Shay Gueron
	// [https://eprint.iacr.org/2011/239.pdf].
	var c uint
	for i := 0; i < n; i++ {
		_ = T[n+i] // bounds check elimination hint

		// Step 1 (T = a × b) is computed as a large pen-and-paper column
		// multiplication of two numbers with n base-2^_W digits. If we just
		// wanted to produce 2n-wide T, we would do
		//
		//   for i := 0; i < n; i++ {
		//       d := bLimbs[i]
		//       T[n+i] = addMulVVW(T[i:n+i], aLimbs, d)
		//   }
		//
		// where d is a digit of the multiplier, T[i:n+i] is the shifted
		// position of the product of that digit, and T[n+i] is the final carry.
		// Note that T[i] isn't modified after processing the i-th digit.
		//
		// Instead of running two loops, one for Step 1 and one for Steps 2–6,
		// the result of Step 1 is computed during the next loop. This is
		// possible because each iteration only uses T[i] in Step 2 and then
		// discards it in Step 6.
		d := bLimbs[i]
		c1 := addMulVVW(T[i:n+i], aLimbs, d)

		// Step 6 is replaced by shifting the virtual window we operate
		// over: T of the algorithm is T[i:] for us. That means that T1 in
		// Step 2 (T mod 2^_W) is simply T[i]. k0 in Step 3 is our m0inv.
		Y := T[i] * m.m0inv

		// Step 4 and 5 add Y × m to T, which as mentioned above is stored
		// at T[i:]. The two carries (from a × d and Y × m) are added up in
		// the next word T[n+i], and the carry bit from that addition is
		// brought forward to the next iteration.
		c2 := addMulVVW(T[i:n+i], mLimbs, Y)
		T[n+i], c = bits.Add(c1, c2, c)
	}

	// Finally for Step 7 we copy the final T window into x, and subtract m
	// if necessary (which as explained in maybeSubtractModulus can be the
	// case both if x >= m, or if x overflowed).
	//
	// The paper suggests in Section 4 that we can do an "Almost Montgomery
	// Multiplication" by subtracting only in the overflow case, but the
	// cost is very similar since the constant time subtraction tells us if
	// x >= m as a side effect, and taking care of the broken invariant is
	// highly undesirable (see https://go.dev/issue/13907).
	copy(x.reset(n).limbs, T[n:])
	x.maybeSubtractModulus(choice(c), m)

	return x
}

//gopherjs:purge
func addMulVVW1024(z, x *uint, y uint) (c uint)

//gopherjs:purge
func addMulVVW1536(z, x *uint, y uint) (c uint)

//gopherjs:purge
func addMulVVW2048(z, x *uint, y uint) (c uint)
