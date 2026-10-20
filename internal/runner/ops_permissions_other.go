//go:build !linux

package runner

import "context"

func (o *WorkspaceOps) ensureDeliveryWorkspaceAccess(context.Context, string) error { return nil }

func secureOperationsDirectory(string) error { return nil }

func secureWorkspaceRunRoots(string) error { return nil }
