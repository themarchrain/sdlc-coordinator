package orchestrator

import "context"

// fakeAssigneeDirectory 是测试用的提及目录:记录仓作用域,按平台 ID 给出显示名/用户名。
type fakeAssigneeDirectory struct {
	mentions     map[int]AssigneeMention
	repositoryID string
}

func newFakeAssigneeDirectory(names map[int]string) fakeAssigneeDirectory {
	mentions := make(map[int]AssigneeMention, len(names))
	for id, name := range names {
		mentions[id] = AssigneeMention{DisplayName: name}
	}
	return fakeAssigneeDirectory{mentions: mentions}
}

func (d fakeAssigneeDirectory) ResolveMentionNames(_ context.Context, repositoryID string, assigneeIDs []int) map[int]AssigneeMention {
	d.repositoryID = repositoryID
	result := make(map[int]AssigneeMention, len(assigneeIDs))
	for _, id := range assigneeIDs {
		if mention, ok := d.mentions[id]; ok {
			result[id] = mention
		}
	}
	return result
}
