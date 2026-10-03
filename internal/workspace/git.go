package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	gitUsernameEnvironment = "COORDINATOR_GIT_USERNAME"
	gitTokenEnvironment    = "COORDINATOR_GIT_TOKEN"
)

type GitCommandError struct {
	Args   []string
	Output string
	Err    error
}

func (e *GitCommandError) Error() string {
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.Args, " "), e.Err, strings.TrimSpace(e.Output))
}

func (e *GitCommandError) Unwrap() error { return e.Err }

type gitRunner interface {
	Run(context.Context, string, []string, ...string) (string, error)
}

type execGitRunner struct{}

// ambientIdentityEnvironmentKeys 是会在 Git 侧压过 config 的环境变量。提交
// 身份的唯一来源是 prepare 阶段写入的仓库本地 config,这些继承键(哪怕来自
// 部署环境误配)一律从子进程环境剔除。
var ambientIdentityEnvironmentKeys = []string{
	"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL",
}

func sanitizeGitEnvironment(entries []string) []string {
	filtered := make([]string, 0, len(entries))
	for _, entry := range entries {
		ambient := false
		for _, key := range ambientIdentityEnvironmentKeys {
			if strings.HasPrefix(entry, key+"=") {
				ambient = true
				break
			}
		}
		if !ambient {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (execGitRunner) Run(ctx context.Context, dir string, environment []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	env = append(env, environment...)
	cmd.Env = sanitizeGitEnvironment(env)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), &GitCommandError{Args: append([]string(nil), args...), Output: string(out), Err: err}
	}
	return string(out), nil
}

func credentialFetchArguments(credential *GitCredential, args []string) ([]string, []string) {
	if credential == nil || strings.TrimSpace(credential.Token) == "" {
		return args, nil
	}
	username := strings.TrimSpace(credential.Username)
	if username == "" {
		username = "oauth2"
	}
	helper := `credential.helper=!f() { echo "username=$` + gitUsernameEnvironment + `"; echo "password=$` + gitTokenEnvironment + `"; }; f`
	withHelper := make([]string, 0, len(args)+4)
	withHelper = append(withHelper,
		"-c", "credential.helper=",
		"-c", helper,
	)
	withHelper = append(withHelper, args...)
	// 凭证经 credential helper 脚本传递：git 以 sh 执行 "!f(){...}"，脚本从下面注入的
	// 两个环境变量读取用户名与 token（缺省用户名 "oauth2" 是 GitLab 约定，实测 Gitea
	// 对"任意用户名 + 有效 token"同样放行）。继承完整环境只是保留 sh/PATH 等运行前提
	// 的稳妥写法，并不是某次认证故障的修复：2026-08-30 的 "remote: Failed to
	// authenticate user" 表明 helper 已把凭证送达、根因是全局 codehost token 值配置
	// 错误；b066465 的提交信息把它误归因为"白名单丢 PATH"，准确结论以本注释为准。
	environment := append(os.Environ(),
		gitUsernameEnvironment+"="+username,
		gitTokenEnvironment+"="+credential.Token,
		"GCM_INTERACTIVE=Never",
		"GIT_TERMINAL_PROMPT=0",
	)
	return withHelper, environment
}
