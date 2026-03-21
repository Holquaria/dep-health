package scorer

import (
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"dep-health/models"
)

// DetectConflicts performs a second pass over scored dependencies, examining
// each package's PeerConstraints (drawn from its *latest* version's metadata)
// against the versions currently pinned in the same repo.
//
// Two outcomes are possible when a conflict is found:
//
//   - CascadeGroup — the peer's latest version satisfies the constraint, so
//     both packages can be upgraded together.  A shared group identifier
//     (sorted "+" joined package names) is written onto every member.
//
//   - BlockedBy — even the peer's latest version cannot satisfy the
//     constraint.  The upgrade has no safe path until the peer publishes a
//     compatible release.
//
// The slice is mutated in place and returned for chaining.
func DetectConflicts(scored []models.ScoredDependency) []models.ScoredDependency {
	if len(scored) == 0 {
		return scored
	}

	// Index by name for O(1) lookup during constraint checks.
	byIdx := make(map[string]int, len(scored))
	for i, d := range scored {
		byIdx[d.Name] = i
	}

	uf := newUnionFind()

	for i := range scored {
		if len(scored[i].PeerConstraints) == 0 {
			continue
		}

		for peerName, constraintStr := range scored[i].PeerConstraints {
			peerIdx, ok := byIdx[peerName]
			if !ok {
				// Peer is not installed in this repo — nothing to check.
				continue
			}

			c, err := semver.NewConstraint(constraintStr)
			if err != nil {
				// Malformed constraint in the registry metadata — skip safely.
				continue
			}

			currentV, err := semver.NewVersion(scored[peerIdx].CurrentVersion)
			if err != nil {
				continue
			}

			if c.Check(currentV) {
				// Current peer version already satisfies the constraint — no conflict.
				continue
			}

			// ── Conflict detected ─────────────────────────────────────────────
			// Upgrading scored[i] to its latest version would require a peer
			// version that the repo does not currently pin.

			if scored[peerIdx].LatestVersion == "" {
				scored[i].BlockedBy = appendUnique(scored[i].BlockedBy, peerName)
				continue
			}

			latestV, err := semver.NewVersion(scored[peerIdx].LatestVersion)
			if err != nil {
				scored[i].BlockedBy = appendUnique(scored[i].BlockedBy, peerName)
				continue
			}

			if c.Check(latestV) {
				// Peer's latest satisfies the constraint → cascade upgrade.
				uf.union(scored[i].Name, peerName)
			} else {
				// Even the peer's latest is incompatible → blocked.
				scored[i].BlockedBy = appendUnique(scored[i].BlockedBy, peerName)
			}
		}
	}

	// ── Assign CascadeGroup strings ───────────────────────────────────────────
	// Build connected components from the union-find, then write the sorted
	// member list back onto every member in that component.
	groups := buildGroups(uf, scored)
	for i := range scored {
		root := uf.find(scored[i].Name)
		if members, ok := groups[root]; ok && len(members) > 1 {
			scored[i].CascadeGroup = strings.Join(members, "+")
		}
	}

	return scored
}

// buildGroups collects all union-find components that have more than one
// member, returning a map of root → sorted member names.
func buildGroups(uf *unionFind, scored []models.ScoredDependency) map[string][]string {
	groups := make(map[string][]string)
	for _, d := range scored {
		if !uf.contains(d.Name) {
			continue
		}
		root := uf.find(d.Name)
		groups[root] = append(groups[root], d.Name)
	}
	for root := range groups {
		sort.Strings(groups[root])
	}
	return groups
}

// appendUnique appends s to slice only if it is not already present.
func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

// ── Union-Find ────────────────────────────────────────────────────────────────

// unionFind is a path-compressed disjoint-set used to group packages that must
// be upgraded together.  Lexicographically smaller names become roots so that
// group identifiers are stable regardless of traversal order.
type unionFind struct {
	parent map[string]string
}

func newUnionFind() *unionFind {
	return &unionFind{parent: make(map[string]string)}
}

// contains reports whether x has been added to the union-find.
func (u *unionFind) contains(x string) bool {
	_, ok := u.parent[x]
	return ok
}

// find returns the root of x's component.  If x is not in the structure, it
// returns x without mutating state (safe for the CascadeGroup assignment loop).
func (u *unionFind) find(x string) string {
	if _, ok := u.parent[x]; !ok {
		return x // not part of any union
	}
	if u.parent[x] != x {
		u.parent[x] = u.find(u.parent[x]) // path compression
	}
	return u.parent[x]
}

// union merges the components of x and y.  The lexicographically smaller root
// wins to keep group IDs deterministic.
func (u *unionFind) union(x, y string) {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
	}
	if _, ok := u.parent[y]; !ok {
		u.parent[y] = y
	}
	rx, ry := u.find(x), u.find(y)
	if rx == ry {
		return
	}
	if rx < ry {
		u.parent[ry] = rx
	} else {
		u.parent[rx] = ry
	}
}
