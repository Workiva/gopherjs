//go:build js && gopherjs

package tests

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
	_ "unsafe"

	"github.com/google/go-cmp/cmp"

	"github.com/gopherjs/gopherjs/js"
)

type callFrame struct { // same as runtime.basicFrame
	FuncName string
	File     string
	Line     int
	Col      int
}

// parseCallFrame creates a stack iterator for the given string then
// reads the first line off of it, parses that line, and returns the result.
// This is very similar to how `callstack` parses several frames in a stack.
func parseCallFrame(input string) callFrame {
	frame := js.Global.Call("$stackIter", input).Invoke()
	if frame == nil || frame == js.Undefined {
		return callFrame{} // no frame could be read
	}
	return callFrame{
		FuncName: frame.Index(0).String(),
		File:     frame.Index(1).String(),
		Line:     frame.Index(2).Int(),
		Col:      frame.Index(3).Int(),
	}
}

func Test_parseCallFrame(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  callFrame
	}{
		{
			name:  "Chrome 96.0.4664.110 on Linux #1",
			input: "at foo (eval at $b (https://gopherjs.github.io/playground/playground.js:102:11836), <anonymous>:25887:60)",
			want:  callFrame{FuncName: "foo", File: "https://gopherjs.github.io/playground/playground.js", Line: 102, Col: 11836},
		},
		{
			name:  "Chrome 96, anonymous eval",
			input: "	at eval (<anonymous>)",
			want:  callFrame{FuncName: "eval", File: "<anonymous>"},
		},
		{
			name:  "Chrome 96, anonymous Array.forEach",
			input: "	at Array.forEach (<anonymous>)",
			want:  callFrame{FuncName: "Array.forEach", File: "<anonymous>"},
		},
		{
			name:  "Chrome 96, file location only",
			input: "at https://ajax.googleapis.com/ajax/libs/angularjs/1.2.18/angular.min.js:31:225",
			want:  callFrame{FuncName: "<none>", File: "https://ajax.googleapis.com/ajax/libs/angularjs/1.2.18/angular.min.js", Line: 31, Col: 225},
		},
		{
			name:  "Chrome 96, aliased function",
			input: "at k.e.$externalizeWrapper.e.$externalizeWrapper [as run] (https://gopherjs.github.io/playground/playground.js:5:30547)",
			want:  callFrame{FuncName: "run", File: "https://gopherjs.github.io/playground/playground.js", Line: 5, Col: 30547},
		},
		{
			name:  "Node.js v12.22.5",
			input: "    at Script.runInThisContext (vm.js:120:18)",
			want:  callFrame{FuncName: "Script.runInThisContext", File: "vm.js", Line: 120, Col: 18},
		},
		{
			name:  "Node.js v12.22.5, aliased function",
			input: "at REPLServer.runBound [as eval] (domain.js:440:12)",
			want:  callFrame{FuncName: "eval", File: "domain.js", Line: 440, Col: 12},
		},
		{
			name:  "Firefox 78.15.0esr Linux",
			input: "getEvalResult@resource://devtools/server/actors/webconsole/eval-with-debugger.js:231:24",
			want:  callFrame{FuncName: "getEvalResult", File: "resource://devtools/server/actors/webconsole/eval-with-debugger.js", Line: 231, Col: 24},
		},
		{
			name:  "Firefox anonymous function",
			input: "@filename.js:10:15",
			want:  callFrame{FuncName: "<none>", File: "filename.js", Line: 10, Col: 15},
		},
		{
			name:  "Firefox no column number",
			input: "foo@bar.js:42",
			want:  callFrame{FuncName: "foo", File: "bar.js", Line: 42},
		},
		{
			name:  "Firefox no line or column",
			input: "foo@bar.js",
			want:  callFrame{FuncName: "foo", File: "bar.js"},
		},
		{
			name:  "Firefox file with colons in path",
			input: "baz@http://example.com/script.js:100:5",
			want:  callFrame{FuncName: "baz", File: "http://example.com/script.js", Line: 100, Col: 5},
		},
		{
			name:  "Firefox file colons in path without a column",
			input: "baz@http://example.com/script.js:100",
			want:  callFrame{FuncName: "baz", File: "http://example.com/script.js", Line: 100},
		},
		{
			name:  "Firefox file colons in path without a line or column",
			input: "baz@http://example.com/script.js",
			want:  callFrame{FuncName: "baz", File: "http://example.com/script.js"},
		},
		{
			name:  "Firefox anonymous function with no line or column",
			input: "@eval",
			want:  callFrame{FuncName: "<none>", File: "eval"},
		},
		{
			name:  "Chrome frame with parens",
			input: "    at Script.runInThisContext (vm.js:120:18)",
			want:  callFrame{FuncName: "Script.runInThisContext", File: "vm.js", Line: 120, Col: 18},
		},
		{
			name:  "Chrome aliased function",
			input: "at REPLServer.runBound [as eval] (domain.js:440:12)",
			want:  callFrame{FuncName: "eval", File: "domain.js", Line: 440, Col: 12},
		},
		{
			name:  "Chrome file location only, no function",
			input: "at https://example.com/angular.min.js:31:225",
			want:  callFrame{FuncName: "<none>", File: "https://example.com/angular.min.js", Line: 31, Col: 225},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseCallFrame(tt.input); got != tt.want {
				t.Errorf("Unexpected result:\n\tgot:  %+v\n\twant: %+v", got, tt.want)
			}
		})
	}
}

func TestBuildPlatform(t *testing.T) {
	if runtime.GOOS != "js" {
		t.Errorf("Got runtime.GOOS=%q. Want: %q.", runtime.GOOS, "js")
	}
	if runtime.GOARCH != "ecmascript" {
		t.Errorf("Got runtime.GOARCH=%q. Want: %q.", runtime.GOARCH, "ecmascript")
	}
}

type funcName string

func masked(_ funcName) funcName { return "<MASKED>" }

type callStack []funcName

func (c *callStack) capture() {
	*c = nil
	pc := [100]uintptr{}
	depth := runtime.Callers(0, pc[:])
	frames := runtime.CallersFrames(pc[:depth])
	for true {
		frame, more := frames.Next()
		*c = append(*c, funcName(frame.Function))
		if !more {
			break
		}
	}
}

func TestCallers(t *testing.T) {
	// Some of the GopherJS function names don't match upstream Go, or even the
	// function names in the Go source when minified.
	// Until https://github.com/gopherjs/gopherjs/issues/1085 is resolved, the
	// mismatch is difficult to avoid, but we can at least use "masked" frames to
	// make sure the number of frames matches expected.
	opts := cmp.Comparer(func(a, b funcName) bool {
		if a == masked("") || b == masked("") {
			return true
		}
		return a == b
	})

	t.Run("Normal", func(t *testing.T) {
		got := callStack{}
		want := callStack{
			"runtime.Callers",
			"github.com/gopherjs/gopherjs/tests.callStack.capture",
			"github.com/gopherjs/gopherjs/tests.TestCallers.func2",
			"testing.tRunner",
			"runtime.goexit",
		}

		got.capture()
		if diff := cmp.Diff(want, got, opts); diff != "" {
			t.Errorf("runtime.Callers() returned a diff (-want,+got):\n%s", diff)
		}
	})

	t.Run("Deferred", func(t *testing.T) {
		got := callStack{}
		want := callStack{
			"runtime.Callers",
			"github.com/gopherjs/gopherjs/tests.callStack.capture",
			// For some reason function epilog where deferred calls are invoked doesn't
			// get source-mapped to the original source properly, which causes node
			// not to map the function name to the original.
			masked("github.com/gopherjs/gopherjs/tests.TestCallers.func3"),
			"testing.tRunner",
			"runtime.goexit",
		}

		defer func() {
			if diff := cmp.Diff(want, got, opts); diff != "" {
				t.Errorf("runtime.Callers() returned a diff (-want,+got):\n%s", diff)
			}
		}()
		defer got.capture()
	})

	t.Run("Recover", func(t *testing.T) {
		got := callStack{}
		defer func() {
			recover()
			got.capture()

			want := callStack{
				"runtime.Callers",
				"github.com/gopherjs/gopherjs/tests.callStack.capture",
				"github.com/gopherjs/gopherjs/tests.TestCallers.func4.func1",
				"runtime.gopanic",
				"github.com/gopherjs/gopherjs/tests.TestCallers.func4",
				"testing.tRunner",
				"runtime.goexit",
			}
			if diff := cmp.Diff(want, got, opts); diff != "" {
				t.Errorf("runtime.Callers() returned a diff (-want,+got):\n%s", diff)
			}
		}()
		panic("panic")
	})
}

// Need this to tunnel into `internal/godebug` and run a test
// without causing a dependency cycle with the `testing` package.
//
//go:linkname godebug_setUpdate runtime.godebug_setUpdate
func godebug_setUpdate(update func(string, string))

func Test_GoDebugInjection(t *testing.T) {
	buf := []string{}
	update := func(def, env string) {
		if def != `` {
			t.Errorf(`Expected the default value to be empty but got %q`, def)
		}
		buf = append(buf, strconv.Quote(env))
	}
	check := func(want string) {
		if got := strings.Join(buf, `, `); got != want {
			t.Errorf(`Unexpected result: got: %q, want: %q`, got, want)
		}
		buf = buf[:0]
	}

	// Call it multiple times to ensure that the watcher is only injected once.
	// Each one of these calls should emit an update first, then when GODEBUG is set.
	godebug_setUpdate(update)
	godebug_setUpdate(update)
	check(`"", ""`) // two empty strings for initial update calls.

	t.Setenv(`GODEBUG`, `gopherJSTest=ben`)
	check(`"gopherJSTest=ben"`) // must only be once for update for new value.

	godebug_setUpdate(update)
	check(`"gopherJSTest=ben"`) // must only be once for initial update with already set value.

	t.Setenv(`GODEBUG`, `gopherJSTest=tom`)
	t.Setenv(`GODEBUG`, `gopherJSTest=sam`)
	t.Setenv(`NOT_GODEBUG`, `gopherJSTest=bob`)
	check(`"gopherJSTest=tom", "gopherJSTest=sam"`)
}

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
// "after" is the ns/op after changing to use "captureStackTrace", moving
// parsing to prelude, and reducing line slices.
//
// | depth |  before |   after | %diff |
// |:-----:|--------:|--------:|------:|
// |   1   |  36,933 |  14,724 | 39.87 |
// |   3   | 116,012 |  42,596 | 36.72 |
// |   5   | 209,388 |  71,881 | 34.33 |
// |   7   | 314,133 | 100,600 | 32.02 |
// |   9   | 422,581 | 127,421 | 30.15 |
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
