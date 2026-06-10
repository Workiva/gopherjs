//go:build js

package http_test

import "testing"

func TestTimeoutHandlerSuperfluousLogs(t *testing.T) {
	// The test expects nested anonymous functions to be named "Foo.func1.2",
	// bug GopherJS generates "Foo.func1.func2". Otherwise the test works as
	// expected.
	t.Skip("GopherJS uses different synthetic function names.")
}

func TestHTTP2WriteDeadlineExtendedOnNewRequest(t *testing.T) {
	// Test depends on httptest.NewUnstartedServer
	t.Skip("Network access not supported by GopherJS.")
}

//gopherjs:replace
func testWriteDeadlineExtendedOnNewRequest(t *testing.T, mode testMode) {
	// The test hardcodes a timeout, ts.Config.WriteTimeout = 250ms, which is
	// also used as the TLS handshake deadline, so the handshake takes too long
	// causing the h2 subtest fails with an "i/o timeout" during the TLS handshake.
	t.Skip("hardcoded WriteTimeout (250ms) is too short for GopherJS TLS handshake")
}
