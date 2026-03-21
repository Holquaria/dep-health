package scorer_test

import (
	"strings"
	"testing"

	"dep-health/models"
	"dep-health/scorer"
)

// makeDep is a test helper that builds a ScoredDependency from the fields
// relevant to conflict detection.
func makeDep(name, current, latest string, peerConstraints map[string]string) models.ScoredDependency {
	return models.ScoredDependency{
		EnrichedDependency: models.EnrichedDependency{
			Dependency: models.Dependency{
				Name:           name,
				CurrentVersion: current,
				Ecosystem:      "npm",
			},
			LatestVersion:   latest,
			PeerConstraints: peerConstraints,
		},
	}
}

// ── Cascade (resolvable) ──────────────────────────────────────────────────────

func TestDetectConflicts_CascadeGroup(t *testing.T) {
	// next@15 requires react@^19.  react's latest is 19.0.0 (satisfies ^19).
	// → next and react must be upgraded together.
	deps := []models.ScoredDependency{
		makeDep("next", "13.0.0", "15.0.0", map[string]string{
			"react": "^19.0.0",
		}),
		makeDep("react", "18.2.0", "19.0.0", nil),
	}

	result := scorer.DetectConflicts(deps)

	nextDep := findDep(t, result, "next")
	reactDep := findDep(t, result, "react")

	if nextDep.CascadeGroup == "" {
		t.Error("next: expected a CascadeGroup, got empty string")
	}
	if reactDep.CascadeGroup == "" {
		t.Error("react: expected a CascadeGroup, got empty string")
	}
	if nextDep.CascadeGroup != reactDep.CascadeGroup {
		t.Errorf("next and react should share a cascade group; got %q and %q",
			nextDep.CascadeGroup, reactDep.CascadeGroup)
	}
	if !strings.Contains(nextDep.CascadeGroup, "next") || !strings.Contains(nextDep.CascadeGroup, "react") {
		t.Errorf("cascade group %q should contain both 'next' and 'react'", nextDep.CascadeGroup)
	}
	if len(nextDep.BlockedBy) != 0 {
		t.Errorf("next should not be blocked; BlockedBy = %v", nextDep.BlockedBy)
	}
}

// ── Blocked (unresolvable) ────────────────────────────────────────────────────

func TestDetectConflicts_BlockedBy(t *testing.T) {
	// next@15 requires react@^19, but react's latest is still 18.3.0.
	// → next is blocked; there is no path to upgrade.
	deps := []models.ScoredDependency{
		makeDep("next", "13.0.0", "15.0.0", map[string]string{
			"react": "^19.0.0",
		}),
		makeDep("react", "18.2.0", "18.3.0", nil),
	}

	result := scorer.DetectConflicts(deps)

	nextDep := findDep(t, result, "next")

	if len(nextDep.BlockedBy) == 0 {
		t.Error("next: expected BlockedBy to contain 'react'")
	}
	if nextDep.BlockedBy[0] != "react" {
		t.Errorf("next: BlockedBy[0] = %q, want \"react\"", nextDep.BlockedBy[0])
	}
	if nextDep.CascadeGroup != "" {
		t.Errorf("next: should have no CascadeGroup, got %q", nextDep.CascadeGroup)
	}
}

// ── No conflict ───────────────────────────────────────────────────────────────

func TestDetectConflicts_NoConflict(t *testing.T) {
	// some-pkg@2 requires react@^18.  react is at 18.2.0 — already satisfies.
	// → no cascade, no blocked.
	deps := []models.ScoredDependency{
		makeDep("some-pkg", "1.0.0", "2.0.0", map[string]string{
			"react": "^18.0.0",
		}),
		makeDep("react", "18.2.0", "18.3.0", nil),
	}

	result := scorer.DetectConflicts(deps)

	for _, d := range result {
		if d.CascadeGroup != "" {
			t.Errorf("%s: unexpected CascadeGroup %q", d.Name, d.CascadeGroup)
		}
		if len(d.BlockedBy) != 0 {
			t.Errorf("%s: unexpected BlockedBy %v", d.Name, d.BlockedBy)
		}
	}
}

// ── Peer not in repo ──────────────────────────────────────────────────────────

func TestDetectConflicts_PeerNotInRepo(t *testing.T) {
	// some-pkg requires an external peer that isn't installed in this repo.
	// → should skip gracefully; no panic, no false positives.
	deps := []models.ScoredDependency{
		makeDep("some-pkg", "1.0.0", "2.0.0", map[string]string{
			"external-peer-not-installed": "^5.0.0",
		}),
	}

	result := scorer.DetectConflicts(deps)

	d := findDep(t, result, "some-pkg")
	if d.CascadeGroup != "" || len(d.BlockedBy) != 0 {
		t.Errorf("some-pkg: expected no conflict signals for absent peer, got cascade=%q blocked=%v",
			d.CascadeGroup, d.BlockedBy)
	}
}

// ── Three-way cascade ─────────────────────────────────────────────────────────

func TestDetectConflicts_ThreeWayCascade(t *testing.T) {
	// next@15 requires react@^19.
	// react@19 requires react-dom@^19.
	// react-dom@19 is available.
	// → all three must move together.
	deps := []models.ScoredDependency{
		makeDep("next", "13.0.0", "15.0.0", map[string]string{
			"react": "^19.0.0",
		}),
		makeDep("react", "18.2.0", "19.0.0", map[string]string{
			"react-dom": "^19.0.0",
		}),
		makeDep("react-dom", "18.2.0", "19.0.0", nil),
	}

	result := scorer.DetectConflicts(deps)

	groups := map[string]string{}
	for _, d := range result {
		groups[d.Name] = d.CascadeGroup
	}

	if groups["next"] == "" || groups["react"] == "" || groups["react-dom"] == "" {
		t.Errorf("all three packages should be in a cascade group: %v", groups)
	}
	if !(groups["next"] == groups["react"] && groups["react"] == groups["react-dom"]) {
		t.Errorf("all three should share the same cascade group: %v", groups)
	}
	if !strings.Contains(groups["next"], "react-dom") {
		t.Errorf("cascade group should mention react-dom: %q", groups["next"])
	}
}

// ── Mixed: cascade + blocked on separate peers ────────────────────────────────

func TestDetectConflicts_CascadeAndBlocked(t *testing.T) {
	// next@15 requires:
	//   react@^19   → react latest is 19.0.0 → cascade
	//   some-ui@^3  → some-ui latest is 2.9.0 → blocked
	deps := []models.ScoredDependency{
		makeDep("next", "13.0.0", "15.0.0", map[string]string{
			"react":   "^19.0.0",
			"some-ui": "^3.0.0",
		}),
		makeDep("react", "18.2.0", "19.0.0", nil),
		makeDep("some-ui", "2.0.0", "2.9.0", nil),
	}

	result := scorer.DetectConflicts(deps)

	nextDep := findDep(t, result, "next")

	if nextDep.CascadeGroup == "" {
		t.Error("next: expected a CascadeGroup (with react)")
	}
	if len(nextDep.BlockedBy) == 0 {
		t.Error("next: expected BlockedBy to contain 'some-ui'")
	}
	if nextDep.BlockedBy[0] != "some-ui" {
		t.Errorf("next: BlockedBy[0] = %q, want \"some-ui\"", nextDep.BlockedBy[0])
	}
}

// ── No peer constraints at all ────────────────────────────────────────────────

func TestDetectConflicts_NoPeerConstraints(t *testing.T) {
	deps := []models.ScoredDependency{
		makeDep("lodash", "4.17.11", "4.17.21", nil),
		makeDep("express", "4.18.0", "4.19.2", nil),
	}

	result := scorer.DetectConflicts(deps)

	for _, d := range result {
		if d.CascadeGroup != "" || len(d.BlockedBy) != 0 {
			t.Errorf("%s: unexpected conflict signals on dep with no peer constraints", d.Name)
		}
	}
}

// ── Empty input ───────────────────────────────────────────────────────────────

func TestDetectConflicts_Empty(t *testing.T) {
	result := scorer.DetectConflicts(nil)
	if result != nil {
		t.Errorf("expected nil result for nil input, got %v", result)
	}

	result = scorer.DetectConflicts([]models.ScoredDependency{})
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %v", result)
	}
}

// ── Determinism ───────────────────────────────────────────────────────────────

func TestDetectConflicts_GroupIDIsDeterministic(t *testing.T) {
	// Run the same input in both orders.  The cascade group name must be
	// identical regardless of which package appears first in the slice.
	make2 := func(nextFirst bool) []models.ScoredDependency {
		a := makeDep("next", "13.0.0", "15.0.0", map[string]string{"react": "^19.0.0"})
		b := makeDep("react", "18.2.0", "19.0.0", nil)
		if nextFirst {
			return []models.ScoredDependency{a, b}
		}
		return []models.ScoredDependency{b, a}
	}

	r1 := scorer.DetectConflicts(make2(true))
	r2 := scorer.DetectConflicts(make2(false))

	g1 := findDep(t, r1, "next").CascadeGroup
	g2 := findDep(t, r2, "next").CascadeGroup

	if g1 != g2 {
		t.Errorf("cascade group ID should be order-independent; got %q vs %q", g1, g2)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func findDep(t *testing.T, deps []models.ScoredDependency, name string) models.ScoredDependency {
	t.Helper()
	for _, d := range deps {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("dep %q not found in result", name)
	return models.ScoredDependency{}
}
