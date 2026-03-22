// Package resolver enriches raw dependencies with the latest published version
// from each ecosystem's registry and known CVEs from OSV.dev.
package resolver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"golang.org/x/mod/module"

	"dep-health/models"
)

const (
	npmRegistryBase    = "https://registry.npmjs.org"
	pypiRegistryBase   = "https://pypi.org/pypi"
	mavenCentralSearch = "https://search.maven.org/solrsearch/select"
	osvBatchURL        = "https://api.osv.dev/v1/querybatch"

	defaultConcurrency = 10
)

var defaultHTTPClient = &http.Client{Timeout: 15 * time.Second}

// Resolver enriches dependencies concurrently using package registries and
// the OSV.dev vulnerability database.
type Resolver struct {
	HTTPClient  *http.Client
	Concurrency int
	// NPMRegistryURL overrides the npm registry base URL.  Leave empty to use
	// the default (https://registry.npmjs.org).  Set in tests to point at a
	// local httptest.Server.
	NPMRegistryURL string
	// PyPIRegistryURL overrides the PyPI registry base URL.  Leave empty to use
	// the default (https://pypi.org/pypi).  Set in tests to point at a local
	// httptest.Server.
	PyPIRegistryURL string
	// MavenCentralURL overrides the Maven Central search endpoint.  Leave empty
	// to use the default (https://search.maven.org/solrsearch/select).  Set in
	// tests to point at a local httptest.Server.
	MavenCentralURL string
	// OSVBatchURL overrides the OSV.dev batch endpoint.  Leave empty to use
	// the default.  Set in tests to point at a local httptest.Server.
	OSVBatchURL string
}

// New creates a Resolver with sensible defaults.
func New() *Resolver {
	return &Resolver{
		HTTPClient:  defaultHTTPClient,
		Concurrency: defaultConcurrency,
	}
}

// npmBase returns the npm registry base URL, falling back to the package constant.
func (r *Resolver) npmBase() string {
	if r.NPMRegistryURL != "" {
		return r.NPMRegistryURL
	}
	return npmRegistryBase
}

// pypiBase returns the PyPI registry base URL, falling back to the package constant.
func (r *Resolver) pypiBase() string {
	if r.PyPIRegistryURL != "" {
		return r.PyPIRegistryURL
	}
	return pypiRegistryBase
}

// mavenBase returns the Maven Central search URL, falling back to the package constant.
func (r *Resolver) mavenBase() string {
	if r.MavenCentralURL != "" {
		return r.MavenCentralURL
	}
	return mavenCentralSearch
}

// osvEndpoint returns the OSV batch URL, falling back to the package constant.
func (r *Resolver) osvEndpoint() string {
	if r.OSVBatchURL != "" {
		return r.OSVBatchURL
	}
	return osvBatchURL
}

// Enrich concurrently fetches the latest version for each dependency, then
// performs a single batch OSV.dev query for all CVEs.
func (r *Resolver) Enrich(ctx context.Context, deps []models.Dependency) ([]models.EnrichedDependency, error) {
	enriched := make([]models.EnrichedDependency, len(deps))

	sem := make(chan struct{}, r.Concurrency)
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	for i, dep := range deps {
		wg.Add(1)
		go func(idx int, d models.Dependency) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ed, err := r.resolveOne(ctx, d)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", d.Name, err))
				// Preserve the raw dependency even when enrichment fails.
				enriched[idx] = models.EnrichedDependency{Dependency: d}
				return
			}
			enriched[idx] = ed
		}(i, dep)
	}
	wg.Wait()

	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d resolution error(s), first: %v\n", len(errs), errs[0])
	}

	// Single batch call to OSV.dev for all dependencies.
	if err := r.enrichVulnerabilities(ctx, enriched); err != nil {
		fmt.Fprintf(os.Stderr, "warning: vulnerability lookup failed: %v\n", err)
	}

	return enriched, nil
}

// resolveOne dispatches to the appropriate registry based on the ecosystem.
func (r *Resolver) resolveOne(ctx context.Context, dep models.Dependency) (models.EnrichedDependency, error) {
	switch dep.Ecosystem {
	case "npm":
		return r.resolveNPM(ctx, dep)
	case "go":
		return r.resolveGoProxy(ctx, dep)
	case "pypi":
		return r.resolvePyPI(ctx, dep)
	case "maven":
		return r.resolveMaven(ctx, dep)
	default:
		// Return the raw dep unchanged; other ecosystems are not yet implemented.
		return models.EnrichedDependency{Dependency: dep}, nil
	}
}

// ── npm registry ──────────────────────────────────────────────────────────────

// npmVersionMeta holds the per-version fields we need from the npm registry.
// The registry returns much more per version; we decode only what we use.
type npmVersionMeta struct {
	PeerDependencies map[string]string `json:"peerDependencies"`
}

// npmPackageInfo is the subset of the npm registry response we need.
type npmPackageInfo struct {
	DistTags map[string]string         `json:"dist-tags"`
	Versions map[string]npmVersionMeta `json:"versions"`
}

func (r *Resolver) resolveNPM(ctx context.Context, dep models.Dependency) (models.EnrichedDependency, error) {
	url := fmt.Sprintf("%s/%s", r.npmBase(), dep.Name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.EnrichedDependency{Dependency: dep}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return models.EnrichedDependency{Dependency: dep}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.EnrichedDependency{Dependency: dep},
			fmt.Errorf("npm registry returned HTTP %d for %s", resp.StatusCode, dep.Name)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.EnrichedDependency{Dependency: dep}, err
	}

	var info npmPackageInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return models.EnrichedDependency{Dependency: dep}, err
	}

	latest := info.DistTags["latest"]
	if latest == "" {
		return models.EnrichedDependency{Dependency: dep},
			fmt.Errorf("no 'latest' dist-tag for %s", dep.Name)
	}

	gap := computeSeverityGap(dep.CurrentVersion, latest)
	behind := countVersionsBehind(dep.CurrentVersion, latest, info.Versions)

	// Extract peer dependency constraints declared by the *latest* version.
	// These are used downstream by scorer.DetectConflicts to identify cascade
	// upgrades and blocked dependencies.
	var peerConstraints map[string]string
	if meta, ok := info.Versions[latest]; ok && len(meta.PeerDependencies) > 0 {
		peerConstraints = meta.PeerDependencies
	}

	return models.EnrichedDependency{
		Dependency:      dep,
		LatestVersion:   latest,
		SeverityGap:     gap,
		VersionsBehind:  behind,
		PeerConstraints: peerConstraints,
	}, nil
}

// computeSeverityGap returns "major", "minor", or "patch" based on the
// semver distance between current and latest.
func computeSeverityGap(current, latest string) string {
	cur, err := semver.NewVersion(current)
	if err != nil {
		return "unknown"
	}
	lat, err := semver.NewVersion(latest)
	if err != nil {
		return "unknown"
	}
	switch {
	case lat.Major() > cur.Major():
		return "major"
	case lat.Minor() > cur.Minor():
		return "minor"
	default:
		return "patch"
	}
}

// countVersionsBehind counts published semver versions in the range
// (current, latest] so callers know exactly how many releases they missed.
func countVersionsBehind(current, latest string, versions map[string]npmVersionMeta) int {
	cur, err := semver.NewVersion(current)
	if err != nil {
		return 0
	}
	lat, err := semver.NewVersion(latest)
	if err != nil {
		return 0
	}
	if !cur.LessThan(lat) {
		return 0
	}

	var svs []*semver.Version
	for v := range versions {
		sv, err := semver.NewVersion(v)
		if err != nil {
			continue
		}
		// Count versions strictly greater than current, up to and including latest.
		if sv.GreaterThan(cur) && (sv.LessThan(lat) || sv.Equal(lat)) {
			svs = append(svs, sv)
		}
	}
	return len(svs)
}

// sortedVersionKeys returns registry version keys in ascending semver order.
// Used for debugging / future features; exported so callers can list skipped versions.
func sortedVersionKeys(versions map[string]npmVersionMeta) []string {
	var svs []*semver.Version
	for v := range versions {
		sv, err := semver.NewVersion(v)
		if err == nil {
			svs = append(svs, sv)
		}
	}
	sort.Sort(semver.Collection(svs))
	out := make([]string, len(svs))
	for i, sv := range svs {
		out[i] = sv.Original()
	}
	return out
}

// ── Go module proxy ───────────────────────────────────────────────────────────

const goProxyBase = "https://proxy.golang.org"

// goLatestResponse is the JSON body returned by the Go module proxy @latest endpoint.
type goLatestResponse struct {
	Version string `json:"Version"`
	Time    string `json:"Time"`
}

// resolveGoProxy fetches the latest version and full version list for a Go
// module from proxy.golang.org, then computes the version gap and lag count.
func (r *Resolver) resolveGoProxy(ctx context.Context, dep models.Dependency) (models.EnrichedDependency, error) {
	// The proxy requires uppercase characters in module paths to be escaped
	// as "!" + lowercase (e.g. "github.com/BurntSushi/toml" → "github.com/!burnt!sushi/toml").
	escaped, err := module.EscapePath(dep.Name)
	if err != nil {
		return models.EnrichedDependency{Dependency: dep},
			fmt.Errorf("escaping module path %q: %w", dep.Name, err)
	}

	base := fmt.Sprintf("%s/%s", goProxyBase, escaped)

	latest, err := r.goProxyLatest(ctx, base)
	if err != nil {
		return models.EnrichedDependency{Dependency: dep}, err
	}

	// Version list is best-effort: missing it degrades VersionsBehind to 0
	// but doesn't block the rest of the pipeline.
	versionList, _ := r.goProxyList(ctx, base)

	gap := computeSeverityGap(dep.CurrentVersion, latest)
	behind := countVersionsBehindList(dep.CurrentVersion, latest, versionList)

	return models.EnrichedDependency{
		Dependency:     dep,
		LatestVersion:  latest,
		SeverityGap:    gap,
		VersionsBehind: behind,
	}, nil
}

func (r *Resolver) goProxyLatest(ctx context.Context, base string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/@latest", nil)
	if err != nil {
		return "", err
	}

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// ok
	case http.StatusNotFound, http.StatusGone:
		return "", fmt.Errorf("module not found in Go proxy (HTTP %d)", resp.StatusCode)
	default:
		return "", fmt.Errorf("Go proxy returned HTTP %d", resp.StatusCode)
	}

	var info goLatestResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("decoding @latest response: %w", err)
	}
	if info.Version == "" {
		return "", fmt.Errorf("@latest returned empty version")
	}
	return info.Version, nil
}

// goProxyList fetches the @v/list endpoint and returns all known versions for
// the module as a slice of strings (e.g. ["v1.0.0", "v1.1.0", "v2.0.0"]).
func (r *Resolver) goProxyList(ctx context.Context, base string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/@v/list", nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Go proxy @v/list returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var versions []string
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if v := strings.TrimSpace(line); v != "" {
			versions = append(versions, v)
		}
	}
	return versions, nil
}

// countVersionsBehindList counts published semver versions in the range
// (current, latest] from a plain string slice (used for Go modules).
func countVersionsBehindList(current, latest string, versions []string) int {
	cur, err := semver.NewVersion(current)
	if err != nil {
		return 0
	}
	lat, err := semver.NewVersion(latest)
	if err != nil {
		return 0
	}
	if !cur.LessThan(lat) {
		return 0
	}

	count := 0
	for _, v := range versions {
		sv, err := semver.NewVersion(v)
		if err != nil {
			continue
		}
		if sv.GreaterThan(cur) && (sv.LessThan(lat) || sv.Equal(lat)) {
			count++
		}
	}
	return count
}

// ── PyPI registry ─────────────────────────────────────────────────────────────

// pypiInfo is the subset of the PyPI JSON API response we need.
type pypiInfo struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	Releases map[string]json.RawMessage `json:"releases"`
}

// resolvePyPI fetches the latest version for a Python package from pypi.org.
func (r *Resolver) resolvePyPI(ctx context.Context, dep models.Dependency) (models.EnrichedDependency, error) {
	url := fmt.Sprintf("%s/%s/json", r.pypiBase(), dep.Name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return models.EnrichedDependency{Dependency: dep}, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return models.EnrichedDependency{Dependency: dep}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return models.EnrichedDependency{Dependency: dep},
			fmt.Errorf("package %q not found on PyPI", dep.Name)
	}
	if resp.StatusCode != http.StatusOK {
		return models.EnrichedDependency{Dependency: dep},
			fmt.Errorf("PyPI returned HTTP %d for %s", resp.StatusCode, dep.Name)
	}

	var info pypiInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return models.EnrichedDependency{Dependency: dep}, err
	}

	latest := info.Info.Version
	if latest == "" {
		return models.EnrichedDependency{Dependency: dep},
			fmt.Errorf("no version found in PyPI response for %s", dep.Name)
	}

	gap := computeSeverityGap(dep.CurrentVersion, latest)
	behind := countVersionsBehindPyPI(dep.CurrentVersion, latest, info.Releases)

	return models.EnrichedDependency{
		Dependency:     dep,
		LatestVersion:  latest,
		SeverityGap:    gap,
		VersionsBehind: behind,
	}, nil
}

// countVersionsBehindPyPI counts published versions in (current, latest] from
// the PyPI releases map.  Pre-release versions (alpha, beta, rc) are excluded.
func countVersionsBehindPyPI(current, latest string, releases map[string]json.RawMessage) int {
	cur, err := semver.NewVersion(current)
	if err != nil {
		return 0
	}
	lat, err := semver.NewVersion(latest)
	if err != nil {
		return 0
	}
	if !cur.LessThan(lat) {
		return 0
	}
	count := 0
	for v := range releases {
		sv, err := semver.NewVersion(v)
		if err != nil {
			continue
		}
		if sv.Prerelease() != "" {
			continue
		}
		if sv.GreaterThan(cur) && (sv.LessThan(lat) || sv.Equal(lat)) {
			count++
		}
	}
	return count
}

// ── Maven Central search API ──────────────────────────────────────────────────

// mavenSearchResponse is the outer wrapper returned by Maven Central's Solr API.
type mavenSearchResponse struct {
	Response struct {
		NumFound int `json:"numFound"`
		Docs     []struct {
			LatestVersion string `json:"latestVersion"`
			Version       string `json:"v"` // only set in core=gav queries
		} `json:"docs"`
	} `json:"response"`
}

// resolveMaven looks up the latest version for a Maven artifact on Maven Central.
//
// The dep.Name must be in "groupId:artifactId" format as produced by PomScanner.
// Two requests are made:
//  1. Latest version query → populates LatestVersion and SeverityGap.
//  2. Version list query   → populates VersionsBehind (best-effort, 0 on failure).
func (r *Resolver) resolveMaven(ctx context.Context, dep models.Dependency) (models.EnrichedDependency, error) {
	parts := strings.SplitN(dep.Name, ":", 2)
	if len(parts) != 2 {
		return models.EnrichedDependency{Dependency: dep},
			fmt.Errorf("invalid maven coordinate %q (expected groupId:artifactId)", dep.Name)
	}
	g, a := parts[0], parts[1]

	// Request 1: latest version.
	latest, err := r.mavenLatest(ctx, g, a)
	if err != nil {
		return models.EnrichedDependency{Dependency: dep}, err
	}

	// Request 2: full version list for VersionsBehind (best-effort).
	versions, _ := r.mavenVersionList(ctx, g, a)

	gap := computeSeverityGap(dep.CurrentVersion, latest)
	behind := countVersionsBehindList(dep.CurrentVersion, latest, versions)

	return models.EnrichedDependency{
		Dependency:     dep,
		LatestVersion:  latest,
		SeverityGap:    gap,
		VersionsBehind: behind,
	}, nil
}

func (r *Resolver) mavenLatest(ctx context.Context, g, a string) (string, error) {
	url := fmt.Sprintf("%s?q=g:%s+AND+a:%s&wt=json&rows=1", r.mavenBase(), g, a)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Maven Central returned HTTP %d for %s:%s", resp.StatusCode, g, a)
	}

	var result mavenSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding Maven Central response: %w", err)
	}
	if result.Response.NumFound == 0 || len(result.Response.Docs) == 0 {
		return "", fmt.Errorf("artifact %s:%s not found on Maven Central", g, a)
	}

	latest := result.Response.Docs[0].LatestVersion
	if latest == "" {
		return "", fmt.Errorf("no latestVersion in Maven Central response for %s:%s", g, a)
	}
	return latest, nil
}

// mavenVersionList fetches all known versions for a Maven artifact using the
// core=gav endpoint (up to 200 results — sufficient for VersionsBehind counting).
func (r *Resolver) mavenVersionList(ctx context.Context, g, a string) ([]string, error) {
	url := fmt.Sprintf("%s?q=g:%s+AND+a:%s&core=gav&wt=json&rows=200", r.mavenBase(), g, a)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Maven Central (core=gav) returned HTTP %d", resp.StatusCode)
	}

	var result mavenSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	versions := make([]string, 0, len(result.Response.Docs))
	for _, doc := range result.Response.Docs {
		if doc.Version != "" {
			versions = append(versions, doc.Version)
		}
	}
	return versions, nil
}

// ── OSV.dev batch vulnerability lookup ───────────────────────────────────────

type osvQuery struct {
	Package struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	} `json:"package"`
	Version string `json:"version"`
}

type osvBatchRequest struct {
	Queries []osvQuery `json:"queries"`
}

type osvVuln struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Severity []struct {
		Type  string `json:"type"`
		Score string `json:"score"`
	} `json:"severity"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
	References []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"references"`
}

type osvBatchResponse struct {
	Results []struct {
		Vulns []osvVuln `json:"vulns"`
	} `json:"results"`
}

// ecosystemToOSV maps dep-health ecosystem names to the identifiers OSV.dev expects.
func ecosystemToOSV(eco string) string {
	switch strings.ToLower(eco) {
	case "npm":
		return "npm"
	case "pypi":
		return "PyPI"
	case "go":
		return "Go"
	case "maven":
		return "Maven"
	default:
		return eco
	}
}

// enrichVulnerabilities performs a single OSV.dev batch query and attaches any
// found vulnerabilities to the corresponding EnrichedDependency elements.
func (r *Resolver) enrichVulnerabilities(ctx context.Context, deps []models.EnrichedDependency) error {
	if len(deps) == 0 {
		return nil
	}

	// Build one OSV query per dependency, preserving index alignment with deps.
	queries := make([]osvQuery, len(deps))
	for i, d := range deps {
		var q osvQuery
		q.Package.Name = d.Name
		q.Package.Ecosystem = ecosystemToOSV(d.Ecosystem)
		q.Version = d.CurrentVersion
		queries[i] = q
	}

	payload, err := json.Marshal(osvBatchRequest{Queries: queries})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.osvEndpoint(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OSV API returned HTTP %d", resp.StatusCode)
	}

	var osvResp osvBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&osvResp); err != nil {
		return err
	}

	for i, result := range osvResp.Results {
		if i >= len(deps) {
			break
		}
		for _, v := range result.Vulns {
			sev := v.DatabaseSpecific.Severity
			if sev == "" && len(v.Severity) > 0 {
				sev = v.Severity[0].Score
			}

			// Pick the most informative reference URL.
			url := ""
			for _, ref := range v.References {
				if ref.Type == "ADVISORY" || ref.Type == "WEB" {
					url = ref.URL
					break
				}
			}
			if url == "" && len(v.References) > 0 {
				url = v.References[0].URL
			}

			deps[i].Vulnerabilities = append(deps[i].Vulnerabilities, models.Vulnerability{
				ID:       v.ID,
				Severity: sev,
				Summary:  v.Summary,
				URL:      url,
			})
		}
	}

	return nil
}
