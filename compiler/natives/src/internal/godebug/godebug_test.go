//go:build js

package godebug_test

import "testing"

//gopherjs:replace
func TestMetrics(t *testing.T) {
	t.Skip(`This test requires runtime metrics to be implemented. GopherJS does not yet support runtime metrics.`)
}
