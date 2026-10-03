package results

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TrailerMarker 是 Coordinator 追加的追溯尾注前缀。AI 写入的 commit_message
// 中出现它一律拒绝:尾注只能由 Coordinator 在提交落地前统一追加,Agent 自填
// 或改写不受信任。
const TrailerMarker = "Coordinator-Run-ID:"

const (
	MaxCommitTitleRunes   = 100
	MaxCommitMessageBytes = 8192
)

// commitTitlePattern 锁定 type: summary 形态;type 枚举外的前缀(含 scope
// 形态与旧 ai: 前缀)都因不匹配被拒。
var commitTitlePattern = regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert): \S`)

// NormalizeCommitMessage 把 AI 回传的 commit_message 归一为提交正文形态并做
// 确定性校验:去首尾空白、CRLF 归一;标题必须是 type: 单行摘要,标题与正文
// 之间空一行;正文可选、不强制分点。整条消息有字节上限,控制字符与伪造尾注
// 一律拒绝。返回值即最终提交正文,不含追溯尾注。
func NormalizeCommitMessage(message string) (string, error) {
	normalized := strings.ReplaceAll(message, "\r\n", "\n")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return "", fmt.Errorf("commit_message title is required")
	}
	if strings.ContainsRune(normalized, '\r') {
		return "", fmt.Errorf("commit_message must not contain stray carriage returns")
	}
	if strings.Contains(normalized, TrailerMarker) {
		return "", fmt.Errorf("commit_message must not contain the coordinator run trailer")
	}
	if len(normalized) > MaxCommitMessageBytes {
		return "", fmt.Errorf("commit_message must not exceed %d bytes", MaxCommitMessageBytes)
	}
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	normalized = strings.Join(lines, "\n")
	if utf8.RuneCountInString(lines[0]) > MaxCommitTitleRunes {
		return "", fmt.Errorf("commit_message title must not exceed %d characters", MaxCommitTitleRunes)
	}
	if !commitTitlePattern.MatchString(lines[0]) {
		return "", fmt.Errorf("commit_message title must be type: summary with type in feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert")
	}
	if len(lines) > 1 && lines[1] != "" {
		return "", fmt.Errorf("commit_message body must be separated from the title by a blank line")
	}
	for _, line := range lines {
		for _, r := range line {
			if unicode.IsControl(r) && r != '\t' {
				return "", fmt.Errorf("commit_message must not contain control characters")
			}
		}
	}
	return normalized, nil
}
