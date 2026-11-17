// Package config loads and validates runtime configuration from the
// environment. Configuration is read once at startup and never mutated
// afterwards, so it can be shared safely across goroutines.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every runtime setting the API needs.
type Config struct {
	// Server
	ListenAddr        string
	PublicURL         string
	AllowedOrigins    []string
	ShutdownTimeout   time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxRequestBodyLen int64

	// Database
	DatabaseURL            string
	DatabaseMaxConns       int32
	DatabaseMinConns       int32
	DatabaseConnectTimeout time.Duration
	DatabaseQueryTimeout   time.Duration

	// Sessions and cookies
	SessionCookieName string
	SessionAbsoluteTTL time.Duration
	SessionIdleTTL     time.Duration
	CookieSecure       bool
	CSRFSecret         []byte

	// Password hashing (Argon2id)
	Argon2Time        uint32
	Argon2MemoryKiB   uint32
	Argon2Threads     uint8
	Argon2KeyLength   uint32
	Argon2SaltLength  uint32

	// SDK keys
	SDKKeyPepper []byte

	// Rate limiting
	LoginRateLimitMax      int
	LoginRateLimitWindow    time.Duration

	// SSE
	SSEHeartbeatInterval  time.Duration
	SSEMaxConnection      time.Duration
	SSESubscriberQueue    int
	SSEMaxConnectionsKey  int
}

// Load reads configuration from the environment and validates it.
//
// It returns an error rather than a partially populated Config, so a
// misconfigured deployment fails at startup instead of at first request.
func Load() (*Config, error) {
	c := &Config{
		ListenAddr:      getEnv("API_LISTEN_ADDR", ":8080"),
		PublicURL:       getEnv("API_PUBLIC_URL", "http://localhost:8080"),
		AllowedOrigins:  splitCSV(getEnv("ALLOWED_DASHBOARD_ORIGINS", "http://localhost:3000")),
		ShutdownTimeout: getDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		ReadTimeout:     getDuration("HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:    getDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:     getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		MaxRequestBodyLen: getInt64("MAX_REQUEST_BODY_BYTES", 1<<20),

		DatabaseURL:            os.Getenv("DATABASE_URL"),
		DatabaseMaxConns:       int32(getInt("DATABASE_MAX_CONNS", 10)),
		DatabaseMinConns:       int32(getInt("DATABASE_MIN_CONNS", 2)),
		DatabaseConnectTimeout: getDuration("DATABASE_CONNECT_TIMEOUT_SECONDS", 10*time.Second),
		DatabaseQueryTimeout:   getDuration("DATABASE_QUERY_TIMEOUT_SECONDS", 5*time.Second),

		SessionCookieName:  getEnv("SESSION_COOKIE_NAME", "ff_session"),
		SessionAbsoluteTTL: getDuration("SESSION_ABSOLUTE_TTL_HOURS", 24*time.Hour),
		SessionIdleTTL:     getDuration("SESSION_IDLE_TTL_HOURS", 8*time.Hour),
		CookieSecure:       getBool("COOKIE_SECURE", false),
		CSRFSecret:         []byte(os.Getenv("CSRF_SECRET")),

		Argon2Time:       uint32(getInt("ARGON2_TIME", 3)),
		Argon2MemoryKiB:  uint32(getInt("ARGON2_MEMORY_KIB", 64*1024)),
		Argon2Threads:    uint8(getInt("ARGON2_THREADS", 2)),
		Argon2KeyLength:  uint32(getInt("ARGON2_KEY_LENGTH", 32)),
		Argon2SaltLength: uint32(getInt("ARGON2_SALT_LENGTH", 16)),

		SDKKeyPepper: []byte(os.Getenv("SDK_KEY_PEPPER")),

		LoginRateLimitMax:   getInt("LOGIN_RATE_LIMIT_MAX", 5),
		LoginRateLimitWindow: getDuration("LOGIN_RATE_LIMIT_WINDOW_MINUTES", 15*time.Minute),

		SSEHeartbeatInterval: getDuration("SSE_HEARTBEAT_INTERVAL_SECONDS", 15*time.Second),
		SSEMaxConnection:     getDuration("SSE_MAX_CONNECTION_SECONDS", 3600*time.Second),
		SSESubscriberQueue:   getInt("SSE_SUBSCRIBER_QUEUE_SIZE", 16),
		SSEMaxConnectionsKey: getInt("SSE_MAX_CONNECTIONS_PER_KEY", 10),
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	var errs []error

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(c.CSRFSecret) < 32 {
		errs = append(errs, errors.New("CSRF_SECRET is required and must be at least 32 bytes"))
	}
	if c.SessionAbsoluteTTL <= 0 {
		errs = append(errs, errors.New("SESSION_ABSOLUTE_TTL_HOURS must be positive"))
	}
	if c.SessionIdleTTL <= 0 {
		errs = append(errs, errors.New("SESSION_IDLE_TTL_HOURS must be positive"))
	}
	if c.DatabaseMaxConns < 1 {
		errs = append(errs, errors.New("DATABASE_MAX_CONNS must be at least 1"))
	}
	if c.DatabaseMinConns < 0 || c.DatabaseMinConns > c.DatabaseMaxConns {
		errs = append(errs, errors.New("DATABASE_MIN_CONNS must be between 0 and DATABASE_MAX_CONNS"))
	}
	if c.Argon2MemoryKiB < 8*1024 {
		errs = append(errs, errors.New("ARGON2_MEMORY_KIB must be at least 8192"))
	}
	if c.Argon2Time < 1 {
		errs = append(errs, errors.New("ARGON2_TIME must be at least 1"))
	}
	if c.SSESubscriberQueue < 1 {
		errs = append(errs, errors.New("SSE_SUBSCRIBER_QUEUE_SIZE must be at least 1"))
	}
	if c.LoginRateLimitMax < 1 {
		errs = append(errs, errors.New("LOGIN_RATE_LIMIT_MAX must be at least 1"))
	}

	return errors.Join(errs...)
}

// getEnv returns the environment value for key, or fallback if unset or empty.
func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getInt64(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

// getDuration reads an integer value and interprets it as the given unit.
func getDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return time.Duration(n) * fallback
}

// splitCSV parses a comma-separated environment value.
func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// String renders the config for a startup log. Secrets are never included.
func (c *Config) String() string {
	return fmt.Sprintf(
		"listen=%s public_url=%s origins=%v db_max_conns=%d db_min_conns=%d "+
			"session_ttl=%s idle_ttl=%s cookie_secure=%t sse_heartbeat=%s sse_max_conn=%s "+
			"login_rate=%d/%s csrf_secret_set=%t sdk_pepper_set=%t",
		c.ListenAddr, c.PublicURL, c.AllowedOrigins,
		c.DatabaseMaxConns, c.DatabaseMinConns,
		c.SessionAbsoluteTTL, c.SessionIdleTTL, c.CookieSecure,
		c.SSEHeartbeatInterval, c.SSEMaxConnection,
		c.LoginRateLimitMax, c.LoginRateLimitWindow,
		len(c.CSRFSecret) > 0, len(c.SDKKeyPepper) > 0,
	)
}
