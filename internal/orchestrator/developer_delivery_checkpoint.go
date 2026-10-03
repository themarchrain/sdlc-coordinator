package orchestrator

import (
	"context"

	"sdlc-coordinator/internal/model"
)

// DeveloperDeliveryCheckpoints 是 Developer 交付检查点端口:实现由装配层接
// 入(租户库),编排层在 prepare 成功后钉基线、在 commit 与 push 之间写交付
// SHA。租户库对 Agent 不可写,检查点是提交归属的唯一受信凭据;实现必须保
// 证 run 唯一与条件更新,不得出现同 run 两笔"合法"交付提交。
type DeveloperDeliveryCheckpoints interface {
	RecordBaseline(ctx context.Context, record *model.DeveloperDeliveryCheckpoint) error
	Load(ctx context.Context, runID string) (*model.DeveloperDeliveryCheckpoint, error)
	RecordCommit(ctx context.Context, runID, commitSHA string) error
}
