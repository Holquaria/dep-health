package scanner

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"

	"dep-health/models"
)

// GoModScanner parses Go module go.mod manifest files.
//
// Both direct and indirect dependencies are included — indirect deps can still
// carry CVEs that affect the compiled binary.  Modules replaced with a local
// filesystem path (replace A => ../local) are excluded because they have no
// registry entry to look up.  Versioned replacements (replace A => B v1.2.3)
// are included under their original module path at the version stated in the
// require block.
type GoModScanner struct{}

func (s *GoModScanner) Name() string { return "go/go.mod" }

func (s *GoModScanner) Matches(path string) bool {
	return filepath.Base(path) == "go.mod"
}

func (s *GoModScanner) Parse(path string, repoURL string) ([]models.Dependency, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	f, err := modfile.Parse(path, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Collect modules replaced by a local filesystem path.
	// These cannot be resolved against the Go module proxy.
	localReplacements := make(map[string]bool, len(f.Replace))
	for _, r := range f.Replace {
		if r.New.Version == "" {
			// New.Path is a directory (e.g. "../local-pkg"), not a module path.
			localReplacements[r.Old.Path] = true
		}
	}

	deps := make([]models.Dependency, 0, len(f.Require))
	for _, req := range f.Require {
		if localReplacements[req.Mod.Path] {
			continue
		}
		deps = append(deps, models.Dependency{
			// Go module versions always carry the "v" prefix (e.g. "v1.2.3").
			// Masterminds/semver handles this transparently.
			Name:           req.Mod.Path,
			CurrentVersion: req.Mod.Version,
			Ecosystem:      "go",
			ManifestPath:   path,
			RepoURL:        repoURL,
		})
	}

	return deps, nil
}
