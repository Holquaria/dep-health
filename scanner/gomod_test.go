package scanner_test

import (
	"os"
	"path/filepath"
	"testing"

	"dep-health/scanner"
)

// fixture builds a go.mod file in a temp dir and returns its path.
func writeGoMod(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing go.mod: %v", err)
	}
	return path
}

func TestGoModScanner_Matches(t *testing.T) {
	s := &scanner.GoModScanner{}

	cases := []struct {
		path  string
		match bool
	}{
		{"go.mod", true},
		{"/some/repo/go.mod", true},
		{"go.sum", false},
		{"go.mod.bak", false},
		{"package.json", false},
	}
	for _, tc := range cases {
		got := s.Matches(tc.path)
		if got != tc.match {
			t.Errorf("Matches(%q) = %v, want %v", tc.path, got, tc.match)
		}
	}
}

func TestGoModScanner_ParseDirect(t *testing.T) {
	path := writeGoMod(t, `module github.com/example/myapp

go 1.22.0

require (
	github.com/spf13/cobra v1.8.0
	github.com/gin-gonic/gin v1.9.1
	golang.org/x/net v0.20.0
)
`)
	s := &scanner.GoModScanner{}
	deps, err := s.Parse(path, "https://github.com/example/myapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(deps) != 3 {
		t.Fatalf("expected 3 dependencies, got %d", len(deps))
	}

	byName := make(map[string]string, len(deps))
	for _, d := range deps {
		byName[d.Name] = d.CurrentVersion
	}

	want := map[string]string{
		"github.com/spf13/cobra":   "v1.8.0",
		"github.com/gin-gonic/gin": "v1.9.1",
		"golang.org/x/net":         "v0.20.0",
	}
	for name, wantVer := range want {
		gotVer, ok := byName[name]
		if !ok {
			t.Errorf("module %q not found in parsed deps", name)
			continue
		}
		if gotVer != wantVer {
			t.Errorf("module %q: want version %q, got %q", name, wantVer, gotVer)
		}
	}

	for _, d := range deps {
		if d.Ecosystem != "go" {
			t.Errorf("module %q: want ecosystem \"go\", got %q", d.Name, d.Ecosystem)
		}
		if d.ManifestPath != path {
			t.Errorf("module %q: want manifest path %q, got %q", d.Name, path, d.ManifestPath)
		}
		if d.RepoURL != "https://github.com/example/myapp" {
			t.Errorf("module %q: unexpected repo URL %q", d.Name, d.RepoURL)
		}
	}
}

func TestGoModScanner_IncludesIndirect(t *testing.T) {
	path := writeGoMod(t, `module github.com/example/myapp

go 1.22.0

require github.com/spf13/cobra v1.8.0

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)
`)
	s := &scanner.GoModScanner{}
	deps, err := s.Parse(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All three requires (direct + indirect) should be present.
	if len(deps) != 3 {
		t.Fatalf("expected 3 dependencies (direct + indirect), got %d", len(deps))
	}

	byName := make(map[string]bool, len(deps))
	for _, d := range deps {
		byName[d.Name] = true
	}
	for _, want := range []string{
		"github.com/spf13/cobra",
		"github.com/inconshreveable/mousetrap",
		"github.com/spf13/pflag",
	} {
		if !byName[want] {
			t.Errorf("expected %q to be present", want)
		}
	}
}

func TestGoModScanner_LocalReplacementExcluded(t *testing.T) {
	path := writeGoMod(t, `module github.com/example/myapp

go 1.22.0

require (
	github.com/spf13/cobra v1.8.0
	github.com/example/local-pkg v0.0.0-00010101000000-000000000000
)

replace github.com/example/local-pkg => ../local-pkg
`)
	s := &scanner.GoModScanner{}
	deps, err := s.Parse(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// local-pkg is replaced with a filesystem path — it must be excluded.
	for _, d := range deps {
		if d.Name == "github.com/example/local-pkg" {
			t.Errorf("locally-replaced module should be excluded but was found: %+v", d)
		}
	}

	if len(deps) != 1 {
		t.Errorf("expected 1 dep (cobra only), got %d", len(deps))
	}
}

func TestGoModScanner_VersionedReplacementIncluded(t *testing.T) {
	// Versioned replace (A => B v1.2.3) — the original module is still included
	// under its original path because the registry has an equivalent module.
	path := writeGoMod(t, `module github.com/example/myapp

go 1.22.0

require (
	github.com/spf13/cobra v1.8.0
	github.com/old/pkg v1.0.0
)

replace github.com/old/pkg v1.0.0 => github.com/new/pkg v1.0.1
`)
	s := &scanner.GoModScanner{}
	deps, err := s.Parse(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byName := make(map[string]string, len(deps))
	for _, d := range deps {
		byName[d.Name] = d.CurrentVersion
	}

	// The original module path and require-block version should be preserved.
	if ver, ok := byName["github.com/old/pkg"]; !ok {
		t.Error("versioned-replaced module should still be included")
	} else if ver != "v1.0.0" {
		t.Errorf("versioned-replaced module: want version v1.0.0, got %q", ver)
	}
}

func TestGoModScanner_MajorVersionSuffix(t *testing.T) {
	// Modules with /v2, /v3, … suffixes are valid Go modules and should parse correctly.
	path := writeGoMod(t, `module github.com/example/myapp

go 1.22.0

require (
	github.com/spf13/cobra v1.8.0
	github.com/Masterminds/semver/v3 v3.2.1
)
`)
	s := &scanner.GoModScanner{}
	deps, err := s.Parse(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byName := make(map[string]string, len(deps))
	for _, d := range deps {
		byName[d.Name] = d.CurrentVersion
	}

	if ver, ok := byName["github.com/Masterminds/semver/v3"]; !ok {
		t.Error("major-version module not found")
	} else if ver != "v3.2.1" {
		t.Errorf("major-version module: want v3.2.1, got %q", ver)
	}
}

func TestGoModScanner_InvalidGoMod(t *testing.T) {
	path := writeGoMod(t, `this is not a valid go.mod file !!!`)

	s := &scanner.GoModScanner{}
	_, err := s.Parse(path, "")
	if err == nil {
		t.Error("expected error for invalid go.mod, got nil")
	}
}

func TestGoModScanner_EmptyRequires(t *testing.T) {
	path := writeGoMod(t, `module github.com/example/myapp

go 1.22.0
`)
	s := &scanner.GoModScanner{}
	deps, err := s.Parse(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 dependencies, got %d", len(deps))
	}
}

func TestDiscover_GoMod(t *testing.T) {
	dir := t.TempDir()

	// Root go.mod.
	writeJSON(t, filepath.Join(dir, "go.mod"), `module github.com/example/root

go 1.22.0

require github.com/spf13/cobra v1.8.0
`)

	// Nested module (monorepo sub-module).
	subDir := filepath.Join(dir, "tools", "gen")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(subDir, "go.mod"), `module github.com/example/root/tools/gen

go 1.22.0

require golang.org/x/tools v0.18.0
`)

	// vendor directory must be skipped.
	vendorDir := filepath.Join(dir, "vendor", "github.com", "some", "dep")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(vendorDir, "go.mod"), `module github.com/some/dep

go 1.21.0

require github.com/should/not-appear v1.0.0
`)

	deps, err := scanner.Discover(dir, "", scanner.DefaultScanners())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the two root-level modules should appear; vendor is skipped.
	if len(deps) != 2 {
		t.Errorf("expected 2 dependencies (vendor excluded), got %d", len(deps))
		for _, d := range deps {
			t.Logf("  found: %s@%s from %s", d.Name, d.CurrentVersion, d.ManifestPath)
		}
	}
}
