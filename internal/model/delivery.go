package model

import "strings"

func DeliveryBranchForTask(taskID string) string {
	var normalized strings.Builder
	for _, runeValue := range strings.TrimSpace(taskID) {
		switch {
		case runeValue >= 'a' && runeValue <= 'z', runeValue >= 'A' && runeValue <= 'Z', runeValue >= '0' && runeValue <= '9', runeValue == '.', runeValue == '_', runeValue == '-':
			normalized.WriteRune(runeValue)
		default:
			normalized.WriteByte('-')
		}
	}
	if normalized.Len() == 0 {
		normalized.WriteString("task")
	}
	return "agent/" + normalized.String()
}

func DeliveryBranch(task *Task) string {
	if task == nil {
		return DeliveryBranchForTask("")
	}
	if branch := strings.TrimSpace(task.DeliveryBranch); branch != "" {
		return branch
	}
	return DeliveryBranchForTask(task.TaskID)
}
