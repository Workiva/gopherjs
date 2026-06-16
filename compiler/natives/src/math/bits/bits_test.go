//go:build js

package bits

import (
	"math/rand"
	"runtime"
	"testing"
)

const (
	nativeOverrideTestCount = 1024
	randSeed                = 0xC0FFEE
)

func Test_NativeOverrides_LeadingZeros32(t *testing.T) {
	if runtime.GOOS != "js" {
		t.Skip("native bit functions use JS-specific features")
	}
	r := rand.New(rand.NewSource(randSeed))
	for i := 0; i < nativeOverrideTestCount; i++ {
		x := r.Uint32()
		got := LeadingZeros32(x)
		want := _gopherjs_original_LeadingZeros32(x)
		if got != want {
			t.Errorf("unexpected result from LeadingZeros32(0x%04X): got: %d, want: %d\n", x, got, want)
		}
	}
}

func Test_NativeOverrides_LeadingZeros64(t *testing.T) {
	if runtime.GOOS != "js" {
		t.Skip("native bit functions use JS-specific features")
	}
	r := rand.New(rand.NewSource(randSeed))
	for i := 0; i < nativeOverrideTestCount; i++ {
		x := r.Uint64()
		got := LeadingZeros64(x)
		want := _gopherjs_original_LeadingZeros64(x)
		if got != want {
			t.Errorf("unexpected result from LeadingZeros64(0x%08X): got: %d, want: %d\n", x, got, want)
		}
	}
}
