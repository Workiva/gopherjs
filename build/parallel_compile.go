package build

import (
	"fmt"
	"go/types"
	"runtime"

	"golang.org/x/sync/errgroup"

	"github.com/gopherjs/gopherjs/compiler"
	"github.com/gopherjs/gopherjs/compiler/sources"
)

// compilePackagesParallel compiles packages that are not already up to date
// concurrently. Packages are independent once PrepareAllSources has run.
func (s *Session) compilePackagesParallel(allSources []*sources.Sources, tContext *types.Context) error {
	archives := make([]*compiler.Archive, len(allSources))
	var g errgroup.Group
	g.SetLimit(runtime.GOMAXPROCS(0))
	for i, srcs := range allSources {
		if _, ok := s.UpToDateArchives[srcs.ImportPath]; ok {
			continue
		}
		i, srcs := i, srcs
		g.Go(func() error {
			archive, err := compiler.Compile(srcs, tContext, s.options.Minify)
			archives[i] = archive
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	for i, srcs := range allSources {
		if archives[i] == nil {
			continue
		}
		if s.options.Verbose {
			fmt.Println(srcs.ImportPath)
		}
		s.UpToDateArchives[srcs.ImportPath] = archives[i]
	}
	return nil
}
