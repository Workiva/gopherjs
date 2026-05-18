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

type Container1 struct {
	Embedded
	Do string
}

type Container2 struct {
	*Embedded
	Do string
}

type Container3 struct {
	Embedded
}

type Container4 struct {
	*Embedded
}

type Container5 struct {
	*Embedded
}

func (e *Container5) Do() string { return `Try to do` }

type Container6 struct {
	Doer
	Do string
}

type EmbedAnother struct{}

func (e *EmbedAnother) Do() string { return `Do it again` }

type Container7 struct {
	*Embedded
	*EmbedAnother
}

type EmbedHolder struct {
	*EmbedAnother
}

type Container8 struct {
	*Embedded
	EmbedHolder
}

type Container9 struct {
	*Embedded
	*Container1
}

type Do struct {
	*Embedded
}

type Container10 struct {
	*Do
}

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
}
