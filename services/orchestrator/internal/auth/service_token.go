// Package auth issues and verifies the short-lived tokens services use to call
// one another.
//
// This replaces a single shared secret sent as a static header. That secret
// never expired, was identical for every caller, and could not be rotated
// without restarting everything at once — one leak granted permanent, blanket
// internal access. These tokens are signed, expire in minutes, and say who
// issued them.
package auth

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ServiceRole marks a token as belonging to a service rather than a person, so
// a downstream can tell the difference — a service must not inherit a user's
// permissions, or an admin's.
const ServiceRole = "service"

const (
	// tokenTTL is deliberately short. These are minted on demand, so there is no
	// reason to hand out long-lived credentials.
	tokenTTL = 5 * time.Minute

	// refreshBefore re-mints ahead of expiry so a request never carries a token
	// that expires in flight.
	refreshBefore = 30 * time.Second
)

// Claims is what a service token asserts. user_id and role match the shape the
// NestJS services already validate, so they need no special handling.
type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// TokenIssuer mints service tokens, caching the current one until it is close
// to expiring.
type TokenIssuer struct {
	secret      []byte
	serviceName string

	mu      sync.Mutex
	token   string
	expires time.Time
}

func NewTokenIssuer(secret, serviceName string) (*TokenIssuer, error) {
	if secret == "" {
		return nil, fmt.Errorf("a signing secret is required to issue service tokens")
	}

	return &TokenIssuer{secret: []byte(secret), serviceName: serviceName}, nil
}

// Token returns a valid service token, minting a new one when the cached one is
// close to expiring.
func (i *TokenIssuer) Token() (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.token != "" && time.Now().Before(i.expires.Add(-refreshBefore)) {
		return i.token, nil
	}

	now := time.Now()
	expires := now.Add(tokenTTL)

	claims := Claims{
		UserID: i.serviceName,
		Role:   ServiceRole,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.serviceName,
			Subject:   i.serviceName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign service token: %w", err)
	}

	i.token = signed
	i.expires = expires

	return signed, nil
}

// Verifier checks inbound service tokens.
type Verifier struct {
	secret []byte
}

func NewVerifier(secret string) (*Verifier, error) {
	if secret == "" {
		return nil, fmt.Errorf("a signing secret is required to verify service tokens")
	}

	return &Verifier{secret: []byte(secret)}, nil
}

// VerifyService accepts a token only if it is validly signed, unexpired, and
// carries the service role.
func (v *Verifier) VerifyService(token string) (*Claims, error) {
	var claims Claims

	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		// Pin the algorithm. Without this a caller could present an unsigned
		// token, or one signed with a public key we happen to trust elsewhere.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return v.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))

	if err != nil {
		return nil, fmt.Errorf("invalid service token: %w", err)
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("service token is not valid")
	}
	if claims.Role != ServiceRole {
		// A user's token — even an admin's — must not be usable to report
		// delivery outcomes.
		return nil, fmt.Errorf("token role %q is not %q", claims.Role, ServiceRole)
	}

	return &claims, nil
}
