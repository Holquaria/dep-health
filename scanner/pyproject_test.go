package scanner_test

import (
	"os"
	"path/filepath"
	"testing"

	"dep-health/scanner"
)

var pyprojectScanner = &scanner.PyprojectScanner{}

func writePyproject(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pyproject.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing pyproject.toml: %v", err)
	}
	return path
}

func TestPyprojectScanner_Matches(t *testing.T) {
	cases := []struct {
		path  string
		match bool
	}{
		{"pyproject.toml", true},
		{"/some/path/pyproject.toml", true},
		{"package.toml", false},
		{"pyproject.yaml", false},
		{"setup.cfg", false},
	}
	for _, tc := range cases {
		got := pyprojectScanner.Matches(tc.path)
		if got != tc.match {
			t.Errorf("Matches(%q) = %v, want %v", tc.path, got, tc.match)
		}
	}
}

// ── PEP 621 ──────────────────────────────────────────────────────────────────

func TestPyprojectScanner_PEP621_Dependencies(t *testing.T) {
	content := `
[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[project]
name = "myapp"
version = "1.0.0"
dependencies = [
    "requests>=2.28.0",
    "flask==2.3.0",
    "click>=8.0",
]
`
	path := writePyproject(t, content)
	deps, err := pyprojectScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d: %v", len(deps), deps)
	}
	byName := map[string]string{}
	for _, d := range deps {
		byName[d.Name] = d.CurrentVersion
		if d.Ecosystem != "pypi" {
			t.Errorf("%s: ecosystem = %q, want 'pypi'", d.Name, d.Ecosystem)
		}
	}
	if byName["requests"] != "2.28.0" {
		t.Errorf("requests = %q, want '2.28.0'", byName["requests"])
	}
	if byName["flask"] != "2.3.0" {
		t.Errorf("flask = %q, want '2.3.0'", byName["flask"])
	}
}

func TestPyprojectScanner_PEP621_OptionalDependencies(t *testing.T) {
	content := `
[project]
name = "myapp"
dependencies = ["requests>=2.28.0"]

[project.optional-dependencies]
dev = [
    "pytest>=7.0",
    "black>=23.0",
]
docs = [
    "sphinx==6.2.0",
]
`
	path := writePyproject(t, content)
	deps, err := pyprojectScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 4 {
		t.Fatalf("expected 4 deps (1 main + 3 optional), got %d: %v", len(deps), deps)
	}
}

func TestPyprojectScanner_PEP621_ExtrasInSpecifier(t *testing.T) {
	content := `
[project]
name = "myapp"
dependencies = [
    "requests[security]>=2.28.0",
]
`
	path := writePyproject(t, content)
	deps, err := pyprojectScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Name != "requests" {
		t.Errorf("extras not stripped: name = %q, want 'requests'", deps[0].Name)
	}
	if deps[0].CurrentVersion != "2.28.0" {
		t.Errorf("version = %q, want '2.28.0'", deps[0].CurrentVersion)
	}
}

// ── Poetry ───────────────────────────────────────────────────────────────────

func TestPyprojectScanner_Poetry_Dependencies(t *testing.T) {
	content := `
[tool.poetry]
name = "myapp"
version = "1.0.0"

[tool.poetry.dependencies]
python = "^3.8"
requests = "^2.28.0"
flask = "^2.3.0"
click = {version = "^8.0", extras = ["dev"]}

[tool.poetry.dev-dependencies]
pytest = "^7.0"
`
	path := writePyproject(t, content)
	deps, err := pyprojectScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	byName := map[string]string{}
	for _, d := range deps {
		byName[d.Name] = d.CurrentVersion
	}

	// python should be filtered out
	if _, ok := byName["python"]; ok {
		t.Error("'python' should be excluded from deps")
	}
	if byName["requests"] != "2.28.0" {
		t.Errorf("requests = %q, want '2.28.0'", byName["requests"])
	}
	if byName["flask"] != "2.3.0" {
		t.Errorf("flask = %q, want '2.3.0'", byName["flask"])
	}
	// click uses table form with version key
	if byName["click"] != "8.0" {
		t.Errorf("click = %q, want '8.0'", byName["click"])
	}
	if byName["pytest"] != "7.0" {
		t.Errorf("pytest = %q, want '7.0'", byName["pytest"])
	}
}

func TestPyprojectScanner_Poetry_GroupDependencies(t *testing.T) {
	content := `
[tool.poetry.dependencies]
requests = "^2.28.0"

[tool.poetry.group.dev.dependencies]
pytest = "^7.0"
black = "^23.0"

[tool.poetry.group.docs.dependencies]
sphinx = "^6.2.0"
`
	path := writePyproject(t, content)
	deps, err := pyprojectScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 4 {
		t.Fatalf("expected 4 deps (1 main + 3 groups), got %d: %v", len(deps), deps)
	}
}

func TestPyprojectScanner_Poetry_WildcardSkipped(t *testing.T) {
	content := `
[tool.poetry.dependencies]
python = "*"
requests = "^2.28.0"
`
	path := writePyproject(t, content)
	deps, err := pyprojectScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// python="*" skipped (wildcard), python also excluded by name filter
	for _, d := range deps {
		if d.Name == "python" {
			t.Error("'python' with wildcard version should be excluded")
		}
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep (requests), got %d: %v", len(deps), deps)
	}
}

func TestPyprojectScanner_Empty(t *testing.T) {
	content := `
[build-system]
requires = ["setuptools"]
build-backend = "setuptools.build_meta"
`
	path := writePyproject(t, content)
	deps, err := pyprojectScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(deps))
	}
}

func TestPyprojectScanner_RepoURL(t *testing.T) {
	content := `
[project]
dependencies = ["flask==2.3.0"]
`
	path := writePyproject(t, content)
	deps, err := pyprojectScanner.Parse(path, "https://gitlab.company.com/org/repo")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) == 0 {
		t.Fatal("expected 1 dep")
	}
	if deps[0].RepoURL != "https://gitlab.company.com/org/repo" {
		t.Errorf("RepoURL = %q", deps[0].RepoURL)
	}
}
