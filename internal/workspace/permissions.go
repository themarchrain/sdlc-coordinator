package workspace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

func prepareWorkspacePermissions(root string, privateRunRoot bool) error {
	if privateRunRoot {
		return restrictRunWorkspaceRoot(root)
	}
	return relaxSharedWorkspacePermissions(root)
}

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

// restrictRunWorkspaceRoot keeps an idle run inaccessible to other Agent UIDs.
// The Runner owns and traverses the run; the privileged CLI launcher temporarily
// transfers only the active run to Agent and returns it before finalization.
func restrictRunWorkspaceRoot(root string) error {
	info, err := os.Lstat(filepath.Clean(root))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("run workspace root must be a real directory")
	}
	return os.Chmod(root, 0o700)
}
