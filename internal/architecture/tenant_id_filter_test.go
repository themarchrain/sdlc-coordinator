package architecture_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBusinessCodeDoesNotFilterByTenantID 锁定红线：
// 业务库禁止「全局句柄 + WHERE tenant_id」形态。控制面（users/tenants/
// memberships）按组织切分是合法的，排除 internal/controlplane。
func TestBusinessCodeDoesNotFilterByTenantID(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	needles := []string{
		`Where("tenant_id`,
		`Where("tenant_id =`,
		`.Where("tenant_id`,
	}
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			switch name {
			case ".git", "node_modules", "vendor", "dist", ".worktrees":
				// .worktrees 是本仓其他分支的嵌套检出,不属于本分支业务树。
				return filepath.SkipDir
			}
			if name == "controlplane" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.Contains(filepath.ToSlash(rel), "/controlplane/") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		body := string(raw)
		for _, needle := range needles {
			if strings.Contains(body, needle) {
				violations = append(violations, rel)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("business code must not filter by tenant_id:\n  %s", strings.Join(violations, "\n  "))
	}
}
