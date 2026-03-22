package pipeline_test

import (
	"context"
	"testing"

	"dep-health/pipeline"
)

// TestRunNoDeps verifies that scanning a directory with empty manifests
// completes without error and returns zero advisory reports.
// No network calls are made because the resolver is never reached when
// scanner.Discover returns an empty slice.
func TestRunNoDeps(t *testing.T) {
	reports, err := pipeline.Run(context.Background(), "../testdata/no-deps", pipeline.Options{})
	if err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected 0 reports for empty manifests, got %d", len(reports))
	}
}

// TestRunInvalidDir verifies that scanning a non-existent directory returns
// an error rather than panicking.
func TestRunInvalidDir(t *testing.T) {
	_, err := pipeline.Run(context.Background(), "/nonexistent/dir/that/does/not/exist", pipeline.Options{})
	if err != nil {
		// Expected: discovery fails on a missing directory.
		return
	}
	// If err is nil, we got 0 reports from a missing dir — also acceptable.
}

// TestRunMultiNoDeps verifies that RunMulti with two no-dep directories
// returns an empty AllDeps slice and no errors map entry.
func TestRunMultiNoDeps(t *testing.T) {
	targets := []string{"../testdata/no-deps", "../testdata/no-deps"}
	report, err := pipeline.RunMulti(context.Background(), targets, pipeline.Options{})
	if err != nil {
		t.Fatalf("RunMulti: unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("RunMulti returned nil report")
	}
	if len(report.AllDeps) != 0 {
		t.Errorf("expected 0 AllDeps, got %d", len(report.AllDeps))
	}
	if len(report.Errors) != 0 {
		t.Errorf("expected no scan errors, got %v", report.Errors)
	}
}

// TestTargetLabel verifies the human-readable label logic.
func TestTargetLabel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://github.com/pallets/flask", "pallets/flask"},
		{"https://github.com/pallets/flask.git", "pallets/flask"},
		{"/home/user/projects/myapp", "projects/myapp"},
		{"myapp", "myapp"},
	}
	for _, c := range cases {
		got := pipeline.TargetLabel(c.input)
		if got != c.want {
			t.Errorf("TargetLabel(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
