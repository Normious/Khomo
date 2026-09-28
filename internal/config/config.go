package config

import (
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port     int
	Env      string
	LogLevel string
	Database string

	DefaultTimeoutMs       int
	DefaultMaxRetries      int
	DefaultRateLimitPerMin int

	CBFailureThreshold int
	CBSuccessThreshold int
	CBTimeoutSeconds   int
	CBHalfOpenMaxCalls int

	HealthCheckEnabled         bool
	HealthCheckIntervalSeconds int
	HealthCheckTimeoutSeconds  int
	HealthCheckPath            string

	TrustProxyHeaders  bool
	PreserveHostHeader bool

	LogRequestBody  bool
	LogResponseBody bool
	LogSampleRate   float64

	MaxRequestSizeBytes  int64
	MaxResponseSizeBytes int64
}

func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		Port:     getInt("PORT", 4019),
		Env:      getString("ENV", "production"),
		LogLevel: getString("LOG_LEVEL", "info"),
		Database: getString("DATABASE_PATH", "./data/khomo.db"),

		DefaultTimeoutMs:       getInt("DEFAULT_TIMEOUT_MS", 30000),
		DefaultMaxRetries:      getInt("DEFAULT_MAX_RETRIES", 2),
		DefaultRateLimitPerMin: getInt("DEFAULT_RATE_LIMIT_PER_MINUTE", 600),

		CBFailureThreshold: getInt("CB_FAILURE_THRESHOLD", 5),
		CBSuccessThreshold: getInt("CB_SUCCESS_THRESHOLD", 2),
		CBTimeoutSeconds:   getInt("CB_TIMEOUT_SECONDS", 30),
		CBHalfOpenMaxCalls: getInt("CB_HALF_OPEN_MAX_CALLS", 3),

		HealthCheckEnabled:         getBool("HEALTH_CHECK_ENABLED", true),
		HealthCheckIntervalSeconds: getInt("HEALTH_CHECK_INTERVAL_SECONDS", 30),
		HealthCheckTimeoutSeconds:  getInt("HEALTH_CHECK_TIMEOUT_SECONDS", 5),
		HealthCheckPath:            getString("HEALTH_CHECK_PATH", "/health"),

		TrustProxyHeaders:  getBool("TRUST_PROXY_HEADERS", true),
		PreserveHostHeader: getBool("PRESERVE_HOST_HEADER", false),

		LogRequestBody:  getBool("LOG_REQUEST_BODY", false),
		LogResponseBody: getBool("LOG_RESPONSE_BODY", false),
		LogSampleRate:   getFloat("LOG_SAMPLE_RATE", 1.0),

		MaxRequestSizeBytes:  int64(getInt("MAX_REQUEST_SIZE_BYTES", 10*1024*1024)),
		MaxResponseSizeBytes: int64(getInt("MAX_RESPONSE_SIZE_BYTES", 50*1024*1024)),
	}
}

func (c *Config) SetupLogger() {
	var level slog.Level
	switch c.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}

func getString(k, d string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return d
}

func getInt(k string, d int) int {
	if v, ok := os.LookupEnv(k); ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return d
}

func getBool(k string, d bool) bool {
	if v, ok := os.LookupEnv(k); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return d
}

func getFloat(k string, d float64) float64 {
	if v, ok := os.LookupEnv(k); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return d
}
