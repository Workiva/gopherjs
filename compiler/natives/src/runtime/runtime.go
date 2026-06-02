//go:build js

package runtime

import "github.com/gopherjs/gopherjs/js"

const (
	GOOS     = "js"
	GOARCH   = "ecmascript"
	Compiler = "gopherjs"
)

// The Error interface identifies a run time error.
type Error interface {
	error

	// RuntimeError is a no-op function but
	// serves to distinguish types that are run time
	// errors from ordinary errors: a type is a
	// run time error if it has a RuntimeError method.
	RuntimeError()
}

// TODO(nevkontakte): In the upstream, this struct is meant to be compatible
// with reflect.rtype, but here we use a minimal stub that satisfies the API
// TypeAssertionError expects, which we dynamically instantiate in $assertType().
type _type struct{ str string }

func (t *_type) string() string  { return t.str }
func (t *_type) pkgpath() string { return "" }

// A TypeAssertionError explains a failed type assertion.
type TypeAssertionError struct {
	_interface    *_type
	concrete      *_type
	asserted      *_type
	missingMethod string // one method needed by Interface, missing from Concrete
}

func (*TypeAssertionError) RuntimeError() {}

func (e *TypeAssertionError) Error() string {
	inter := "interface"
	if e._interface != nil {
		inter = e._interface.string()
	}
	as := e.asserted.string()
	if e.concrete == nil {
		return "interface conversion: " + inter + " is nil, not " + as
	}
	cs := e.concrete.string()
	if e.missingMethod == "" {
		msg := "interface conversion: " + inter + " is " + cs + ", not " + as
		if cs == as {
			// provide slightly clearer error message
			if e.concrete.pkgpath() != e.asserted.pkgpath() {
				msg += " (types from different packages)"
			} else {
				msg += " (types from different scopes)"
			}
		}
		return msg
	}
	return "interface conversion: " + cs + " is not " + as +
		": missing method " + e.missingMethod
}

// A PanicNilError happens when code calls panic(nil).
//
// Before Go 1.21, programs that called panic(nil) observed recover returning nil.
// Starting in Go 1.21, programs that call panic(nil) observe recover returning a *PanicNilError.
// Programs can change back to the old behavior by setting GODEBUG=panicnil=1.
type PanicNilError struct {
	// This field makes PanicNilError structurally different from
	// any other struct in this package, and the _ makes it different
	// from any struct in other packages too.
	// This avoids any accidental conversions being possible
	// between this struct and some other struct sharing the same fields,
	// like happened in go.dev/issue/56603.
	_ [0]*PanicNilError
}

func (*PanicNilError) Error() string { return "panic called with nil argument" }
func (*PanicNilError) RuntimeError() {}

func newPanicNilError() *PanicNilError { return new(PanicNilError) }

func init() {
	jsPkg := js.Global.Get("$packages").Get("github.com/gopherjs/gopherjs/js")
	js.Global.Set("$jsObjectPtr", jsPkg.Get("Object").Get("ptr"))
	js.Global.Set("$jsErrorPtr", jsPkg.Get("Error").Get("ptr"))
	js.Global.Set("$throwRuntimeError", js.InternalObject(throw))
	js.Global.Set("$newPanicNilError", js.InternalObject(newPanicNilError))
	buildVersion = js.Global.Get("$goVersion").String()
	// Prepare the prelude's $panicnil flag from GODEBUG at startup.
	syncPanicNilFromGodebug(getEnvString(godebugEnvKey))
	// avoid dead code elimination
	var e error
	e = &TypeAssertionError{}
	_ = e
}

func GOROOT() string {
	process := js.Global.Get("process")
	if process == js.Undefined || process.Get("env") == js.Undefined {
		return "/"
	}
	if v := process.Get("env").Get("GOPHERJS_GOROOT"); v != js.Undefined && v.String() != "" {
		// GopherJS-specific GOROOT value takes precedence.
		return v.String()
	} else if v := process.Get("env").Get("GOROOT"); v != js.Undefined && v.String() != "" {
		return v.String()
	}
	// sys.DefaultGoroot is now gone, can't use it as fallback anymore.
	// TODO: See if a better solution is needed.
	return "/usr/local/go"
}

func Breakpoint() { js.Debugger() }

var (
	// JavaScript runtime doesn't provide access to low-level execution position
	// counters, so we emulate them by recording positions we've encountered in
	// Caller() and Callers() functions and assigning them arbitrary integer values.
	//
	// We use the map and the slice below to convert a "file:line" position
	// into an integer position counter and then to a Func instance.
	//
	// Index 0 is reserved as the "unknown PC": upstream Go documents
	// PC=0 as "no caller available" (see e.g. log/slog.Record.PC), and packages
	// using PCs, initialize a uintptr to zero and expect runtime.CallersFrames
	// and runtime.FuncForPC to treat it as unknown.
	knownPositions   = map[basicFrame]uintptr{}
	positionCounters = []*Func{nil}
)

func registerPosition(frame basicFrame) uintptr {
	if pc, found := knownPositions[frame]; found {
		return pc
	}
	f := &Func{
		name: frame.FuncName,
		file: frame.File,
		line: frame.Line,
	}
	pc := uintptr(len(positionCounters))
	positionCounters = append(positionCounters, f)
	knownPositions[frame] = pc
	return pc
}

// basicFrame contains stack trace information extracted from JS stack trace.
type basicFrame struct {
	FuncName string
	File     string
	Line     int
	Col      int
}

func callstack(skip, limit int) []basicFrame {
	skip += 2 // +1 for callstack's own frame and one for getRawCallStack
	stackTraceLimit := skip + limit + len(knownFrames) + len(hiddenFrames)
	stack := getRawCallstack(stackTraceLimit)
	return parseCallstack(stack, skip, limit)
}

// getRawCallstack gets the stack limited with the `stackTraceLimit`.
// The returned stack will include this function call and any default error header.
func getRawCallstack(stackTraceLimit int) *js.Object {
	e := js.Global.Get("Error")

	// Limit stack to only the size we need then reset it.
	oldLimit := e.Get("stackTraceLimit")
	defer e.Set("stackTraceLimit", oldLimit)
	e.Set("stackTraceLimit", stackTraceLimit)

	if e.Get("captureStackTrace") != js.Undefined {
		target := js.Global.Get("Object").New()
		e.Call("captureStackTrace", target)
		return target.Get("stack")
	}
	return e.New().Get("stack")
}

// lineOffset gets the offset to the first character after the `count`-th newline,
// This returns the start of the `count`-th line in the given js string.
func lineOffset(str *js.Object, count int) int {
	pos := 0
	len := str.Length()
	for i := 0; i < count && pos < len; i++ {
		nl := str.Call("indexOf", "\n", pos).Int()
		if nl < 0 {
			return len
		}
		pos = nl + 1
	}
	return pos
}

var (
	// These functions are GopherJS-specific and don't have counterparts in
	// upstream Go runtime. To improve interoperability, we filter them out from
	// the stack trace.
	hiddenFrames = map[string]bool{
		"$callDeferred": true,
	}
	// The following GopherJS prelude functions have differently-named
	// counterparts in the upstream Go runtime. Some standard library code relies
	// on the names matching, so we perform this substitution.
	knownFrames = map[string]string{
		"$panic":     "runtime.gopanic",
		"$goroutine": "runtime.goexit",
	}
)

func parseCallstack(stack *js.Object, skip, limit int) []basicFrame {
	// If `new Error().stack` doesn't exist like on older IE versions
	// or something went wrong getting the stack, just return empty.
	if stack == js.Undefined {
		return []basicFrame{}
	}

	// Drop any tailing "\n" (and any trailing space before first "at " if there is one).
	stack = stack.Call("trim")

	// V8 prepends an "Error" or "Error: msg" header line that isn't a frame.
	// Firefox and Safari do not. Detect by checking for "@" or starts with "at ".
	// This check could still be wrong if the Error message itself has an "@" in it,
	// (e.g. `new Error("a@b")`), however since we not calling error with an
	// error message, it should be fine.
	firstLine := stack
	if firstNl := stack.Call("indexOf", "\n").Int(); firstNl >= 0 {
		firstLine = stack.Call("substring", 0, firstNl)
	}
	if !firstLine.Call("includes", "@").Bool() && !firstLine.Call("startsWith", "at ").Bool() {
		skip++ // skip "Error" header line.
	}

	// Remove the skipped amount of the stack and the error header if there was one.
	stack = stack.Call("substring", lineOffset(stack, skip))
	if stack.Length() == 0 {
		return []basicFrame{}
	}
	lines := stack.Call("split", "\n")

	// Parse all the frames skipping frames as needed.
	frames := []basicFrame{}
	l := lines.Length()
	for i := 0; i < l && len(frames) < limit; i++ {
		frame := ParseCallFrame(lines.Index(i))
		if hiddenFrames[frame.FuncName] {
			continue
		}
		if alias, ok := knownFrames[frame.FuncName]; ok {
			frame.FuncName = alias
		}
		frames = append(frames, frame)
		if frame.FuncName == "runtime.goexit" {
			break // We've reached the bottom of the goroutine stack.
		}
	}
	return frames
}

// callFramePosRegex will break up a <file>:<line>:<column> pattern where the
// file may also contain colons and the column or line/column are optional.
var callFramePosRegex = js.Global.Get("RegExp").New(`^\s*(.+?)(?::(\d+)(?::(\d+))?)?\s*$`)

func parseCallFramePos(fnName string, pos *js.Object) basicFrame {
	if pos.String() == "<anonymous>" {
		return basicFrame{FuncName: fnName, File: "<anonymous>"}
	}
	file, line, col := ``, 0, 0
	m := callFramePosRegex.Call(`exec`, pos)
	if m != js.Undefined {
		file = m.Index(1).String()
		line = m.Index(2).Int()
		col = m.Index(3).Int()
	}
	return basicFrame{FuncName: fnName, File: file, Line: line, Col: col}
}

// ParseCallFrame is exported for the sake of testing. See this discussion for context https://github.com/gopherjs/gopherjs/pull/1097/files/561e6381406f04ccb8e04ef4effedc5c7887b70f#r776063799
//
// TLDR; never use this function!
func ParseCallFrame(info *js.Object) basicFrame {
	// FireFox
	if atIdx := info.Call("indexOf", "@").Int(); atIdx >= 0 {
		fnName := info.Call("substring", 0, atIdx).String()
		if len(fnName) == 0 {
			fnName = "<none>"
		}
		return parseCallFramePos(fnName, info.Call("substring", atIdx+1))
	}

	// Chrome / Node.js
	if atLeadIdx := info.Call("indexOf", "at ").Int(); atLeadIdx >= 0 {
		info = info.Call("substring", atLeadIdx+3)
	}
	openIdx := info.Call("lastIndexOf", "(").Int()
	if openIdx == -1 {
		// No-parens form: "at file:line:col"
		return parseCallFramePos("<none>", info)
	}

	// With-parens form: "at func (file:line:col)"
	pos := info.Call("substring", openIdx+1, info.Call("indexOf", ")").Int())
	fn := info.Call("substring", 0, info.Call("indexOf", "(").Int()).Call("trim")
	if idx := fn.Call("indexOf", "[as ").Int(); idx > 0 {
		closeIdx := fn.Call("indexOf", "]").Int()
		if closeIdx < 0 {
			closeIdx = fn.Length()
		}
		fn = fn.Call("substring", idx+4, closeIdx).Call("trim")
	}
	return parseCallFramePos(fn.String(), pos)
}

func Caller(skip int) (pc uintptr, file string, line int, ok bool) {
	skip = skip + 1 /*skip Caller's own frame*/
	frames := callstack(skip, 1)
	if len(frames) != 1 {
		return 0, "", 0, false
	}
	frame := frames[0]
	pc = registerPosition(frame)
	return pc, frame.File, frame.Line, true
}

// Callers fills the slice pc with the return program counters of function
// invocations on the calling goroutine's stack. The argument skip is the number
// of stack frames to skip before recording in pc, with 0 identifying the frame
// for Callers itself and 1 identifying the caller of Callers. It returns the
// number of entries written to pc.
//
// The returned call stack represents the logical Go call stack, which excludes
// certain runtime-internal call frames that would be present in the raw
// JavaScript stack trace. This is done to improve interoperability with the
// upstream Go. Use JavaScript native APIs to access the raw call stack.
//
// To translate these PCs into symbolic information such as function names and
// line numbers, use CallersFrames. CallersFrames accounts for inlined functions
// and adjusts the return program counters into call program counters. Iterating
// over the returned slice of PCs directly is discouraged, as is using FuncForPC
// on any of the returned PCs, since these cannot account for inlining or return
// program counter adjustment.
func Callers(skip int, pc []uintptr) int {
	frames := callstack(skip, len(pc))
	for i, frame := range frames {
		pc[i] = registerPosition(frame)
	}
	return len(frames)
}

// CallersFrames takes a slice of PC values returned by Callers and prepares to
// return function/file/line information. Done is true when no more frames are
// available.
//
// GopherJS notes:
//   - PCs that didn't come from Caller/Callers (e.g. a function pointer obtained
//     via reflect.ValueOf(fn).Pointer()) are not in our positionCounters.
//     For those, FuncForPC returns nil and we emit a Frame with the original PC
//     and empty symbol fields, like Go will.
//   - GopherJS's internal frames such as $callDeferred and $goroutine were
//     already filtered (or aliased) by callstack at capture time, so anything
//     reaching CallersFrames is either a real Go frame or a PC we can't resolve.
func CallersFrames(callers []uintptr) *Frames {
	result := Frames{}
	for _, pc := range callers {
		fun := FuncForPC(pc)
		if fun == nil {
			result.frames = append(result.frames, Frame{PC: pc})
			continue
		}
		result.frames = append(result.frames, Frame{
			PC:       pc,
			Func:     fun,
			Function: fun.name,
			File:     fun.file,
			Line:     fun.line,
			Entry:    fun.Entry(),
		})
	}
	return &result
}

type Frames struct {
	frames  []Frame
	current int
}

func (ci *Frames) Next() (frame Frame, more bool) {
	if ci.current >= len(ci.frames) {
		return Frame{}, false
	}
	f := ci.frames[ci.current]
	ci.current++
	return f, ci.current < len(ci.frames)
}

type Frame struct {
	PC       uintptr
	Func     *Func
	Function string
	File     string
	Line     int
	Entry    uintptr
}

func GC() {}

func Goexit() {
	js.Global.Get("$curGoroutine").Set("exit", true)
	js.Global.Call("$throw", nil)
}

func GOMAXPROCS(int) int { return 1 }

func Gosched() {
	c := make(chan struct{})
	js.Global.Call("$setTimeout", js.InternalObject(func() { close(c) }), 0)
	<-c
}

func NumCPU() int { return 1 }

func NumGoroutine() int {
	return js.Global.Get("$totalGoroutines").Int()
}

type MemStats struct {
	// General statistics.
	Alloc      uint64 // bytes allocated and still in use
	TotalAlloc uint64 // bytes allocated (even if freed)
	Sys        uint64 // bytes obtained from system (sum of XxxSys below)
	Lookups    uint64 // number of pointer lookups
	Mallocs    uint64 // number of mallocs
	Frees      uint64 // number of frees

	// Main allocation heap statistics.
	HeapAlloc    uint64 // bytes allocated and still in use
	HeapSys      uint64 // bytes obtained from system
	HeapIdle     uint64 // bytes in idle spans
	HeapInuse    uint64 // bytes in non-idle span
	HeapReleased uint64 // bytes released to the OS
	HeapObjects  uint64 // total number of allocated objects

	// Low-level fixed-size structure allocator statistics.
	//	Inuse is bytes used now.
	//	Sys is bytes obtained from system.
	StackInuse  uint64 // bytes used by stack allocator
	StackSys    uint64
	MSpanInuse  uint64 // mspan structures
	MSpanSys    uint64
	MCacheInuse uint64 // mcache structures
	MCacheSys   uint64
	BuckHashSys uint64 // profiling bucket hash table
	GCSys       uint64 // GC metadata
	OtherSys    uint64 // other system allocations

	// Garbage collector statistics.
	NextGC        uint64 // next collection will happen when HeapAlloc ≥ this amount
	LastGC        uint64 // end time of last collection (nanoseconds since 1970)
	PauseTotalNs  uint64
	PauseNs       [256]uint64 // circular buffer of recent GC pause durations, most recent at [(NumGC+255)%256]
	PauseEnd      [256]uint64 // circular buffer of recent GC pause end times
	NumGC         uint32
	GCCPUFraction float64 // fraction of CPU time used by GC
	EnableGC      bool
	DebugGC       bool

	// Per-size allocation statistics.
	// 61 is NumSizeClasses in the C code.
	BySize [61]struct {
		Size    uint32
		Mallocs uint64
		Frees   uint64
	}
}

func ReadMemStats(m *MemStats) {
	// TODO(nevkontakte): This function is effectively unimplemented and may
	// lead to silent unexpected behaviors. Consider panicing explicitly.
}

func SetFinalizer(x, f any) {
	// TODO(nevkontakte): This function is effectively unimplemented and may
	// lead to silent unexpected behaviors. Consider panicing explicitly.
}

type Func struct {
	name string
	file string
	line int

	opaque struct{} // unexported field to disallow conversions
}

func (_ *Func) Entry() uintptr { return 0 }

func (f *Func) FileLine(pc uintptr) (file string, line int) {
	if f == nil {
		return "", 0
	}
	return f.file, f.line
}

func (f *Func) Name() string {
	if f == nil || f.name == "" {
		return "<unknown>"
	}
	return f.name
}

func FuncForPC(pc uintptr) *Func {
	ipc := int(pc)
	if ipc <= 0 || ipc >= len(positionCounters) {
		// Since we are faking position counters, the only valid way to obtain one
		// is through a Caller() or Callers() function. If pc is out of positionCounters
		// bounds it must have been obtained in some other way, which is unexpected.
		// FuncForPC in Go returns nil for a PC that does not correspond to a
		// known function. so returning nil for PCs we cannot resolvable, even if
		// Go could resolve it, lets the callers keep working with empty symbols.
		// For example:
		//   - log/slog passes PC=0 through CallersFrames when a Record was
		//     created without a real caller (record.go's PC field)
		//   - test/fixedbugs/issue29735.go deliberately walks past the
		//     end of a function looking for the next. This will cause it to
		//     not succeed at what it is trying to do but will allow the test to pass.
		//   - test/fixedbugs/issue58300.go and test/fixedbugs/issue58300b.go give
		//     FuncForPC a function pointer from `reflect.ValueOf(fn).Pointer()`,
		//     which is not produced by Caller/Callers and so isn't in our table.
		return nil
	}
	return positionCounters[ipc]
}

var MemProfileRate int = 512 * 1024

func SetBlockProfileRate(rate int) {
}

func SetMutexProfileFraction(rate int) int {
	// TODO: Investigate this. If it's possible to implement, consider doing so, otherwise remove this comment.
	return 0
}

// Stack formats a stack trace of the calling goroutine into buf and returns the
// number of bytes written to buf. If all is true, Stack formats stack traces of
// all other goroutines into buf after the trace for the current goroutine.
//
// Unlike runtime.Callers(), it returns an unprocessed, runtime-specific text
// representation of the JavaScript stack trace.
func Stack(buf []byte, all bool) int {
	s := js.Global.Get("Error").New().Get("stack")
	if s == js.Undefined {
		return 0
	}
	return copy(buf, s.Call("substring", s.Call("indexOf", "\n").Int()+1).String())
}

func LockOSThread() {}

func UnlockOSThread() {}

var buildVersion string // Set by init()

func Version() string {
	return buildVersion
}

func StartTrace() error { return nil }
func StopTrace()        {}
func ReadTrace() []byte

// We fake a cgo environment to catch errors. Therefore we have to implement this and always return 0
func NumCgoCall() int64 {
	return 0
}

func KeepAlive(any) {}

// An errorString represents a runtime error described by a single string.
type errorString string

func (e errorString) RuntimeError() {}

func (e errorString) Error() string {
	return "runtime error: " + string(e)
}

func throw(s string) {
	panic(errorString(s))
}

func nanotime() int64 {
	const millisecond = 1_000_000
	return js.Global.Get("Date").New().Call("getTime").Int64() * millisecond
}

const godebugEnvKey = `GODEBUG`

var godebugUpdate func(def, env string)

// godebug_setUpdate implements the setUpdate in src/internal/godebug/godebug.go
func godebug_setUpdate(update func(def, env string)) {
	godebugUpdate = update
	godebugEnv := getEnvString(godebugEnvKey)
	godebug_notify(godebugEnvKey, godebugEnv)
}

// godebug_setNewIncNonDefault implements the setNewIncNonDefault in
// src/internal/godebug/godebug.go.
// GOPHERJS: The GopherJS runtime doesn't need this function so we can remove it.
//
//gopherjs:purge
func godebug_setNewIncNonDefault(newIncNonDefault func(string) func())

func getEnvString(key string) string {
	process := js.Global.Get(`process`)
	if process == js.Undefined {
		return ``
	}

	env := process.Get(`env`)
	if env == js.Undefined {
		return ``
	}

	value := env.Get(key)
	if value == js.Undefined {
		return ``
	}

	return value.String()
}

// godebug_notify is the function is called by syscall anytime an environment
// variable is set or unset. It emits the GODEBUG setting if it was changed.
func godebug_notify(key, value string) {
	if key != godebugEnvKey {
		return
	}

	if update := godebugUpdate; update != nil {
		godebugDefault := ``
		update(godebugDefault, value)
	}

	// Keep the prelude's $panicnil flag in sync with GODEBUG even when the
	// program never imported internal/godebug in which case `update` is nil.
	syncPanicNilFromGodebug(value)
}

// syncPanicNilFromGodebug parses the given GODEBUG value for `panicnil=N`
// and writes the result into the prelude's `$panicnil` value.
// $panicnil mirrors the runtime's `debug.panicnil` GODEBUG setting.
// When set to "1" (i.e. `GODEBUG=panicnil=1`), `panic(nil)` keeps the pre-go1.21
// behavior of being recoverable as a real `nil`.
// When "0" (the go1.21+ default), `panic(nil)` is wrapped into a
// `*runtime.PanicNilError` so `recover()` returns a non-nil error.
func syncPanicNilFromGodebug(godebug string) {
	panicnil := `0`
	if godebug != `` {
		// Parse on the JS side to avoid pulling `strings` into the runtime package.
		// The regex matches `panicnil=<digit>` in the comma-separated GODEBUG value.
		re := js.Global.Get(`RegExp`).New(`(?:^|,)panicnil=(\d+)(?:,|$)`)
		m := re.Call(`exec`, godebug)
		if m != nil && m != js.Undefined && m.Length() >= 2 {
			panicnil = m.Index(1).String()
		}
	}
	js.Global.Set("$panicnil", panicnil)
}
