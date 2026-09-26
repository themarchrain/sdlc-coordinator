package workers

import (
	"testing"

	"sdlc-coordinator/internal/model"
)

// 验证任务是只读面:工作区按冻结 SHA 只读检出,提示词也声明只读纪律,
// 任务包的 AllowedRepos 口径不得把目标仓标成可写误导执行者。
func TestBuildRunnerTaskPackageTesterAllowedReposAreReadOnly(t *testing.T) {
	task := &model.Task{
		TaskID: "task-tester-pkg", DedupeKey: "verify:task-tester-pkg", Type: "verify_merge_request", Role: "TesterWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", Status: model.TaskStatusPending,
		ContextJSON: `{"project":{"path":"group/project"},"merge_request":{"iid":21,"head_sha":"abc123","source_branch":"feature/change"},"repos":[{"name":"project","branch":"feature/change","access":"readable"}]}`,
	}
	pkg, err := BuildRunnerTaskPackage(task, &model.Run{RunID: "run-tester-pkg", TaskID: task.TaskID, WorkerRole: task.Role})
	if err != nil {
		t.Fatalf("build tester task package: %v", err)
	}
	if len(pkg.AllowedRepos.Writable) != 0 {
		t.Fatalf("tester must not be granted writable repos: %#v", pkg.AllowedRepos)
	}
	if len(pkg.AllowedRepos.Readable) != 1 || pkg.AllowedRepos.Readable[0] != "group/project" {
		t.Fatalf("tester readable surface must be the review target: %#v", pkg.AllowedRepos)
	}
	if pkg.Delivery != nil {
		t.Fatalf("tester has no delivery semantics: %#v", pkg.Delivery)
	}
}

// 阻塞式对照:developer 任务的 AllowedRepos 既有口径不变(目标仓可写)。
func TestBuildRunnerTaskPackageDeveloperAllowedReposKeepWritable(t *testing.T) {
	task := &model.Task{
		TaskID: "task-dev-pkg", DedupeKey: "dev:task-dev-pkg", Type: "develop_from_issue", Role: "DeveloperWorker",
		SourceRepo: "group/project", TargetRepo: "group/project", Status: model.TaskStatusPending,
		ContextJSON: `{"project":{"path":"group/project"},"repos":[{"name":"project","branch":"master","access":"writable"}]}`,
	}
	pkg, err := BuildRunnerTaskPackage(task, &model.Run{RunID: "run-dev-pkg", TaskID: task.TaskID, WorkerRole: task.Role})
	if err != nil {
		t.Fatalf("build developer task package: %v", err)
	}
	if len(pkg.AllowedRepos.Writable) != 1 || pkg.AllowedRepos.Writable[0] != "group/project" {
		t.Fatalf("developer writable surface must stay: %#v", pkg.AllowedRepos)
	}
}
