package controlplane

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// inviteAlphabet 采用 Crockford Base32 字符集（去 I/L/O/U），人可转录且无歧义
// 字符。
const inviteAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"

// passwordAlphabet 用于生成初始密码：剔除易混淆字符的全字母数字集。
const passwordAlphabet = "23456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ"

const (
	inviteCodeLength    = 12
	initialPasswordLen  = 12
	inviteDisplayGroups = 4
	inviteDisplayChunk  = 4
)

// generateInviteCode 生成 12 位随机码，展示形态 XXXX-XXXX-XXXX。
func generateInviteCode() (string, error) {
	body, err := randomString(inviteAlphabet, inviteCodeLength)
	if err != nil {
		return "", err
	}
	return formatInviteCode(body), nil
}

// formatInviteCode 把 12 位码按 4-4-4 分组展示。
func formatInviteCode(body string) string {
	if len(body) != inviteCodeLength {
		return body
	}
	return body[0:4] + "-" + body[4:8] + "-" + body[8:12]
}

// normalizeInviteCode 规范化用户输入：去连字符/下划线/空白、转大写，与生成
// 字符集一致；大小写与分隔符混排的同一枚码解析结果相同。
func normalizeInviteCode(raw string) string {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	replacer := strings.NewReplacer("-", "", "_", "", " ", "")
	return replacer.Replace(normalized)
}

// hashInviteCode 邀请码只存 sha256，明文仅在签发响应出现一次。
func hashInviteCode(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// generateInitialPassword 生成管理员开通/密码重置的一次性初始密码。
func generateInitialPassword() (string, error) {
	return randomString(passwordAlphabet, initialPasswordLen)
}

func randomString(alphabet string, length int) (string, error) {
	builder := strings.Builder{}
	builder.Grow(length)
	max := big.NewInt(int64(len(alphabet)))
	for i := 0; i < length; i++ {
		index, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate random string: %w", err)
		}
		builder.WriteByte(alphabet[index.Int64()])
	}
	return builder.String(), nil
}
