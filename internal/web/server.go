package web

import (
	"context"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/oauth2"

	"vilicus/internal/config"
	"vilicus/internal/discord"
	"vilicus/internal/logging"
	"vilicus/internal/store"
)

type Server struct {
	Config    *config.Config
	Store     *store.Store
	Bot       *discord.Bot
	OAuth     *oauth2.Config
	Templates map[string]*template.Template
	Router    *chi.Mux
	rateMap   sync.Map

	startedAt time.Time

	confirmTokens sync.Map

	loginFails sync.Map

	rejectedGlobal atomic.Int64
	rejectedAuth   atomic.Int64
	rejectedWrite  atomic.Int64

	stopJanitor chan struct{}
	janitorDone chan struct{}
}

type rateBucket struct {
	tokens    float64
	lastCheck time.Time
	mu        sync.Mutex
}

type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	Avatar        string `json:"avatar"`
}

type SessionContextKey string

const SessionKey SessionContextKey = "session"

const (
	rateIdleEvictAfter = 30 * time.Minute
	rateJanitorPeriod  = 10 * time.Minute
)

func New(cfg *config.Config, st *store.Store, b *discord.Bot) (*Server, error) {
	tmplPath := filepath.Join("web", "templates", "*.html")
	funcs := template.FuncMap{
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	}

	layoutPath := filepath.Join("web", "templates", "layout.html")
	pagePaths, err := filepath.Glob(tmplPath)
	if err != nil {
		return nil, err
	}
	tmpls := make(map[string]*template.Template, len(pagePaths))
	for _, p := range pagePaths {
		base := filepath.Base(p)
		if base == "layout.html" {
			continue
		}
		key := strings.TrimSuffix(base, ".html")
		t := template.New(key).Funcs(funcs)
		if base == "login.html" {
			t, err = t.ParseFiles(p)
		} else {
			t, err = t.ParseFiles(layoutPath, p)
		}
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", base, err)
		}
		tmpls[key] = t
	}

	oauthCfg := &oauth2.Config{
		ClientID:     cfg.OAuthClientID,
		ClientSecret: cfg.OAuthClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://discord.com/api/oauth2/authorize",
			TokenURL: "https://discord.com/api/oauth2/token",
		},
		RedirectURL: cfg.OAuthRedirectURL,
		Scopes:      []string{"identify"},
	}

	srv := &Server{
		Config:      cfg,
		Store:       st,
		Bot:         b,
		OAuth:       oauthCfg,
		Templates:   tmpls,
		Router:      chi.NewRouter(),
		startedAt:   time.Now(),
		stopJanitor: make(chan struct{}),
		janitorDone: make(chan struct{}),
	}

	srv.startRateJanitor()
	srv.routes()
	return srv, nil
}

func (s *Server) Close() {
	close(s.stopJanitor)
	<-s.janitorDone
}

func (s *Server) startRateJanitor() {
	go func() {
		defer close(s.janitorDone)
		tick := time.NewTicker(rateJanitorPeriod)
		defer tick.Stop()
		for {
			select {
			case <-s.stopJanitor:
				return
			case <-tick.C:
				cutoff := time.Now().Add(-rateIdleEvictAfter)
				s.rateMap.Range(func(key, val any) bool {
					if bk, ok := val.(*rateBucket); ok {
						bk.mu.Lock()
						idle := bk.lastCheck.Before(cutoff)
						bk.mu.Unlock()
						if idle {
							s.rateMap.Delete(key)
						}
					}
					return true
				})
				s.sweepConfirmTokens()
				s.sweepLoginFails(cutoff)
			}
		}
	}()
}

func (s *Server) routes() {
	r := s.Router

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(logging.HTTPMiddleware)
	r.Use(s.securityHeaders)
	r.Use(s.globalRateLimit)

	r.Get("/login", s.handleLogin)
	r.Get("/auth/discord", s.authRateLimit(s.handleAuthDiscord))
	r.Get("/auth/callback", s.authRateLimit(s.handleAuthCallback))
	r.Get("/auth/logout", s.handleAuthLogout)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.Store.Ping(ctx); err != nil {
			http.Error(w, "db unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	staticDir := filepath.Join("web", "static")
	if _, err := os.Stat(staticDir); err == nil {
		r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	}

	r.Group(func(pr chi.Router) {
		pr.Use(s.authMiddleware)
		pr.Get("/", s.handleDashboard)
		pr.Get("/guilds", s.handleGuilds)
		pr.Post("/guilds", s.requireWriter(s.writeRateLimit(s.handleGuildsUpdate)))
		pr.Get("/guilds/export", s.requireWriter(s.handleGuildExport))
		pr.Post("/guilds/import", s.requireWriter(s.writeRateLimit(s.handleGuildImport)))
		pr.Get("/logs", s.handleLogs)
		pr.Get("/analytics", s.handleAnalytics)
		pr.Get("/cases", s.handleCases)
		pr.Get("/cases/view", s.handleCaseDetail)
		pr.Post("/cases/note", s.requireWriter(s.writeRateLimit(s.handleCaseNoteAdd)))
		pr.Post("/cases/deactivate", s.requireWriter(s.writeRateLimit(s.handleCaseDeactivate)))
		pr.Get("/community", s.handleCommunity)
		pr.Get("/admins", s.handleAdmins)
		pr.Post("/admins", s.requireSuperadmin(s.writeRateLimit(s.handleAdminsAdd)))
		pr.Post("/admins/delete", s.requireSuperadmin(s.writeRateLimit(s.handleAdminsDelete)))
		pr.Post("/admins/role", s.requireSuperadmin(s.writeRateLimit(s.handleAdminsRole)))
		pr.Post("/admins/logoutall", s.requireSuperadmin(s.writeRateLimit(s.handleAdminsLogoutAll)))
		pr.Post("/admins/guilds/grant", s.requireSuperadmin(s.writeRateLimit(s.handleGuildAdminGrant)))
		pr.Post("/admins/guilds/revoke", s.requireSuperadmin(s.writeRateLimit(s.handleGuildAdminRevoke)))
		pr.Get("/settings", s.handleSettings)
		pr.Post("/settings", s.requireSuperadmin(s.writeRateLimit(s.handleSettingsUpdate)))
		pr.Get("/console", s.handleConsole)
		pr.Post("/console/exec", s.requireWriter(s.writeRateLimit(s.handleConsoleExec)))
		pr.Get("/maintenance", s.requireSuperadmin(s.handleMaintenance))
		pr.Post("/maintenance/backup", s.requireSuperadmin(s.writeRateLimit(s.handleMaintenanceBackup)))
		pr.Post("/maintenance/prune", s.requireSuperadmin(s.writeRateLimit(s.handleMaintenancePrune)))
		pr.Post("/maintenance/vacuum", s.requireSuperadmin(s.writeRateLimit(s.handleMaintenanceVacuum)))
		pr.Post("/maintenance/archive", s.requireSuperadmin(s.writeRateLimit(s.handleMaintenanceArchive)))
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Router.ServeHTTP(w, r)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host
}

func (s *Server) trustedClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if len(s.Config.TrustedProxies) == 0 {
		return host
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}
	trusted := false
	for _, n := range s.Config.TrustedProxies {
		if n.Contains(ip) {
			trusted = true
			break
		}
	}
	if !trusted {
		return host
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		cand := net.ParseIP(strings.TrimSpace(parts[i]))
		if cand == nil {
			continue
		}
		untrusted := true
		for _, n := range s.Config.TrustedProxies {
			if n.Contains(cand) {
				untrusted = false
				break
			}
		}
		if untrusted {
			return cand.String()
		}
	}
	return host
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data: https:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if s.Config.CookieSecure {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

const (
	writeRefillPerSec = 10.0 / 60.0
	writeBurst        = 10.0
)

func (s *Server) writeRateLimit(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !takeToken(s.getBucket("write_"+s.trustedClientIP(r), writeBurst), writeRefillPerSec, writeBurst) {
			s.rejectedWrite.Add(1)
			http.Error(w, "Write rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		fn(w, r)
	}
}

func (s *Server) getBucket(ip string, cap float64) *rateBucket {
	val, ok := s.rateMap.Load(ip)
	if !ok {
		b := &rateBucket{tokens: cap, lastCheck: time.Now()}
		actual, _ := s.rateMap.LoadOrStore(ip, b)
		return actual.(*rateBucket)
	}
	return val.(*rateBucket)
}

func takeToken(b *rateBucket, refillPerSec, cap float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	b.tokens += now.Sub(b.lastCheck).Seconds() * refillPerSec
	b.lastCheck = now
	if b.tokens > cap {
		b.tokens = cap
	}
	if b.tokens < 1.0 {
		return false
	}
	b.tokens -= 1.0
	return true
}

func (s *Server) globalRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !takeToken(s.getBucket(s.trustedClientIP(r), 60.0), 30.0, 60.0) {
			s.rejectedGlobal.Add(1)
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authRateLimit(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !takeToken(s.getBucket("auth_"+s.trustedClientIP(r), 5.0), 0.5, 5.0) {
			s.rejectedAuth.Add(1)
			http.Error(w, "Auth rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		fn(w, r)
	}
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("vilicus_session")
		if err != nil || c.Value == "" {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		sess, err := s.Store.GetSession(r.Context(), c.Value)
		if err != nil || sess == nil {
			http.SetCookie(w, &http.Cookie{
				Name:     "vilicus_session",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				Secure:   s.Config.CookieSecure,
				SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		role := s.resolveRole(r.Context(), sess)
		if role == "" {
			_ = s.Store.DeleteSession(r.Context(), sess.ID)
			http.SetCookie(w, &http.Cookie{
				Name: "vilicus_session", Value: "", Path: "/", MaxAge: -1,
				HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
			})
			http.Redirect(w, r, "/login?err=Access+revoked", http.StatusFound)
			return
		}

		ctx := context.WithValue(r.Context(), SessionKey, sess)
		ctx = context.WithValue(ctx, RoleKey, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, page, title, nav string, extra map[string]any) {
	data := map[string]any{
		"Title":      title,
		"CurrentNav": nav,
		"BotName":    s.Store.GetSetting("bot_name", "Vilicus"),
	}

	if sessVal := r.Context().Value(SessionKey); sessVal != nil {
		if sess, ok := sessVal.(*store.Session); ok {
			data["User"] = sess
			data["CSRF"] = s.csrfMap(sess)
		}
	}

	switch roleOf(r) {
	case store.RoleSuperadmin:
		data["Role"], data["IsSuper"], data["CanWrite"] = store.RoleSuperadmin, true, true
	case store.RoleViewer:
		data["Role"], data["IsSuper"], data["CanWrite"] = store.RoleViewer, false, false
	default:
		data["Role"], data["IsSuper"], data["CanWrite"] = store.RoleAdmin, false, true
	}

	for k, v := range extra {
		data[k] = v
	}

	t, ok := s.Templates[page]
	if !ok {
		http.Error(w, "unknown page: "+page, http.StatusInternalServerError)
		return
	}
	root := "layout.html"
	if page == "login" {
		root = "login.html"
	}
	_ = t.ExecuteTemplate(w, root, data)
}

