//go:build js

package testenv

// HasSrc reports whether the entire source tree is available under GOROOT.
// In GopherJS, the Go source tree is not available at runtime.
func HasSrc() bool {
	return false
}
