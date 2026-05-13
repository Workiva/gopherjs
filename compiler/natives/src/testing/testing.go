//go:build js

package testing

import "github.com/gopherjs/gopherjs/js"

func init() {
	testBinary = js.Global.Get("$testBinary").String()
}
