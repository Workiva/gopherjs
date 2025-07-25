package tests

import (
	"go/token"
	"math"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/gopherjs/gopherjs/tests/otherpkg"
)

func TestSyntax1(t *testing.T) {
	a := 42
	if *&*&a != 42 {
		t.Fail()
	}
}

func TestPointerEquality(t *testing.T) {
	a := 1
	b := 1
	if &a != &a || &a == &b {
		t.Fail()
	}
	m := make(map[*int]int)
	m[&a] = 2
	m[&b] = 3
	if m[&a] != 2 || m[&b] != 3 {
		t.Fail()
	}

	for {
		c := 1
		d := 1
		if &c != &c || &c == &d {
			t.Fail()
		}
		break
	}

	s := struct {
		e int
		f int
	}{1, 1}
	if &s.e != &s.e || &s.e == &s.f {
		t.Fail()
	}

	g := [3]int{1, 2, 3}
	if &g[0] != &g[0] || &g[:][0] != &g[0] || &g[:][0] != &g[:][0] {
		t.Fail()
	}
}

type SingleValue struct {
	Value uint16
}

type OtherSingleValue struct {
	Value uint16
}

func TestStructKey(t *testing.T) {
	m := make(map[SingleValue]int)
	m[SingleValue{Value: 1}] = 42
	m[SingleValue{Value: 2}] = 43
	if m[SingleValue{Value: 1}] != 42 || m[SingleValue{Value: 2}] != 43 || reflect.ValueOf(m).MapIndex(reflect.ValueOf(SingleValue{Value: 1})).Interface() != 42 {
		t.Fail()
	}

	m2 := make(map[interface{}]int)
	m2[SingleValue{Value: 1}] = 42
	m2[SingleValue{Value: 2}] = 43
	m2[OtherSingleValue{Value: 1}] = 44
	if m2[SingleValue{Value: 1}] != 42 || m2[SingleValue{Value: 2}] != 43 || m2[OtherSingleValue{Value: 1}] != 44 || reflect.ValueOf(m2).MapIndex(reflect.ValueOf(SingleValue{Value: 1})).Interface() != 42 {
		t.Fail()
	}
}

func TestSelectOnNilChan(t *testing.T) {
	var c1 chan bool
	c2 := make(chan bool)

	go func() {
		close(c2)
	}()

	select {
	case <-c1:
		t.Fail()
	case <-c2:
		// ok
	}
}

type StructA struct {
	x int
}

type StructB struct {
	StructA
}

func TestEmbeddedStruct(t *testing.T) {
	a := StructA{
		42,
	}
	b := StructB{
		StructA: a,
	}
	b.x = 0
	if a.x != 42 {
		t.Fail()
	}
}

func TestMapStruct(t *testing.T) {
	a := StructA{
		42,
	}
	m := map[int]StructA{
		1: a,
	}
	m[2] = a
	a.x = 0
	if m[1].x != 42 || m[2].x != 42 {
		t.Fail()
	}
}

func TestUnnamedParameters(t *testing.T) {
	ok := false
	defer func() {
		if !ok {
			t.Fail()
		}
	}()
	blockingWithUnnamedParameter(false) // used to cause non-blocking call error, which is ignored by testing
	ok = true
}

func blockingWithUnnamedParameter(bool) {
	c := make(chan int, 1)
	c <- 42
}

func TestGotoLoop(t *testing.T) {
	goto loop
loop:
	for i := 42; ; {
		if i != 42 {
			t.Fail()
		}
		break
	}
}

func TestMaxUint64(t *testing.T) {
	if math.MaxUint64 != 18446744073709551615 {
		t.Fail()
	}
}

func TestCopyBuiltin(t *testing.T) {
	{
		s := []string{"a", "b", "c"}
		copy(s, s[1:])
		if s[0] != "b" || s[1] != "c" || s[2] != "c" {
			t.Fail()
		}
	}
	{
		s := []string{"a", "b", "c"}
		copy(s[1:], s)
		if s[0] != "a" || s[1] != "a" || s[2] != "b" {
			t.Fail()
		}
	}
}

func TestPointerOfStructConversion(t *testing.T) {
	type A struct {
		Value int
	}

	type B A

	type AP *A

	a1 := &A{Value: 1}
	b1 := (*B)(a1)
	b1.Value = 2
	a2 := (*A)(b1)
	a2.Value = 3
	b2 := (*B)(a2)
	b2.Value = 4
	if a1 != a2 || b1 != b2 || a1.Value != 4 || a2.Value != 4 || b1.Value != 4 || b2.Value != 4 {
		t.Fail()
	}

	if got := reflect.TypeOf((AP)(&A{Value: 1})); got.String() != "tests.AP" {
		t.Errorf("Got: reflect.TypeOf((AP)(&A{Value: 1})) = %v. Want: tests.AP.", got)
	}
}

func TestCompareStruct(t *testing.T) {
	type A struct {
		Value int
	}

	a := A{42}
	var b interface{} = a
	x := A{0}

	if a != b || a == x || b == x {
		t.Fail()
	}
}

func TestLoopClosure(t *testing.T) {
	type S struct{ fn func() int }
	var fns []*S
	for i := 0; i < 2; i++ {
		z := i
		fns = append(fns, &S{
			fn: func() int {
				return z
			},
		})
	}
	for i, f := range fns {
		if f.fn() != i {
			t.Fail()
		}
	}
}

func TestLoopClosureWithStruct(t *testing.T) {
	type T struct{ A int }
	ts := []T{{0}, {1}, {2}}
	fns := make([]func() T, 3)
	for i, t := range ts {
		t := t
		fns[i] = func() T {
			return t
		}
	}
	for i := range fns {
		if fns[i]().A != i {
			t.Fail()
		}
	}
}

func TestNilInterfaceError(t *testing.T) {
	defer func() {
		if err := recover(); err == nil || !strings.Contains(err.(error).Error(), "nil pointer dereference") {
			t.Fail()
		}
	}()
	var err error
	_ = err.Error()
}

func TestIndexOutOfRangeError(t *testing.T) {
	defer func() {
		if err := recover(); err == nil || !strings.Contains(err.(error).Error(), "index out of range") {
			t.Fail()
		}
	}()
	x := []int{1, 2, 3}[10]
	_ = x
}

func TestNilAtLhs(t *testing.T) {
	type F func(string) string
	var f F
	if nil != f {
		t.Fail()
	}
}

func TestZeroResultByPanic(t *testing.T) {
	if zero() != 0 {
		t.Fail()
	}
}

func zero() int {
	defer func() {
		recover()
	}()
	panic("")
}

func TestNumGoroutine(t *testing.T) {
	n := runtime.NumGoroutine()
	c := make(chan bool)
	go func() {
		<-c
		<-c
		<-c
		<-c
	}()
	c <- true
	c <- true
	c <- true
	if got, want := runtime.NumGoroutine(), n+1; got != want {
		t.Errorf("runtime.NumGoroutine(): Got %d, want %d.", got, want)
	}
	c <- true
}

func TestMapAssign(t *testing.T) {
	x := 0
	m := map[string]string{}
	x, m["foo"] = 5, "bar"
	if x != 5 || m["foo"] != "bar" {
		t.Fail()
	}
}

func TestSwitchStatement(t *testing.T) {
	zero := 0
	var interfaceZero interface{} = zero
	switch {
	case interfaceZero:
		t.Fail()
	default:
		// ok
	}
}

func TestAddAssignOnPackageVar(t *testing.T) {
	otherpkg.Test = 0
	otherpkg.Test += 42
	if otherpkg.Test != 42 {
		t.Fail()
	}
}

func TestPointerOfPackageVar(t *testing.T) {
	otherpkg.Test = 42
	p := &otherpkg.Test
	if *p != 42 {
		t.Fail()
	}
}

func TestFuncInSelect(t *testing.T) {
	f := func(_ func()) chan int {
		return make(chan int, 1)
	}
	select {
	case <-f(func() {}):
	case _ = <-f(func() {}):
	case f(func() {}) <- 42:
	}
}

func TestEscapeAnalysisOnForLoopVariableScope(t *testing.T) {
	for i := 0; ; {
		p := &i
		time.Sleep(0)
		i = 42
		if *p != 42 {
			t.Fail()
		}
		break
	}
}

func TestGoStmtWithStructArg(t *testing.T) {
	type S struct {
		i int
	}

	f := func(s S, c chan int) {
		c <- s.i
		c <- s.i
	}

	c := make(chan int)
	s := S{42}
	go f(s, c)
	s.i = 0
	if <-c != 42 {
		t.Fail()
	}
	if <-c != 42 {
		t.Fail()
	}
}

type methodExprCallType int

func (i methodExprCallType) test() int {
	return int(i) + 2
}

func TestMethodExprCall(t *testing.T) {
	if methodExprCallType.test(40) != 42 {
		t.Fail()
	}
}

func TestCopyOnSend(t *testing.T) {
	type S struct{ i int }
	c := make(chan S, 2)
	go func() {
		var s S
		s.i = 42
		c <- s
		select {
		case c <- s:
		}
		s.i = 10
	}()
	if (<-c).i != 42 {
		t.Fail()
	}
	if (<-c).i != 42 {
		t.Fail()
	}
}

func TestEmptySelectCase(t *testing.T) {
	ch := make(chan int, 1)
	ch <- 42

	v := 0
	select {
	case v = <-ch:
	}
	if v != 42 {
		t.Fail()
	}
}

var (
	a int
	b int
	C int
	D int
)

var (
	a1 = &a
	a2 = &a
	b1 = &b
	C1 = &C
	C2 = &C
	D1 = &D
)

func TestPkgVarPointers(t *testing.T) {
	if a1 != a2 || a1 == b1 || C1 != C2 || C1 == D1 {
		t.Fail()
	}
}

func TestStringMap(t *testing.T) {
	m := make(map[string]interface{})
	if m["__proto__"] != nil {
		t.Fail()
	}
	m["__proto__"] = 42
	if m["__proto__"] != 42 {
		t.Fail()
	}
}

type Int int

func (i Int) Value() int {
	return int(i)
}

func (i *Int) ValueByPtr() int {
	return int(*i)
}

func TestWrappedTypeMethod(t *testing.T) {
	i := Int(42)
	p := &i
	if p.Value() != 42 {
		t.Fail()
	}
}

type EmbeddedInt struct {
	Int
}

func TestEmbeddedMethod(t *testing.T) {
	e := EmbeddedInt{42}
	if e.ValueByPtr() != 42 {
		t.Fail()
	}
}

func TestBoolConvert(t *testing.T) {
	if !reflect.ValueOf(true).Convert(reflect.TypeOf(true)).Bool() {
		t.Fail()
	}
}

func TestGoexit(t *testing.T) {
	go func() {
		runtime.Goexit()
	}()
}

func TestShift(t *testing.T) {
	if x := uint(32); uint32(1)<<x != 0 {
		t.Fail()
	}
	if x := uint64(0); uint32(1)<<x != 1 {
		t.Fail()
	}
	if x := uint(4294967295); x>>32 != 0 {
		t.Fail()
	}
	if x := uint(4294967295); x>>35 != 0 {
		t.Fail()
	}
}

func TestTrivialSwitch(t *testing.T) {
	for {
		switch {
		default:
			break
		}
		return
	}
	t.Fail() //nolint:govet // unreachable code intentional for test
}

func TestTupleFnReturnImplicitCast(t *testing.T) {
	var ycalled int = 0
	x := func(fn func() (int, error)) (interface{}, error) {
		return fn()
	}
	y, _ := x(func() (int, error) {
		ycalled++
		return 14, nil
	})
	if y != 14 || ycalled != 1 {
		t.Fail()
	}
}

var tuple2called = 0

func tuple1() (interface{}, error) {
	return tuple2()
}

func tuple2() (int, error) {
	tuple2called++
	return 14, nil
}

func TestTupleReturnImplicitCast(t *testing.T) {
	x, _ := tuple1()
	if x != 14 || tuple2called != 1 {
		t.Fail()
	}
}

func TestDeferNamedTupleReturnImplicitCast(t *testing.T) {
	var ycalled int = 0
	var zcalled int = 0
	z := func() {
		zcalled++
	}
	x := func(fn func() (int, error)) (i interface{}, e error) {
		defer z()
		i, e = fn()
		return
	}
	y, _ := x(func() (int, error) {
		ycalled++
		return 14, nil
	})
	if y != 14 || ycalled != 1 || zcalled != 1 {
		t.Fail()
	}
}

func TestSliceOfString(t *testing.T) {
	defer func() {
		if err := recover(); err == nil || !strings.Contains(err.(error).Error(), "slice bounds out of range") {
			t.Fail()
		}
	}()

	str := "foo"
	print(str[0:10])
}

func TestSliceOutOfRange(t *testing.T) {
	defer func() {
		if err := recover(); err == nil || !strings.Contains(err.(error).Error(), "slice bounds out of range") {
			t.Fail()
		}
	}()

	a := make([]byte, 4)
	b := a[8:]
	_ = b
}

type R struct{ v int }

func (r R) Val() int {
	return r.v
}

func TestReceiverCapture(t *testing.T) {
	r := R{1}
	f1 := r.Val
	r = R{2}
	f2 := r.Val
	if f1() != 1 || f2() != 2 {
		t.Fail()
	}
}

func TestTypeConversion(t *testing.T) {
	i1, i2, i3 := 4, 2, 2
	if (i1-i2)/i3 != int(i1-i2)/int(i3) {
		t.Fail()
	}
	f1, f2, f3 := 4.0, 2.0, 2.0
	if (f1-f2)/f3 != float64(f1-f2)/float64(f3) {
		t.Fail()
	}
}

// See https://github.com/gopherjs/gopherjs/issues/851.
func TestSlicingNilSlice(t *testing.T) {
	t.Run("StaysNil", func(t *testing.T) {
		var s []int
		s = s[:]
		if s != nil {
			t.Errorf("nil slice became non-nil after slicing with s[:]: %#v, want []int(nil)", s)
		}
		s = nil
		s = s[0:0]
		if s != nil {
			t.Errorf("nil slice became non-nil after slicing with s[0:0]: %#v, want []int(nil)", s)
		}
		s = nil
		s = s[0:0:0]
		if s != nil {
			t.Errorf("nil slice became non-nil after slicing with s[0:0:0]: %#v, want []int(nil)", s)
		}
	})
	t.Run("Panics", func(t *testing.T) {
		defer func() {
			if err := recover(); err == nil || !strings.Contains(err.(error).Error(), "slice bounds out of range") {
				t.Error("slicing nil slice out of range didn't panic, want panic")
			}
		}()
		var s []int
		s = s[5:10]
	})
	t.Run("DoesNotBecomeNil", func(t *testing.T) {
		s := []int{}
		s = s[:]
		if s == nil {
			t.Errorf("non-nil slice became nil after slicing: %#v, want []int{}", s)
		}
	})
}

func TestConvertingNilSlice(t *testing.T) {
	type mySlice []byte

	a := []byte(nil)
	if a != nil {
		t.Errorf("[]byte(nil) != nil")
	}

	b := mySlice(a)
	if b != nil {
		t.Errorf("mySlice([]byte(nil)) != nil")
	}

	c := mySlice(nil)
	if c != nil {
		t.Errorf("mySlice(nil) != nil")
	}
}

// Ensure that doing an interface conversion that fails
// produces an expected error type with the right error text.
func TestInterfaceConversionRuntimeError(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("got no panic, want panic")
		}
		re, ok := r.(runtime.Error)
		if !ok {
			t.Fatalf("got %T (%s), want runtime.Error", r, r)
		}
		if got, want := re.Error(), "interface conversion: int is not tests.I: missing method Get"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}()
	type I interface {
		Get() int
	}
	e := (interface{})(0)
	_ = e.(I)
}

func TestReflectMapIterationAndDelete(t *testing.T) {
	m := map[string]int{
		"one":   1,
		"two":   2,
		"three": 3,
	}
	iter := reflect.ValueOf(m).MapRange()
	for iter.Next() {
		delete(m, iter.Key().String())
	}
	if got, want := len(m), 0; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func TestUntypedNil(t *testing.T) {
	// This test makes sure GopherJS compiler is able to correctly infer the
	// desired type of an untyped nil ident.
	// See https://github.com/gopherjs/gopherjs/issues/1011 for details.

	// Code below is based on test cases from https://golang.org/cl/284052.
	var _ *int = nil
	var _ func() = nil
	var _ []byte = nil
	var _ map[int]int = nil
	var _ chan int = nil
	var _ interface{} = nil

	{
		var (
			x *int = nil
			_      = x
		)
	}
	{
		var (
			x func() = nil
			_        = x
		)
	}
	{
		var (
			x []byte = nil
			_        = x
		)
	}
	{
		var (
			x map[int]int = nil
			_             = x
		)
	}
	{
		var (
			x chan int = nil
			_          = x
		)
	}
	{
		var (
			x interface{} = nil
			_             = x
		)
	}

	{
		var (
			x *int
			_ = x == nil
		)
	}
	{
		var (
			x func()
			_ = x == nil
		)
	}
	{
		var (
			x []byte
			_ = x == nil
		)
	}
	{
		var (
			x map[int]int
			_ = x == nil
		)
	}
	{
		var (
			x chan int
			_ = x == nil
		)
	}
	{
		var (
			x interface{}
			_ = x == nil
		)
	}
	_ = (*int)(nil)
	_ = (func())(nil)
	_ = ([]byte)(nil)
	_ = (map[int]int)(nil)
	_ = (chan int)(nil)
	_ = (interface{})(nil)
	{
		f := func(*int) {}
		f(nil)
	}
	{
		f := func(func()) {}
		f(nil)
	}
	{
		f := func([]byte) {}
		f(nil)
	}
	{
		f := func(map[int]int) {}
		f(nil)
	}
	{
		f := func(chan int) {}
		f(nil)
	}
	{
		f := func(interface{}) {}
		f(nil)
	}
	{
		f := func(*int) {}
		f(nil)
	}
	{
		f := func(*int) {}
		f(nil)
	}
	{
		f := func(*int) {}
		f(nil)
	}
	{
		f := func(*int) {}
		f(nil)
	}
}

func TestVersion(t *testing.T) {
	if got := runtime.Version(); !strings.HasPrefix(got, "go1.") {
		t.Fatalf("Got: runtime.Version() returned %q. Want: a valid Go version.", got)
	}
}

// https://github.com/gopherjs/gopherjs/issues/1163
func TestReflectSetForEmbed(t *testing.T) {
	type Point struct {
		x int
		y int
	}
	type Embed struct {
		value bool
		point Point
	}
	type A struct {
		Embed
	}
	c := &A{}
	c.value = true
	c.point = Point{100, 200}
	in := reflect.ValueOf(c).Elem()
	v := reflect.New(in.Type())
	e := v.Elem()
	f0 := e.Field(0)
	e.Set(in)
	if e.Field(0) != f0 {
		t.Fatalf("reflect.Set got %v, want %v", f0, e.Field(0))
	}
}

func TestAssignImplicitConversion(t *testing.T) {
	type S struct{}
	type SP *S

	t.Run("Pointer to named type", func(t *testing.T) {
		var sp SP = &S{}
		if got := reflect.TypeOf(sp); got.String() != "tests.SP" {
			t.Errorf("Got: reflect.TypeOf(sp) = %v. Want: tests.SP", got)
		}
	})

	t.Run("Anonymous struct to named type", func(t *testing.T) {
		var s S = struct{}{}
		if got := reflect.TypeOf(s); got.String() != "tests.S" {
			t.Errorf("Got: reflect.TypeOf(s) = %v. Want: tests.S", got)
		}
	})

	t.Run("Named type to anonymous type", func(t *testing.T) {
		var x struct{} = S{}
		if got := reflect.TypeOf(x); got.String() != "struct {}" {
			t.Errorf("Got: reflect.TypeOf(x) = %v. Want: struct {}", got)
		}
	})
}

func TestCompositeLiterals(t *testing.T) {
	type S struct{}
	type SP *S

	s1 := []*S{{}}
	if got := reflect.TypeOf(s1[0]); got.String() != "*tests.S" {
		t.Errorf("Got: reflect.TypeOf(s1[0]) = %v. Want: *tests.S", got)
	}

	s2 := []SP{{}}
	if got := reflect.TypeOf(s2[0]); got.String() != "tests.SP" {
		t.Errorf("Got: reflect.TypeOf(s2[0]) = %v. Want: tests.SP", got)
	}
}

func TestFileSetSize(t *testing.T) {
	type tokenFileSet struct {
		// This type remained essentially consistent from go1.16 to go1.21.
		mutex sync.RWMutex
		base  int
		files []*token.File
		_     *token.File // changed to atomic.Pointer[token.File] in go1.19
	}
	n1 := unsafe.Sizeof(tokenFileSet{})
	n2 := unsafe.Sizeof(token.FileSet{})
	if n1 != n2 {
		t.Errorf("Got: unsafe.Sizeof(token.FileSet{}) %v, Want: %v", n2, n1)
	}
}

// TestCrossPackageGenericFuncCalls ensures that generic functions from other
// packages can be called correctly.
func TestCrossPackageGenericFuncCalls(t *testing.T) {
	var wantInt int
	if got := otherpkg.Zero[int](); got != wantInt {
		t.Errorf(`Got: otherpkg.Zero[int]() = %v, Want: %v`, got, wantInt)
	}

	var wantStr string
	if got := otherpkg.Zero[string](); got != wantStr {
		t.Errorf(`Got: otherpkg.Zero[string]() = %q, Want: %q`, got, wantStr)
	}
}

// TestCrossPackageGenericCasting ensures that generic types from other
// packages can be used in a type cast.
// The cast looks like a function call but should be treated as a type conversion.
func TestCrossPackageGenericCasting(t *testing.T) {
	fn := otherpkg.GetterHandle[int](otherpkg.Zero[int])
	var wantInt int
	if got := fn(); got != wantInt {
		t.Errorf(`Got: otherpkg.GetterHandle[int](otherpkg.Zero[int]) = %v, Want: %v`, got, wantInt)
	}
}
