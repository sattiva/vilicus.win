package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"vilicus/internal/config"
	"vilicus/internal/store"
)

func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

type fixture struct {
	srv *Server
	st  *store.Store
	cfg *config.Config
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	fx := &fixture{cfg: &config.Config{
		SessionSecret: "test-secret-not-a-real-key",
		CookieSecure:  false,
		BackupDir:     t.TempDir(),
		AdminUserIDs:  []string{"seed-super"},
	}}

	st, err := store.Open(t.TempDir() + "/webtest.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fx.st = st

	srv, err := New(fx.cfg, st, nil)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	t.Cleanup(srv.Close)
	fx.srv = srv

	return fx
}

func (fx *fixture) login(t *testing.T, uid, role string) string {
	t.Helper()

	ctx := context.Background()
	switch role {
	case store.RoleSuperadmin:
		if uid != "seed-super" {
			if err := fx.st.AddAdmin(ctx, uid, "op-"+uid, store.RoleSuperadmin); err != nil {
				t.Fatalf("add superadmin: %v", err)
			}
		}
	case store.RoleAdmin, store.RoleViewer:
		if err := fx.st.AddAdmin(ctx, uid, "op-"+uid, role); err != nil {
			t.Fatalf("add admin: %v", err)
		}
	}

	sess := &store.Session{
		ID:            "sess-" + uid + "-" + role,
		DiscordUserID: uid,
		Username:      "op-" + uid,
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}
	if err := fx.st.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return sess.ID
}

func (fx *fixture) get(cookie, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "vilicus_session", Value: cookie})
	}
	rec := httptest.NewRecorder()
	fx.srv.ServeHTTP(rec, req)
	return rec
}

func (fx *fixture) post(cookie, target string, form url.Values) *httptest.ResponseRecorder {
	body := strings.NewReader(form.Encode())
	req := httptest.NewRequest(http.MethodPost, target, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "vilicus_session", Value: cookie})
	}
	rec := httptest.NewRecorder()
	fx.srv.ServeHTTP(rec, req)
	return rec
}

func (fx *fixture) sessFor(t *testing.T, cookie string) *store.Session {
	t.Helper()
	sess, err := fx.st.GetSession(context.Background(), cookie)
	if err != nil || sess == nil {
		t.Fatalf("get session %s: %v", cookie, err)
	}
	return sess
}

func TestAnonymousRedirectedToLogin(t *testing.T) {
	fx := newFixture(t)

	for _, path := range []string{"/", "/guilds", "/cases", "/console", "/maintenance"} {
		rec := fx.get("", path)
		if rec.Code != http.StatusFound {
			t.Errorf("%s anonymous: got %d, want 302", path, rec.Code)
			continue
		}
		if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/login") {
			t.Errorf("%s anonymous: Location %q, want /login", path, loc)
		}
	}
}

func TestRoleMatrix(t *testing.T) {
	fx := newFixture(t)

	viewer := fx.login(t, "u-viewer", store.RoleViewer)
	admin := fx.login(t, "u-admin", store.RoleAdmin)
	super := fx.login(t, "seed-super", store.RoleSuperadmin)

	t.Run("viewer reads everything writable by nobody", func(t *testing.T) {
		for _, path := range []string{"/", "/guilds", "/logs", "/analytics", "/cases"} {
			if rec := fx.get(viewer, path); rec.Code != http.StatusOK {
				t.Errorf("viewer GET %s: got %d, want 200", path, rec.Code)
			}
		}
		for _, tc := range []struct{ method, path string }{
			{http.MethodPost, "/cases/note"},
			{http.MethodPost, "/settings"},
			{http.MethodPost, "/guilds"},
			{http.MethodPost, "/console/exec"},
			{http.MethodPost, "/maintenance/prune"},
		} {
			if rec := fx.post(viewer, tc.path, url.Values{}); rec.Code != http.StatusForbidden {
				t.Errorf("viewer POST %s: got %d, want 403", tc.path, rec.Code)
			}
		}
	})

	t.Run("maintenance is superadmin terrain end to end", func(t *testing.T) {
		if rec := fx.get(admin, "/maintenance"); rec.Code != http.StatusForbidden {
			t.Errorf("admin GET /maintenance: got %d, want 403", rec.Code)
		}
		if rec := fx.get(super, "/maintenance"); rec.Code != http.StatusOK {
			t.Errorf("super GET /maintenance: got %d, want 200", rec.Code)
		}
	})

	t.Run("admin writes only assigned guilds", func(t *testing.T) {
		ctx := context.Background()
		if err := fx.st.AddGuildAdmin(ctx, "111111111111111111", "u-admin", "seed-super"); err != nil {
			t.Fatalf("grant guild: %v", err)
		}
		sess := fx.sessFor(t, admin)
		form := url.Values{"guild_id": {"222222222222222222"}, "prefix": {"."}, "csrf_token": {fx.srv.csrfToken(sess, csrfGuilds)}}

		if rec := fx.post(admin, "/guilds", form); rec.Code != http.StatusForbidden {
			t.Errorf("unassigned guild write: got %d, want 403", rec.Code)
		}

		form.Set("guild_id", "111111111111111111")
		rec := fx.post(admin, "/guilds", form)
		if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/guilds?saved=1") {
			t.Errorf("assigned guild write: got %d %q, want 302 /guilds?saved=1", rec.Code, rec.Header().Get("Location"))
		}
	})

	t.Run("superadmin settings write", func(t *testing.T) {
		sess := fx.sessFor(t, super)
		form := url.Values{"bot_name": {"Vilicus"}, "accent_color": {"#5865F2"}, "csrf_token": {fx.srv.csrfToken(sess, csrfSettings)}}
		rec := fx.post(super, "/settings", form)
		if rec.Code != http.StatusFound {
			t.Errorf("settings write: got %d, want 302", rec.Code)
		}
	})
}

func TestInstantRevocation(t *testing.T) {
	fx := newFixture(t)
	admin := fx.login(t, "u-gone", store.RoleAdmin)

	if rec := fx.get(admin, "/"); rec.Code != http.StatusOK {
		t.Fatalf("pre-revoke GET /: got %d, want 200", rec.Code)
	}

	if err := fx.st.DeleteAdmin(context.Background(), "u-gone"); err != nil {
		t.Fatalf("delete admin: %v", err)
	}

	rec := fx.get(admin, "/")
	if rec.Code != http.StatusFound {
		t.Fatalf("post-revoke GET /: got %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "Access+revoked") {
		t.Errorf("post-revoke Location %q, want Access+revoked", loc)
	}
	if sess, _ := fx.st.GetSession(context.Background(), admin); sess != nil {
		t.Error("session should be deleted server-side on revocation")
	}
}

func TestCSRFIsPurposeBoundAndEpochBound(t *testing.T) {
	fx := newFixture(t)
	super := fx.login(t, "seed-super", store.RoleSuperadmin)
	sess := fx.sessFor(t, super)

	wrongForm := url.Values{
		"bot_name":   {"Vilicus"},
		"csrf_token": {fx.srv.csrfToken(sess, csrfConsoleExec)},
	}
	if rec := fx.post(super, "/settings", wrongForm); rec.Code != http.StatusForbidden {
		t.Errorf("cross-purpose token: got %d, want 403", rec.Code)
	}

	garbage := url.Values{"bot_name": {"Vilicus"}, "csrf_token": {"deadbeef"}}
	if rec := fx.post(super, "/settings", garbage); rec.Code != http.StatusForbidden {
		t.Errorf("garbage token: got %d, want 403", rec.Code)
	}

	rightForm := url.Values{"bot_name": {"Renamed"}, "accent_color": {"#5865F2"}, "csrf_token": {fx.srv.csrfToken(sess, csrfSettings)}}
	if rec := fx.post(super, "/settings", rightForm); rec.Code != http.StatusFound {
		t.Fatalf("valid token pre-bump: got %d, want 302", rec.Code)
	}
	if err := fx.st.BumpAllSessionEpochs(context.Background()); err != nil {
		t.Fatalf("bump epochs: %v", err)
	}
	if rec := fx.post(super, "/settings", rightForm); rec.Code != http.StatusForbidden {
		t.Errorf("stale-epoch token: got %d, want 403", rec.Code)
	}

	sess2 := fx.sessFor(t, super)
	rightForm.Set("csrf_token", fx.srv.csrfToken(sess2, csrfSettings))
	if rec := fx.post(super, "/settings", rightForm); rec.Code != http.StatusFound {
		t.Errorf("fresh-epoch token: got %d, want 302", rec.Code)
	}
}

func TestMaintenanceActionsAudited(t *testing.T) {
	fx := newFixture(t)
	super := fx.login(t, "seed-super", store.RoleSuperadmin)
	sess := fx.sessFor(t, super)

	form := url.Values{"csrf_token": {fx.srv.csrfToken(sess, csrfMaintVacuum)}}
	rec := fx.post(super, "/maintenance/vacuum", form)
	if rec.Code != http.StatusFound {
		t.Fatalf("vacuum: got %d, want 302", rec.Code)
	}

	audit, err := fx.st.ListDashAudit(context.Background(), 5)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	found := false
	for _, a := range audit {
		if a.Action == "maintenance_vacuum" && a.ActorID == "seed-super" {
			found = true
		}
	}
	if !found {
		t.Error("vacuum not recorded in dashboard audit log")
	}
}

func TestCSRFSecretRotationGrace(t *testing.T) {
	fx := newFixture(t)
	fx.cfg.SessionSecretOld = "previous-secret-during-rotation"
	super := fx.login(t, "seed-super", store.RoleSuperadmin)
	sess := fx.sessFor(t, super)

	oldToken := csrfTokenFor([]byte(fx.cfg.SessionSecretOld), sess, csrfSettings)
	liveToken := fx.srv.csrfToken(sess, csrfSettings)

	if !fx.srv.validateCSRF(sess, csrfSettings, liveToken) {
		t.Error("live-secret token must validate")
	}
	if !fx.srv.validateCSRF(sess, csrfSettings, oldToken) {
		t.Error("old-secret token should validate inside the grace window")
	}

	fx.srv.startedAt = time.Now().Add(-25 * time.Hour)
	if fx.srv.validateCSRF(sess, csrfSettings, oldToken) {
		t.Error("old-secret token should expire after the grace window")
	}
	if !fx.srv.validateCSRF(sess, csrfSettings, liveToken) {
		t.Error("live-secret token must survive grace-window expiry")
	}

	fx.srv.startedAt = time.Now()
	fx.cfg.SessionSecretOld = ""
	if fx.srv.validateCSRF(sess, csrfSettings, oldToken) {
		t.Error("old-secret token must be rejected when no old secret is configured")
	}
}

func TestLoginLockout(t *testing.T) {
	fx := newFixture(t)

	const uid = "u-victim"
	for i := 1; i < lockoutThreshold; i++ {
		if locked := fx.srv.recordLoginFail(uid); locked {
			t.Fatalf("failure %d triggered lockout early", i)
		}
		if fx.srv.isLockedOut(uid) {
			t.Fatalf("locked out after %d failures", i)
		}
	}

	if locked := fx.srv.recordLoginFail(uid); !locked {
		t.Error("threshold-crossing failure did not report lockout")
	}
	if !fx.srv.isLockedOut(uid) {
		t.Fatal("account not locked at threshold")
	}
	if locked := fx.srv.recordLoginFail(uid); locked {
		t.Error("duplicate lockout report while already locked")
	}

	val, _ := fx.srv.loginFails.Load(uid)
	rec := val.(*failRecord)
	rec.mu.Lock()
	rec.lockedUntil = time.Now().Add(-time.Minute)
	rec.mu.Unlock()
	if fx.srv.isLockedOut(uid) {
		t.Error("lockout should lift after its duration")
	}

	val, _ = fx.srv.loginFails.Load(uid)
	rec = val.(*failRecord)
	rec.mu.Lock()
	rec.windowStart = time.Now().Add(-lockoutWindow - time.Minute)
	rec.mu.Unlock()
	for i := 1; i < lockoutThreshold; i++ {
		if locked := fx.srv.recordLoginFail(uid); locked {
			t.Fatalf("stale-window failure %d re-locked prematurely", i)
		}
	}
	if locked := fx.srv.recordLoginFail(uid); !locked {
		t.Error("full failure run after window reset did not lock")
	}
}

func TestLogoutAllKillSwitch(t *testing.T) {
	fx := newFixture(t)
	super := fx.login(t, "seed-super", store.RoleSuperadmin)
	bystander := fx.login(t, "u-bystander", store.RoleAdmin)

	sess := fx.sessFor(t, super)
	form := url.Values{"csrf_token": {fx.srv.csrfToken(sess, csrfLogoutAll)}}

	if rec := fx.post(bystander, "/admins/logoutall", form); rec.Code != http.StatusForbidden {
		t.Errorf("non-super kill switch: got %d, want 403", rec.Code)
	}
	badForm := url.Values{"csrf_token": {fx.srv.csrfToken(fx.sessFor(t, super), csrfSettings)}}
	if rec := fx.post(super, "/admins/logoutall", badForm); rec.Code != http.StatusForbidden {
		t.Errorf("wrong-purpose kill switch: got %d, want 403", rec.Code)
	}

	rec := fx.post(super, "/admins/logoutall", form)
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Fatalf("kill switch: got %d %q, want 302 /login", rec.Code, rec.Header().Get("Location"))
	}

	ctx := context.Background()
	for _, cookie := range []string{super, bystander} {
		if s, _ := fx.st.GetSession(ctx, cookie); s != nil {
			t.Errorf("session %s survived the kill switch", cookie)
		}
	}

	audit, err := fx.st.ListDashAudit(ctx, 10)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	found := false
	for _, a := range audit {
		if a.Action == "logout_all" && a.ActorID == "seed-super" {
			found = true
		}
	}
	if !found {
		t.Error("logout_all not recorded in dashboard audit log")
	}
}

func TestWriteRateLimiter(t *testing.T) {
	fx := newFixture(t)

	passes := 0
	h := fx.srv.writeRateLimit(func(w http.ResponseWriter, _ *http.Request) { passes++ })

	req := httptest.NewRequest(http.MethodPost, "/cases/note", nil)
	for i := 0; i <= int(writeBurst); i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		want := http.StatusOK
		if i == int(writeBurst) {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("write %d: got %d, want %d", i+1, rec.Code, want)
		}
	}
	if passes != int(writeBurst) {
		t.Errorf("handler ran %d times, want %d", passes, int(writeBurst))
	}
	if n := fx.srv.rejectedWrite.Load(); n != 1 {
		t.Errorf("rejectedWrite counter = %d, want 1", n)
	}
}

