package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"

	"dep-health/models"
	"dep-health/pipeline"
)

var scanMultiCmd = &cobra.Command{
	Use:   "scan-multi <target> <target> [...]",
	Short: "Scan multiple repositories and aggregate results with cross-repo scoring",
	Long: `scan-multi accepts two or more local paths or remote git URLs, scans each
one, then produces an aggregated risk report.  Cross-repo prevalence (the 10%
weight in the scoring formula) is computed from the actual results, so packages
that are outdated across many repos are ranked higher.

Examples:
  dep-health scan-multi ./frontend ./backend ./shared
  dep-health scan-multi https://github.com/org/api https://github.com/org/web
  dep-health scan-multi ./local-service https://github.com/org/platform`,
	Args: cobra.MinimumNArgs(2),
	RunE: runScanMulti,
}

var flagMultiJSON bool

func init() {
	scanMultiCmd.Flags().BoolVar(&flagMultiJSON, "json", false, "Emit full MultiRepoReport as JSON")
}

func runScanMulti(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	step := func(msg string) { fmt.Fprintf(os.Stderr, "→ %s\n", msg) }

	report, err := pipeline.RunMulti(ctx, args, pipeline.Options{OnProgress: step})
	if err != nil {
		return err
	}

	if report == nil || len(report.AllDeps) == 0 {
		fmt.Fprintln(os.Stderr, "No outdated dependencies found across the scanned repositories.")
		return nil
	}

	if flagMultiJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	// ── Summary header ─────────────────────────────────────────────────────────
	fmt.Fprintln(os.Stderr)
	fmt.Printf("Scanned %d repos · %d outdated deps · %d CVEs · %d cascade groups · %d blocked\n\n",
		report.Stats.TotalRepos,
		report.Stats.TotalOutdated,
		report.Stats.TotalCVEs,
		report.Stats.CascadeGroups,
		report.Stats.BlockedDeps,
	)

	// Per-repo breakdown.
	for _, target := range args {
		label := pipeline.TargetLabel(target)
		deps := report.PerRepo[label]
		if deps == nil {
			errMsg := report.Errors[label]
			fmt.Printf("  %-45s  ERROR: %s\n", label, errMsg)
		} else {
			fmt.Printf("  %-45s  %d outdated\n", label, len(deps))
		}
	}
	fmt.Println()

	// ── Combined table ──────────────────────────────────────────────────────────
	printMultiTable(report.AllDeps)
	return nil
}

func printMultiTable(reports []models.AdvisoryReport) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Options(
		tablewriter.WithHeaderAlignmentConfig(tw.CellAlignment{Global: tw.AlignLeft}),
		tablewriter.WithRowAlignmentConfig(tw.CellAlignment{Global: tw.AlignLeft}),
		tablewriter.WithRowAutoWrap(tw.WrapTruncate),
	)
	table.Header("#", "Repo", "Package", "Current", "Latest", "Gap", "Behind", "CVEs", "Score")

	for i, r := range reports {
		cveStr := "-"
		if n := len(r.Vulnerabilities); n > 0 {
			cveStr = fmt.Sprintf("%d (%s)", n, r.Vulnerabilities[0].Severity)
		}
		gapStr := r.SeverityGap
		if r.LatestVersion == "" {
			gapStr = "n/a"
		}
		table.Append([]string{ //nolint:errcheck
			fmt.Sprintf("%d", i+1),
			r.RepoSource,
			r.Name,
			r.CurrentVersion,
			orDash(r.LatestVersion),
			gapStr,
			fmt.Sprintf("%d", r.VersionsBehind),
			cveStr,
			colorScore(r.RiskScore, fmt.Sprintf("%.1f", r.RiskScore)),
		})
	}

	table.Footer("", "", "", "", "", "", "", "",
		fmt.Sprintf("%d total", len(reports)),
	)
	table.Render() //nolint:errcheck
	fmt.Println()
}
