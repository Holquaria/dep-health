package scanner_test

import (
	"os"
	"path/filepath"
	"testing"

	"dep-health/scanner"
)

func writeRequirements(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "requirements.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp requirements.txt: %v", err)
	}
	return path
}

var reqScanner = &scanner.RequirementsTxtScanner{}

func TestRequirementsScanner_Name(t *testing.T) {
	if got := reqScanner.Name(); got == "" {
		t.Error("Name() returned empty string")
	}
}

func TestRequirementsScanner_Matches(t *testing.T) {
	cases := []struct {
		path  string
		match bool
	}{
		{"requirements.txt", true},
		{"requirements-dev.txt", true},
		{"requirements-test.txt", true},
		{"prod-requirements.txt", false},
		{"go.mod", false},
		{"package.json", false},
		{"requirements.lock", false},
	}

	for _, tc := range cases {
		got := reqScanner.Matches(tc.path)
		if got != tc.match {
			t.Errorf("Matches(%q) = %v, want %v", tc.path, got, tc.match)
		}
	}
}

func TestRequirementsScanner_PinnedVersion(t *testing.T) {
	path := writeRequirements(t, "requests==2.28.0\n")
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Name != "requests" {
		t.Errorf("name = %q, want 'requests'", deps[0].Name)
	}
	if deps[0].CurrentVersion != "2.28.0" {
		t.Errorf("version = %q, want '2.28.0'", deps[0].CurrentVersion)
	}
	if deps[0].Ecosystem != "pypi" {
		t.Errorf("ecosystem = %q, want 'pypi'", deps[0].Ecosystem)
	}
}

func TestRequirementsScanner_MinVersionSpecifier(t *testing.T) {
	path := writeRequirements(t, "flask>=2.0.0\n")
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].CurrentVersion != "2.0.0" {
		t.Errorf("version = %q, want '2.0.0'", deps[0].CurrentVersion)
	}
}

func TestRequirementsScanner_CompatibleRelease(t *testing.T) {
	path := writeRequirements(t, "Django~=3.2.0\n")
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Name != "Django" {
		t.Errorf("name = %q, want 'Django'", deps[0].Name)
	}
	if deps[0].CurrentVersion != "3.2.0" {
		t.Errorf("version = %q, want '3.2.0'", deps[0].CurrentVersion)
	}
}

func TestRequirementsScanner_ExtrasStripped(t *testing.T) {
	path := writeRequirements(t, "requests[security]==2.28.0\n")
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Name != "requests" {
		t.Errorf("extras not stripped: name = %q, want 'requests'", deps[0].Name)
	}
}

func TestRequirementsScanner_EnvMarkerStripped(t *testing.T) {
	path := writeRequirements(t, "pywin32==305; sys_platform=='win32'\n")
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].Name != "pywin32" {
		t.Errorf("name = %q, want 'pywin32'", deps[0].Name)
	}
	if deps[0].CurrentVersion != "305" {
		t.Errorf("version = %q, want '305'", deps[0].CurrentVersion)
	}
}

func TestRequirementsScanner_CommentsSkipped(t *testing.T) {
	content := `# This is a comment
requests==2.28.0
# another comment
flask>=2.0.0
`
	path := writeRequirements(t, content)
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 2 {
		t.Errorf("expected 2 deps, got %d", len(deps))
	}
}

func TestRequirementsScanner_InlineCommentStripped(t *testing.T) {
	path := writeRequirements(t, "requests==2.28.0  # pinned for security\n")
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].CurrentVersion != "2.28.0" {
		t.Errorf("version = %q, want '2.28.0'", deps[0].CurrentVersion)
	}
}

func TestRequirementsScanner_BareName_Skipped(t *testing.T) {
	path := writeRequirements(t, "requests\n")
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for bare name, got %d", len(deps))
	}
}

func TestRequirementsScanner_PipOptionsSkipped(t *testing.T) {
	content := `-r base.txt
--index-url https://pypi.org/simple/
requests==2.28.0
`
	path := writeRequirements(t, content)
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 1 {
		t.Errorf("expected 1 dep (options skipped), got %d", len(deps))
	}
}

func TestRequirementsScanner_MultiplePackages(t *testing.T) {
	content := `numpy==1.24.0
pandas>=1.5.0
scipy~=1.10.0
matplotlib==3.7.1
`
	path := writeRequirements(t, content)
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 4 {
		t.Errorf("expected 4 deps, got %d: %v", len(deps), deps)
	}
}

func TestRequirementsScanner_Empty(t *testing.T) {
	path := writeRequirements(t, "")
	deps, err := reqScanner.Parse(path, "")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps for empty file, got %d", len(deps))
	}
}

func TestRequirementsScanner_RepoURL(t *testing.T) {
	path := writeRequirements(t, "requests==2.28.0\n")
	deps, err := reqScanner.Parse(path, "https://github.com/org/repo")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if deps[0].RepoURL != "https://github.com/org/repo" {
		t.Errorf("RepoURL = %q, want 'https://github.com/org/repo'", deps[0].RepoURL)
	}
}
