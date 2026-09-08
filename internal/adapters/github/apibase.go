package github

import (
	"net/url"
	"strings"
)

// APIBaseURL 把注册仓填写的平台地址派生为 REST API 基址。
// github.com 的网页/git 域与 API 域分离,而注册仓行的 base_url 同时喂给
// git 克隆(CloneURL)与 REST 调用两方,REST 侧统一在此派生:
//
//	https://github.com     → https://api.github.com
//	https://api.github.com → 原样(兼容直接填 API 域的存量配置)
//	其他 host              → 原样(GHE 按文档填完整 API 地址;httptest 等测试地址透传)
func APIBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return trimmed
	}
	switch strings.ToLower(parsed.Host) {
	case "github.com", "www.github.com":
		return "https://api.github.com"
	default:
		return trimmed
	}
}
