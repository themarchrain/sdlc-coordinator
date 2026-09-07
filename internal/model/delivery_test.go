package model

import "testing"

func TestDeliveryBranchForTaskIsStableAndNamespaced(t *testing.T) {
	if got := DeliveryBranchForTask("task-delivery-1"); got != "agent/task-delivery-1" {
		t.Fatalf("unexpected delivery branch %q", got)
	}
}

func TestDeliveryBranchPrefersPersistedTaskValue(t *testing.T) {
	task := &Task{TaskID: "task-delivery-1", DeliveryBranch: "alice/issue-13-20260727"}
	if got := DeliveryBranch(task); got != "alice/issue-13-20260727" {
		t.Fatalf("unexpected persisted delivery branch %q", got)
	}

	task.DeliveryBranch = ""
	if got := DeliveryBranch(task); got != "agent/task-delivery-1" {
		t.Fatalf("unexpected legacy fallback branch %q", got)
	}
}
