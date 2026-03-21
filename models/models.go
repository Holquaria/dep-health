// Package models defines the core data structures shared across dep-health packages.
package models

// Dependency represents a raw dependency discovered in a manifest file.
type Dependency struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
	Ecosystem      string `json:"ecosystem"` // npm | pypi | go | maven
	ManifestPath   string `json:"manifest_path"`
	RepoURL        string `json:"repo_url"`
}

// Vulnerability represents a known security vulnerability affecting a dependency.
type Vulnerability struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	URL      string `json:"url"`
}

// EnrichedDependency embeds Dependency and adds registry and vulnerability data.
type EnrichedDependency struct {
	Dependency
	LatestVersion   string            `json:"latest_version"`
	SeverityGap     string            `json:"severity_gap"` // patch | minor | major
	VersionsBehind  int               `json:"versions_behind"`
	Vulnerabilities []Vulnerability   `json:"vulnerabilities"`
	// PeerConstraints maps peer package names to the semver constraint string
	// declared by this package's *latest* version (e.g. {"react": "^19.0.0"}).
	// Populated for npm packages; nil for other ecosystems.
	PeerConstraints map[string]string `json:"peer_constraints,omitempty"`
}

// ScoredDependency embeds EnrichedDependency and adds risk scoring data.
type ScoredDependency struct {
	EnrichedDependency
	RiskScore      float64  `json:"risk_score"`
	CrossRepoCount int      `json:"cross_repo_count"`
	// RepoSource identifies which repository this dependency came from.
	// Set by pipeline.RunMulti; empty in single-repo scans.
	RepoSource     string   `json:"repo_source,omitempty"`
	Reasons        []string `json:"reasons"`
	// BlockedBy lists peer packages whose *latest* version still cannot satisfy
	// this package's peer constraint — meaning this upgrade has no safe path yet.
	BlockedBy []string `json:"blocked_by,omitempty"`
	// CascadeGroup is a "+" joined, sorted list of package names that must be
	// upgraded together (e.g. "next+react"). Empty for standalone upgrades.
	CascadeGroup string `json:"cascade_group,omitempty"`
}

// AdvisoryReport embeds ScoredDependency and adds AI-generated upgrade guidance.
type AdvisoryReport struct {
	ScoredDependency
	Summary         string   `json:"summary"`
	BreakingChanges []string `json:"breaking_changes"`
	MigrationSteps  []string `json:"migration_steps"`
	PRUrl           string   `json:"pr_url,omitempty"`
}

// MultiRepoReport is returned by pipeline.RunMulti and contains the aggregated
// results of scanning multiple repositories.
type MultiRepoReport struct {
	Targets []string                    `json:"targets"`
	PerRepo map[string][]AdvisoryReport `json:"per_repo"`
	AllDeps []AdvisoryReport            `json:"all_deps"`
	Stats   MultiRepoStats              `json:"stats"`
	Errors  map[string]string           `json:"errors,omitempty"`
}

// MultiRepoStats holds aggregate statistics for a multi-repo scan.
type MultiRepoStats struct {
	TotalRepos    int `json:"total_repos"`
	TotalOutdated int `json:"total_outdated"`
	TotalCVEs     int `json:"total_cves"`
	CascadeGroups int `json:"cascade_groups"`
	BlockedDeps   int `json:"blocked_deps"`
}
