// Package advisor generates human-readable summaries and migration guidance
// for scored dependencies.  The Advisor interface is intentionally thin so
// that the stub implementation can be swapped for a real Anthropic API client
// without touching the rest of the pipeline.
package advisor

import (
	"context"
	"fmt"

	"dep-health/models"
)

// Advisor generates an AdvisoryReport for a single scored dependency.
type Advisor interface {
	Advise(ctx context.Context, dep models.ScoredDependency) (models.AdvisoryReport, error)
}

// ── Stub implementation ───────────────────────────────────────────────────────

// StubAdvisor returns deterministic placeholder reports without calling any
// external API.  Wire in AnthropicAdvisor (coming soon) once an API key is
// available — both satisfy the Advisor interface.
type StubAdvisor struct{}

// NewStub creates a StubAdvisor ready for use.
func NewStub() *StubAdvisor { return &StubAdvisor{} }

// Advise returns a placeholder AdvisoryReport based on the scored dependency's
// metadata.  No network calls are made.
func (a *StubAdvisor) Advise(_ context.Context, dep models.ScoredDependency) (models.AdvisoryReport, error) {
	summary := fmt.Sprintf(
		"Upgrade %s from %s to %s. Risk score: %.1f/100.",
		dep.Name, dep.CurrentVersion, dep.LatestVersion, dep.RiskScore,
	)

	var breaking []string
	if dep.SeverityGap == "major" {
		breaking = []string{
			fmt.Sprintf(
				"Major version bump (%s → %s) likely contains breaking API changes.",
				dep.CurrentVersion, dep.LatestVersion,
			),
			"Review the CHANGELOG or official migration guide before upgrading.",
		}
	}

	steps := buildMigrationSteps(dep)

	return models.AdvisoryReport{
		ScoredDependency: dep,
		Summary:          summary,
		BreakingChanges:  breaking,
		MigrationSteps:   steps,
		PRUrl:            "", // populated by the PR-creation step (not yet implemented)
	}, nil
}

// buildMigrationSteps returns ecosystem-appropriate upgrade instructions.
func buildMigrationSteps(dep models.ScoredDependency) []string {
	var steps []string

	switch dep.Ecosystem {
	case "npm":
		steps = append(steps,
			fmt.Sprintf("npm install %s@%s", dep.Name, dep.LatestVersion),
			"Run your full test suite: npm test",
			"Inspect deprecation warnings in the install output.",
		)
	case "pypi":
		steps = append(steps,
			fmt.Sprintf("pip install --upgrade %s==%s", dep.Name, dep.LatestVersion),
			"Re-run your tests: pytest",
		)
	case "go":
		steps = append(steps,
			fmt.Sprintf("go get %s@v%s", dep.Name, dep.LatestVersion),
			"go mod tidy",
			"go test ./...",
		)
	default:
		steps = append(steps,
			fmt.Sprintf("Upgrade %s to %s using your package manager.", dep.Name, dep.LatestVersion),
			"Run your full test suite after upgrading.",
		)
	}

	if len(dep.Vulnerabilities) > 0 {
		steps = append(steps, fmt.Sprintf(
			"Verify that the %d known CVE(s) are resolved in v%s (check the release notes).",
			len(dep.Vulnerabilities), dep.LatestVersion,
		))
	}

	return steps
}

// ── Anthropic implementation (stub placeholder) ───────────────────────────────

// AnthropicAdvisor will call the Anthropic Messages API to generate rich,
// context-aware summaries and migration steps.  Implementation is deferred
// pending API key wiring — this placeholder satisfies the interface contract
// at compile time so the import graph stays valid.
//
// Usage (once implemented):
//
//	adv := advisor.NewAnthropic(cfg.AnthropicAPIKey)
//	report, err := adv.Advise(ctx, scoredDep)
type AnthropicAdvisor struct {
	apiKey string
}

// NewAnthropic creates an AnthropicAdvisor.  Panics if apiKey is empty.
func NewAnthropic(apiKey string) *AnthropicAdvisor {
	if apiKey == "" {
		panic("advisor: ANTHROPIC_API_KEY must not be empty")
	}
	return &AnthropicAdvisor{apiKey: apiKey}
}

// Advise calls the Anthropic API — NOT YET IMPLEMENTED.
// Falls back to StubAdvisor behaviour until the real implementation lands.
func (a *AnthropicAdvisor) Advise(ctx context.Context, dep models.ScoredDependency) (models.AdvisoryReport, error) {
	// TODO: replace with real Anthropic API call.
	// Suggested prompt template:
	//
	//   "You are a software dependency security advisor. Summarise the upgrade
	//    from {Name} {CurrentVersion} to {LatestVersion} in 2–3 sentences.
	//    List any breaking changes and provide numbered migration steps.
	//    Focus on practical guidance for engineers."
	//
	// Wire up using the /claude-api skill or the anthropic-go SDK.
	stub := NewStub()
	return stub.Advise(ctx, dep)
}
