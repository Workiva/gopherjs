package session

import (
	"fmt"
	"go/build"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/neelance/sourcemap"

	"github.com/gopherjs/gopherjs/build/cache"
	"github.com/gopherjs/gopherjs/build/context"
	"github.com/gopherjs/gopherjs/build/packageData"
	"github.com/gopherjs/gopherjs/compiler"
)

// Session manages internal state GopherJS requires to perform a build.
//
// This is the main interface to GopherJS build system. Session lifetime is
// roughly equivalent to a single GopherJS tool invocation.
type Session struct {
	options    *Options
	xctx       context.XContext
	buildCache cache.BuildCache

	// Binary archives produced during the current session and assumed to be
	// up to date with input sources and dependencies. In the -w ("watch") mode
	// must be cleared upon entering watching.
	UpToDateArchives map[string]*compiler.Archive
	Types            map[string]*types.Package
	Watcher          *fsnotify.Watcher
}

// NewSession creates a new GopherJS build session.
func NewSession(options *Options) (*Session, error) {
	options.Verbose = options.Verbose || options.Watch

	s := &Session{
		options:          options,
		UpToDateArchives: make(map[string]*compiler.Archive),
	}
	s.xctx = context.NewBuildContext(s.InstallSuffix(), s.options.BuildTags)
	env := s.xctx.Env()

	// Go distribution version check.
	if err := compiler.CheckGoVersion(env.GOROOT); err != nil {
		return nil, err
	}

	s.buildCache = cache.BuildCache{
		GOOS:          env.GOOS,
		GOARCH:        env.GOARCH,
		GOROOT:        env.GOROOT,
		GOPATH:        env.GOPATH,
		BuildTags:     append([]string{}, env.BuildTags...),
		Minify:        options.Minify,
		TestedPackage: options.TestedPackage,
	}
	s.Types = make(map[string]*types.Package)
	if options.Watch {
		if out, err := exec.Command("ulimit", "-n").Output(); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && n < 1024 {
				fmt.Printf("Warning: The maximum number of open file descriptors is very low (%d). Change it with 'ulimit -n 8192'.\n", n)
			}
		}

		var err error
		s.Watcher, err = fsnotify.NewWatcher()
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

// XContext returns the session's build context.
func (s *Session) XContext() context.XContext { return s.xctx }

// InstallSuffix returns the suffix added to the generated output file.
func (s *Session) InstallSuffix() string {
	if s.options.Minify {
		return "min"
	}
	return ""
}

// GoRelease returns Go release version this session is building with.
func (s *Session) GoRelease() string {
	return compiler.GoRelease(s.xctx.Env().GOROOT)
}

// BuildFiles passed to the GopherJS tool as if they were a package.
//
// A ephemeral package will be created with only the provided files. This
// function is intended for use with, for example, `gopherjs run main.go`.
func (s *Session) BuildFiles(filenames []string, pkgObj string, cwd string) error {
	if len(filenames) == 0 {
		return fmt.Errorf("no input sources are provided")
	}

	normalizedDir := func(filename string) string {
		d := filepath.Dir(filename)
		if !filepath.IsAbs(d) {
			d = filepath.Join(cwd, d)
		}
		return filepath.Clean(d)
	}

	// Ensure all source files are in the same directory.
	dirSet := map[string]bool{}
	for _, file := range filenames {
		dirSet[normalizedDir(file)] = true
	}
	dirList := []string{}
	for dir := range dirSet {
		dirList = append(dirList, dir)
	}
	sort.Strings(dirList)
	if len(dirList) != 1 {
		return fmt.Errorf("named files must all be in one directory; have: %v", strings.Join(dirList, ", "))
	}

	root := dirList[0]
	ctx := build.Default
	ctx.UseAllFiles = true
	ctx.ReadDir = func(dir string) ([]fs.FileInfo, error) {
		n := len(filenames)
		infos := make([]fs.FileInfo, n)
		for i := 0; i < n; i++ {
			info, err := os.Stat(filenames[i])
			if err != nil {
				return nil, err
			}
			infos[i] = info
		}
		return infos, nil
	}
	p, err := ctx.Import(".", root, 0)
	if err != nil {
		return err
	}
	p.Name = "main"
	p.ImportPath = "main"

	pkg := &packageData.PackageData{
		Package: p,
		// This ephemeral package doesn't have a unique import path to be used as a
		// build cache key, so we never cache it.
		SrcModTime: time.Now().Add(time.Hour),
		bctx:       &goCtx(s.xctx.Env()).bctx,
	}

	for _, file := range filenames {
		if !strings.HasSuffix(file, ".inc.js") {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}
		info, err := os.Stat(file)
		if err != nil {
			return fmt.Errorf("failed to stat %s: %w", file, err)
		}
		pkg.JSFiles = append(pkg.JSFiles, packageData.JSFile{
			Path:    filepath.Join(pkg.Dir, filepath.Base(file)),
			ModTime: info.ModTime(),
			Content: content,
		})
	}

	archive, err := s.BuildPackage(pkg)
	if err != nil {
		return err
	}
	if s.Types["main"].Name() != "main" {
		return fmt.Errorf("cannot build/run non-main package")
	}
	return s.WriteCommandPackage(archive, pkgObj)
}

// BuildImportPath loads and compiles package with the given import path.
//
// Relative paths are interpreted relative to the current working dir.
func (s *Session) BuildImportPath(path string) (*compiler.Archive, error) {
	_, archive, err := s.buildImportPathWithSrcDir(path, "")
	return archive, err
}

// buildImportPathWithSrcDir builds the package specified by the import path.
//
// Relative import paths are interpreted relative to the passed srcDir. If
// srcDir is empty, current working directory is assumed.
func (s *Session) buildImportPathWithSrcDir(path string, srcDir string) (*packageData.PackageData, *compiler.Archive, error) {
	pkg, err := s.xctx.Import(path, srcDir, 0)
	if s.Watcher != nil && pkg != nil { // add watch even on error
		s.Watcher.Add(pkg.Dir)
	}
	if err != nil {
		return nil, nil, err
	}

	archive, err := s.BuildPackage(pkg)
	if err != nil {
		return nil, nil, err
	}

	return pkg, archive, nil
}

// getExeModTime will determine the mod time of the GopherJS binary
// the first time this is called and cache the result for subsequent calls.
var getExeModTime = func() func() time.Time {
	var (
		once   sync.Once
		result time.Time
	)
	getTime := func() {
		gopherjsBinary, err := os.Executable()
		if err == nil {
			var fileInfo os.FileInfo
			fileInfo, err = os.Stat(gopherjsBinary)
			if err == nil {
				result = fileInfo.ModTime()
				return
			}
		}
		os.Stderr.WriteString("Could not get GopherJS binary's modification timestamp. Please report issue.\n")
		result = time.Now()
	}
	return func() time.Time {
		once.Do(getTime)
		return result
	}
}()

// BuildPackage compiles an already loaded package.
func (s *Session) BuildPackage(pkg *packageData.PackageData) (*compiler.Archive, error) {
	if archive, ok := s.UpToDateArchives[pkg.ImportPath]; ok {
		return archive, nil
	}

	if exeModTime := getExeModTime(); exeModTime.After(pkg.SrcModTime) {
		pkg.SrcModTime = exeModTime
	}

	for _, importedPkgPath := range pkg.Imports {
		if importedPkgPath == "unsafe" {
			continue
		}
		importedPkg, _, err := s.buildImportPathWithSrcDir(importedPkgPath, pkg.Dir)
		if err != nil {
			return nil, err
		}

		if impModTime := importedPkg.SrcModTime; impModTime.After(pkg.SrcModTime) {
			pkg.SrcModTime = impModTime
		}
	}

	if fileModTime := pkg.FileModTime(); fileModTime.After(pkg.SrcModTime) {
		pkg.SrcModTime = fileModTime
	}

	if !s.options.NoCache {
		archive := s.buildCache.LoadArchive(pkg.ImportPath, pkg.SrcModTime, s.Types)
		if archive != nil {
			s.UpToDateArchives[pkg.ImportPath] = archive
			// Existing archive is up to date, no need to build it from scratch.
			return archive, nil
		}
	}

	// Existing archive is out of date or doesn't exist, let's build the package.
	fileSet := token.NewFileSet()
	files, overlayJsFiles, err := parseAndAugment(s.xctx, pkg, pkg.IsTest, fileSet)
	if err != nil {
		return nil, err
	}
	embed, err := embedFiles(pkg, fileSet, files)
	if err != nil {
		return nil, err
	}
	if embed != nil {
		files = append(files, embed)
	}

	importContext := &compiler.ImportContext{
		Packages: s.Types,
		Import:   s.ImportResolverFor(pkg),
	}
	archive, err := compiler.Compile(pkg.ImportPath, files, fileSet, importContext, s.options.Minify)
	if err != nil {
		return nil, err
	}

	for _, jsFile := range append(pkg.JSFiles, overlayJsFiles...) {
		archive.IncJSCode = append(archive.IncJSCode, []byte("\t(function() {\n")...)
		archive.IncJSCode = append(archive.IncJSCode, jsFile.Content...)
		archive.IncJSCode = append(archive.IncJSCode, []byte("\n\t}).call($global);\n")...)
	}

	if s.options.Verbose {
		fmt.Println(pkg.ImportPath)
	}

	s.buildCache.StoreArchive(archive, time.Now())
	s.UpToDateArchives[pkg.ImportPath] = archive

	return archive, nil
}

// ImportResolverFor returns a function which returns a compiled package archive
// given an import path.
func (s *Session) ImportResolverFor(pkg *packageData.PackageData) func(string) (*compiler.Archive, error) {
	return func(path string) (*compiler.Archive, error) {
		if archive, ok := s.UpToDateArchives[path]; ok {
			return archive, nil
		}
		_, archive, err := s.buildImportPathWithSrcDir(path, pkg.Dir)
		return archive, err
	}
}

// SourceMappingCallback returns a call back for compiler.SourceMapFilter
// configured for the current build session.
func (s *Session) SourceMappingCallback(m *sourcemap.Map) func(generatedLine, generatedColumn int, originalPos token.Position) {
	return newMappingCallback(m, s.xctx.Env().GOROOT, s.xctx.Env().GOPATH, s.options.MapToLocalDisk)
}

// WriteCommandPackage writes the final JavaScript output file at pkgObj path.
func (s *Session) WriteCommandPackage(archive *compiler.Archive, pkgObj string) error {
	if err := os.MkdirAll(filepath.Dir(pkgObj), 0o777); err != nil {
		return err
	}
	codeFile, err := os.Create(pkgObj)
	if err != nil {
		return err
	}
	defer codeFile.Close()

	sourceMapFilter := &compiler.SourceMapFilter{Writer: codeFile}
	if s.options.CreateMapFile {
		m := &sourcemap.Map{File: filepath.Base(pkgObj)}
		mapFile, err := os.Create(pkgObj + ".map")
		if err != nil {
			return err
		}

		defer func() {
			m.WriteTo(mapFile)
			mapFile.Close()
			fmt.Fprintf(codeFile, "//# sourceMappingURL=%s.map\n", filepath.Base(pkgObj))
		}()

		sourceMapFilter.MappingCallback = s.SourceMappingCallback(m)
	}

	deps, err := compiler.ImportDependencies(archive, func(path string) (*compiler.Archive, error) {
		if archive, ok := s.UpToDateArchives[path]; ok {
			return archive, nil
		}
		_, archive, err := s.buildImportPathWithSrcDir(path, "")
		return archive, err
	})
	if err != nil {
		return err
	}
	return compiler.WriteProgramCode(deps, sourceMapFilter, s.GoRelease())
}

// newMappingCallback creates a new callback for source map generation.
func newMappingCallback(m *sourcemap.Map, goroot, gopath string, localMap bool) func(generatedLine, generatedColumn int, originalPos token.Position) {
	return func(generatedLine, generatedColumn int, originalPos token.Position) {
		if !originalPos.IsValid() {
			m.AddMapping(&sourcemap.Mapping{GeneratedLine: generatedLine, GeneratedColumn: generatedColumn})
			return
		}

		file := originalPos.Filename

		switch hasGopathPrefix, prefixLen := hasGopathPrefix(file, gopath); {
		case localMap:
			// no-op:  keep file as-is
		case hasGopathPrefix:
			file = filepath.ToSlash(file[prefixLen+4:])
		case strings.HasPrefix(file, goroot):
			file = filepath.ToSlash(file[len(goroot)+4:])
		default:
			file = filepath.Base(file)
		}

		m.AddMapping(&sourcemap.Mapping{GeneratedLine: generatedLine, GeneratedColumn: generatedColumn, OriginalFile: file, OriginalLine: originalPos.Line, OriginalColumn: originalPos.Column})
	}
}

// hasGopathPrefix returns true and the length of the matched GOPATH workspace,
// iff file has a prefix that matches one of the GOPATH workspaces.
func hasGopathPrefix(file, gopath string) (hasGopathPrefix bool, prefixLen int) {
	gopathWorkspaces := filepath.SplitList(gopath)
	for _, gopathWorkspace := range gopathWorkspaces {
		gopathWorkspace = filepath.Clean(gopathWorkspace)
		if strings.HasPrefix(file, gopathWorkspace) {
			return true, len(gopathWorkspace)
		}
	}
	return false, 0
}

// WaitForChange watches file system events and returns if either when one of
// the source files is modified.
func (s *Session) WaitForChange() {
	// Will need to re-validate up-to-dateness of all archives, so flush them from
	// memory.
	s.UpToDateArchives = map[string]*compiler.Archive{}
	s.Types = map[string]*types.Package{}

	s.options.PrintSuccess("watching for changes...\n")
	for {
		select {
		case ev := <-s.Watcher.Events:
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 || filepath.Base(ev.Name)[0] == '.' {
				continue
			}
			if !strings.HasSuffix(ev.Name, ".go") && !strings.HasSuffix(ev.Name, ".inc.js") {
				continue
			}
			s.options.PrintSuccess("change detected: %s\n", ev.Name)
		case err := <-s.Watcher.Errors:
			s.options.PrintError("watcher error: %s\n", err.Error())
		}
		break
	}

	go func() {
		for range s.Watcher.Events {
			// consume, else Close() may deadlock
		}
	}()
	s.Watcher.Close()
}
