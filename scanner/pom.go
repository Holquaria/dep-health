package scanner

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dep-health/models"
)

// PomScanner parses Maven pom.xml files.
//
// Handles:
//   - <dependencies> — runtime, compile, test, provided scopes
//   - <dependencyManagement><dependencies> — BOM entries and imported BOMs
//   - <parent> — treated as a versioned dependency (spring-boot-starter-parent, etc.)
//   - ${property.name} version references resolved from <properties>
//
// Skips: system-scoped dependencies (local JARs with <systemPath>), and
// dependencies with no version that cannot be resolved from properties.
// Java 8 projects (java.version=1.8) are handled identically to later versions —
// the pom.xml format is determined by Maven, not the Java source version.
type PomScanner struct{}

func (s *PomScanner) Name() string { return "maven/pom.xml" }

func (s *PomScanner) Matches(path string) bool {
	return filepath.Base(path) == "pom.xml"
}

// ── XML model ────────────────────────────────────────────────────────────────

type pomXML struct {
	XMLName xml.Name `xml:"project"`

	// <parent> — a versioned Maven artifact that acts as the project's parent POM.
	Parent pomCoord `xml:"parent"`

	// <properties> contains key-value pairs declared as child elements with
	// arbitrary tag names, e.g. <spring.version>5.3.23</spring.version>.
	// encoding/xml's ",any" trick captures them all generically.
	Properties pomProperties `xml:"properties"`

	Dependencies struct {
		Deps []pomDep `xml:"dependency"`
	} `xml:"dependencies"`

	DependencyManagement struct {
		Dependencies struct {
			Deps []pomDep `xml:"dependency"`
		} `xml:"dependencies"`
	} `xml:"dependencyManagement"`
}

type pomCoord struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type pomDep struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Type       string `xml:"type"`
	Optional   string `xml:"optional"`
}

// pomProperties uses encoding/xml's any-element trick to collect all children
// of <properties> as a generic slice, then exposes them as a map.
type pomProperties struct {
	Entries []struct {
		XMLName xml.Name
		Value   string `xml:",chardata"`
	} `xml:",any"`
}

func (p pomProperties) toMap() map[string]string {
	m := make(map[string]string, len(p.Entries))
	for _, e := range p.Entries {
		key := e.XMLName.Local
		val := strings.TrimSpace(e.Value)
		if key != "" && val != "" {
			m[key] = val
		}
	}
	return m
}

// ── Parser ───────────────────────────────────────────────────────────────────

func (s *PomScanner) Parse(path string, repoURL string) ([]models.Dependency, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var pom pomXML
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	props := pom.Properties.toMap()

	var deps []models.Dependency

	addDep := func(d pomDep) {
		// system scope means a local JAR with a <systemPath> — not in any registry.
		if strings.EqualFold(d.Scope, "system") {
			return
		}
		// BOM imports in dependencyManagement have type=pom and scope=import —
		// include these because upgrading a BOM version matters.

		g := resolveProperty(d.GroupID, props)
		a := resolveProperty(d.ArtifactID, props)
		v := resolveProperty(d.Version, props)

		if g == "" || a == "" || v == "" {
			// No usable version (inherited from parent BOM, not locally declared).
			return
		}

		deps = append(deps, models.Dependency{
			Name:           g + ":" + a,
			CurrentVersion: v,
			Ecosystem:      "maven",
			ManifestPath:   path,
			RepoURL:        repoURL,
		})
	}

	// <parent> — treat as a first-class dependency so stale parent POMs surface.
	if pom.Parent.GroupID != "" && pom.Parent.ArtifactID != "" && pom.Parent.Version != "" {
		v := resolveProperty(pom.Parent.Version, props)
		if v != "" {
			deps = append(deps, models.Dependency{
				Name:           pom.Parent.GroupID + ":" + pom.Parent.ArtifactID,
				CurrentVersion: v,
				Ecosystem:      "maven",
				ManifestPath:   path,
				RepoURL:        repoURL,
			})
		}
	}

	for _, d := range pom.Dependencies.Deps {
		addDep(d)
	}
	for _, d := range pom.DependencyManagement.Dependencies.Deps {
		addDep(d)
	}

	return deps, nil
}

// resolveProperty replaces a ${property.name} token with its value from props.
// Returns the raw string unchanged when it is not a property reference.
// Returns "" when the reference exists but has no value in props.
func resolveProperty(s string, props map[string]string) string {
	if !strings.HasPrefix(s, "${") || !strings.HasSuffix(s, "}") {
		return s
	}
	key := s[2 : len(s)-1]
	return props[key] // empty string when not found
}
