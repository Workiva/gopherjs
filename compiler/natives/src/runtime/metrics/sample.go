//go:build js

package metrics

import "unsafe"

//gopherjs:replace No-op since runtime does not define metrics yet.
func runtime_readMetrics(unsafe.Pointer, int, int) {}
