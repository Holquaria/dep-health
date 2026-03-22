package server

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
)

const (
	sessionCookieName = "dep_health_session"
	sessionMaxAge     = 90 * 24 * 60 * 60 // 90 days
)

type ctxKey string

const ctxSession ctxKey = "session_id"

// sessionMiddleware reads or creates an anonymous session cookie and injects
// the session ID into the request context.  Every browser gets a unique UUID
// on first visit — no login required.
func sessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sid := ""
		if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
			sid = c.Value
		}
		if sid == "" {
			sid = newSessionID()
			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    sid,
				Path:     "/",
				MaxAge:   sessionMaxAge,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}

		ctx := context.WithValue(r.Context(), ctxSession, sid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sessionID extracts the session ID from the request context.
// Returns "" if the middleware has not run (should not happen for API routes).
func sessionID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxSession).(string); ok {
		return v
	}
	return ""
}

// newSessionID generates a random UUID v4 string.
func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	// Set version (4) and variant (RFC 4122).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
