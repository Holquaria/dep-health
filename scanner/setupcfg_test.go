package scanner_test

import (
	"os"
	"path/filepath"
	"testing"

	"dep-health/scanner"
)

var setupCfgScanner = &scanner.SetupCfgScanner{}

func writeSetupCfg(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "setup.cfg")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing setup.cfg: %v", err)
	}
	return path
}

func TestSetupCfgScanner_Matches(t *testing.T) {
	cases := []struct {
		path  string
		match bool
	}{
		{"setup.cfg", true},
		{"/some/path/setup.cfg", true},
		{"setup.py", false},
		{"pyproject.toml", false},
		{"mysetup.cfg", false},
	}
	for _, tc := range cases {
		got := setupCfgScanner.Matches(tc.path)
		if got != tc.match {
			t.Errorf("Matches(%q) = %v, want %v", tc.path, got, tc.match)
		}
	}
}

func TestSetupCfgScanner_InstallRequires_Multiline(t *testing.T) {
	content := `
[metadata]
name = myapp
version = 1.0.0

[options]
python_requires = >=3.8
install_requires =
    requests>=2.28.0
    flask==2.3.0
    click>=8.0
`
	path := writeSetupCfg(t, content)
	deps, err := setupCfgScanner.Parse(path, "")
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
			t.Errorf("dep %s: ecosystem = %q, want 'pypi'", d.Name, d.Ecosystem)
		}
	}
	if byName["requests"] != "2.28.0" {
		t.Errorf("requests version = %q, want '2.28.0'", byName["requests"])
	}
	if byName["flask"] != "2.3.0" {
		t.Errorf("flask version = %q, want '2.3.0'", byName["flask"])
	}
}

func TestSetupCfgScanner_ExtrasRequire(t *testing.T) {
	content := `
[options]
install_requires =
    requests>=2.28.0

[options.extras_require]
dev =
    pytest>=7.0
    black>=23.0
docs =
    sphinx==6.2.0
`
	path := writeSetupCfg(t, content)
	deps, err := setupCfgScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 4 {
		t.Fatalf("expected 4 deps (1 install + 3 extras), got %d: %v", len(deps), deps)
	}
}

func TestSetupCfgScanner_InlineComment_Stripped(t *testing.T) {
	content := `
[options]
install_requires =
    requests>=2.28.0  # HTTP library
    flask==2.3.0      # web framework
`
	path := writeSetupCfg(t, content)
	deps, err := setupCfgScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}
}

func TestSetupCfgScanner_BareNameSkipped(t *testing.T) {
	content := `
[options]
install_requires =
    requests
    flask==2.3.0
`
	path := writeSetupCfg(t, content)
	deps, err := setupCfgScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// bare "requests" has no version — should be skipped
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep (bare name skipped), got %d: %v", len(deps), deps)
	}
	if deps[0].Name != "flask" {
		t.Errorf("name = %q, want 'flask'", deps[0].Name)
	}
}

func TestSetupCfgScanner_NoInstallRequires(t *testing.T) {
	content := `
[metadata]
name = myapp
version = 1.0.0

[options]
packages = find:
`
	path := writeSetupCfg(t, content)
	deps, err := setupCfgScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(deps))
	}
}

func TestSetupCfgScanner_RepoURL(t *testing.T) {
	content := `
[options]
install_requires =
    flask==2.3.0
`
	path := writeSetupCfg(t, content)
	deps, err := setupCfgScanner.Parse(path, "https://gitlab.company.com/org/repo")
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
