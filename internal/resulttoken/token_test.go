package resulttoken

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIssueVerifyRoundTrip(t *testing.T) {
	secret, err := NewSecret()
	if err != nil {
		t.Fatalf("new secret: %v", err)
	}
	now := time.Now()
	token, err := Issue(secret, Claims{TaskID: "task-1", RunID: "run-1", Role: "DeveloperWorker"}, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := Verify(secret, token, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.TaskID != "task-1" || claims.RunID != "run-1" || claims.Role != "DeveloperWorker" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if claims.ExpiresAt <= now.Add(time.Minute).Unix() {
		t.Fatalf("token must carry a future expiry: %d", claims.ExpiresAt)
	}
}

func TestVerifyRejectsTamperedAndForeignTokens(t *testing.T) {
	secret, _ := NewSecret()
	other, _ := NewSecret()
	now := time.Now()
	token, err := Issue(secret, Claims{TaskID: "task-1", RunID: "run-1", Role: "DeveloperWorker"}, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// Flip the first payload character: a guaranteed content change, so the
	// signature check fails deterministically on every run.
	tampered := []byte(token)
	if tampered[0] == 'A' {
		tampered[0] = 'B'
	} else {
		tampered[0] = 'A'
	}
	if _, err := Verify(secret, string(tampered), now); err == nil {
		t.Fatal("tampered token must fail")
	}
	// Refilled tail bytes are non-canonical base64 (the padding bits of the
	// final character are not zero), so the strict signature decode rejects
	// this deterministically as well.
	if _, err := Verify(secret, token[:len(token)-2]+"xx", now); err == nil {
		t.Fatal("tampered token must fail")
	}
	if _, err := Verify(other, token, now); err == nil {
		t.Fatal("foreign secret must fail")
	}
	if _, err := Verify(secret, "garbage", now); err == nil {
		t.Fatal("malformed token must fail")
	}
	if _, err := Verify(secret, "", now); err == nil {
		t.Fatal("empty token must fail")
	}
}

func TestVerifyRejectsNonCanonicalSignature(t *testing.T) {
	secret, _ := NewSecret()
	now := time.Now()
	token, err := Issue(secret, Claims{TaskID: "task-1", RunID: "run-1", Role: "DeveloperWorker"}, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// Raise the two padding bits of the final base64 character: the decoded
	// signature stays identical, so only the canonicality check can reject it.
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	sig := []byte(token)
	last := sig[len(sig)-1]
	idx := strings.IndexByte(alphabet, last)
	if idx < 0 || idx%4 != 0 {
		t.Fatalf("expected canonical final character, got %q", last)
	}
	sig[len(sig)-1] = alphabet[idx+1]
	if _, err := Verify(secret, string(sig), now); err == nil {
		t.Fatal("non-canonical signature must fail")
	}
}

func TestVerifyReportsExpiredSeparately(t *testing.T) {
	secret, _ := NewSecret()
	now := time.Now()
	token, err := Issue(secret, Claims{TaskID: "task-1", RunID: "run-1", Role: "DeveloperWorker"}, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	_, err = Verify(secret, token, now.Add(6*time.Hour))
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expired token must return ErrTokenExpired, got %v", err)
	}
}

func TestIssueBindsTTL(t *testing.T) {
	secret, _ := NewSecret()
	now := time.Now()
	token, err := Issue(secret, Claims{TaskID: "t", RunID: "r", Role: "R", TTL: time.Hour}, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := Verify(secret, token, now.Add(90*time.Minute)); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("token must expire after TTL, got %v", err)
	}
	if _, err := Verify(secret, token, now.Add(30*time.Minute)); err != nil {
		t.Fatalf("token must live within TTL: %v", err)
	}
}

func TestNewSecretIsRandom(t *testing.T) {
	first, err := NewSecret()
	if err != nil {
		t.Fatalf("new secret: %v", err)
	}
	second, err := NewSecret()
	if err != nil {
		t.Fatalf("new secret: %v", err)
	}
	if len(first) != 32 || len(second) != 32 {
		t.Fatalf("secrets must be 32 bytes: %d/%d", len(first), len(second))
	}
	if string(first) == string(second) {
		t.Fatal("secrets must be random")
	}
	if strings.Contains(string(first), "\x00") && strings.Contains(string(second), "\x00") {
		t.Fatal("degenerate secret")
	}
}
