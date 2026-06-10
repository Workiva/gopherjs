//go:build js && wasm

package http_test

import "testing"

func testTransportGCRequest(t *testing.T, mode testMode, body bool) {
	t.Skip("The test relies on runtime.SetFinalizer(), which is not supported by GopherJS.")
}
