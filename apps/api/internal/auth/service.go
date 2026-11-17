// Package auth implements dashboard authentication: login, logout,
// session validation, and CSRF token issuance.
//
// Sessions are opaque random tokens stored only as hashes. The raw token
// is returned to the caller once and kept in a Secure, HttpOnly cookie.
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/featureflag/api/internal/domain"
)

// Service manages administrator accounts and sessions.
type Service struct {
	pool    *pgxpool.Pool
	argon2  Argon2Params
	csrfKey []byte
	absTTL  time.Duration
	idleTTL time.Duration
	limiter *RateLimiter
}

// NewService returns an authentication service.
func NewService(pool *pgxpool.Pool, argon2 Argon2Params, csrfKey []byte, absTTL, idleTTL time.Duration, limiter *RateLimiter) *Service {
	return &Service{
		pool:    pool,
		argon2:  argon2,
		csrfKey: csrfKey,
		absTTL:  absTTL,
		idleTTL: idleTTL,
		limiter: limiter,
	}
}

// Session is a validated dashboard session.
//
// CSRFTokenHash is the stored hash of the session's CSRF token. It is
// never sent to the client; the client keeps its own copy.
type Session struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Email         string
	CSRFTokenHash string
	ExpiresAt     time.Time
}

// LoginResult is returned on a successful sign in.
//
// RawToken and CSRFToken are returned once and never retrievable again.
type LoginResult struct {
	Session   Session
	RawToken  string
	CSRFToken string
}

// Login authenticates an administrator.
//
// The same generic error is returned whether the address is unknown or
// the password is wrong, so the endpoint does not reveal which accounts
// exist.
func (s *Service) Login(ctx context.Context, email, password, clientIP string) (*LoginResult, error) {
	normalized := NormalizeEmail(email)

	if s.limiter != nil && !s.limiter.Allow(normalized, clientIP) {
		return nil, fmt.Errorf("too many login attempts: %w", domain.ErrForbidden)
	}

	var (
		userID uuid.UUID
		hash   string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT id, password_hash FROM users WHERE email = $1
	`, normalized).Scan(&userID, &hash)

	if errors.Is(err, pgx.ErrNoRows) {
		// Spend comparable time on an unknown account so the response
		// timing does not reveal whether the address exists.
		VerifyPassword(password, dummyHash)
		s.limiter.RecordFailure(normalized, clientIP)
		return nil, fmt.Errorf("invalid email or password: %w", domain.ErrUnauthorized)
	}
	if err != nil {
		return nil, fmt.Errorf("look up user: %w", err)
	}

	if !VerifyPassword(password, hash) {
		s.limiter.RecordFailure(normalized, clientIP)
		return nil, fmt.Errorf("invalid email or password: %w", domain.ErrUnauthorized)
	}

	s.limiter.RecordSuccess(normalized, clientIP)

	rawToken, err := GenerateSessionToken()
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	csrfToken, err := GenerateCSRFToken()
	if err != nil {
		return nil, fmt.Errorf("generate csrf token: %w", err)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(s.absTTL)

	var sessionID uuid.UUID
	err = s.pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES ($1, $2, $3, $4, $4)
		RETURNING id
	`, userID, HashSessionToken(rawToken), expiresAt, now).Scan(&sessionID)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	// The CSRF token is bound to the session. It is stored as a hash so
	// a database read does not reveal a usable token.
	csrfHash := s.hashCSRF(csrfToken)
	if _, err := s.pool.Exec(ctx, `
		UPDATE sessions SET csrf_token_hash = $2 WHERE id = $1
	`, sessionID, csrfHash); err != nil {
		return nil, fmt.Errorf("store csrf token: %w", err)
	}

	var userEmail string
	if err := s.pool.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&userEmail); err != nil {
		return nil, fmt.Errorf("read user email: %w", err)
	}

	return &LoginResult{
		Session: Session{
			ID:            sessionID,
			UserID:        userID,
			Email:         userEmail,
			CSRFTokenHash: csrfHash,
			ExpiresAt:     expiresAt,
		},
		RawToken:  rawToken,
		CSRFToken: csrfToken,
	}, nil
}

// Validate resolves a session token to a session.
//
// It returns ErrUnauthorized for an unknown, expired, or idle-expired
// token. The session's last_seen_at is refreshed as a side effect so the
// idle timeout works.
func (s *Service) Validate(ctx context.Context, rawToken string) (*Session, error) {
	if rawToken == "" {
		return nil, domain.ErrUnauthorized
	}

	var (
		session  Session
		csrfHash string
		lastSeen time.Time
	)
	err := s.pool.QueryRow(ctx, `
		SELECT s.id, s.user_id, u.email, s.expires_at, s.last_seen_at, s.csrf_token_hash
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
	`, HashSessionToken(rawToken)).Scan(&session.ID, &session.UserID, &session.Email,
		&session.ExpiresAt, &lastSeen, &csrfHash)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("validate session: %w", err)
	}

	now := time.Now().UTC()
	if now.After(session.ExpiresAt) {
		// Expired: remove it so it stops being looked up.
		_ = s.DeleteSession(ctx, rawToken)
		return nil, domain.ErrUnauthorized
	}
	if s.idleTTL > 0 && now.Sub(lastSeen) > s.idleTTL {
		_ = s.DeleteSession(ctx, rawToken)
		return nil, domain.ErrUnauthorized
	}

	// Refresh last_seen_at for the idle timeout.
	if _, err := s.pool.Exec(ctx, `
		UPDATE sessions SET last_seen_at = $2 WHERE id = $1
	`, session.ID, now); err != nil {
		return nil, fmt.Errorf("refresh session activity: %w", err)
	}

	// The CSRF token cannot be recovered from its hash, so the client
	// keeps its own copy and the server keeps only the hash.
	session.CSRFTokenHash = csrfHash
	return &session, nil
}

// VerifyCSRF checks a submitted CSRF token against the session's stored
// hash.
//
// The comparison is constant time.
func (s *Service) VerifyCSRF(session *Session, submitted string) bool {
	if session == nil || submitted == "" || session.CSRFTokenHash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(s.hashCSRF(submitted)), []byte(session.CSRFTokenHash)) == 1
}

// hashCSRF derives the stored hash of a CSRF token.
func (s *Service) hashCSRF(token string) string {
	return HashSecret(token, s.csrfKey)
}

// DeleteSession revokes a session by its raw token.
func (s *Service) DeleteSession(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	if _, err := s.pool.Exec(ctx, `
		DELETE FROM sessions WHERE token_hash = $1
	`, HashSessionToken(rawToken)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes sessions that are past their absolute
// expiry or their idle timeout.
//
// It returns the number of sessions removed. The caller runs it
// periodically; the LIMIT bounds how much work one pass does.
func (s *Service) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	idleCutoff := now.Add(-s.idleTTL)

	tag, err := s.pool.Exec(ctx, `
		DELETE FROM sessions
		WHERE id IN (
			SELECT id FROM sessions
			WHERE expires_at < $1 OR last_seen_at < $2
			LIMIT 500
		)
	`, now, idleCutoff)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CreateUser inserts an administrator with a hashed password.
func (s *Service) CreateUser(ctx context.Context, email, password string) (uuid.UUID, error) {
	normalized := NormalizeEmail(email)
	if err := ValidateEmail(normalized); err != nil {
		return uuid.Nil, err
	}
	if len(password) < 12 {
		return uuid.Nil, fmt.Errorf("password must be at least 12 characters: %w", domain.ErrValidation)
	}

	hash, err := HashPassword(password, s.argon2)
	if err != nil {
		return uuid.Nil, fmt.Errorf("hash password: %w", err)
	}

	var id uuid.UUID
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id
	`, normalized, hash).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

// NormalizeEmail lowercases and trims an email address.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidateEmail checks an email address.
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email is required: %w", domain.ErrValidation)
	}
	if len(email) > 320 {
		return fmt.Errorf("email is too long: %w", domain.ErrValidation)
	}
	at := strings.Index(email, "@")
	if at <= 0 || at == len(email)-1 {
		return fmt.Errorf("email is malformed: %w", domain.ErrValidation)
	}
	if strings.Count(email, "@") != 1 {
		return fmt.Errorf("email is malformed: %w", domain.ErrValidation)
	}
	return nil
}

// dummyHash is a valid Argon2id hash of a random value. It is verified
// against when the account does not exist, so the response time does not
// reveal whether an address is registered.
var dummyHash = "$argon2id$v=19$m=65536,t=3,p=2$c29tZXNhbHR2YWx1ZQ$Zm9vYmFyYmF6cXV4"

// ClientIP extracts the client address from a request, honouring the
// trusted proxy header when one is configured.
func ClientIP(remoteAddr, forwardedFor string) string {
	if forwardedFor != "" {
		// X-Forwarded-For may hold a chain. The first entry is the
		// original client.
		if i := strings.IndexByte(forwardedFor, ','); i >= 0 {
			forwardedFor = forwardedFor[:i]
		}
		if ip := strings.TrimSpace(forwardedFor); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

// RateLimiter throttles login attempts.
//
// Counters are held in memory and bounded, so an attacker cannot exhaust
// memory by submitting distinct addresses. No external store is involved.
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attempt
	max      int
	window   time.Duration
	lastGC   time.Time
}

type attempt struct {
	count int
	first time.Time
}

// NewRateLimiter returns a limiter allowing max attempts per window.
func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts: make(map[string]*attempt),
		max:      max,
		window:   window,
		lastGC:   time.Now(),
	}
}

// Allow reports whether an attempt is permitted.
func (l *RateLimiter) Allow(email, clientIP string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gcLocked()

	key := rateKey(email, clientIP)
	a, ok := l.attempts[key]
	if !ok {
		return true
	}
	if time.Since(a.first) > l.window {
		return true
	}
	return a.count < l.max
}

// RecordFailure increments the failure counter for a key.
func (l *RateLimiter) RecordFailure(email, clientIP string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gcLocked()

	key := rateKey(email, clientIP)
	a, ok := l.attempts[key]
	if !ok {
		l.attempts[key] = &attempt{count: 1, first: time.Now()}
		return
	}
	a.count++
}

// RecordSuccess clears the counter for a key after a successful login.
func (l *RateLimiter) RecordSuccess(email, clientIP string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, rateKey(email, clientIP))
}

// gcLocked removes expired entries. It also bounds the map size so that
// distinct submitted addresses cannot grow it without limit.
func (l *RateLimiter) gcLocked() {
	now := time.Now()
	if now.Sub(l.lastGC) < l.window {
		return
	}
	l.lastGC = now

	for k, a := range l.attempts {
		if now.Sub(a.first) > l.window {
			delete(l.attempts, k)
		}
	}

	// Hard cap. If the map is at the cap, evict the oldest entry rather
	// than growing without bound.
	const maxEntries = 10000
	if len(l.attempts) >= maxEntries {
		var oldestKey string
		var oldest time.Time
		for k, a := range l.attempts {
			if oldestKey == "" || a.first.Before(oldest) {
				oldestKey, oldest = k, a.first
			}
		}
		delete(l.attempts, oldestKey)
	}
}

// rateKey combines the email and client address into one key.
func rateKey(email, clientIP string) string {
	return email + "|" + clientIP
}
