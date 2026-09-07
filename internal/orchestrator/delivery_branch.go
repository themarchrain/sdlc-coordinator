package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"sdlc-coordinator/internal/model"
)

var deliveryBranchTimezone = time.FixedZone("UTC+8", 8*60*60)

type issueDeliveryAssignee struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type issueDeliveryContext struct {
	IID         int                     `json:"iid"`
	Assignees   []issueDeliveryAssignee `json:"assignees"`
	AssigneeIDs []int                   `json:"assignee_ids"`
}

func (o *Orchestrator) prepareDeliveryBranch(task *model.Task) error {
	if task == nil {
		return nil
	}
	if strings.TrimSpace(task.DeliveryBranch) != "" {
		return nil
	}
	if task.Role != "DeveloperWorker" || task.Type != "develop_from_issue" {
		task.DeliveryBranch = model.DeliveryBranchForTask(task.TaskID)
		return nil
	}

	issue, err := issueDeliveryIdentity(task.ContextJSON)
	if err != nil {
		return err
	}
	if issue.IID <= 0 {
		task.DeliveryBranch = model.DeliveryBranchForTask(task.TaskID)
		return nil
	}

	username := ""
	for _, assignee := range issue.Assignees {
		if candidate := normalizeBranchUsername(assignee.Username); candidate != "" {
			username = candidate
			break
		}
	}
	if username == "" {
		for _, candidate := range o.assigneeBranchUsernamesFor(context.Background(), taskRepositoryID(task), issue.AssigneeIDs) {
			if normalized := normalizeBranchUsername(candidate); normalized != "" {
				username = normalized
				break
			}
		}
	}
	if username == "" {
		username = "developer-agent"
	}

	createdAt := task.CreatedAt
	if createdAt.IsZero() {
		createdAt = o.now()
	}
	task.DeliveryBranch = fmt.Sprintf("%s/issue-%d-%s", username, issue.IID, createdAt.In(deliveryBranchTimezone).Format("20060102"))
	return nil
}

func issueDeliveryIdentity(raw string) (issueDeliveryContext, error) {
	var context struct {
		Issue issueDeliveryContext `json:"issue"`
	}
	if strings.TrimSpace(raw) == "" {
		return issueDeliveryContext{}, nil
	}
	if err := json.Unmarshal([]byte(raw), &context); err != nil {
		return issueDeliveryContext{}, fmt.Errorf("decode task context for delivery branch: %w", err)
	}
	return context.Issue, nil
}

func normalizeBranchUsername(value string) string {
	var normalized strings.Builder
	for _, runeValue := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case runeValue >= 'a' && runeValue <= 'z', runeValue >= '0' && runeValue <= '9', runeValue == '.', runeValue == '_', runeValue == '-':
			normalized.WriteRune(runeValue)
		default:
			normalized.WriteByte('-')
		}
	}
	return strings.Trim(normalized.String(), "._-")
}

func emailLocalPart(email string) string {
	email = strings.TrimSpace(email)
	if at := strings.IndexByte(email, '@'); at >= 0 {
		return email[:at]
	}
	return email
}
