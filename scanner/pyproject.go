package scanner

import (
	"fmt"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"dep-health/models"
)

// PyprojectScanner parses pyproject.toml files.
//
// Supported formats:
//   - PEP 621 (modern standard): [project] dependencies / [project.optional-dependencies]
//   - Poetry: [tool.poetry.dependencies] / [tool.poetry.dev-dependencies] /
//     [tool.poetry.group.<name>.dependencies]
type PyprojectScanner struct{}

func (s *PyprojectScanner) Name() string { return "pypi/pyproject.toml" }

func (s *PyprojectScanner) Matches(path string) bool {
	return filepath.Base(path) == "pyproject.toml"
}

// pyprojectTOML mirrors the sections of pyproject.toml we care about.
// Fields we don't need are ignored by the TOML decoder.
type pyprojectTOML struct {
	Project struct {
		Dependencies         []string            `toml:"dependencies"`
		OptionalDependencies map[string][]string `toml:"optional-dependencies"`
	} `toml:"project"`

	Tool struct {
		Poetry struct {
			// Values can be a version string ("^2.0") or an inline table
			// ({version = "^2.0", extras = [...]}).  We decode as interface{}
			// and handle both forms in poetryVersion().
			Dependencies    map[string]interface{} `toml:"dependencies"`
			DevDependencies map[string]interface{} `toml:"dev-dependencies"`
			Group           map[string]struct {
				Dependencies map[string]interface{} `toml:"dependencies"`
			} `toml:"group"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

func (s *PyprojectScanner) Parse(path string, repoURL string) ([]models.Dependency, error) {
	var cfg pyprojectTOML
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	var deps []models.Dependency
	add := func(name, version string) {
		v := cleanVersion(version)
		if v == "" || name == "" || name == "python" {
			return
		}
		deps = append(deps, models.Dependency{
			Name:           name,
			CurrentVersion: v,
			Ecosystem:      "pypi",
			ManifestPath:   path,
			RepoURL:        repoURL,
		})
	}

	// ── PEP 621 ──────────────────────────────────────────────────────────────
	// [project] dependencies = ["requests>=2.28.0", "flask==2.3.0"]
	for _, spec := range cfg.Project.Dependencies {
		name, version, ok := parseRequirement(spec)
		if ok {
			add(name, version)
		}
	}
	// [project.optional-dependencies] — include dev/test groups too.
	for _, specs := range cfg.Project.OptionalDependencies {
		for _, spec := range specs {
			name, version, ok := parseRequirement(spec)
			if ok {
				add(name, version)
			}
		}
	}

	// ── Poetry ────────────────────────────────────────────────────────────────
	// [tool.poetry.dependencies] requests = "^2.28.0"
	for name, val := range cfg.Tool.Poetry.Dependencies {
		if v := poetryVersion(val); v != "" {
			add(name, v)
		}
	}
	// [tool.poetry.dev-dependencies]
	for name, val := range cfg.Tool.Poetry.DevDependencies {
		if v := poetryVersion(val); v != "" {
			add(name, v)
		}
	}
	// [tool.poetry.group.<name>.dependencies]
	for _, group := range cfg.Tool.Poetry.Group {
		for name, val := range group.Dependencies {
			if v := poetryVersion(val); v != "" {
				add(name, v)
			}
		}
	}

	return deps, nil
}

// poetryVersion extracts a version string from a Poetry dependency value.
// Poetry allows two forms:
//
//	string form:  requests = "^2.28.0"
//	table form:   requests = {version = "^2.0", extras = ["security"]}
//
// Returns "" when the value is "*" (unconstrained) or unparseable.
func poetryVersion(val interface{}) string {
	switch v := val.(type) {
	case string:
		if v == "*" {
			return ""
		}
		return v
	case map[string]interface{}:
		if ver, ok := v["version"].(string); ok && ver != "*" {
			return ver
		}
	}
	return ""
}
