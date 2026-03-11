package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr             string
	StoreBackend         string
	DatabaseDSN          string
	MigrationsPath       string
	RedisAddr            string
	RedisPassword        string
	MobileMoneySecret    string
	PortalTitle          string
	AdminTokenHeader     string
	MetricsEnabled       bool
	AsynqEnabled         bool
	AsynqConcurrency     int
	AsynqSweepCron       string
	RateLimitEnabled     bool
	RateLimitRequests    int
	RateLimitWindowSecs  int
	GatewayDisconnectURL string
	GatewayAuthToken     string
	RadiusEnabled        bool
	RadiusAuthAddr       string
	RadiusAcctAddr       string
	RadiusSecret         string
	RadiusCoAAddr        string
	RadiusCoASecret      string
	RouterOSAPIURL       string
	RouterOSAPIToken     string
}

func Load() Config {
	cfg := Config{
		HTTPAddr:             getenv("HTTP_ADDR", ":8080"),
		StoreBackend:         getenv("STORE_BACKEND", "postgres"),
		DatabaseDSN:          getenv("DB_DSN", "postgres://wifi_man:wifi_man@localhost:5432/wifi_man?sslmode=disable"),
		MigrationsPath:       getenv("MIGRATIONS_PATH", "migrations"),
		RedisAddr:            getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:        getenv("REDIS_PASSWORD", ""),
		MobileMoneySecret:    getenv("MOBILE_MONEY_SECRET", ""),
		PortalTitle:          getenv("PORTAL_TITLE", "Wi-Fi Access Portal"),
		AdminTokenHeader:     getenv("ADMIN_TOKEN_HEADER", "X-Role"),
		MetricsEnabled:       getenvBool("METRICS_ENABLED", true),
		AsynqEnabled:         getenvBool("ASYNQ_ENABLED", false),
		AsynqConcurrency:     getenvInt("ASYNQ_CONCURRENCY", 5),
		AsynqSweepCron:       getenv("ASYNQ_SWEEP_CRON", "@every 1m"),
		RateLimitEnabled:     getenvBool("RATE_LIMIT_ENABLED", true),
		RateLimitRequests:    getenvInt("RATE_LIMIT_REQUESTS", 15),
		RateLimitWindowSecs:  getenvInt("RATE_LIMIT_WINDOW_SECS", 60),
		GatewayDisconnectURL: getenv("GATEWAY_DISCONNECT_URL", ""),
		GatewayAuthToken:     getenv("GATEWAY_AUTH_TOKEN", ""),
		RadiusEnabled:        getenvBool("RADIUS_ENABLED", true),
		RadiusAuthAddr:       getenv("RADIUS_AUTH_ADDR", ":1812"),
		RadiusAcctAddr:       getenv("RADIUS_ACCT_ADDR", ":1813"),
		RadiusSecret:         getenv("RADIUS_SECRET", ""),
		RadiusCoAAddr:        getenv("RADIUS_COA_ADDR", ""),
		RadiusCoASecret:      getenv("RADIUS_COA_SECRET", ""),
		RouterOSAPIURL:       getenv("ROUTEROS_API_URL", ""),
		RouterOSAPIToken:     getenv("ROUTEROS_API_TOKEN", ""),
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}
