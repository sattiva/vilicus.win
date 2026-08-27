package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"vilicus/internal/logging"
	"vilicus/internal/store"
)

func (s *Server) sessionCookie(value string, expires time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     "vilicus_session",
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.Config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("vilicus_session")
	if err == nil && c.Value != "" {
		if sess, _ := s.Store.GetSession(r.Context(), c.Value); sess != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}

	errMsg := r.URL.Query().Get("err")
	s.render(w, r, "login", "Login", "", map[string]any{
		"Error": errMsg,
	})
}

func (s *Server) handleAuthDiscord(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		slog.Error("state generation failed", "err", err)
		http.Redirect(w, r, "/login?err=Internal+error", http.StatusFound)
		return
	}
	state := hex.EncodeToString(b)

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   s.Config.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	url := s.OAuth.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	reqID := logging.GetID(r.Context())
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value == "" {
		slog.Warn("oauth state cookie missing", "req_id", reqID)
		http.Redirect(w, r, "/login?err=State+mismatch", http.StatusFound)
		return
	}
	if subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(r.URL.Query().Get("state"))) != 1 {
		slog.Warn("oauth state mismatch", "req_id", reqID)
		http.Redirect(w, r, "/login?err=State+mismatch", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "oauth_state", Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/login?err=Missing+code", http.StatusFound)
		return
	}

	tok, err := s.OAuth.Exchange(r.Context(), code)
	if err != nil {
		slog.Warn("oauth exchange failed", "err", err, "req_id", reqID)
		http.Redirect(w, r, "/login?err=OAuth+failed", http.StatusFound)
		return
	}

	client := s.OAuth.Client(r.Context(), tok)
	resp, err := client.Get("https://discord.com/api/v10/users/@me")
	if err != nil {
		slog.Warn("failed getting discord user", "err", err, "req_id", reqID)
		http.Redirect(w, r, "/login?err=Failed+user+fetch", http.StatusFound)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var du DiscordUser
	if err := json.Unmarshal(body, &du); err != nil {
		http.Redirect(w, r, "/login?err=Parse+error", http.StatusFound)
		return
	}

	if s.isLockedOut(du.ID) {
		slog.Warn("login refused: account locked", "user_id", du.ID, "req_id", reqID)
		http.Redirect(w, r, "/login?err=Account+temporarily+locked", http.StatusFound)
		return
	}

	if !s.Store.IsAdmin(r.Context(), du.ID, s.Config.AdminUserIDs) {
		if locked := s.recordLoginFail(du.ID); locked {
			slog.Warn("account locked after repeated failed logins", "user_id", du.ID, "req_id", reqID)
			_ = s.Store.LogDashAudit(r.Context(), du.ID, "login_lockout",
				"account locked: "+strconv.Itoa(lockoutThreshold)+"+ failed logins in 15m",
				s.trustedClientIP(r), reqID)
		}
		slog.Warn("unauthorized login attempt", "user_id", du.ID, "username", du.Username, "req_id", reqID)
		http.Redirect(w, r, "/login?err=Unauthorized+access", http.StatusFound)
		return
	}
	s.clearLoginFails(du.ID)

	sessBytes := make([]byte, 24)
	if _, err := rand.Read(sessBytes); err != nil {
		slog.Error("session id generation failed", "err", err, "req_id", reqID)
		http.Redirect(w, r, "/login?err=Session+error", http.StatusFound)
		return
	}
	sessID := hex.EncodeToString(sessBytes)

	sess := &store.Session{
		ID:            sessID,
		DiscordUserID: du.ID,
		Username:      du.Username,
		Avatar:        du.Avatar,
		ExpiresAt:     time.Now().Add(time.Hour * 24 * 7),
	}

	if err := s.Store.CreateSession(r.Context(), sess); err != nil {
		slog.Error("session creation failed", "err", err, "req_id", reqID)
		http.Redirect(w, r, "/login?err=Session+error", http.StatusFound)
		return
	}

	http.SetCookie(w, s.sessionCookie(sessID, sess.ExpiresAt, 0))

	slog.Info("admin logged in", "user_id", du.ID, "username", du.Username, "req_id", reqID)

	_ = s.Store.LogDashAudit(r.Context(), du.ID, "login", "dashboard login", s.trustedClientIP(r), reqID)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("vilicus_session")
	if err == nil && c.Value != "" {
		if sess, _ := s.Store.GetSession(r.Context(), c.Value); sess != nil {
			_ = s.Store.LogDashAudit(r.Context(), sess.DiscordUserID, "logout", "dashboard logout", s.trustedClientIP(r), logging.GetID(r.Context()))
		}
		_ = s.Store.DeleteSession(r.Context(), c.Value)
	}

	http.SetCookie(w, s.sessionCookie("", time.Time{}, -1))
	http.SetCookie(w, &http.Cookie{
		Name: "oauth_state", Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/login", http.StatusFound)
}

