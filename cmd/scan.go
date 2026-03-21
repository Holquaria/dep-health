package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"dep-health/models"
	"dep-health/pipeline"
)

var scanCmd = &cobra.Command{
	Use:   "scan <directory>",
	Short: "Scan a local directory for outdated and vulnerable dependencies",
	Long: `scan walks the given directory for supported manifest files (package.json,
go.mod, requirements.txt — more to come), resolves each dependency against
its ecosystem registry, queries OSV.dev for known CVEs, and prints a
risk-scored table sorted from highest to lowest risk.`,
	Args: cobra.ExactArgs(1),
	RunE: runScan,
}

var (
	flagRepoURL  string
	flagTopN     int
	flagMinScore float64
	flagJSON     bool
)

func init() {
	scanCmd.Flags().StringVar(&flagRepoURL, "repo", "", "Repository URL attached to discovered dependencies (informational)")
	scanCmd.Flags().IntVar(&flagTopN, "top", 0, "Limit output to the N highest-risk dependencies (0 = all)")
	scanCmd.Flags().Float64Var(&flagMinScore, "min-score", 0, "Exclude dependencies with a risk score below this threshold (0–100)")
	scanCmd.Flags().BoolVar(&flagJSON, "json", false, "Emit results as JSON instead of a table")
}

func runScan(cmd *cobra.Command, args []string) error {
	dir := args[0]
	ctx := context.Background()

	step := func(msg string) { fmt.Fprintf(os.Stderr, "→ %s\n", msg) }

	reports, err := pipeline.Run(ctx, dir, pipeline.Options{
		RepoURL:    flagRepoURL,
		OnProgress: step,
	})
	if err != nil {
		return err
	}
	if len(reports) == 0 {
		fmt.Fprintln(os.Stderr, "No supported manifest files found.")
		return nil
	}

	filtered := filterReports(reports, flagMinScore, flagTopN)
	if len(filtered) == 0 {
		fmt.Fprintln(os.Stderr, "No dependencies match the filter criteria.")
		return nil
	}

	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(filtered)
	}

	fmt.Fprintln(os.Stderr)
	printTable(filtered)
	return nil
}

// filterReports removes reports below minScore and truncates to topN.
func filterReports(reports []models.AdvisoryReport, minScore float64, topN int) []models.AdvisoryReport {
	var out []models.AdvisoryReport
	for _, r := range reports {
		if r.RiskScore >= minScore {
			out = append(out, r)
		}
	}
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}

// ── Rendering ─────────────────────────────────────────────────────────────────

const colMaxReasons = 55

func printTable(reports []models.AdvisoryReport) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{
		"#", "Package", "Current", "Latest", "Gap", "Behind", "CVEs", "Score", "Flags", "Top Reason",
	})
	table.SetBorder(true)
	table.SetRowLine(false)
	table.SetAutoWrapText(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetColMinWidth(1, 25) // Package column

	scoreColours := func(score float64) tablewriter.Colors {
		switch {
		case score >= 70:
			return tablewriter.Colors{tablewriter.FgRedColor, tablewriter.Bold}
		case score >= 40:
			return tablewriter.Colors{tablewriter.FgYellowColor}
		default:
			return tablewriter.Colors{tablewriter.FgGreenColor}
		}
	}
	flagColours := func(flags string) tablewriter.Colors {
		switch flags {
		case "BLOCKED":
			return tablewriter.Colors{tablewriter.FgRedColor, tablewriter.Bold}
		case "CASCADE":
			return tablewriter.Colors{tablewriter.FgYellowColor}
		default:
			return tablewriter.Colors{}
		}
	}

	for i, r := range reports {
		cveStr := "-"
		if n := len(r.Vulnerabilities); n > 0 {
			cveStr = fmt.Sprintf("%d (%s)", n, r.Vulnerabilities[0].Severity)
		}

		topReason := "-"
		if len(r.Reasons) > 0 {
			topReason = truncate(r.Reasons[0], colMaxReasons)
		}

		gapStr := r.SeverityGap
		if r.LatestVersion == "" {
			gapStr = "n/a"
		}

		flags := ""
		if len(r.BlockedBy) > 0 {
			flags = "BLOCKED"
		} else if r.CascadeGroup != "" {
			flags = "CASCADE"
		}

		table.Rich(
			[]string{
				fmt.Sprintf("%d", i+1),
				r.Name,
				r.CurrentVersion,
				orDash(r.LatestVersion),
				gapStr,
				fmt.Sprintf("%d", r.VersionsBehind),
				cveStr,
				fmt.Sprintf("%.1f", r.RiskScore),
				flags,
				topReason,
			},
			[]tablewriter.Colors{
				{}, {}, {}, {}, {}, {}, {},
				scoreColours(r.RiskScore),
				flagColours(flags),
				{},
			},
		)
	}

	table.SetFooter([]string{
		"", "", "", "", "", "", "", "",
		fmt.Sprintf("%d", len(reports)),
		"total",
	})
	table.Render()

	fmt.Println()
	printMigrationHints(reports)
	printCascadeGroups(reports)
	printBlockedDeps(reports)
}

// printMigrationHints shows the top-3 migration step lists beneath the table.
func printMigrationHints(reports []models.AdvisoryReport) {
	limit := 3
	if len(reports) < limit {
		limit = len(reports)
	}
	if limit == 0 {
		return
	}

	fmt.Println("── Migration hints (top risk) ──────────────────────────────")
	for _, r := range reports[:limit] {
		fmt.Printf("\n  %s (%s → %s)  score=%.1f\n", r.Name, r.CurrentVersion, r.LatestVersion, r.RiskScore)
		for j, step := range r.MigrationSteps {
			fmt.Printf("    %d. %s\n", j+1, step)
		}
		if len(r.BreakingChanges) > 0 {
			fmt.Printf("    ⚠  %s\n", strings.Join(r.BreakingChanges, " "))
		}
	}
	fmt.Println()
}

// printCascadeGroups lists every cascade group and the versions involved.
func printCascadeGroups(reports []models.AdvisoryReport) {
	byName := make(map[string]models.AdvisoryReport, len(reports))
	for _, r := range reports {
		byName[r.Name] = r
	}

	seen := make(map[string]bool)
	var groups []string
	for _, r := range reports {
		if r.CascadeGroup != "" && !seen[r.CascadeGroup] {
			seen[r.CascadeGroup] = true
			groups = append(groups, r.CascadeGroup)
		}
	}
	if len(groups) == 0 {
		return
	}

	fmt.Println("── Cascade Groups (must upgrade together) ──────────────────")
	for _, g := range groups {
		members := strings.Split(g, "+")
		fmt.Printf("\n  Cascade Group: %s (must update together)\n",
			strings.Join(members, " + "))
		for _, name := range members {
			r, ok := byName[name]
			if !ok {
				fmt.Printf("    • %-35s (not in filtered results)\n", name)
				continue
			}
			fmt.Printf("    • %-35s %s  →  %s\n", name, r.CurrentVersion, r.LatestVersion)
		}
	}
	fmt.Println()
}

// printBlockedDeps lists every dependency whose upgrade path is blocked.
func printBlockedDeps(reports []models.AdvisoryReport) {
	byName := make(map[string]models.AdvisoryReport, len(reports))
	for _, r := range reports {
		byName[r.Name] = r
	}

	var blocked []models.AdvisoryReport
	for _, r := range reports {
		if len(r.BlockedBy) > 0 {
			blocked = append(blocked, r)
		}
	}
	if len(blocked) == 0 {
		return
	}

	fmt.Println("── Blocked Dependencies ────────────────────────────────────")
	for _, r := range blocked {
		for _, peerName := range r.BlockedBy {
			constraint := r.PeerConstraints[peerName]
			peerLatest := "unknown"
			if peer, ok := byName[peerName]; ok && peer.LatestVersion != "" {
				peerLatest = peer.LatestVersion
			}
			fmt.Printf("\n  BLOCKED: %s@%s requires %s %s\n",
				r.Name, r.LatestVersion, peerName, constraint)
			fmt.Printf("    but %s's latest (%s) does not satisfy %s\n",
				peerName, peerLatest, constraint)
			fmt.Printf("    Action: wait for %s to publish a release satisfying %s\n",
				peerName, constraint)
		}
	}
	fmt.Println()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
