package scanner_test

import (
	"os"
	"path/filepath"
	"testing"

	"dep-health/scanner"
)

func TestPackageJSONScanner_Matches(t *testing.T) {
	s := &scanner.PackageJSONScanner{}

	cases := []struct {
		path  string
		match bool
	}{
		{"package.json", true},
		{"/some/repo/package.json", true},
		{"/repo/node_modules/lodash/package.json", true}, // Matches is path-agnostic; Discover skips node_modules
		{"requirements.txt", false},
		{"go.mod", false},
		{"package.json.bak", false},
	}

	for _, tc := range cases {
		got := s.Matches(tc.path)
		if got != tc.match {
			t.Errorf("Matches(%q) = %v, want %v", tc.path, got, tc.match)
		}
	}
}

func TestPackageJSONScanner_Parse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")

	content := `{
		"name": "my-app",
		"version": "1.0.0",
		"dependencies": {
			"lodash":  "^4.17.11",
			"express": "~4.18.0"
		},
		"devDependencies": {
			"jest":       "^29.0.0",
			"typescript": ">=4.9.0"
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing package.json: %v", err)
	}

	s := &scanner.PackageJSONScanner{}
	deps, err := s.Parse(path, "https://github.com/example/my-app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(deps) != 4 {
		t.Fatalf("expected 4 dependencies, got %d", len(deps))
	}

	byName := make(map[string]string, len(deps))
	for _, d := range deps {
		byName[d.Name] = d.CurrentVersion
	}

	wantVersions := map[string]string{
		"lodash":     "4.17.11",
		"express":    "4.18.0",
		"jest":       "29.0.0",
		"typescript": "4.9.0",
	}
	for pkg, want := range wantVersions {
		got, ok := byName[pkg]
		if !ok {
			t.Errorf("package %q not found in parsed deps", pkg)
			continue
		}
		if got != want {
			t.Errorf("package %q: want version %q, got %q", pkg, want, got)
		}
	}

	for _, d := range deps {
		if d.Ecosystem != "npm" {
			t.Errorf("package %q: want ecosystem \"npm\", got %q", d.Name, d.Ecosystem)
		}
		if d.ManifestPath != path {
			t.Errorf("package %q: want manifest path %q, got %q", d.Name, path, d.ManifestPath)
		}
		if d.RepoURL != "https://github.com/example/my-app" {
			t.Errorf("package %q: unexpected repo URL %q", d.Name, d.RepoURL)
		}
	}
}

func TestPackageJSONScanner_VersionPrefixStripping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")

	content := `{
		"dependencies": {
			"a": "^1.2.3",
			"b": "~2.0.0",
			"c": ">=3.0.0",
			"d": "4.0.0",
			"e": "*",
			"f": "workspace:*",
			"g": "file:../local-pkg"
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &scanner.PackageJSONScanner{}
	deps, err := s.Parse(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byName := make(map[string]string, len(deps))
	for _, d := range deps {
		byName[d.Name] = d.CurrentVersion
	}

	wantVersions := map[string]string{
		"a": "1.2.3",
		"b": "2.0.0",
		"c": "3.0.0",
		"d": "4.0.0",
	}
	for pkg, want := range wantVersions {
		got, ok := byName[pkg]
		if !ok {
			t.Errorf("package %q not found", pkg)
			continue
		}
		if got != want {
			t.Errorf("package %q: want %q, got %q", pkg, want, got)
		}
	}

	// Wildcard and protocol-based specifiers should be excluded.
	for _, excluded := range []string{"e", "f", "g"} {
		if _, ok := byName[excluded]; ok {
			t.Errorf("package %q with non-pinnable version should be excluded", excluded)
		}
	}
}

func TestDiscover_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()

	// Root manifest.
	writeJSON(t, filepath.Join(dir, "package.json"), `{"dependencies":{"react":"^18.0.0"}}`)

	// Nested manifest inside a sub-package (should be found).
	sub := filepath.Join(dir, "packages", "core")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(sub, "package.json"), `{"dependencies":{"lodash":"^4.0.0"}}`)

	// Manifest inside node_modules (must be skipped).
	nm := filepath.Join(dir, "node_modules", "some-pkg")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(nm, "package.json"), `{"dependencies":{"ignored":"1.0.0"}}`)

	deps, err := scanner.Discover(dir, "", scanner.DefaultScanners())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(deps) != 2 {
		t.Errorf("expected 2 dependencies (node_modules excluded), got %d", len(deps))
		for _, d := range deps {
			t.Logf("  found: %s@%s from %s", d.Name, d.CurrentVersion, d.ManifestPath)
		}
	}
}

func TestPackageJSONScanner_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")

	if err := os.WriteFile(path, []byte(`{ this is not valid json`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &scanner.PackageJSONScanner{}
	_, err := s.Parse(path, "")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// writeJSON is a test helper that writes a JSON string to path.
func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
