package resulttoken

import (
	"errors"
	"testing"
	"time"
)

// TestDeploymentSecretCrossInstanceVerification 同一部署级密钥下：A 实例签发
// 的 token B 实例可验（A15/A14 的签名共享面）；不同密钥验签必拒。
func TestDeploymentSecretCrossInstanceVerification(t *testing.T) {
	secret, err := SecretFromHex("6ba7b8109dad11d180b400c04fd430c86ba7b8109dad11d180b400c04fd430c8")
	if err != nil {
		t.Fatalf("parse deployment secret: %v", err)
	}
	now := time.Now()
	token, err := Issue(secret, Claims{TaskID: "task-1", RunID: "run-1", Role: "DeveloperWorker"}, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	claims, err := Verify(secret, token, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("instance B must verify instance A's token: %v", err)
	}
	if claims.TaskID != "task-1" || claims.RunID != "run-1" {
		t.Fatalf("claims mismatch: %+v", claims)
	}

	other, err := NewSecret()
	if err != nil {
		t.Fatalf("random secret: %v", err)
	}
	if _, err := Verify(other, token, now.Add(time.Minute)); err == nil || errors.Is(err, ErrTokenExpired) {
		t.Fatalf("token must not verify under a different deployment secret, got %v", err)
	}
}

// TestSecretFromHexRejectsMalformed 部署级密钥的形状门禁：非 hex/长度不符
// 一律拒绝（启动路径 fail-fast），杜绝弱密钥静默生效。
func TestSecretFromHexRejectsMalformed(t *testing.T) {
	for name, encoded := range map[string]string{
		"not hex":    "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
		"short":      "6ba7b8109dad11d180b400c04fd430c8",
		"long":       "6ba7b8109dad11d180b400c04fd430c86ba7b8109dad11d180b400c04fd430c8ff",
		"empty-ish":  "  ",
		"odd length": "abc",
	} {
		if _, err := SecretFromHex(encoded); err == nil {
			t.Fatalf("%s must be rejected, got nil error", name)
		}
	}
	// 合法边界：64 hex 字符带首尾空白仍可解析。
	if _, err := SecretFromHex(" 6ba7b8109dad11d180b400c04fd430c86ba7b8109dad11d180b400c04fd430c8 "); err != nil {
		t.Fatalf("padded valid secret must parse: %v", err)
	}
}
