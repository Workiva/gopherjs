// This test is based on https://github.com/gopherjs/gopherjs/issues/757
// The code was failing because the JS would fail on the shadowed method, `Do`.
// Go says that a struct field name shadows a method being promoted from an
// embedded type. This test checks the shadowing of methods as described in 757
// and a bunch other types of shadowing to ensure we are handling shadowing
// correctly, for example we also check if two embedded methods have ambiguous
// method names making Go unable to select either.
package main

import "fmt"

type Doer interface{ Do() string }

type Embedded struct{}

func (e *Embedded) Do() string { return `Do` }

type EmbedAnother struct{}

func (e *EmbedAnother) Do() string { return `Do it again` }

type ValEmbed struct{}

func (ValEmbed) Do() string { return `Do value` }

// `Container1.Do` shadows `Embedded.Do`.
type Container1 struct {
	Embedded
	Do string
}

// `Container2.Do` shadows `Embedded.Do`.
type Container2 struct {
	*Embedded
	Do string
}

// `Embedded.Do` is callable from `*Container3` but not `*Container3.Embedded`
// since `*Container3.Do` calls `Do` with `*Embedded` and `Do` can not be called
// with a non-pointer to embedded.
type Container3 struct{ Embedded }

// This complements `Container3` to check that `Container4.Do` can be called
// and `Container4.Embedded.Do` can also be called.
type Container4 struct{ *Embedded }

// `Container5.Do` shadows `Embedded.Do`.
type Container5 struct{ *Embedded }

func (e *Container5) Do() string { return `Try to do` }

// `Container6.Do` shadows the interface's method `Doer.Do`.
type Container6 struct {
	Doer
	Do string
}

// `Container7.Embedded.Do` and `Container7.EmbedAnother.Do` are ambiguous
// so `Container7.Do` can not be called.
type Container7 struct {
	*Embedded
	*EmbedAnother
}

// Similar `Container7` but checks the ambiguity goes away when the methods
// are not in contention at the same level of embedding.
// `Container8.EmbedHolder.EmbedAnother.Do` is "deeper" than `Container8.Embedded.Do`
// so `Container8.Embedded.Do` is called with `Container.Do`, even through
// `Container8.EmbedHolder.Do` is also able to be called.
type (
	EmbedHolder struct{ *EmbedAnother }
	Container8  struct {
		*Embedded
		EmbedHolder
	}
)

// The field `Container1.Do` is ambiguous with the method `Embedded.Do`.
type Container9 struct {
	*Embedded
	*Container1
}

// The method `Do` found in the field `Do` (e.g. `Do.Embedded.Do`) is ambiguous
// with embedded field `Do` itself.
type (
	Do          struct{ *Embedded }
	Container10 struct{ *Do }
)

// The ambiguity in `Container7` means that `Container7` does not have a `Do`
// method, meaning `Container11.Do` is not ambiguous and will call `Container11.Embedded.Do`.
type Container11 struct {
	*Embedded
	*Container7
}

// `Container12.DoerHolder.Doer` and `Container12.Embedded` are ambiguous
// even though `DoerHolder.Doer` is "deeper" (see `Container8`), because embedded
// interfaces contribute methods to the embedding interface meaning
// `DoerHolder` contains a `Do` method thus `Container12.DoerHolder.Do` is at
// the same level as `Container12.Embedded.Do`.
type (
	DoerHolder  interface{ Doer }
	Container12 struct {
		DoerHolder
		*Embedded
	}
)

// The field `Do` is a func type so it is callable via `Container13.Do` and
// will block `Embedded.Do` just like the string in `Container1` does.
// However `Container13` does not duck-type to `Doer` because `Do` is a field
// so `Container13.Do` will not be called during `doIt`.
type Container13 struct {
	*Embedded
	Do func() string
}

// The embedded value must still promote the `Do` method implemented with a value receiver.
type Container14 struct{ ValEmbed }

// The `Do` field in `WithDoField` and the `Do` method in `Embedded`
// are at the same level so cause `Do` to be ambiguous in `Container15`.
// Even though `WithDoField.Do` can be called, since `Do` is a field `WithDoField`
// will not duck-type to `Doer`.
type (
	WithDoField struct{ Do func() string }
	Container15 struct {
		*Embedded
		*WithDoField
	}
)

// doIt does a runtime type check to test the `$methodSet` method in the prelude,
// since a compile time type check (e.g. `var _ Doer = ...`) is done in the
// Go type checker.
func doIt(a any) {
	fmt.Printf("%18T: ", a)
	if aa, ok := a.(Doer); ok {
		fmt.Println(aa.Do())
	} else {
		fmt.Println(`Or do not`)
	}
}

func main() {
	doIt(&Embedded{})

	fmt.Println()
	c1 := &Container1{}
	doIt(c1)
	doIt(c1.Embedded)
	doIt(&c1.Embedded)

	fmt.Println()
	c2 := &Container2{}
	doIt(c2)
	doIt(c2.Embedded)

	fmt.Println()
	c3 := &Container3{}
	doIt(c3)
	doIt(c3.Embedded)

	fmt.Println()
	c4 := &Container4{}
	doIt(c4)
	doIt(c4.Embedded)

	fmt.Println()
	c5 := &Container5{}
	doIt(c5)
	doIt(c5.Embedded)

	fmt.Println()
	c6 := &Container6{Doer: &Embedded{}}
	doIt(c6)
	doIt(c6.Doer)

	fmt.Println()
	c7 := &Container7{}
	doIt(c7)
	doIt(c7.Embedded)
	doIt(c7.EmbedAnother)

	fmt.Println()
	c8 := &Container8{}
	doIt(c8)
	doIt(c8.Embedded)
	doIt(c8.EmbedHolder)
	doIt(c8.EmbedAnother)

	fmt.Println()
	c9 := &Container9{}
	doIt(c9)
	doIt(c9.Embedded)
	doIt(c9.Container1)

	fmt.Println()
	c10 := &Container10{Do: &Do{}}
	doIt(c10)
	doIt(c10.Do)

	fmt.Println()
	c11 := &Container11{Container7: &Container7{}}
	doIt(c11)
	doIt(c11.Embedded)
	doIt(c11.Container7)

	fmt.Println()
	c12 := &Container12{DoerHolder: &Embedded{}}
	doIt(c12)
	doIt(c12.DoerHolder)

	fmt.Println()
	c13 := &Container13{Do: func() string { return `Don't` }}
	doIt(c13)
	doIt(c13.Embedded)

	fmt.Println()
	c14 := &Container14{}
	doIt(c14)

	fmt.Println()
	c15 := &Container15{WithDoField: &WithDoField{Do: func() string { return `Do not try` }}}
	doIt(c15)
	doIt(c15.Embedded)
	doIt(c15.WithDoField)
}
