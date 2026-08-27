package config

import (
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const (
	AccentColorGrey  = 0x2B2D31
	FlagComponentsV2 = 1 << 15
)

type Config struct {
	BotToken           string
	AppID              string
	GuildID            string
	OAuthClientID      string
	OAuthClientSecret  string
	OAuthRedirectURL   string
	SessionSecret      string
	SessionSecretOld   string
	AdminUserIDs       []string
	HTTPPort           int
	DBPath             string
	LogPath            string
	LogLevel           string
	LogFormat          string
	RetentionDays      int
	RetentionAuditDays int
	BackupDir          string
	CookieSecure       bool
	TrustedProxies     []*net.IPNet
	PprofEnabled       bool

	LavalinkHost     string
	LavalinkPort     int
	LavalinkPassword string
	LavalinkSecure   bool
}

func Load() *Config {
	_ = godotenv.Load()

	c := &Config{
		BotToken:          os.Getenv("DISCORD_BOT_TOKEN"),
		AppID:             os.Getenv("DISCORD_APP_ID"),
		GuildID:           os.Getenv("DISCORD_GUILD_ID"),
		OAuthClientID:     os.Getenv("DISCORD_OAUTH_CLIENT_ID"),
		OAuthClientSecret: os.Getenv("DISCORD_OAUTH_CLIENT_SECRET"),
		OAuthRedirectURL:  getEnv("DISCORD_OAUTH_REDIRECT_URL", "http://localhost:8080/auth/callback"),
		SessionSecret:     getEnv("SESSION_SECRET", "vilicus-secret-key-must-be-changed-in-prod-32b"),
		SessionSecretOld:   getEnv("SESSION_SECRET_OLD", ""),
		HTTPPort:           getEnvInt("HTTP_PORT", 8080),
		DBPath:             getEnv("DB_PATH", "data/vilicus.db"),
		LogPath:            getEnv("LOG_PATH", "data/vilicus.log"),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		LogFormat:          getEnv("LOG_FORMAT", "text"),
		RetentionDays:      getEnvInt("LOG_RETENTION_DAYS", 30),
		RetentionAuditDays: getEnvInt("RETENTION_AUDIT_DAYS", 180),
		BackupDir:          getEnv("BACKUP_DIR", "backups"),
		CookieSecure:       getEnvBool("COOKIE_SECURE", true),
		PprofEnabled:       getEnvBool("PPROF_ENABLED", false),

		LavalinkHost:     os.Getenv("LAVALINK_HOST"),
		LavalinkPort:     getEnvInt("LAVALINK_PORT", 2333),
		LavalinkPassword: os.Getenv("LAVALINK_PASSWORD"),
		LavalinkSecure:   getEnvBool("LAVALINK_SECURE", false),
	}

	adminsRaw := os.Getenv("ADMIN_USER_IDS")
	if adminsRaw != "" {
		parts := strings.Split(adminsRaw, ",")
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t != "" {
				c.AdminUserIDs = append(c.AdminUserIDs, t)
			}
		}
	}

	for _, raw := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			if strings.Contains(raw, ":") {
				raw += "/128"
			} else {
				raw += "/32"
			}
		}
		_, ipnet, err := net.ParseCIDR(raw)
		if err != nil {
			continue
		}
		c.TrustedProxies = append(c.TrustedProxies, ipnet)
	}

	return c
}

func getEnv(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}

func getEnvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getEnvBool(k string, def bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

