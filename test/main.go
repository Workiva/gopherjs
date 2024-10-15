package main

import (
	"github.com/gopherjs/gopherjs/js"
	"github.com/gopherjs/gopherjs/test/subpkg"
)

func main() {
	js.Global.Get("console").Call("log", subpkg.Data())
}
