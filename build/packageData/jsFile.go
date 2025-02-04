package packageData

import (
	"time"
)

// JSFile represents a *.inc.js file metadata and content.
type JSFile struct {
	Path    string // Full file path for the build context the file came from.
	ModTime time.Time
	Content []byte
}
