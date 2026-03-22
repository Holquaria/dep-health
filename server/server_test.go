package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"dep-health/server"
	"dep-health/store"
)

func newTestServer(t *testing.T) *server.Server {
	t.Helper()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return server.New(st)
}

func TestListScansEmpty(t *testing.T) {
	srv := newTestServer(t)

	r := httptest.NewRequest(http.MethodGet, "/api/scans", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var result []any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestGetScanNotFound(t *testing.T) {
	srv := newTestServer(t)

	r := httptest.NewRequest(http.MethodGet, "/api/scans/9999", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetScanBadID(t *testing.T) {
	srv := newTestServer(t)

	r := httptest.NewRequest(http.MethodGet, "/api/scans/not-a-number", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTriggerScanMissingBody(t *testing.T) {
	srv := newTestServer(t)

	body := bytes.NewBufferString(`{}`)
	r := httptest.NewRequest(http.MethodPost, "/api/scans", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTriggerScanAccepted(t *testing.T) {
	srv := newTestServer(t)

	body := bytes.NewBufferString(`{"dir": "/tmp/proj"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/scans", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusAccepted)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["id"]; !ok {
		t.Error("response missing 'id' field")
	}
}

func TestTriggerMultiScanTooFewTargets(t *testing.T) {
	srv := newTestServer(t)

	body := bytes.NewBufferString(`{"targets": ["/tmp/only-one"]}`)
	r := httptest.NewRequest(http.MethodPost, "/api/scans/multi", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTriggerMultiScanAccepted(t *testing.T) {
	srv := newTestServer(t)

	body := bytes.NewBufferString(`{"targets": ["/tmp/a", "/tmp/b"]}`)
	r := httptest.NewRequest(http.MethodPost, "/api/scans/multi", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestGitURLAutoPromotion(t *testing.T) {
	srv := newTestServer(t)

	// Passing a git URL as "dir" should be auto-promoted and accepted (202),
	// not rejected as an invalid path.
	body := bytes.NewBufferString(`{"dir": "https://github.com/org/repo"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/scans", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want %d — git URL in dir field should be auto-promoted",
			w.Code, http.StatusAccepted)
	}
}
