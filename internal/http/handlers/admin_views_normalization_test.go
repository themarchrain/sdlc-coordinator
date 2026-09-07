package handlers

import (
	"strings"
	"testing"
	"time"

	"sdlc-coordinator/internal/model"
)

func TestResultSubmissionSummaryExposesNormalizationTrail(t *testing.T) {
	submission := model.ResultSubmission{
		SubmissionID: "submission-1", TaskID: "task-1", RunID: "run-1",
		Source: "mcp", ToolName: "submit_developer_result", Role: "DeveloperWorker",
		PayloadHash: "hash", Status: model.ResultSubmissionStatusSucceeded,
		NormalizationJSON: `[{"op":"rename_key","path":"taskId","before":"taskId","after":"task_id"}]`,
		CreatedAt:         time.Now(),
	}
	summary := resultSubmissionSummary(submission)
	if summary["normalization_status"] != "normalized" {
		t.Fatalf("normalized submission must be reported, got %#v", summary["normalization_status"])
	}
	detail := resultSubmissionDetail(submission)
	if !strings.Contains(detail["normalization_json"].(string), "rename_key") {
		t.Fatalf("detail must carry the raw trail: %#v", detail["normalization_json"])
	}

	clean := submission
	clean.NormalizationJSON = ""
	cleanSummary := resultSubmissionSummary(clean)
	if cleanSummary["normalization_status"] != "clean" {
		t.Fatalf("clean submission must be reported, got %#v", cleanSummary["normalization_status"])
	}
}
