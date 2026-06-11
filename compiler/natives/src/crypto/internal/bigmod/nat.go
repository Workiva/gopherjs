//go:build js

package bigmod

import "math/bits"

// GOPHERJS: The original has specialized cases for 1024, 1536, and 2048 bit
// sizes that use addMulVVW1024/1536/2048 which take *uint pointers.
// Those functions then use unsafe.Slice to convert back to slices.
// In GopherJS, creating pointers via &slice[i] generates an $indexPtr.
// We avoid this by always using the no-asm slice-based implementation
// which calls addMulVVW with slices directly.
//
//gopherjs:replace
func (x *Nat) montgomeryMul(a *Nat, b *Nat, m *Modulus) *Nat {
	n := len(m.nat.limbs)
	mLimbs := m.nat.limbs[:n]
	aLimbs := a.limbs[:n]
	bLimbs := b.limbs[:n]

	T := make([]uint, n*2)

	var c uint
	for i := 0; i < n; i++ {
		_ = T[n+i] // bounds check elimination hint
		d := bLimbs[i]
		c1 := addMulVVW(T[i:n+i], aLimbs, d)
		Y := T[i] * m.m0inv
		c2 := addMulVVW(T[i:n+i], mLimbs, Y)
		T[n+i], c = bits.Add(c1, c2, c)
	}

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
