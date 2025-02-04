package packageData

import (
	"fmt"
	"go/build"
	"go/token"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/tools/go/buildutil"
)

// PackageData is an extension of go/build.Package with additional metadata
// GopherJS requires.
type PackageData struct {
	*build.Package
	JSFiles []JSFile
	// IsTest is true if the package is being built for running tests.
	IsTest     bool
	SrcModTime time.Time
	UpToDate   bool
	// If true, the package does not have a corresponding physical directory on disk.
	IsVirtual bool

	bctx *build.Context // The original build context this package came from.
}

func NewPackageData(pkg *build.Package, isVirtual bool, jsFiles []JSFile, bctx *build.Context) *PackageData {
	return &PackageData{
		Package:   pkg,
		IsVirtual: isVirtual,
		JSFiles:   jsFiles,
		bctx:      bctx,
	}
}

func (p PackageData) String() string {
	return fmt.Sprintf("%s [is_test=%v]", p.ImportPath, p.IsTest)
}

// FileModTime returns the most recent modification time of the package's source
// files. This includes all .go and .inc.js that would be included in the build,
// but excludes any dependencies.
func (p PackageData) FileModTime() time.Time {
	newest := time.Time{}
	for _, file := range p.JSFiles {
		if file.ModTime.After(newest) {
			newest = file.ModTime
		}
	}

	// Unfortunately, build.Context methods don't allow us to Stat and individual
	// file, only to enumerate a directory. So we first get mtimes for all files
	// in the package directory, and then pick the newest for the relevant GoFiles.
	mtimes := map[string]time.Time{}
	files, err := buildutil.ReadDir(p.bctx, p.Dir)
	if err != nil {
		log.Errorf("Failed to enumerate files in the %q in context %v: %s. Assuming time.Now().", p.Dir, p.bctx, err)
		return time.Now()
	}
	for _, file := range files {
		mtimes[file.Name()] = file.ModTime()
	}

	for _, file := range p.GoFiles {
		t, ok := mtimes[file]
		if !ok {
			log.Errorf("No mtime found for source file %q of package %q, assuming time.Now().", file, p.Name)
			return time.Now()
		}
		if t.After(newest) {
			newest = t
		}
	}
	return newest
}

// InternalBuildContext returns the build context that produced the package.
//
// WARNING: This function is a part of internal API and will be removed in
// future.
func (p *PackageData) InternalBuildContext() *build.Context {
	return p.bctx
}

// TestPackage returns a variant of the package with "internal" tests.
func (p *PackageData) TestPackage() *PackageData {
	return &PackageData{
		Package: &build.Package{
			Name:            p.Name,
			ImportPath:      p.ImportPath,
			Dir:             p.Dir,
			GoFiles:         append(p.GoFiles, p.TestGoFiles...),
			Imports:         append(p.Imports, p.TestImports...),
			EmbedPatternPos: joinEmbedPatternPos(p.EmbedPatternPos, p.TestEmbedPatternPos),
		},
		IsTest:  true,
		JSFiles: p.JSFiles,
		bctx:    p.bctx,
	}
}

// XTestPackage returns a variant of the package with "external" tests.
func (p *PackageData) XTestPackage() *PackageData {
	return &PackageData{
		Package: &build.Package{
			Name:            p.Name + "_test",
			ImportPath:      p.ImportPath + "_test",
			Dir:             p.Dir,
			GoFiles:         p.XTestGoFiles,
			Imports:         p.XTestImports,
			EmbedPatternPos: p.XTestEmbedPatternPos,
		},
		IsTest: true,
		bctx:   p.bctx,
	}
}

// InstallPath returns the path where "gopherjs install" command should place the
// generated output.
func (p *PackageData) InstallPath() string {
	if p.IsCommand() {
		name := filepath.Base(p.ImportPath) + ".js"
		// For executable packages, mimic go tool behavior if possible.
		if gobin := os.Getenv("GOBIN"); gobin != "" {
			return filepath.Join(gobin, name)
		} else if gopath := os.Getenv("GOPATH"); gopath != "" {
			return filepath.Join(gopath, "bin", name)
		} else if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "go", "bin", name)
		}
	}
	return p.PkgObj
}

func joinEmbedPatternPos(m1, m2 map[string][]token.Position) map[string][]token.Position {
	if len(m1) == 0 && len(m2) == 0 {
		return nil
	}
	m := make(map[string][]token.Position)
	for k, v := range m1 {
		m[k] = v
	}
	for k, v := range m2 {
		m[k] = append(m[k], v...)
	}
	return m
}
