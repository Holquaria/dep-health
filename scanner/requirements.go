package scanner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dep-health/models"
)

// RequirementsTxtScanner parses Python requirements.txt files into dependencies.
// It handles the common formats: name==version, name>=version, name~=version,
// extras (name[extra]==version), environment markers (name==version; python_version>='3'),
// and comments.  Only dependencies with a pinned or minimum version specifier
// are included; bare names without versions are skipped.
type RequirementsTxtScanner struct{}

func (s *RequirementsTxtScanner) Name() string { return "pypi/requirements.txt" }

// Matches accepts requirements.txt and files that start with "requirements-"
// and end with ".txt" (e.g. requirements-dev.txt, requirements-test.txt).
func (s *RequirementsTxtScanner) Matches(path string) bool {
	base := filepath.Base(path)
	return base == "requirements.txt" ||
		(strings.HasPrefix(base, "requirements-") && strings.HasSuffix(base, ".txt"))
}

// Parse reads a requirements file line by line and returns a Dependency for
// each line that specifies a package with a version constraint.
func (s *RequirementsTxtScanner) Parse(path string, repoURL string) ([]models.Dependency, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	var deps []models.Dependency
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines, comments, and pip options (e.g. -r, -c, --index-url).
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}

		// Strip inline comments.
		if idx := strings.Index(line, " #"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}

		// Strip environment markers (everything after ';').
		if idx := strings.Index(line, ";"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}

		// Strip extras: name[extra1,extra2]==version → name==version.
		name, version, ok := parseRequirement(line)
		if !ok || version == "" {
			continue
		}

		deps = append(deps, models.Dependency{
			Name:           name,
			CurrentVersion: version,
			Ecosystem:      "pypi",
			ManifestPath:   path,
			RepoURL:        repoURL,
		})
	}

	if err := scanner.Err(); err != nil {
		return deps, fmt.Errorf("scanning %s: %w", path, err)
	}

	return deps, nil
}

// parseRequirement extracts the package name and version from a single
// requirement line.  Returns ok=false when the line cannot be parsed.
//
// Supported specifiers: ==, >=, ~=, <=, !=, >  (version is the right-hand side
// of the first specifier found).
func parseRequirement(line string) (name, version string, ok bool) {
	// Find the first version operator.
	operators := []string{"==", "~=", ">=", "<=", "!=", ">", "<"}
	opIdx := -1
	opLen := 0
	for _, op := range operators {
		if idx := strings.Index(line, op); idx != -1 {
			if opIdx == -1 || idx < opIdx {
				opIdx = idx
				opLen = len(op)
			}
		}
	}

	if opIdx == -1 {
		// No version specifier — skip (e.g. bare "requests").
		return "", "", false
	}

	rawName := strings.TrimSpace(line[:opIdx])
	rawVersion := strings.TrimSpace(line[opIdx+opLen:])

	// Strip extras from name: requests[security] → requests.
	if idx := strings.Index(rawName, "["); idx != -1 {
		rawName = strings.TrimSpace(rawName[:idx])
	}

	// A version constraint may list multiple specifiers separated by commas
	// (e.g. ">=1.0,<2.0"). Take the first bare version number.
	if idx := strings.Index(rawVersion, ","); idx != -1 {
		rawVersion = strings.TrimSpace(rawVersion[:idx])
	}

	// Strip any remaining operator prefix from the version value
	// (can happen with ">1.0" → rawVersion starts with the number already,
	// but ">=" consumed only ">=", so rawVersion is "1.0.0<2" after comma strip).
	rawVersion = strings.TrimLeft(rawVersion, "<>=!~")

	if rawName == "" || rawVersion == "" {
		return "", "", false
	}

	return rawName, rawVersion, true
}
