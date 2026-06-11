package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const cookieName = "sms_sess"
const sessionTTL = 8 * time.Hour

// User holds the data stored in a session (read from app_users at login).
type User struct {
	ID       int64
	PersonID int64 // app_users.person_id — 0 if no linked person record
	Username string
	FullName string
	Role     string
}

type sessionEntry struct {
	User   User
	Expiry time.Time
}

type Sessions struct {
	mu   sync.Mutex
	data map[string]sessionEntry
}

func NewSessions() *Sessions {
	return &Sessions{data: make(map[string]sessionEntry)}
}

// Create writes a new session cookie for u.
func (s *Sessions) Create(w http.ResponseWriter, u User) {
	tok := newToken()
	s.mu.Lock()
	s.data[tok] = sessionEntry{User: u, Expiry: time.Now().Add(sessionTTL)}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    tok,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// Delete removes the session and clears the cookie.
func (s *Sessions) Delete(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.mu.Lock()
		delete(s.data, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", MaxAge: -1, Path: "/"})
}

type ctxKey struct{}

// Middleware redirects unauthenticated requests to /login.
func (s *Sessions) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		s.mu.Lock()
		entry, ok := s.data[c.Value]
		s.mu.Unlock()
		if !ok || time.Now().After(entry.Expiry) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, entry.User)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Current returns the authenticated user from the request context.
func Current(r *http.Request) (User, bool) {
	u, ok := r.Context().Value(ctxKey{}).(User)
	return u, ok
}

func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
