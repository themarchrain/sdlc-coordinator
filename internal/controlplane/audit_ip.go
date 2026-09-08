package controlplane

import (
	"context"
	"strings"
)

type auditSourceIPKey struct{}

// WithAuditSourceIP 把直连对端 IP 写入 ctx，供审计与业务同事务落库。
func WithAuditSourceIP(ctx context.Context, ip string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ctx
	}
	return context.WithValue(ctx, auditSourceIPKey{}, ip)
}

func auditSourceIP(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	ip, _ := ctx.Value(auditSourceIPKey{}).(string)
	return ip
}
