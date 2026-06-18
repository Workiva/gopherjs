//go:build js

package testenv

// GOPHERJS: HasSrc reports whether the entire source tree is available
// under GOROOT. Since GopherJS doesn't have the untranspiled Go source tree
// available at runtime, this is always false.
func HasSrc() bool {
	return false
}
