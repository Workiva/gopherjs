//go:build js

package tests

import "testing"

// `t.Helper()` can slow down tests because it hits the call stack which is also
// slow. So these benchmarks are to help us improve our call stack throughput.
//
// The `Helper()` function on `testing.T` and `testing.B` are the same method
// implemented by `testing.common` so by measuring the benchmark `Helper()`,
// we're also measuring the test `Helper()`.
//
// Each `helper{N}` function calls t.Helper() then chains to `helper{N-1}`,
// building up both real call depth and the number of Helper() invocations
// per top-level call. This lets us measure how cost scales with stack depth.
//
// Here are the measured resultsfrom this benchmark.
// "before" is the ns/op before any changes were made to optimize `Helper()`.
//
// | depth |  before |
// |:-----:|--------:|
// |   1   |  36,933 |
// |   3   | 116,012 |
// |   5   | 209,388 |
// |   7   | 314,133 |
// |   9   | 422,581 |
//

func helper1(tb testing.TB) { tb.Helper() }
func helper2(tb testing.TB) { tb.Helper(); helper1(tb) }
func helper3(tb testing.TB) { tb.Helper(); helper2(tb) }
func helper4(tb testing.TB) { tb.Helper(); helper3(tb) }
func helper5(tb testing.TB) { tb.Helper(); helper4(tb) }
func helper6(tb testing.TB) { tb.Helper(); helper5(tb) }
func helper7(tb testing.TB) { tb.Helper(); helper6(tb) }
func helper8(tb testing.TB) { tb.Helper(); helper7(tb) }
func helper9(tb testing.TB) { tb.Helper(); helper8(tb) }

func BenchmarkTestingHelper(b *testing.B) {
	tests := []struct {
		name string
		hndl func(b testing.TB)
	}{
		{name: "Depth 1", hndl: helper1},
		{name: "Depth 3", hndl: helper3},
		{name: "Depth 5", hndl: helper5},
		{name: "Depth 7", hndl: helper7},
		{name: "Depth 9", hndl: helper9},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				tt.hndl(b)
			}
		})
	}
}
