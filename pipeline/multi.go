package pipeline

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"dep-health/models"
)

// RunMulti scans multiple repositories sequentially and returns an aggregated
// report with cross-repo risk scores.  Each target is either a local directory
// path or a remote git URL — detection is automatic.
func RunMulti(ctx context.Context, targets []string, opts Options) (*models.MultiRepoReport, error) {
	perRepo := make(map[string][]models.AdvisoryReport, len(targets))
	scanErrors := make(map[string]string)
	var allDeps []models.AdvisoryReport

	for _, target := range targets {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		label := TargetLabel(target)

		repoOpts := Options{Concurrency: opts.Concurrency}
		if isGitURL(target) {
			repoOpts.GitURL = target
			repoOpts.RepoURL = target
		}
		if opts.OnProgress != nil {
			captured := label
			repoOpts.OnProgress = func(msg string) {
				opts.OnProgress(fmt.Sprintf("[%s] %s", captured, msg))
			}
		}

		dir := ""
		if !isGitURL(target) {
			dir = target
		}

		if opts.OnProgress != nil {
			opts.OnProgress(fmt.Sprintf("[%s] starting …", label))
		}

		reports, err := Run(ctx, dir, repoOpts)
		if err != nil {
			scanErrors[label] = err.Error()
			if opts.OnProgress != nil {
				opts.OnProgress(fmt.Sprintf("[%s] error: %v", label, err))
			}
			perRepo[label] = nil
			continue
		}

		for i := range reports {
			reports[i].RepoSource = label
		}
		perRepo[label] = reports
		allDeps = append(allDeps, reports...)
	}

	// Count how many distinct repos each (name, ecosystem) pair appears in.
	type depKey struct{ Name, Ecosystem string }
	repoSets := make(map[depKey]map[string]struct{})
	for _, r := range allDeps {
		k := depKey{r.Name, r.Ecosystem}
		if repoSets[k] == nil {
			repoSets[k] = make(map[string]struct{})
		}
		repoSets[k][r.RepoSource] = struct{}{}
	}

	// Update CrossRepoCount and add cross-repo contribution to RiskScore.
	// Single-repo runs always use crossCount=0 (0 pts), so any positive count
	// is purely additive to the existing score.
	for i := range allDeps {
		k := depKey{allDeps[i].Name, allDeps[i].Ecosystem}
		count := len(repoSets[k])
		if count > allDeps[i].CrossRepoCount {
			applyNewCrossRepoCount(&allDeps[i], count)
		}
	}

	// Re-sort by updated score.
	sort.Slice(allDeps, func(i, j int) bool {
		return allDeps[i].RiskScore > allDeps[j].RiskScore
	})

	report := &models.MultiRepoReport{
		Targets: targets,
		PerRepo: perRepo,
		AllDeps: allDeps,
		Stats:   buildStats(len(targets), allDeps),
	}
	if len(scanErrors) > 0 {
		report.Errors = scanErrors
	}
	return report, nil
}

// applyNewCrossRepoCount updates CrossRepoCount, adjusts RiskScore for the
// cross-repo weight (10%), and appends a reason string.
func applyNewCrossRepoCount(r *models.AdvisoryReport, count int) {
	r.CrossRepoCount = count
	f := float64(count) / 10.0
	if f > 1.0 {
		f = 1.0
	}
	r.RiskScore += f * 10.0 // weightCrossRepo(0.10) × 100
	if count > 1 {
		r.Reasons = append(r.Reasons, fmt.Sprintf("outdated in %d repos", count))
	}
}

// buildStats computes aggregate statistics from the flat dep list.
func buildStats(totalRepos int, deps []models.AdvisoryReport) models.MultiRepoStats {
	cascades := make(map[string]struct{})
	cves, blocked := 0, 0
	for _, r := range deps {
		cves += len(r.Vulnerabilities)
		if len(r.BlockedBy) > 0 {
			blocked++
		}
		if r.CascadeGroup != "" {
			cascades[r.RepoSource+":"+r.CascadeGroup] = struct{}{}
		}
	}
	return models.MultiRepoStats{
		TotalRepos:    totalRepos,
		TotalOutdated: len(deps),
		TotalCVEs:     cves,
		CascadeGroups: len(cascades),
		BlockedDeps:   blocked,
	}
}

// TargetLabel returns a short human-readable label for a scan target.
// Exported so cmd/scan_multi.go can use the same labelling logic.
func TargetLabel(target string) string {
	t := strings.TrimSuffix(target, ".git")
	t = strings.TrimRight(t, "/\\")
	parts := strings.Split(t, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	if len(parts) == 1 && parts[0] != "" {
		return parts[0]
	}
	return target
}

// isGitURL reports whether s is a remote repository URL.
func isGitURL(s string) bool {
	return strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "git://")
}
