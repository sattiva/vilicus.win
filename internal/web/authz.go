package web

import (
	"context"
	"net/http"

	"vilicus/internal/store"
)


type ctxKey int

const (
	RoleKey ctxKey = iota
)

func roleOf(r *http.Request) string {
	if v, ok := r.Context().Value(RoleKey).(string); ok {
		return v
	}
	return ""
}

func (s *Server) resolveRole(ctx context.Context, sess *store.Session) string {
	if adm, err := s.Store.GetAdmin(ctx, sess.DiscordUserID); err == nil && adm != nil {
		return store.NormalizeAdminRole(adm.Role)
	}
	for _, id := range s.Config.AdminUserIDs {
		if id == sess.DiscordUserID {
			return store.RoleSuperadmin
		}
	}
	return ""
}

func (s *Server) canManageGuild(r *http.Request, gid string) bool {
	if gid == "" {
		return false
	}
	switch roleOf(r) {
	case store.RoleSuperadmin:
		return true
	case store.RoleAdmin:
		sess := r.Context().Value(SessionKey).(*store.Session)
		return s.Store.IsGuildAdmin(r.Context(), gid, sess.DiscordUserID)
	default:
		return false
	}
}

func forbidden(w http.ResponseWriter) {
	http.Error(w, "Forbidden", http.StatusForbidden)
}

func (s *Server) requireSuperadmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if roleOf(r) != store.RoleSuperadmin {
			forbidden(w)
			return
		}
		next(w, r)
	}
}

func (s *Server) requireWriter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if roleOf(r) == store.RoleViewer {
			forbidden(w)
			return
		}
		next(w, r)
	}
}

