package workspace

import "strings"

// 内置降级身份:注册仓未配置提交身份时,developer 工作区以它作为提交作者与
// 提交者。这是产品级默认署名,不是可配置项;真实身份通过仓库管理配置,成对
// 写入且仅停用态可改。
const (
	FallbackGitAuthorName  = "sdlc-coordinator-dev"
	FallbackGitAuthorEmail = "dev@sdlc-coordinator.local"
)

// resolveGitIdentity 归一化工单带来的提交身份:两侧任一缺失(未配置)即整体
// 降级为内置身份,不做半配拼接。写入点只有 developer 工作区 prepare 一处,
// 配置随本 run 仓库成为快照,运行期不再回读配置面。
func resolveGitIdentity(name, email string) (string, string) {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" || email == "" {
		return FallbackGitAuthorName, FallbackGitAuthorEmail
	}
	return name, email
}
