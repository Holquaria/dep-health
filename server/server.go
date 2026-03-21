// Package server provides the HTTP REST API and SPA frontend for dep-health.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"sync"

	"dep-health/pipeline"
	"dep-health/store"
	"dep-health/web"
)

// Server holds the HTTP mux, store, and in-flight scan state.
type Server struct {
	store *store.Store
	mux   *http.ServeMux

	mu       sync.Mutex
	inflight map[int64]context.CancelFunc
}

// New creates a Server wired to the given store.
func New(st *store.Store) *Server {
	s := &Server{
		store:    st,
		mux:      http.NewServeMux(),
		inflight: make(map[int64]context.CancelFunc),
	}
	s.routes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/scans", s.listScans)
	s.mux.HandleFunc("GET /api/scans/{id}", s.getScan)
	s.mux.HandleFunc("POST /api/scans", s.triggerScan)

	// SPA fallback — all other requests serve the React app.
	s.mux.Handle("/", spaHandler())
}

// ── API handlers ──────────────────────────────────────────────────────────────

func (s *Server) listScans(w http.ResponseWriter, r *http.Request) {
	runs, err := s.store.ListScans()
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}
	jsonOK(w, runs)
}

func (s *Server) getScan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, fmt.Errorf("invalid id"), http.StatusBadRequest)
		return
	}
	run, deps, err := s.store.GetScan(id)
	if err != nil {
		jsonError(w, err, http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]any{"run": run, "deps": deps})
}

type triggerRequest struct {
	Dir     string `json:"dir"`      // absolute local path (mutually exclusive with GitURL)
	GitURL  string `json:"git_url"`  // remote git URL to clone and scan
	RepoURL string `json:"repo_url"` // informational URL stored against the run
}

func (s *Server) triggerScan(w http.ResponseWriter, r *http.Request) {
	var req triggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, fmt.Errorf("invalid request body"), http.StatusBadRequest)
		return
	}
	if req.Dir == "" && req.GitURL == "" {
		jsonError(w, fmt.Errorf("body must include dir or git_url"), http.StatusBadRequest)
		return
	}

	// For remote scans, store the git URL as the repo URL when not explicitly set.
	if req.RepoURL == "" && req.GitURL != "" {
		req.RepoURL = req.GitURL
	}

	scanDir := req.Dir
	if scanDir == "" {
		scanDir = req.GitURL // placeholder; pipeline.Run will clone it
	}

	runID, err := s.store.CreateScanRun(scanDir, req.RepoURL)
	if err != nil {
		jsonError(w, err, http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.inflight[runID] = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.inflight, runID)
			s.mu.Unlock()
			cancel()
		}()

		reports, scanErr := pipeline.Run(ctx, req.Dir, pipeline.Options{
			RepoURL: req.RepoURL,
			GitURL:  req.GitURL,
		})
		if err := s.store.FinishScanRun(runID, scanErr); err != nil {
			return
		}
		if scanErr == nil && len(reports) > 0 {
			s.store.SaveDeps(runID, reports) //nolint:errcheck
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{"id": runID, "status": "running"}) //nolint:errcheck
}

// ── SPA handler ───────────────────────────────────────────────────────────────

// spaHandler serves files from the embedded web/dist FS.
// Requests for files that exist are served directly; everything else falls back
// to index.html so that React Router can handle client-side navigation.
func spaHandler() http.Handler {
	distFS, err := fs.Sub(web.FS, "dist")
	if err != nil {
		// Embedding not ready — show a helpful message.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w,
				"Frontend not built. Run: cd frontend && npm install && npm run build",
				http.StatusServiceUnavailable)
		})
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try the exact path first.
		f, err := distFS.Open(r.URL.Path[1:]) // strip leading /
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// Fall back to index.html for SPA routing.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func jsonError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}) //nolint:errcheck
}
