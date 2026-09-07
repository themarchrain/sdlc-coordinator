package workspace

import (
	"slices"
	"strings"
	"testing"
)

func TestCredentialFetchArgumentsKeepsSecretOutOfArguments(t *testing.T) {
	t.Parallel()

	const token = "top-secret-token"
	args, environment := credentialFetchArguments(&GitCredential{
		Username: "oauth2",
		Token:    token,
	}, []string{"fetch", "origin"})

	joinedArgs := strings.Join(args, " ")
	if strings.Contains(joinedArgs, token) {
		t.Fatalf("git arguments contain token: %q", joinedArgs)
	}
	if !strings.Contains(joinedArgs, gitUsernameEnvironment) || !strings.Contains(joinedArgs, gitTokenEnvironment) {
		t.Fatalf("credential helper does not reference environment names: %q", joinedArgs)
	}
	if len(args) < 4 || args[0] != "-c" || args[1] != "credential.helper=" || args[2] != "-c" {
		t.Fatalf("credential helpers are not reset before custom helper: %#v", args)
	}
	if !strings.HasPrefix(args[3], "credential.helper=!f()") {
		t.Fatalf("custom credential helper is missing after reset: %#v", args)
	}
	joinedEnvironment := strings.Join(environment, "\n")
	if !strings.Contains(joinedEnvironment, gitUsernameEnvironment+"=oauth2") || !strings.Contains(joinedEnvironment, gitTokenEnvironment+"="+token) {
		t.Fatalf("git credential environment is incomplete: %q", joinedEnvironment)
	}
	if !slices.Contains(environment, "GCM_INTERACTIVE=Never") {
		t.Fatalf("Git Credential Manager is not forced non-interactive: %#v", environment)
	}
}

// 回归断言：git 子进程环境继承进程完整环境（至少含 PATH），保证 credential helper
// 的 sh 运行前提稳定。这是稳妥性约束，不是历史故障的根因修复：2026-08-30 的认证
// 失败是全局 codehost token 值配置错误（helper 已送达凭证，Gitea 回
// "Failed to authenticate user"），见 git.go 内注释与 b066465 提交信息的勘误。
func TestCredentialFetchArgumentsInheritsProcessEnvironment(t *testing.T) {
	t.Parallel()

	_, environment := credentialFetchArguments(&GitCredential{
		Username: "oauth2",
		Token:    "top-secret-token",
	}, []string{"fetch", "origin"})

	hasPath := false
	for _, entry := range environment {
		if strings.HasPrefix(entry, "PATH=") {
			hasPath = true
			break
		}
	}
	if !hasPath {
		t.Fatalf("git subprocess environment must inherit PATH for the credential helper: %#v", environment)
	}
	if !slices.Contains(environment, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("git prompt must be disabled for non-interactive runs: %#v", environment)
	}
}
