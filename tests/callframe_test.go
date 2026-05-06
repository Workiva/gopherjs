//go:build js

package tests

import (
	"testing"

	"github.com/gopherjs/gopherjs/js"
)

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
// Here are the measured results from this benchmark (run with Node.js v20.9.0).
// "before" is the ns/op before any changes were made to optimize `Helper()`.
// "cST" is the ns/op after adding calls to "captureStackTrace" for V8.
//
// | depth |  before |  cST    |
// |:-----:|--------:|--------:|
// |   1   |  36,933 |  14,979 |
// |   3   | 116,012 |  44,007 |
// |   5   | 209,388 |  78,579 |
// |   7   | 314,133 | 110,000 |
// |   9   | 422,581 | 138,506 |
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

type callFrame struct {
	FuncName string
	File     string
	Line     int
	Col      int
}

func parseFirefoxFrame(input string) callFrame {
	result := js.Global.Call("$parseCallFrameFirefox", input)
	return callFrame{
		FuncName: result.Index(0).String(),
		File:     result.Index(1).String(),
		Line:     result.Index(2).Int(),
		Col:      result.Index(3).Int(),
	}
}

func TestParseCallFrameFirefox(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  callFrame
	}{
		{
			name:  "normal frame",
			input: "getEvalResult@devtools/stuff/eval-with-debugger.js:231:24",
			want:  callFrame{FuncName: "getEvalResult", File: "devtools/stuff/eval-with-debugger.js", Line: 231, Col: 24},
		},
		{
			name:  "anonymous function",
			input: "@filename.js:10:15",
			want:  callFrame{FuncName: "<none>", File: "filename.js", Line: 10, Col: 15},
		},
		{
			name:  "no column number",
			input: "foo@bar.js:42",
			want:  callFrame{FuncName: "foo", File: "bar.js", Line: 42},
		},
		{
			name:  "no line or column",
			input: "foo@bar.js",
			want:  callFrame{FuncName: "foo", File: "bar.js"},
		},
		{
			name:  "file with colons in path",
			input: "baz@http://example.com/script.js:100:5",
			want:  callFrame{FuncName: "baz", File: "http://example.com/script.js", Line: 100, Col: 5},
		},
		{
			name:  "file colons in path without a column",
			input: "baz@http://example.com/script.js:100",
			want:  callFrame{FuncName: "baz", File: "http://example.com/script.js", Line: 100},
		},
		{
			name:  "file colons in path without a line or column",
			input: "baz@http://example.com/script.js",
			want:  callFrame{FuncName: "baz", File: "http://example.com/script.js"},
		},
		{
			name:  "resource with colons in path",
			input: "getEvalResult@resource://devtools/stuff/eval-with-debugger.js:231:24",
			want:  callFrame{FuncName: "getEvalResult", File: "resource://devtools/stuff/eval-with-debugger.js", Line: 231, Col: 24},
		},
		{
			name:  "anonymous with no line or column",
			input: "@eval",
			want:  callFrame{FuncName: "<none>", File: "eval"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFirefoxFrame(tt.input)
			if got != tt.want {
				t.Errorf("parseCallFrameFirefox(%q):\n  got:  %+v\n  want: %+v", tt.input, got, tt.want)
			}
		})
	}
}
