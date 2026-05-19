package tests

import "testing"

// These tests are based on https://github.com/gopherjs/gopherjs/issues/757
// and https://github.com/gopherjs/gopherjs/issues/1003.
//
// The code was failing because the JS would fail on the shadowed method, `Do`.
// Go says that a struct field name shadows a method being promoted from an
// embedded type. This test checks the several ways of shadowing of methods and
// preventing ambiguous selectors from being promoted.
//
// See https://go.dev/ref/spec#Selectors:
// > For a value x of type T or *T where T is not a pointer or interface type,
// > x.f denotes the field or method at the shallowest depth in T where there
// > is such an f. If there is not exactly one f with shallowest depth, the
// > selector expression is illegal.

type (
	Doer            interface{ Do() string }
	DoAnother       interface{ Do() string }
	DoEmbedded      struct{}
	DoEmbeddedAgain struct{}
	DoValueEmbedded struct{}
)

func (*DoEmbedded) Do() string      { return `Do` }
func (*DoEmbeddedAgain) Do() string { return `Do it again` }
func (DoValueEmbedded) Do() string  { return `Do value` }

// When performing `$assertType` the `value.constructor.string` was used for
// the key in `implementedBy` which caused this test to fail because both
// of the `Container` types have the string `*tests.Container` but one does
// cast and the other does not. Switching the key to `value.constructor.id`
// fixes this issue.
func Test_AssertType_ImplementedBy(t *testing.T) {
	{
		type Container struct{ DoEmbedded }
		c := &Container{}
		if _, ok := any(c).(Doer); !ok {
			t.Errorf("cast of %T (#1) is expected to work, but it did not", c)
		}
	}
	{
		type Container struct{ Name string }
		c := &Container{}
		if _, ok := any(c).(Doer); ok {
			t.Errorf("cast of %T (#2) is expected to not work, but it did", c)
		}
	}
}

// This is similar to `Test_AssertType_ImplementedBy` except the loop gaurd,
// `seen`, in `$methodSet` was using `e.typ.string` as the key, which caused
// this test to fail since the `Container`s here both have the string
// `*tests.Containers`. The way they are embedded caused the inner most one
// that defines `func Do() string` to be skipped, thus the cast didn't work.
// Switching the key to `e.typ.id` fixes this issue.
func Test_MethodSet_Seen(t *testing.T) {
	type (
		Container struct{ DoEmbedded }
		Box       struct{ Container }
	)
	{
		type Container struct{ Box }
		c := &Container{}
		if _, ok := any(c).(Doer); !ok {
			t.Errorf("cast of %T (#1) is expected to work, but it did not", c)
		}
	}
}

// THis is similar to `Test_AssertType_ImplementedBy` and `Test_MethodSet_Seen`
// except is for the `$proxies` key in `$pointerOfStructConversion`. This is
// not directly related to shadowing but was found when the issues for the
// prior two tests were found.
func Test_PointerOfStructConversion_Proxies(t *testing.T) {
	type Base struct{ val int }
	base := &Base{val: 42}
	type Container Base
	_ = (*Container)(base)
	type Cont1 = Container
	{
		type Container Base
		c2 := (*Container)(base)
		switch any(c2).(type) {
		case *Cont1:
			// This is the case hit when using `.string` instead of `.id` because
			// the base already proxies to a `*tests.Container` from `Cont1`.
			t.Errorf(`incorrect proxy. %T was the outer definition of Container`, c2)
		case *Container:
			// correct
		default:
			t.Errorf(`incorrect proxy. %T was neither of the Containers`, c2)
		}
	}
}

// `Container.Do` shadows `DoEmbedded.Do`.
// This is based on https://github.com/gopherjs/gopherjs/issues/757
func Test_Shadow1(t *testing.T) {
	type Container struct {
		DoEmbedded
		Do string
	}

	c := &Container{}
	shadowCheck(t, c, `Do not`)
	shadowCheck(t, c.DoEmbedded, `Do not`)
	shadowCheck(t, &c.DoEmbedded, `Do`)
}

// `dontainer.Do` shadows `DoEmbedded.Do`.
func Test_Shadow2(t *testing.T) {
	type Container struct {
		*DoEmbedded
		Do string
	}
	c := &Container{}
	shadowCheck(t, c, `Do not`)
	shadowCheck(t, c.DoEmbedded, `Do`)
}

// `embedded.Do` is callable from `*Container` but not `*Container.DoEmbedded`
// since `*Container.Do` calls `Do` with `*DoEmbedded` and `Do` can not be called
// with a non-pointer to embedded.
func Test_Shadow3(t *testing.T) {
	type Container struct{ DoEmbedded }

	c := &Container{}
	shadowCheck(t, c, `Do`)
	shadowCheck(t, c.DoEmbedded, `Do not`)
}

// This complements `Test_Shadow3` to check that `Container.Do` can be called
// and `Container.DoEmbedded.Do` can also be called.
func Test_Shadow4(t *testing.T) {
	type Container struct{ *DoEmbedded }

	c := &Container{}
	shadowCheck(t, c, `Do`)
	shadowCheck(t, c.DoEmbedded, `Do`)
}

type Shadow5Container struct{ *DoEmbedded }

func (e *Shadow5Container) Do() string { return `Try to do` }

// `Container5.Do` shadows `DoEmbedded.Do`.
func Test_Shadow5(t *testing.T) {
	c := &Shadow5Container{}
	shadowCheck(t, c, `Try to do`)
	shadowCheck(t, c.DoEmbedded, `Do`)
}

// `Container.Do` shadows the interface's method `Doer.Do`.
func Test_Shadow6(t *testing.T) {
	type Container struct {
		Doer
		Do string
	}

	c := &Container{Doer: &DoEmbedded{}}
	shadowCheck(t, c, `Do not`)
	shadowCheck(t, c.Doer, `Do`)
}

// `Container.DoEmbedded.Do` and `Container.DoEmbeddedAgain.Do` are ambiguous
// so `Container.Do` can not be called.
func Test_Shadow7(t *testing.T) {
	type Container struct {
		*DoEmbedded
		*DoEmbeddedAgain
	}

	c := &Container{}
	shadowCheck(t, c, `Do not`)
	shadowCheck(t, c.DoEmbedded, `Do`)
	shadowCheck(t, c.DoEmbeddedAgain, `Do it again`)
}

// This is similar to `Test_Shadow7` but checks the ambiguity goes away when the methods
// are not in contention at the same level of embedding.
// `Container.EmbedHolder.DoEmbeddedAgain.Do` is deeper than `Container.DoEmbedded.Do`
// so `Container.DoEmbedded.Do` is called with `Container.Do`, even through
// `Container.EmbedHolder.Do` is also able to be called.
func Test_Shadow8(t *testing.T) {
	type (
		EmbedHolder struct{ *DoEmbeddedAgain }
		Container   struct {
			*DoEmbedded
			EmbedHolder
		}
	)

	c := &Container{}
	shadowCheck(t, c, `Do`)
	shadowCheck(t, c.DoEmbedded, `Do`)
	shadowCheck(t, c.EmbedHolder, `Do it again`)
	shadowCheck(t, c.DoEmbeddedAgain, `Do it again`)
}

// The field `Inner.Do` is ambiguous with the method `DoEmbedded.Do`.
func Test_Shadow9(t *testing.T) {
	type (
		Inner struct {
			DoEmbedded
			Do string
		}
		Container struct {
			*DoEmbedded
			*Inner
		}
	)

	c := &Container{}
	shadowCheck(t, c, `Do not`)
	shadowCheck(t, c.DoEmbedded, `Do`)
	shadowCheck(t, c.Inner, `Do not`)
}

// The method `Do` found in the field `Do` (e.g. `Do.DoEmbedded.Do`) is ambiguous
// with embedded field `Do` itself.
func Test_Shadow10(t *testing.T) {
	type (
		Do        struct{ *DoEmbedded }
		Container struct{ *Do }
	)

	c := &Container{Do: &Do{}}
	shadowCheck(t, c, `Do not`)
	shadowCheck(t, c.Do, `Do`)
}

// The ambiguity in `Inner` means that `Inner` does not have a `Do` method,
// meaning `Container.Do` is not ambiguous and will call `Container.DoEmbedded.Do`.
func Test_Shadow11(t *testing.T) {
	type (
		Inner struct {
			*DoEmbedded
			*DoEmbeddedAgain
		}
		Container struct {
			*DoEmbedded
			*Inner
		}
	)

	c := &Container{Inner: &Inner{}}
	shadowCheck(t, c, `Do`)
	shadowCheck(t, c.DoEmbedded, `Do`)
	shadowCheck(t, c.Inner, `Do not`)
}

// `Container.DoerHolder.Doer` and `Container.DoEmbedded` are ambiguous even
// though `DoerHolder.Doer` is deeper because embedded interfaces contribute
// methods to the embedding interface meaning `DoerHolder` contains a `Do` method
// thus `Container.DoerHolder.Do` is at the same level as `Container.DoEmbedded.Do`.
func Test_Shadow12(t *testing.T) {
	type (
		DoerHolder interface{ Doer }
		Container  struct {
			DoerHolder
			*DoEmbedded
		}
	)

	c := &Container{DoerHolder: &DoEmbedded{}}
	shadowCheck(t, c, `Do not`)
	shadowCheck(t, c.DoerHolder, `Do`)
}

// The field `Do` is a func type so it is callable via `Container.Do` and
// will block `DoEmbedded.Do` just like the string in `Test_Shadow1` does.
// However `Container` does not duck-type to `Doer` because `Do` is a field.
func Test_Shadow13(t *testing.T) {
	type Container struct {
		*DoEmbedded
		Do func() string
	}

	c := &Container{Do: func() string { return `Don't` }}
	shadowCheck(t, c, `Do not`)
	shadowCheck(t, c.DoEmbedded, `Do`)
}

// The embedded value must still promote the `Do` method implemented with a value receiver.
func Test_Shadow14(t *testing.T) {
	type Container struct{ DoValueEmbedded }

	c := &Container{}
	shadowCheck(t, c, `Do value`)
	shadowCheck(t, c.DoValueEmbedded, `Do value`)
}

// The `Do` field in `WithDoField` and the `Do` method in `DoEmbedded`
// are at the same level so cause `Do` to be ambiguous in `Container`.
// Even though `WithDoField.Do` can be called, since `Do` is a field `WithDoField`
// will not duck-type to `Doer`.
func Test_Shadow15(t *testing.T) {
	type (
		WithDoField struct{ Do func() string }
		Container   struct {
			*DoEmbedded
			*WithDoField
		}
	)

	c := &Container{
		WithDoField: &WithDoField{
			Do: func() string { return `Do not try` },
		},
	}
	shadowCheck(t, c, `Do not`)
	shadowCheck(t, c.DoEmbedded, `Do`)
	shadowCheck(t, c.WithDoField, `Do not`)
}

// The two fields are both interfaces in contention because they both define
// `Do` methods meaning that `Container.Do` is ambiguous.
// This is based off of https://github.com/gopherjs/gopherjs/issues/1003
func Test_Shadow16(t *testing.T) {
	type Container struct {
		Doer
		DoAnother
	}

	c := &Container{
		Doer:      &DoEmbedded{},
		DoAnother: &DoEmbedded{},
	}
	shadowCheck(t, c, `Do not`)
	shadowCheck(t, c.Doer, `Do`)
	shadowCheck(t, c.DoAnother, `Do`)
}

// shadowCheck does a runtime type check of a against `Doer` to test the
// `$methodSet` method in the prelude.
func shadowCheck(t *testing.T, a any, want string) {
	t.Helper()
	got := `Do not`
	if aa, ok := a.(Doer); ok {
		got = aa.Do()
	}
	if got != want {
		t.Errorf("expected %T to return %q but got %q\n", a, want, got)
	}
}
