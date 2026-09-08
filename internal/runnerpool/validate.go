package runnerpool

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// 启动校验的失败类别（errors.Is 可判）；错误信息附带租户 ID 与端点值便于排障。
var (
	// ErrSharedEndpoint 同一端点被指派给多个租户：卷防线二前提被破坏
	//（该 runner 必须挂多租户目录，路由出错即越租户可见）。
	ErrSharedEndpoint = errors.New("runner endpoint assigned to multiple tenants")
	// ErrMissingTenant 指派引用了不存在的租户（孤儿行：租户已删或数据漂移）。
	ErrMissingTenant = errors.New("runner assignment references missing tenant")
	// ErrInvalidEndpointURL 端点非绝对 http(s) URL 或缺 host。
	ErrInvalidEndpointURL = errors.New("runner endpoint url must be absolute http(s) with host")
)

// AssignmentShape 是启动校验输入的租户中立形态（调用方从控制面行投影）。
type AssignmentShape struct {
	TenantID    uint
	EndpointURL string
}

// ValidateAssignments 校验已存在的 runner 指派行（启动校验，
// fail-fast 与 ErrControlPlaneNotConfigured 同风格）。任一命中即拒绝启动。
//
// 空表=合法放行——存量部署从未配置过指派，空表判非法会把全部
// 存量部署搞成拒启动。三类校验仅对「已存在的行」生效。
func ValidateAssignments(assignments []AssignmentShape, knownTenants map[uint]bool) error {
	byEndpoint := map[string][]uint{}
	for _, assignment := range assignments {
		if err := validateEndpointURL(assignment.EndpointURL); err != nil {
			return fmt.Errorf("tenant %d endpoint %q: %w", assignment.TenantID, assignment.EndpointURL, err)
		}
		if !knownTenants[assignment.TenantID] {
			return fmt.Errorf("tenant %d: %w", assignment.TenantID, ErrMissingTenant)
		}
		byEndpoint[assignment.EndpointURL] = append(byEndpoint[assignment.EndpointURL], assignment.TenantID)
	}
	endpoints := make([]string, 0, len(byEndpoint))
	for endpoint := range byEndpoint {
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)
	for _, endpoint := range endpoints {
		if assigned := byEndpoint[endpoint]; len(assigned) >= 2 {
			sort.Slice(assigned, func(i, j int) bool { return assigned[i] < assigned[j] })
			return fmt.Errorf("endpoint %q assigned to tenants %v: %w", endpoint, assigned, ErrSharedEndpoint)
		}
	}
	return nil
}

func validateEndpointURL(raw string) error {
	endpoint := strings.TrimSpace(raw)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEndpointURL, err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.TrimSpace(parsed.Host) == "" {
		return ErrInvalidEndpointURL
	}
	return nil
}
