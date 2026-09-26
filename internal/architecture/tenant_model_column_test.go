package architecture_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// detectTenantIDColumns 抽出可测的扫描逻辑：对给定源码样本返回命中的行
// （gorm model 字段同时含 TenantID 与 gorm 标签即命中）。
func detectTenantIDColumns(body string) []string {
	var hits []string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "TenantID") && strings.Contains(line, "gorm:") {
			hits = append(hits, strings.TrimSpace(line))
		}
	}
	return hits
}

// TestBusinessModelsDoNotCarryTenantIDColumn 锁定 model 形态红线：业务库 model 禁止携带 tenant_id 列（分库后隔离靠「租户作用域
// 句柄」，不靠全局表上的租户列）。控制面与 tenantdb 排除；白名单显式列出。
// 扫描器以内嵌违规样本自检（保证该守卫真的能红），再对真实源码树断言干净。
func TestBusinessModelsDoNotCarryTenantIDColumn(t *testing.T) {
	t.Parallel()

	t.Run("scanner flags violating model field", func(t *testing.T) {
		sample := `type Task struct {
	ID      uint   ` + "`gorm:\"primaryKey\"`" + `
	TenantID uint  ` + "`gorm:\"index\"`" + `
}
tenantID := row.TenantID
DB.Where("tenant_id = ?", tenantID)
`
		if hits := detectTenantIDColumns(sample); len(hits) != 1 {
			t.Fatalf("scanner must flag the gorm TenantID column, got %v", hits)
		}
	})

	t.Run("real business tree is clean", func(t *testing.T) {
		root := filepath.Clean(filepath.Join("..", ".."))
		exempt := map[string]bool{
			"controlplane": true,
			"tenantdb":     true,
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
				if exempt[name] {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if hits := detectTenantIDColumns(string(raw)); len(hits) > 0 {
				violations = append(violations, rel+": "+strings.Join(hits, " | "))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		if len(violations) > 0 {
			t.Fatalf("business models must not carry tenant_id columns:\n  %s", strings.Join(violations, "\n  "))
		}
	})
}
