package web

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"vilicus/internal/logging"
	"vilicus/internal/store"
)

const (
	lockoutThreshold = 5
	lockoutWindow    = 15 * time.Minute
	lockoutDuration  = 15 * time.Minute
)

type failRecord struct {
	mu          sync.Mutex
	count       int
	windowStart time.Time
	lockedUntil time.Time
	lastSeen    time.Time
}

func (s *Server) failRecordFor(userID string) *failRecord {
	val, ok := s.loginFails.Load(userID)
	if !ok {
		rec := &failRecord{windowStart: time.Now(), lastSeen: time.Now()}
		actual, _ := s.loginFails.LoadOrStore(userID, rec)
		return actual.(*failRecord)
	}
	return val.(*failRecord)
}

func (s *Server) isLockedOut(userID string) bool {
	val, ok := s.loginFails.Load(userID)
	if !ok {
		return false
	}
	rec := val.(*failRecord)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return time.Now().Before(rec.lockedUntil)
}

func (s *Server) recordLoginFail(userID string) bool {
	rec := s.failRecordFor(userID)
	now := time.Now()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.lastSeen = now
	if now.Sub(rec.windowStart) > lockoutWindow {
		rec.count = 0
		rec.windowStart = now
	}
	rec.count++
	if rec.count >= lockoutThreshold {
		if now.Before(rec.lockedUntil) {
			return false
		}
		rec.lockedUntil = now.Add(lockoutDuration)
		rec.count = 0
		rec.windowStart = now
		return true
	}
	return false
}

func (s *Server) clearLoginFails(userID string) {
	s.loginFails.Delete(userID)
}

func (s *Server) sweepLoginFails(cutoff time.Time) {
	s.loginFails.Range(func(key, val any) bool {
		if rec, ok := val.(*failRecord); ok {
			rec.mu.Lock()
			idle := rec.lastSeen.Before(cutoff)
			rec.mu.Unlock()
			if idle {
				s.loginFails.Delete(key)
			}
		}
		return true
	})
}

func (s *Server) handleAdminsLogoutAll(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(SessionKey).(*store.Session)
	if !s.validateCSRF(sess, csrfLogoutAll, r.FormValue("csrf_token")) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	n, err := s.Store.DeleteAllSessions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.Store.LogDashAudit(r.Context(), sess.DiscordUserID, "logout_all",
		"killed all sessions", s.trustedClientIP(r), logging.GetID(r.Context()))

	http.SetCookie(w, s.sessionCookie("", time.Time{}, -1))
	http.Redirect(w, r, "/login?err=Logged+out+all+devices+%28"+strconv.FormatInt(n, 10)+"+sessions%29", http.StatusFound)
}

