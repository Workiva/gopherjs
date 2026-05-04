//go:build js

package coverage

import "internal/coverage/rtcov"

// GOPHERJS: runtime/coverage declares the following three functions as
// forward declarations with //go:linkname to the runtime package.
// When building with `-cover`, Go registers blobs via `runtime.addCovMeta`
// for the coverage instrumentation to write the coverage data into.
// We don't support runtime coverage yet so instead of implementing
// our own coverage blobs, we can simply return empty for these.
// That will cause the callers to beleive no blobs are registered so no
// coverage is being collected.

//gopherjs:replace
func getCovMetaList() []rtcov.CovMetaBlob { return nil }

//gopherjs:replace
func getCovCounterList() []rtcov.CovCounterBlob { return nil }

//gopherjs:replace
func getCovPkgMap() map[int]int { return nil }
