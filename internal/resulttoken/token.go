// Package resulttoken issues and verifies short-lived per-run HMAC tokens for
// the agent result submission channel. Tokens bind a run identity (task, run,
// worker role) so a compromised agent workspace can only submit results for
// its own run.
package resulttoken

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrTokenExpired reports a well-formed, correctly signed token whose TTL has
// passed. Callers use errors.Is to separate expiry from forgery.
var ErrTokenExpired = errors.New("run token expired")

const tokenTTL = 5 * time.Hour

// Claims is the signed payload of a run token.
type Claims struct {
	TaskID    string        `json:"task_id"`
	RunID     string        `json:"run_id"`
	Role      string        `json:"role"`
	TenantID  uint          `json:"tenant,omitempty"`
	IssuedAt  int64         `json:"iat"`
	ExpiresAt int64         `json:"exp"`
	TTL       time.Duration `json:"-"`
}

// NewSecret generates a fresh 32-byte signing secret. The coordinator keeps it
// in memory only: a restart invalidates all outstanding tokens, which pushes
// in-flight agents onto the text fallback channel.
func NewSecret() ([]byte, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate run token secret: %w", err)
	}
	return secret, nil
}

// Issue signs the claims with the default TTL anchored at now.
func Issue(secret []byte, claims Claims, now time.Time) (string, error) {
	if claims.TTL == 0 {
		claims.TTL = tokenTTL
	}
	claims.IssuedAt = now.Unix()
	claims.ExpiresAt = now.Add(claims.TTL).Unix()
	return issueClaims(secret, claims)
}

func issueClaims(secret []byte, claims Claims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("encode run token claims: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(sign(secret, encoded)), nil
}

// Verify checks format, signature (constant time) and expiry.
func Verify(secret []byte, token string, now time.Time) (Claims, error) {
	var claims Claims
	encodedPayload, encodedSignature, ok := strings.Cut(token, ".")
	if !ok || encodedPayload == "" || encodedSignature == "" {
		return claims, errors.New("run token is malformed")
	}
	if !hmac.Equal(sign(secret, encodedPayload), decodeBase64(encodedSignature)) {
		return claims, errors.New("run token signature is invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return claims, errors.New("run token payload is malformed")
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, errors.New("run token payload is malformed")
	}
	if time.Unix(claims.ExpiresAt, 0).Before(now) {
		return claims, ErrTokenExpired
	}
	return claims, nil
}

func sign(secret []byte, payload string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func decodeBase64(text string) []byte {
	decoded, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return nil
	}
	// Reject non-canonical encodings: the padding bits of the final base64
	// character must be zero, so a modified token can never decode to the
	// same signature bytes.
	if base64.RawURLEncoding.EncodeToString(decoded) != text {
		return nil
	}
	return decoded
}
