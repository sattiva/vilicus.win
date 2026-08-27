package web

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)


const confirmTTL = 5 * time.Minute

func (s *Server) sweepConfirmTokens() {
	now := time.Now()
	s.confirmTokens.Range(func(key, val any) bool {
		if exp, ok := val.(time.Time); ok && exp.Before(now) {
			s.confirmTokens.Delete(key)
		}
		return true
	})
}

func (s *Server) issueConfirmToken(action, gid, uid string) (token, nonce string, exp time.Time) {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	nonce = hex.EncodeToString(raw)
	exp = time.Now().Add(confirmTTL)

	mac := hmac.New(sha256.New, []byte(s.Config.SessionSecret))
	fmt.Fprintf(mac, "confirm|%s|%s|%s|%d|%s", action, gid, uid, exp.Unix(), nonce)
	token = hex.EncodeToString(mac.Sum(nil))

	s.confirmTokens.Store(nonce, exp)
	return token, nonce, exp
}

func (s *Server) consumeConfirmToken(token, nonce, action, gid, uid string) bool {
	if token == "" || nonce == "" {
		return false
	}
	val, ok := s.confirmTokens.Load(nonce)
	if !ok {
		return false
	}
	exp, _ := val.(time.Time)
	if time.Now().After(exp) {
		s.confirmTokens.Delete(nonce)
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.Config.SessionSecret))
	fmt.Fprintf(mac, "confirm|%s|%s|%s|%d|%s", action, gid, uid, exp.Unix(), nonce)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(token)) {
		return false
	}
	_, consumed := s.confirmTokens.LoadAndDelete(nonce)
	return consumed
}

func (s *Server) resolveTargetInfo(ctx context.Context, gid, uid string) map[string]any {
	info := map[string]any{"UserID": uid}
	if s.Bot == nil || s.Bot.Session == nil || gid == "" || uid == "" {
		return info
	}

	if m, _ := s.Bot.Session.State.Member(gid, uid); m != nil {
		info["Name"] = m.User.Username
		info["GlobalName"] = m.User.GlobalName
		info["Bot"] = m.User.Bot
		info["RoleCount"] = len(m.Roles)
		if !m.JoinedAt.IsZero() {
			info["JoinedAt"] = m.JoinedAt.Format("2006-01-02")
		}
	} else if u, err := s.Bot.Session.User(uid); err == nil {
		info["Name"] = u.Username
		info["GlobalName"] = u.GlobalName
		info["Bot"] = u.Bot
		info["NotMember"] = true
	}

	if n, err := s.Store.CountCases(ctx, gid, uid); err == nil {
		info["CaseCount"] = n
	}
	return info
}

