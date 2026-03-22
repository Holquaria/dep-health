package resolver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dep-health/models"
	"dep-health/resolver"
)

// mockNPMResponse builds a minimal npm registry JSON payload for testing.
// versions maps each version string to its peerDependencies.
func mockNPMResponse(latest string, versions map[string]map[string]string) []byte {
	type versionMeta struct {
		PeerDependencies map[string]string `json:"peerDependencies,omitempty"`
	}
	type payload struct {
		DistTags map[string]string      `json:"dist-tags"`
		Versions map[string]versionMeta `json:"versions"`
	}
	p := payload{
		DistTags: map[string]string{"latest": latest},
		Versions: make(map[string]versionMeta, len(versions)),
	}
	for ver, peers := range versions {
		p.Versions[ver] = versionMeta{PeerDependencies: peers}
	}
	b, _ := json.Marshal(p)
	return b
}

// mockOSVEmpty returns an OSV batch response with no vulnerabilities.
func mockOSVEmpty(n int) []byte {
	type result struct {
		Vulns []interface{} `json:"vulns"`
	}
	type resp struct {
		Results []result `json:"results"`
	}
	r := resp{Results: make([]result, n)}
	b, _ := json.Marshal(r)
	return b
}

// newTestResolver creates a Resolver wired to the given test server.
func newTestResolver(server *httptest.Server) *resolver.Resolver {
	r := resolver.New()
	r.NPMRegistryURL = server.URL
	r.OSVBatchURL = server.URL + "/v1/querybatch"
	return r
}

// ── Peer constraint extraction ────────────────────────────────────────────────

func TestResolveNPM_ExtractsPeerConstraints(t *testing.T) {
	// next@15.0.0 declares peerDependencies: {react: ^19.0.0, react-dom: ^19.0.0}
	nextBody := mockNPMResponse("15.0.0", map[string]map[string]string{
		"13.0.0": {"react": "^18.0.0"},
		"14.0.0": {"react": "^18.0.0"},
		"15.0.0": {"react": "^19.0.0", "react-dom": "^19.0.0"},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(nextBody)
	})
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockOSVEmpty(1))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	res := newTestResolver(server)
	enriched, err := res.Enrich(context.Background(), []models.Dependency{
		{Name: "next", CurrentVersion: "13.0.0", Ecosystem: "npm"},
	})
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}
	if len(enriched) != 1 {
		t.Fatalf("expected 1 result, got %d", len(enriched))
	}

	dep := enriched[0]

	if dep.LatestVersion != "15.0.0" {
		t.Errorf("LatestVersion = %q, want \"15.0.0\"", dep.LatestVersion)
	}

	wantPeers := map[string]string{
		"react":     "^19.0.0",
		"react-dom": "^19.0.0",
	}
	for peer, wantConstraint := range wantPeers {
		got, ok := dep.PeerConstraints[peer]
		if !ok {
			t.Errorf("PeerConstraints missing %q", peer)
			continue
		}
		if got != wantConstraint {
			t.Errorf("PeerConstraints[%q] = %q, want %q", peer, got, wantConstraint)
		}
	}
	if len(dep.PeerConstraints) != len(wantPeers) {
		t.Errorf("PeerConstraints has %d entries, want %d", len(dep.PeerConstraints), len(wantPeers))
	}
}

func TestResolveNPM_NoPeerConstraints(t *testing.T) {
	// lodash has no peerDependencies — PeerConstraints should be nil/empty.
	body := mockNPMResponse("4.17.21", map[string]map[string]string{
		"4.17.11": nil,
		"4.17.21": nil,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/lodash", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockOSVEmpty(1))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	res := newTestResolver(server)
	enriched, err := res.Enrich(context.Background(), []models.Dependency{
		{Name: "lodash", CurrentVersion: "4.17.11", Ecosystem: "npm"},
	})
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}
	if len(enriched[0].PeerConstraints) != 0 {
		t.Errorf("expected empty PeerConstraints for lodash, got %v", enriched[0].PeerConstraints)
	}
}

func TestResolveNPM_VersionsBehind(t *testing.T) {
	body := mockNPMResponse("4.17.21", map[string]map[string]string{
		"4.17.11": nil,
		"4.17.12": nil,
		"4.17.20": nil,
		"4.17.21": nil,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/lodash", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockOSVEmpty(1))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	res := newTestResolver(server)
	enriched, err := res.Enrich(context.Background(), []models.Dependency{
		{Name: "lodash", CurrentVersion: "4.17.11", Ecosystem: "npm"},
	})
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// 4.17.12, 4.17.20, 4.17.21 are in range (4.17.11, 4.17.21]
	if enriched[0].VersionsBehind != 3 {
		t.Errorf("VersionsBehind = %d, want 3", enriched[0].VersionsBehind)
	}
}

// ── Concurrent batch ─────────────────────────────────────────────────────────

func TestResolveNPM_MultipleDependencies(t *testing.T) {
	reactBody := mockNPMResponse("19.0.0", map[string]map[string]string{
		"18.2.0": nil,
		"19.0.0": nil,
	})
	nextBody := mockNPMResponse("15.0.0", map[string]map[string]string{
		"13.0.0": {"react": "^18.0.0"},
		"15.0.0": {"react": "^19.0.0"},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/react", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(reactBody)
	})
	mux.HandleFunc("/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(nextBody)
	})
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockOSVEmpty(2))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	res := newTestResolver(server)
	enriched, err := res.Enrich(context.Background(), []models.Dependency{
		{Name: "react", CurrentVersion: "18.2.0", Ecosystem: "npm"},
		{Name: "next", CurrentVersion: "13.0.0", Ecosystem: "npm"},
	})
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}
	if len(enriched) != 2 {
		t.Fatalf("expected 2 results, got %d", len(enriched))
	}

	byName := make(map[string]models.EnrichedDependency, 2)
	for _, d := range enriched {
		byName[d.Name] = d
	}

	if byName["react"].LatestVersion != "19.0.0" {
		t.Errorf("react latest = %q, want 19.0.0", byName["react"].LatestVersion)
	}
	if byName["next"].PeerConstraints["react"] != "^19.0.0" {
		t.Errorf("next peer constraint for react = %q, want ^19.0.0",
			byName["next"].PeerConstraints["react"])
	}
}

// ── LatestInMajor ─────────────────────────────────────────────────────────────

func TestResolveNPM_LatestInMajor(t *testing.T) {
	// Versions span two major lines: 4.17.x and 5.x.
	// Current is 4.17.11 → LatestInMajor should be 4.17.21, LatestVersion should be 5.1.0.
	body := mockNPMResponse("5.1.0", map[string]map[string]string{
		"4.17.11": nil,
		"4.17.15": nil,
		"4.17.21": nil,
		"5.0.0":   nil,
		"5.1.0":   nil,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/express", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockOSVEmpty(1))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	res := newTestResolver(server)
	enriched, err := res.Enrich(context.Background(), []models.Dependency{
		{Name: "express", CurrentVersion: "4.17.11", Ecosystem: "npm"},
	})
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	dep := enriched[0]
	if dep.LatestVersion != "5.1.0" {
		t.Errorf("LatestVersion = %q, want \"5.1.0\"", dep.LatestVersion)
	}
	if dep.LatestInMajor != "4.17.21" {
		t.Errorf("LatestInMajor = %q, want \"4.17.21\"", dep.LatestInMajor)
	}
}

func TestResolveNPM_LatestInMajor_SameLine(t *testing.T) {
	// All versions on the same major line — LatestInMajor should equal LatestVersion.
	body := mockNPMResponse("4.17.21", map[string]map[string]string{
		"4.17.11": nil,
		"4.17.21": nil,
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/lodash", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockOSVEmpty(1))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	res := newTestResolver(server)
	enriched, err := res.Enrich(context.Background(), []models.Dependency{
		{Name: "lodash", CurrentVersion: "4.17.11", Ecosystem: "npm"},
	})
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	dep := enriched[0]
	if dep.LatestInMajor != "4.17.21" {
		t.Errorf("LatestInMajor = %q, want \"4.17.21\"", dep.LatestInMajor)
	}
}

// ── Registry error handling ───────────────────────────────────────────────────

func TestResolveNPM_RegistryNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/nonexistent-pkg", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockOSVEmpty(1))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	res := newTestResolver(server)
	enriched, err := res.Enrich(context.Background(), []models.Dependency{
		{Name: "nonexistent-pkg", CurrentVersion: "1.0.0", Ecosystem: "npm"},
	})
	// Enrich should not propagate the error; it logs a warning and returns
	// the dep with its original fields intact.
	if err != nil {
		t.Fatalf("Enrich returned unexpected error: %v", err)
	}
	if len(enriched) != 1 {
		t.Fatalf("expected 1 result, got %d", len(enriched))
	}
	if enriched[0].LatestVersion != "" {
		t.Errorf("expected empty LatestVersion for 404 package, got %q", enriched[0].LatestVersion)
	}
}
