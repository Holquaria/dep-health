package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// apiRequest is a helper that makes an API request and returns the recorder.
// If cookie is non-empty it is sent as the session cookie.
func apiRequest(srv *server.Server, method, path, body, cookie string) *httptest.ResponseRecorder {
	var bodyReader *bytes.Buffer
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	} else {
		bodyReader = &bytes.Buffer{}
	}
	r := httptest.NewRequest(method, path, bodyReader)
	r.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: "dep_health_session", Value: cookie})
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w
}

// extractSessionCookie returns the dep_health_session cookie value from a response.
func extractSessionCookie(w *httptest.ResponseRecorder) string {
	for _, c := range w.Result().Cookies() {
		if c.Name == "dep_health_session" {
			return c.Value
		}
	}
	return ""
}

func TestListScansEmpty(t *testing.T) {
	srv := newTestServer(t)

	w := apiRequest(srv, http.MethodGet, "/api/scans", "", "")
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

	w := apiRequest(srv, http.MethodGet, "/api/scans/9999", "", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetScanBadID(t *testing.T) {
	srv := newTestServer(t)

	w := apiRequest(srv, http.MethodGet, "/api/scans/not-a-number", "", "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTriggerScanMissingBody(t *testing.T) {
	srv := newTestServer(t)

	w := apiRequest(srv, http.MethodPost, "/api/scans", `{}`, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTriggerScanAccepted(t *testing.T) {
	srv := newTestServer(t)

	w := apiRequest(srv, http.MethodPost, "/api/scans", `{"dir": "/tmp/proj"}`, "")
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

	w := apiRequest(srv, http.MethodPost, "/api/scans/multi", `{"targets": ["/tmp/only-one"]}`, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTriggerMultiScanAccepted(t *testing.T) {
	srv := newTestServer(t)

	w := apiRequest(srv, http.MethodPost, "/api/scans/multi", `{"targets": ["/tmp/a", "/tmp/b"]}`, "")
	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestGitURLAutoPromotion(t *testing.T) {
	srv := newTestServer(t)

	w := apiRequest(srv, http.MethodPost, "/api/scans", `{"dir": "https://github.com/org/repo"}`, "")
	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want %d — git URL in dir field should be auto-promoted",
			w.Code, http.StatusAccepted)
	}
}

// ── Session tests ─────────────────────────────────────────────────────────────

func TestSessionCookieIsSetOnFirstRequest(t *testing.T) {
	srv := newTestServer(t)

	w := apiRequest(srv, http.MethodGet, "/api/scans", "", "")
	sid := extractSessionCookie(w)
	if sid == "" {
		t.Error("expected Set-Cookie with session ID on first request, got none")
	}
	if len(sid) < 32 {
		t.Errorf("session ID looks too short: %q", sid)
	}
}

func TestSessionCookieIsPreserved(t *testing.T) {
	srv := newTestServer(t)

	// First request — get a cookie.
	w1 := apiRequest(srv, http.MethodGet, "/api/scans", "", "")
	sid := extractSessionCookie(w1)
	if sid == "" {
		t.Fatal("no session cookie on first request")
	}

	// Second request — send the cookie back; should NOT get a new Set-Cookie.
	w2 := apiRequest(srv, http.MethodGet, "/api/scans", "", sid)
	newSid := extractSessionCookie(w2)
	if newSid != "" {
		t.Errorf("expected no new Set-Cookie when sending existing session, got %q", newSid)
	}
}

func TestSessionIsolation_ScansNotShared(t *testing.T) {
	srv := newTestServer(t)

	// Session A triggers a scan.
	wA := apiRequest(srv, http.MethodPost, "/api/scans", `{"dir": "/tmp/proj-a"}`, "session-aaa")
	if wA.Code != http.StatusAccepted {
		t.Fatalf("trigger A: status %d", wA.Code)
	}
	var respA map[string]any
	json.Unmarshal(wA.Body.Bytes(), &respA)
	scanID := fmt.Sprintf("%.0f", respA["id"].(float64))

	// Session A can list its scan.
	wListA := apiRequest(srv, http.MethodGet, "/api/scans", "", "session-aaa")
	var runsA []any
	json.Unmarshal(wListA.Body.Bytes(), &runsA)
	if len(runsA) != 1 {
		t.Errorf("session-aaa: expected 1 scan, got %d", len(runsA))
	}

	// Session B should see zero scans.
	wListB := apiRequest(srv, http.MethodGet, "/api/scans", "", "session-bbb")
	var runsB []any
	json.Unmarshal(wListB.Body.Bytes(), &runsB)
	if len(runsB) != 0 {
		t.Errorf("session-bbb: expected 0 scans, got %d", len(runsB))
	}

	// Session B should get 404 for session A's scan ID.
	wGetB := apiRequest(srv, http.MethodGet, "/api/scans/"+scanID, "", "session-bbb")
	if wGetB.Code != http.StatusNotFound {
		t.Errorf("session-bbb accessing session-aaa's scan: got %d, want %d",
			wGetB.Code, http.StatusNotFound)
	}

	// Session A should be able to view its own scan.
	wGetA := apiRequest(srv, http.MethodGet, "/api/scans/"+scanID, "", "session-aaa")
	if wGetA.Code != http.StatusOK {
		t.Errorf("session-aaa accessing own scan: got %d, want %d",
			wGetA.Code, http.StatusOK)
	}
}
