// Package scorer computes a weighted risk score for each EnrichedDependency
// and returns results sorted from highest to lowest risk.
package scorer

import (
	"fmt"
	"sort"
	"strings"

	"dep-health/models"
)

// Weight constants — must sum to 1.0.
const (
	weightCVE           = 0.40
	weightMajorGap      = 0.30
	weightVersionBehind = 0.20
	weightCrossRepo     = 0.10
)

// Normalisation ceilings: anything at or above these values scores 1.0
// for its factor.
const (
	maxVersionsBehind = 20.0
	maxCrossRepo      = 10.0
)

// Score computes and sorts risk scores for all enriched dependencies.
//
// crossRepoCounts is an optional map of package name → number of repos that
// depend on it; pass nil (or an empty map) when single-repo mode is used.
func Score(deps []models.EnrichedDependency, crossRepoCounts map[string]int) []models.ScoredDependency {
	scored := make([]models.ScoredDependency, 0, len(deps))
	for _, dep := range deps {
		scored = append(scored, scoreDep(dep, crossRepoCounts))
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].RiskScore > scored[j].RiskScore
	})
	return scored
}

// scoreDep computes the risk score for a single dependency.
func scoreDep(dep models.EnrichedDependency, crossRepoCounts map[string]int) models.ScoredDependency {
	var reasons []string

	// ── Factor 1: CVE severity (40%) ─────────────────────────────────────────
	cveFactor := cveSeverityFactor(dep.Vulnerabilities)
	if cveFactor > 0 {
		reasons = append(reasons, fmt.Sprintf(
			"%d known CVE(s), highest: %s",
			len(dep.Vulnerabilities),
			maxSeverityLabel(dep.Vulnerabilities),
		))
	}

	// ── Factor 2: Version gap (30%) — LTS-aware ────────────────────────────
	gapFactor := ltsAwareGapFactor(dep)
	if dep.SeverityGap == "major" && dep.LatestInMajor != "" && dep.LatestInMajor != dep.LatestVersion {
		// New major exists but user may be on an active LTS line.
		behindOnMajor := dep.CurrentVersion != dep.LatestInMajor
		if behindOnMajor {
			reasons = append(reasons, fmt.Sprintf(
				"behind on current major line (%s → %s) and newer major available (%s)",
				dep.CurrentVersion, dep.LatestInMajor, dep.LatestVersion,
			))
		} else {
			reasons = append(reasons, fmt.Sprintf(
				"current on your major line (%s); newer major available (%s)",
				dep.CurrentVersion, dep.LatestVersion,
			))
		}
	} else if gapFactor > 0 {
		reasons = append(reasons, fmt.Sprintf(
			"%s version gap (%s → %s)",
			dep.SeverityGap, dep.CurrentVersion, dep.LatestVersion,
		))
	}

	// ── Factor 3: Versions-behind count (20%) ────────────────────────────────
	vbFactor := versionsBehindFactor(dep.VersionsBehind)
	if dep.VersionsBehind > 0 {
		reasons = append(reasons, fmt.Sprintf("%d release(s) behind latest", dep.VersionsBehind))
	}

	// ── Factor 4: Cross-repo prevalence (10%) ────────────────────────────────
	crossCount := 0
	if crossRepoCounts != nil {
		crossCount = crossRepoCounts[dep.Name]
	}
	crFactor := crossRepoFactor(crossCount)
	if crossCount > 1 {
		reasons = append(reasons, fmt.Sprintf("used across %d repositories", crossCount))
	}

	// Final weighted score on a 0–100 scale.
	raw := weightCVE*cveFactor +
		weightMajorGap*gapFactor +
		weightVersionBehind*vbFactor +
		weightCrossRepo*crFactor
	score := raw * 100

	return models.ScoredDependency{
		EnrichedDependency: dep,
		RiskScore:          score,
		CrossRepoCount:     crossCount,
		Reasons:            reasons,
	}
}

// ── Factor helpers ────────────────────────────────────────────────────────────

// cveSeverityFactor returns 0.0–1.0 based on the most severe vulnerability found.
func cveSeverityFactor(vulns []models.Vulnerability) float64 {
	max := 0.0
	for _, v := range vulns {
		if f := severityToFloat(v.Severity); f > max {
			max = f
		}
	}
	return max
}

// severityToFloat maps a severity label or CVSS score string to 0.0–1.0.
func severityToFloat(sev string) float64 {
	switch strings.ToUpper(sev) {
	case "CRITICAL":
		return 1.0
	case "HIGH":
		return 0.8
	case "MEDIUM", "MODERATE":
		return 0.5
	case "LOW":
		return 0.2
	default:
		if sev != "" {
			return 0.3 // Unknown non-empty severity: treat conservatively.
		}
		return 0.0
	}
}

// maxSeverityLabel returns the label of the highest-severity vulnerability.
func maxSeverityLabel(vulns []models.Vulnerability) string {
	order := []string{"CRITICAL", "HIGH", "MEDIUM", "MODERATE", "LOW"}
	for _, label := range order {
		for _, v := range vulns {
			if strings.EqualFold(v.Severity, label) {
				return label
			}
		}
	}
	if len(vulns) > 0 {
		return vulns[0].Severity
	}
	return "NONE"
}

// ltsAwareGapFactor computes the version-gap factor taking LatestInMajor into
// account.  When the user is current on their major line but a new major exists,
// the factor is reduced (informational, not urgent).
func ltsAwareGapFactor(dep models.EnrichedDependency) float64 {
	if dep.SeverityGap != "major" || dep.LatestInMajor == "" || dep.LatestInMajor == dep.LatestVersion {
		// No LTS distinction possible — use the classic factor.
		return versionGapFactor(dep.SeverityGap)
	}
	// A newer major exists and LatestInMajor is within the current line.
	if dep.CurrentVersion == dep.LatestInMajor {
		// Current on their major line — low urgency for the cross-major gap.
		return 0.3
	}
	// Behind within their own major line AND a new major exists — full urgency.
	return 1.0
}

// versionGapFactor converts a SeverityGap string to 0.0–1.0.
func versionGapFactor(gap string) float64 {
	switch gap {
	case "major":
		return 1.0
	case "minor":
		return 0.5
	case "patch":
		return 0.1
	default:
		return 0.0
	}
}

// versionsBehindFactor normalises the release-count lag to 0.0–1.0.
func versionsBehindFactor(behind int) float64 {
	if behind <= 0 {
		return 0
	}
	f := float64(behind) / maxVersionsBehind
	if f > 1.0 {
		f = 1.0
	}
	return f
}

// crossRepoFactor normalises the cross-repo count to 0.0–1.0.
func crossRepoFactor(count int) float64 {
	if count <= 0 {
		return 0
	}
	f := float64(count) / maxCrossRepo
	if f > 1.0 {
		f = 1.0
	}
	return f
}
