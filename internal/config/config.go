package config

import (
	"bufio"
	"os"
	"strings"
)

// Config is the marketplace-api runtime config. All values are read from
// environment variables (with .env fallback for local dev) — CapRover sets
// them via the app's Environment Variables panel.
type Config struct {
	Port           string
	AppURL         string
	AllowedOrigins []string

	// Database (Postgres only — marketplace lives on the new postgres-db,
	// not the shared mysql-db).
	DBHost string
	DBPort string
	DBName string
	DBUser string
	DBPass string

	// Peers
	DeveloperURL string // developer-api: where promote/demote round-trips back to for source manifest
	GraphURL     string // graph: data engine, used to enrich install counts in trending

	// Gateway-trust shared secret. The my.lisaos.dev gateway sends
	// this on every proxied request so we can short-circuit auth.
	InternalSecret string

	// Cache controls
	HomeCacheTTLSeconds int // GET /api/marketplace/home cache TTL (0 = disable)
}

func Load() *Config {
	loadEnvFile(".env")

	origins := splitCSV(env(
		"ALLOWED_ORIGINS",
		"http://localhost:3000,http://localhost:4000,tauri://localhost,https://my.lisaos.dev",
	))

	return &Config{
		Port:           env("PORT", "8000"),
		AppURL:         env("APP_URL", "http://localhost:8000"),
		AllowedOrigins: origins,

		DBHost: env("DB_HOST", "srv-captain--postgres-db"),
		DBPort: env("DB_PORT", "5432"),
		DBName: env("DB_NAME", "marketplace"),
		DBUser: env("DB_USER", "marketplace_app"),
		DBPass: env("DB_PASS", ""),

		DeveloperURL: env("DEVELOPER_URL", "http://srv-captain--developer-api"),
		GraphURL:     env("GRAPH_URL", "http://srv-captain--graph"),

		InternalSecret: env("INTERNAL_SHARED_SECRET", ""),

		HomeCacheTTLSeconds: envInt("HOME_CACHE_TTL", 60),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := env(key, "")
	if v == "" {
		return fallback
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return fallback
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}
