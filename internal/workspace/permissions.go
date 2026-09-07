package workspace

import (
	"io/fs"
	"os"
	"path/filepath"
)

// relaxSharedWorkspacePermissions 把 run 工作区整树放宽为人人可读写(目录 0777、
// 文件 0666)。动机:coordinator 容器(root)与 agent-runner 容器(非 root 的 runner
// 用户)共享同一 ./workspaces bind mount,两容器间无共享 uid/gid 协调机制;不放宽
// 时 runner 侧连 run 目录(0700/0600)都无法遍历,prompt 读取与 repo 写入均以
// permission denied 失败。仅适用于单机/内网联调拓扑。
func relaxSharedWorkspacePermissions(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Chmod(path, 0o777)
		}
		return os.Chmod(path, 0o666)
	})
}
