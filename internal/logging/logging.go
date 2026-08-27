package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

type ctxKey string

const CorrelationKey ctxKey = "req_id"

type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if v := ctx.Value(CorrelationKey); v != nil {
		if id, ok := v.(string); ok && id != "" {
			r.AddAttrs(slog.String("req_id", id))
		}
	}
	return h.Handler.Handle(ctx, r)
}

func Setup(lvlStr, fmtStr, logPath string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(lvlStr) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	_ = os.MkdirAll(filepath.Dir(logPath), 0755)

	rot := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
	}

	w := io.MultiWriter(os.Stdout, rot)
	opts := &slog.HandlerOptions{Level: lvl}

	var base slog.Handler
	if strings.ToLower(fmtStr) == "json" {
		base = slog.NewJSONHandler(w, opts)
	} else {
		base = slog.NewTextHandler(w, opts)
	}

	h := &ContextHandler{Handler: base}
	l := slog.New(h)
	slog.SetDefault(l)
	return l
}

func NewID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func WithID(ctx context.Context, id string) context.Context {
	if id == "" {
		id = NewID()
	}
	return context.WithValue(ctx, CorrelationKey, id)
}

func GetID(ctx context.Context) string {
	if v := ctx.Value(CorrelationKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = NewID()
		}
		ctx := WithID(r.Context(), id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

