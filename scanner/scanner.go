// Package scanner discovers dependency manifest files and parses them into
// a normalised []models.Dependency slice.
package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"dep-health/models"
)

// Scanner is the interface implemented by every manifest-file parser.
// Adding support for a new ecosystem (requirements.txt, go.mod, pom.xml, …)
// means writing a new type that satisfies this interface and registering it
// with DefaultScanners.
type Scanner interface {
	// Name returns a human-readable identifier for the scanner.
	Name() string
	// Matches reports whether this scanner can handle the file at path.
	Matches(path string) bool
	// Parse extracts dependencies from the manifest at path.
	// repoURL is attached to every returned Dependency for cross-repo tracking.
	Parse(path string, repoURL string) ([]models.Dependency, error)
}

// DefaultScanners returns the built-in set of manifest parsers.
func DefaultScanners() []Scanner {
	return []Scanner{
		&PackageJSONScanner{},
		&GoModScanner{},
		&RequirementsTxtScanner{},
	}
}

// Discover walks dir recursively, invoking every registered scanner against
// matching files.  Directories named node_modules, .git, and vendor are
// skipped to avoid false positives and excessive I/O.
func Discover(dir string, repoURL string, scanners []Scanner) ([]models.Dependency, error) {
	var deps []models.Dependency

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "node_modules", ".git", "vendor", ".venv", "__pycache__":
				return filepath.SkipDir
			}
		}
		for _, s := range scanners {
			if s.Matches(path) {
				found, parseErr := s.Parse(path, repoURL)
				if parseErr != nil {
					fmt.Fprintf(os.Stderr, "warning: parsing %s: %v\n", path, parseErr)
					continue
				}
				deps = append(deps, found...)
			}
		}
		return nil
	})

	return deps, err
}

// stripVersionPrefix matches leading semver range operators so they can be removed.
var stripVersionPrefix = regexp.MustCompile(`^[\^~>=<* ]+`)

// cleanVersion strips range operators from a dependency version string and
// returns the bare semver, or an empty string when the specifier is not
// representable as a single version (e.g. workspace:*, file:, link:).
func cleanVersion(v string) string {
	// Protocol-based specifiers have no comparable version.
	for _, pfx := range []string{"workspace:", "file:", "link:", "git+", "git:", "github:"} {
		if strings.HasPrefix(v, pfx) {
			return ""
		}
	}

	v = stripVersionPrefix.ReplaceAllString(v, "")
	// Range expressions like ">=1.0.0 <2.0.0" — take the lower bound only.
	if idx := strings.IndexAny(v, " |"); idx != -1 {
		v = v[:idx]
	}
	return strings.TrimSpace(v)
}

// ── package.json (npm) ────────────────────────────────────────────────────────

// PackageJSONScanner parses npm package.json manifest files.
type PackageJSONScanner struct{}

func (s *PackageJSONScanner) Name() string { return "npm/package.json" }

func (s *PackageJSONScanner) Matches(path string) bool {
	return filepath.Base(path) == "package.json"
}

type packageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func (s *PackageJSONScanner) Parse(path string, repoURL string) ([]models.Dependency, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	var deps []models.Dependency

	add := func(name, rawVersion string) {
		v := cleanVersion(rawVersion)
		if v == "" || name == "" {
			return
		}
		deps = append(deps, models.Dependency{
			Name:           name,
			CurrentVersion: v,
			Ecosystem:      "npm",
			ManifestPath:   path,
			RepoURL:        repoURL,
		})
	}

	for name, ver := range pkg.Dependencies {
		add(name, ver)
	}
	for name, ver := range pkg.DevDependencies {
		add(name, ver)
	}

	return deps, nil
}
