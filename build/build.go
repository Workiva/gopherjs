// Package build implements GopherJS build system.
//
// WARNING: This package's API is treated as internal and currently doesn't
// provide any API stability guarantee, use it at your own risk. If you need a
// stable interface, prefer invoking the gopherjs CLI tool as a subprocess.
package build

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gopherjs/gopherjs/compiler"
	"github.com/gopherjs/gopherjs/compiler/astutil"

	"golang.org/x/tools/go/buildutil"
)

// Import returns details about the Go package named by the import path. If the
// path is a local import path naming a package that can be imported using
// a standard import path, the returned package will set p.ImportPath to
// that path.
//
// In the directory containing the package, .go and .inc.js files are
// considered part of the package except for:
//
//   - .go files in package documentation
//   - files starting with _ or . (likely editor temporary files)
//   - files with build constraints not satisfied by the context
//
// If an error occurs, Import returns a non-nil error and a nil
// *PackageData.
func Import(path string, mode build.ImportMode, installSuffix string, buildTags []string) (*PackageData, error) {
	wd, err := os.Getwd()
	if err != nil {
		// Getwd may fail if we're in GOOS=js mode. That's okay, handle
		// it by falling back to empty working directory. It just means
		// Import will not be able to resolve relative import paths.
		wd = ""
	}
	xctx := NewBuildContext(installSuffix, buildTags)
	return xctx.Import(path, wd, mode)
}

// ImportDir is like Import but processes the Go package found in the named
// directory.
func ImportDir(dir string, mode build.ImportMode, installSuffix string, buildTags []string) (*PackageData, error) {
	xctx := NewBuildContext(installSuffix, buildTags)
	pkg, err := xctx.Import(".", dir, mode)
	if err != nil {
		return nil, err
	}

	return pkg, nil
}

// overrideInfo is used by parseAndAugment methods to manage
// directives and how the overlay and original are merged.
type overrideInfo struct {
	// KeepOriginal indicates that the original code should be kept
	// but the identifier will be prefixed by `_gopherjs_original_foo`.
	// If false the original code is removed.
	keepOriginal bool

	// purgeMethods indicates that this info is for a type and
	// if a method has this type as a receiver should also be removed.
	// If the method is defined in the overlays and therefore has its
	// own overrides, this will be ignored.
	purgeMethods bool

	// overrideSignature is the function definition given in the overlays
	// that should be used to replace the signature in the originals.
	// Only receivers, type parameters, parameters, and results will be used.
	overrideSignature *ast.FuncDecl
}

// parseAndAugment parses and returns all .go files of given pkg.
// Standard Go library packages are augmented with files in compiler/natives folder.
// If isTest is true and pkg.ImportPath has no _test suffix, package is built for running internal tests.
// If isTest is true and pkg.ImportPath has _test suffix, package is built for running external tests.
//
// The native packages are augmented by the contents of natives.FS in the following way.
// The file names do not matter except the usual `_test` suffix. The files for
// native overrides get added to the package (even if they have the same name
// as an existing file from the standard library).
//
//   - For function identifiers that exist in the original and the overrides
//     and have the directive `gopherjs:keep-original`, the original identifier
//     in the AST gets prefixed by `_gopherjs_original_`.
//   - For identifiers that exist in the original and the overrides, and have
//     the directive `gopherjs:purge`, both the original and override are
//     removed. This is for completely removing something which is currently
//     invalid for GopherJS. For any purged types any methods with that type as
//     the receiver are also removed.
//   - For function identifiers that exist in the original and the overrides,
//     and have the directive `gopherjs:override-signature`, the overridden
//     function is removed and the original function's signature is changed
//     to match the overridden function signature. This allows the receiver,
//     type parameters, parameter, and return values to be modified as needed.
//   - Otherwise for identifiers that exist in the original and the overrides,
//     the original is removed.
//   - New identifiers that don't exist in original package get added.
func parseAndAugment(xctx XContext, pkg *PackageData, isTest bool, fileSet *token.FileSet) ([]*ast.File, []JSFile, error) {
	jsFiles, overlayFiles := parseOverlayFiles(xctx, pkg, isTest, fileSet)

	originalFiles, err := parserOriginalFiles(pkg, fileSet)
	if err != nil {
		return nil, nil, err
	}

	overrides := make(map[string]overrideInfo)
	for _, file := range overlayFiles {
		augmentOverlayFile(file, overrides)
	}
	delete(overrides, "init")

	for _, file := range originalFiles {
		augmentOriginalImports(pkg.ImportPath, file)
	}

	if len(overrides) > 0 {
		for _, file := range originalFiles {
			augmentOriginalFile(file, overrides)
		}
	}

	return append(overlayFiles, originalFiles...), jsFiles, nil
}

// parseOverlayFiles loads and parses overlay files
// to augment the original files with.
func parseOverlayFiles(xctx XContext, pkg *PackageData, isTest bool, fileSet *token.FileSet) ([]JSFile, []*ast.File) {
	isXTest := strings.HasSuffix(pkg.ImportPath, "_test")
	importPath := pkg.ImportPath
	if isXTest {
		importPath = importPath[:len(importPath)-5]
	}

	nativesContext := overlayCtx(xctx.Env())
	nativesPkg, err := nativesContext.Import(importPath, "", 0)
	if err != nil {
		return nil, nil
	}

	jsFiles := nativesPkg.JSFiles
	var files []*ast.File
	names := nativesPkg.GoFiles
	if isTest {
		names = append(names, nativesPkg.TestGoFiles...)
	}
	if isXTest {
		names = nativesPkg.XTestGoFiles
	}

	for _, name := range names {
		fullPath := path.Join(nativesPkg.Dir, name)
		r, err := nativesContext.bctx.OpenFile(fullPath)
		if err != nil {
			panic(err)
		}
		// Files should be uniquely named and in the original package directory in order to be
		// ordered correctly
		newPath := path.Join(pkg.Dir, "gopherjs__"+name)
		file, err := parser.ParseFile(fileSet, newPath, r, parser.ParseComments)
		if err != nil {
			panic(err)
		}
		r.Close()

		files = append(files, file)
	}
	return jsFiles, files
}

// parserOriginalFiles loads and parses the original files to augment.
func parserOriginalFiles(pkg *PackageData, fileSet *token.FileSet) ([]*ast.File, error) {
	var files []*ast.File
	var errList compiler.ErrorList
	for _, name := range pkg.GoFiles {
		if !filepath.IsAbs(name) { // name might be absolute if specified directly. E.g., `gopherjs build /abs/file.go`.
			name = filepath.Join(pkg.Dir, name)
		}

		r, err := buildutil.OpenFile(pkg.bctx, name)
		if err != nil {
			return nil, err
		}

		file, err := parser.ParseFile(fileSet, name, r, parser.ParseComments)
		r.Close()
		if err != nil {
			if list, isList := err.(scanner.ErrorList); isList {
				if len(list) > 10 {
					list = append(list[:10], &scanner.Error{Pos: list[9].Pos, Msg: "too many errors"})
				}
				for _, entry := range list {
					errList = append(errList, entry)
				}
				continue
			}
			errList = append(errList, err)
			continue
		}

		files = append(files, file)
	}

	if errList != nil {
		return nil, errList
	}
	return files, nil
}

// augmentOverlayFile is the part of parseAndAugment that processes
// an overlay file AST to collect information such as compiler directives
// and perform any initial augmentation needed to the overlay.
func augmentOverlayFile(file *ast.File, overrides map[string]overrideInfo) {
	anyChange := false
	for i, decl := range file.Decls {
		purgeDecl := astutil.Purge(decl)
		switch d := decl.(type) {
		case *ast.FuncDecl:
			k := astutil.FuncKey(d)
			oi := overrideInfo{
				keepOriginal: astutil.KeepOriginal(d),
			}
			if astutil.OverrideSignature(d) {
				oi.overrideSignature = d
				purgeDecl = true
			}
			overrides[k] = oi
		case *ast.GenDecl:
			for j, spec := range d.Specs {
				purgeSpec := purgeDecl || astutil.Purge(spec)
				switch s := spec.(type) {
				case *ast.TypeSpec:
					overrides[s.Name.Name] = overrideInfo{
						purgeMethods: purgeSpec,
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						overrides[name.Name] = overrideInfo{}
					}
				}
				if purgeSpec {
					anyChange = true
					d.Specs[j] = nil
				}
			}
		}
		if purgeDecl {
			anyChange = true
			file.Decls[i] = nil
		}
	}
	if anyChange {
		finalizeRemovals(file)
		pruneImports(file)
	}
}

// augmentOriginalImports is the part of parseAndAugment that processes
// an original file AST to modify the imports for that file.
func augmentOriginalImports(importPath string, file *ast.File) {
	switch importPath {
	case "crypto/rand", "encoding/gob", "encoding/json", "expvar", "go/token", "log", "math/big", "math/rand", "regexp", "time":
		for _, spec := range file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			if path == "sync" {
				if spec.Name == nil {
					spec.Name = ast.NewIdent("sync")
				}
				spec.Path.Value = `"github.com/gopherjs/gopherjs/nosync"`
			}
		}
	}
}

// augmentOriginalFile is the part of parseAndAugment that processes an
// original file AST to augment the source code using the overrides from
// the overlay files.
func augmentOriginalFile(file *ast.File, overrides map[string]overrideInfo) {
	anyChange := false
	for i, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if info, ok := overrides[astutil.FuncKey(d)]; ok {
				anyChange = true
				removeFunc := true
				if info.keepOriginal {
					// Allow overridden function calls
					// The standard library implementation of foo() becomes _gopherjs_original_foo()
					d.Name.Name = "_gopherjs_original_" + d.Name.Name
					removeFunc = false
				}
				if overSig := info.overrideSignature; overSig != nil {
					d.Recv = overSig.Recv
					d.Type.TypeParams = overSig.Type.TypeParams
					d.Type.Params = overSig.Type.Params
					d.Type.Results = overSig.Type.Results
					removeFunc = false
				}
				if removeFunc {
					file.Decls[i] = nil
				}
			} else if recvKey := astutil.FuncReceiverKey(d); len(recvKey) > 0 {
				// check if the receiver has been purged, if so, remove the method too.
				if info, ok := overrides[recvKey]; ok && info.purgeMethods {
					anyChange = true
					file.Decls[i] = nil
				}
			}
		case *ast.GenDecl:
			for j, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if _, ok := overrides[s.Name.Name]; ok {
						anyChange = true
						d.Specs[j] = nil
					}
				case *ast.ValueSpec:
					if len(s.Names) == len(s.Values) {
						// multi-value context
						// e.g. var a, b = 2, foo[int]()
						// A removal will also remove the value which may be from a
						// function call. This allows us to remove unwanted statements.
						// However, if that call has a side effect which still needs
						// to be run, add the call into the overlay.
						for k, name := range s.Names {
							if _, ok := overrides[name.Name]; ok {
								anyChange = true
								s.Names[k] = nil
								s.Values[k] = nil
							}
						}
					} else {
						// single-value context
						// e.g. var a, b = foo[int]()
						// If a removal from the overlays makes all returned values unused,
						// then remove the function call as well. This allows us to stop
						// unwanted calls if needed. If that call has a side effect which
						// still needs to be run, add the call into the overlay.
						nameRemoved := false
						for _, name := range s.Names {
							if _, ok := overrides[name.Name]; ok {
								nameRemoved = true
								name.Name = `_`
							}
						}
						if nameRemoved {
							removeSpec := true
							for _, name := range s.Names {
								if name.Name != `_` {
									removeSpec = false
									break
								}
							}
							if removeSpec {
								anyChange = true
								d.Specs[j] = nil
							}
						}
					}
				}
			}
		}
	}
	if anyChange {
		finalizeRemovals(file)
		pruneImports(file)
	}
}

// isOnlyImports determines if this file is empty except for imports.
func isOnlyImports(file *ast.File) bool {
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			continue
		}

		// The decl was either a FuncDecl or a non-import GenDecl.
		return false
	}
	return true
}

// pruneImports will remove any unused imports from the file.
//
// This will not remove any dot (`.`) or blank (`_`) imports, unless
// there are no declarations or directives meaning that all the imports
// should be cleared.
// If the removal of code causes an import to be removed, the init's from that
// import may not be run anymore. If we still need to run an init for an import
// which is no longer used, add it to the overlay as a blank (`_`) import.
//
// This uses the given name or guesses at the name using the import path,
// meaning this doesn't work for packages which have a different package name
// from the path, including those paths which are versioned
// (e.g. `github.com/foo/bar/v2` where the package name is `bar`)
// or if the import is defined using a relative path (e.g. `./..`).
// Those cases don't exist in the native for Go, so we should only run
// this pruning when we have native overlays, but not for unknown packages.
func pruneImports(file *ast.File) {
	if isOnlyImports(file) && !astutil.HasDirectivePrefix(file, `//go:linkname `) {
		// The file is empty, remove all imports including any `.` or `_` imports.
		file.Imports = nil
		file.Decls = nil
		return
	}

	unused := make(map[string]int, len(file.Imports))
	for i, in := range file.Imports {
		if name := astutil.ImportName(in); len(name) > 0 {
			unused[name] = i
		}
	}

	// Remove "unused imports" for any import which is used.
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Obj == nil {
				delete(unused, id.Name)
			}
		}
		return len(unused) > 0
	})
	if len(unused) == 0 {
		return
	}

	// Remove "unused imports" for any import used for a directive.
	directiveImports := map[string]string{
		`unsafe`: `//go:linkname `,
		`embed`:  `//go:embed `,
	}
	for name, index := range unused {
		in := file.Imports[index]
		path, _ := strconv.Unquote(in.Path.Value)
		directivePrefix, hasPath := directiveImports[path]
		if hasPath && astutil.HasDirectivePrefix(file, directivePrefix) {
			// since the import is otherwise unused set the name to blank.
			in.Name = ast.NewIdent(`_`)
			delete(unused, name)
		}
	}
	if len(unused) == 0 {
		return
	}

	// Remove all unused import specifications
	isUnusedSpec := map[*ast.ImportSpec]bool{}
	for _, index := range unused {
		isUnusedSpec[file.Imports[index]] = true
	}
	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.GenDecl); ok {
			for i, spec := range d.Specs {
				if other, ok := spec.(*ast.ImportSpec); ok && isUnusedSpec[other] {
					d.Specs[i] = nil
				}
			}
		}
	}

	// Remove the unused import copies in the file
	for _, index := range unused {
		file.Imports[index] = nil
	}

	finalizeRemovals(file)
}

// finalizeRemovals fully removes any declaration, specification, imports
// that have been set to nil. This will also remove any unassociated comment
// groups, including the comments from removed code.
func finalizeRemovals(file *ast.File) {
	fileChanged := false
	for i, decl := range file.Decls {
		switch d := decl.(type) {
		case nil:
			fileChanged = true
		case *ast.GenDecl:
			declChanged := false
			for j, spec := range d.Specs {
				switch s := spec.(type) {
				case nil:
					declChanged = true
				case *ast.ValueSpec:
					specChanged := false
					for _, name := range s.Names {
						if name == nil {
							specChanged = true
							break
						}
					}
					if specChanged {
						s.Names = astutil.Squeeze(s.Names)
						s.Values = astutil.Squeeze(s.Values)
						if len(s.Names) == 0 {
							declChanged = true
							d.Specs[j] = nil
						}
					}
				}
			}
			if declChanged {
				d.Specs = astutil.Squeeze(d.Specs)
				if len(d.Specs) == 0 {
					fileChanged = true
					file.Decls[i] = nil
				}
			}
		}
	}
	if fileChanged {
		file.Decls = astutil.Squeeze(file.Decls)
	}

	file.Imports = astutil.Squeeze(file.Imports)

	file.Comments = nil // clear this first so ast.Inspect doesn't walk it.
	remComments := []*ast.CommentGroup{}
	ast.Inspect(file, func(n ast.Node) bool {
		if cg, ok := n.(*ast.CommentGroup); ok {
			remComments = append(remComments, cg)
		}
		return true
	})
	file.Comments = remComments
}
