package scanner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dep-health/models"
)

// SetupCfgScanner parses setup.cfg files used by setuptools-based Python projects.
//
// Reads [options] install_requires and [options.extras_require] sections,
// both of which list PEP 508 dependency specifiers (same format as requirements.txt).
type SetupCfgScanner struct{}

func (s *SetupCfgScanner) Name() string { return "pypi/setup.cfg" }

func (s *SetupCfgScanner) Matches(path string) bool {
	return filepath.Base(path) == "setup.cfg"
}

func (s *SetupCfgScanner) Parse(path string, repoURL string) ([]models.Dependency, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	var (
		deps          []models.Dependency
		inInstallReqs bool
		inExtrasReq   bool // any subsection of [options.extras_require]
	)

	add := func(spec string) {
		spec = strings.TrimSpace(spec)
		if spec == "" || strings.HasPrefix(spec, "#") {
			return
		}
		name, version, ok := parseRequirement(spec)
		if !ok || version == "" {
			return
		}
		deps = append(deps, models.Dependency{
			Name:           name,
			CurrentVersion: version,
			Ecosystem:      "pypi",
			ManifestPath:   path,
			RepoURL:        repoURL,
		})
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Section header detection.
		if strings.HasPrefix(trimmed, "[") {
			section := strings.ToLower(strings.Trim(trimmed, "[]"))
			inInstallReqs = section == "options" // will be set below on key
			inExtrasReq = strings.HasPrefix(section, "options.extras_require")
			// Reset install_requires tracking when entering a new section.
			if section != "options" {
				inInstallReqs = false
			}
			continue
		}

		// Within [options], look for install_requires key.
		if strings.HasPrefix(trimmed, "install_requires") && strings.Contains(trimmed, "=") {
			inInstallReqs = true
			// Value may be on the same line: install_requires = requests>=2.0
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				if val != "" {
					add(val)
				}
			}
			continue
		}

		// Continuation lines (indented) under install_requires or extras_require.
		if (inInstallReqs || inExtrasReq) && (strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " ")) {
			// Strip inline comments before adding.
			spec := trimmed
			if idx := strings.Index(spec, " #"); idx != -1 {
				spec = strings.TrimSpace(spec[:idx])
			}
			if spec != "" {
				add(spec)
			}
			continue
		}

		// A non-indented non-section line resets continuation state.
		if trimmed != "" && !strings.HasPrefix(trimmed, "[") {
			// Check if this is a different key in [options] — stop tracking install_requires.
			if inInstallReqs && strings.Contains(trimmed, "=") && !strings.HasPrefix(trimmed, "install_requires") {
				inInstallReqs = false
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return deps, fmt.Errorf("scanning %s: %w", path, err)
	}

	return deps, nil
}
