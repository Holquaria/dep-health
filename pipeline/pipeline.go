// Package pipeline exposes the full dep-health scan pipeline as a single
// callable function so that both the CLI and the HTTP server can reuse it
// without duplicating logic.
package pipeline

import (
	"context"
	"fmt"

	"dep-health/advisor"
	"dep-health/config"
	"dep-health/models"
	"dep-health/resolver"
	"dep-health/scanner"
	"dep-health/scorer"
)

// Options configures a single pipeline run.
type Options struct {
	// RepoURL is attached to every discovered dependency for cross-repo tracking.
	RepoURL string
	// Concurrency caps the number of parallel registry requests. 0 = use the
	// value from DEP_HEALTH_MAX_CONCURRENCY (default 10).
	Concurrency int
	// OnProgress, if non-nil, is called with short human-readable status lines
	// as each stage begins.  Safe to leave nil (e.g. when called from the server).
	OnProgress func(msg string)
}

// Run executes the full dep-health pipeline for the given directory and returns
// advisory reports sorted by risk score descending.
//
// Run does not produce any output itself — callers decide whether to print,
// persist, or encode the results.
func Run(ctx context.Context, dir string, opts Options) ([]models.AdvisoryReport, error) {
	progress := opts.OnProgress
	if progress == nil {
		progress = func(string) {}
	}

	cfg := config.Load()

	// ── 1. Discover ───────────────────────────────────────────────────────────
	progress(fmt.Sprintf("Scanning %s …", dir))
	deps, err := scanner.Discover(dir, opts.RepoURL, scanner.DefaultScanners())
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	if len(deps) == 0 {
		return nil, nil
	}
	progress(fmt.Sprintf("Found %d dependencies", len(deps)))

	// ── 2. Enrich ─────────────────────────────────────────────────────────────
	progress("Resolving versions and checking OSV.dev for CVEs …")
	res := resolver.New()
	conc := opts.Concurrency
	if conc <= 0 {
		conc = cfg.MaxConcurrency
	}
	if conc > 0 {
		res.Concurrency = conc
	}
	enriched, err := res.Enrich(ctx, deps)
	if err != nil {
		return nil, fmt.Errorf("resolution: %w", err)
	}

	// ── 3. Score + conflict detection ─────────────────────────────────────────
	progress("Scoring and detecting peer conflicts …")
	scored := scorer.Score(enriched, nil)
	scored = scorer.DetectConflicts(scored)

	// ── 4. Advise ─────────────────────────────────────────────────────────────
	progress("Generating advisory reports …")
	var adv advisor.Advisor
	if cfg.AnthropicAPIKey != "" {
		adv = advisor.NewAnthropic(cfg.AnthropicAPIKey)
	} else {
		adv = advisor.NewStub()
	}

	reports := make([]models.AdvisoryReport, 0, len(scored))
	for _, sd := range scored {
		r, advErr := adv.Advise(ctx, sd)
		if advErr != nil {
			continue
		}
		reports = append(reports, r)
	}

	return reports, nil
}
