package advisor_test

import (
	"context"
	"strings"
	"testing"

	"dep-health/advisor"
	"dep-health/models"
)

// makeScored is a helper that builds a minimal ScoredDependency for tests.
func makeScored(name, ecosystem, current, latest, gap string, cves int) models.ScoredDependency {
	var vulns []models.Vulnerability
	for i := 0; i < cves; i++ {
		vulns = append(vulns, models.Vulnerability{
			ID:       "CVE-2024-0001",
			Severity: "HIGH",
			Summary:  "Test vulnerability",
		})
	}
	return models.ScoredDependency{
		EnrichedDependency: models.EnrichedDependency{
			Dependency: models.Dependency{
				Name:           name,
				Ecosystem:      ecosystem,
				CurrentVersion: current,
			},
			LatestVersion:   latest,
			SeverityGap:     gap,
			Vulnerabilities: vulns,
		},
		RiskScore: 75.0,
		Reasons:   []string{"major version gap"},
	}
}

// TestStubAdvisor_SummaryContainsPackageInfo verifies the summary includes
// the package name and versions.
func TestStubAdvisor_SummaryContainsPackageInfo(t *testing.T) {
	adv := advisor.NewStub()
	dep := makeScored("requests", "pypi", "2.0.0", "2.31.0", "minor", 0)

	report, err := adv.Advise(context.Background(), dep)
	if err != nil {
		t.Fatalf("Advise returned error: %v", err)
	}

	if !strings.Contains(report.Summary, "requests") {
		t.Errorf("summary missing package name: %q", report.Summary)
	}
	if !strings.Contains(report.Summary, "2.0.0") {
		t.Errorf("summary missing current version: %q", report.Summary)
	}
	if !strings.Contains(report.Summary, "2.31.0") {
		t.Errorf("summary missing latest version: %q", report.Summary)
	}
}

// TestStubAdvisor_MajorBump sets BreakingChanges when severity gap is "major".
func TestStubAdvisor_MajorBump(t *testing.T) {
	adv := advisor.NewStub()
	dep := makeScored("lodash", "npm", "3.10.1", "4.17.21", "major", 0)

	report, err := adv.Advise(context.Background(), dep)
	if err != nil {
		t.Fatalf("Advise returned error: %v", err)
	}

	if len(report.BreakingChanges) == 0 {
		t.Error("expected breaking changes for major version bump, got none")
	}
}

// TestStubAdvisor_MinorBumpNoBreaking verifies minor bumps produce no
// BreakingChanges entries.
func TestStubAdvisor_MinorBumpNoBreaking(t *testing.T) {
	adv := advisor.NewStub()
	dep := makeScored("axios", "npm", "1.3.0", "1.6.0", "minor", 0)

	report, err := adv.Advise(context.Background(), dep)
	if err != nil {
		t.Fatalf("Advise returned error: %v", err)
	}

	if len(report.BreakingChanges) != 0 {
		t.Errorf("expected no breaking changes for minor bump, got %v", report.BreakingChanges)
	}
}

// TestStubAdvisor_EcosystemSteps_NPM verifies npm-specific migration steps.
func TestStubAdvisor_EcosystemSteps_NPM(t *testing.T) {
	adv := advisor.NewStub()
	dep := makeScored("react", "npm", "17.0.0", "18.2.0", "major", 0)

	report, err := adv.Advise(context.Background(), dep)
	if err != nil {
		t.Fatalf("Advise returned error: %v", err)
	}

	found := false
	for _, step := range report.MigrationSteps {
		if strings.Contains(step, "npm install") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'npm install' step in MigrationSteps, got %v", report.MigrationSteps)
	}
}

// TestStubAdvisor_EcosystemSteps_PyPI verifies pip-specific migration steps.
func TestStubAdvisor_EcosystemSteps_PyPI(t *testing.T) {
	adv := advisor.NewStub()
	dep := makeScored("django", "pypi", "3.2.0", "4.2.0", "major", 0)

	report, err := adv.Advise(context.Background(), dep)
	if err != nil {
		t.Fatalf("Advise returned error: %v", err)
	}

	found := false
	for _, step := range report.MigrationSteps {
		if strings.Contains(step, "pip install") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'pip install' step in MigrationSteps, got %v", report.MigrationSteps)
	}
}

// TestStubAdvisor_EcosystemSteps_Go verifies go-specific migration steps.
func TestStubAdvisor_EcosystemSteps_Go(t *testing.T) {
	adv := advisor.NewStub()
	dep := makeScored("github.com/gin-gonic/gin", "go", "1.8.0", "1.9.0", "minor", 0)

	report, err := adv.Advise(context.Background(), dep)
	if err != nil {
		t.Fatalf("Advise returned error: %v", err)
	}

	found := false
	for _, step := range report.MigrationSteps {
		if strings.Contains(step, "go get") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'go get' step in MigrationSteps, got %v", report.MigrationSteps)
	}
}

// TestStubAdvisor_CVEMigrationStep verifies that a CVE step is appended when
// vulnerabilities are present.
func TestStubAdvisor_CVEMigrationStep(t *testing.T) {
	adv := advisor.NewStub()
	dep := makeScored("log4j", "maven", "2.14.0", "2.17.0", "patch", 2)

	report, err := adv.Advise(context.Background(), dep)
	if err != nil {
		t.Fatalf("Advise returned error: %v", err)
	}

	found := false
	for _, step := range report.MigrationSteps {
		if strings.Contains(step, "CVE") || strings.Contains(step, "vulnerabilit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected CVE mention in migration steps for dep with vulnerabilities, got %v", report.MigrationSteps)
	}
}

// TestStubAdvisor_Deterministic verifies identical inputs produce identical outputs.
func TestStubAdvisor_Deterministic(t *testing.T) {
	adv := advisor.NewStub()
	dep := makeScored("webpack", "npm", "4.0.0", "5.0.0", "major", 1)

	r1, _ := adv.Advise(context.Background(), dep)
	r2, _ := adv.Advise(context.Background(), dep)

	if r1.Summary != r2.Summary {
		t.Error("StubAdvisor is not deterministic: summary differs between calls")
	}
	if len(r1.MigrationSteps) != len(r2.MigrationSteps) {
		t.Error("StubAdvisor is not deterministic: migration step count differs")
	}
}

// TestStubAdvisor_ReportEmbedsDep verifies the returned report embeds the
// original ScoredDependency.
func TestStubAdvisor_ReportEmbedsDep(t *testing.T) {
	adv := advisor.NewStub()
	dep := makeScored("express", "npm", "4.17.0", "4.18.0", "patch", 0)

	report, err := adv.Advise(context.Background(), dep)
	if err != nil {
		t.Fatalf("Advise returned error: %v", err)
	}

	if report.Name != "express" {
		t.Errorf("expected embedded dep name 'express', got %q", report.Name)
	}
}
