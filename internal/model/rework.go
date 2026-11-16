package model

import "errors"

const TaskTypeReworkMergeRequest = "rework_merge_request"

var ErrMergeRequestNotManaged = errors.New("merge request is not managed by coordinator")
var ErrMergeRequestReworkRequesterUnauthorized = errors.New("merge request rework requester is not authorized")

// ErrTaskObjectNotOpen means the task's target issue or merge request is no
// longer open, so resuming work on it would act on a dead object.
var ErrTaskObjectNotOpen = errors.New("task target object is not open")

// ErrTaskObjectUnavailable means the live object state could not be read.
var ErrTaskObjectUnavailable = errors.New("task target object state is unavailable")

type MergeRequestReworkRequest struct {
	DedupeKey               string
	Source                  string
	ProjectID               string
	ProjectPath             string
	RepositoryURL           string
	RepositoryID            *string
	MergeRequestIID         int
	MergeRequestURL         string
	MergeRequestTitle       string
	MergeRequestDescription string
	SourceBranch            string
	TargetBranch            string
	ExpectedHeadSHA         string
	NoteID                  int
	NoteURL                 string
	NoteType                string
	DiscussionID            string
	ResumeContext           string
	RequesterID             int
	RequesterName           string
	RequesterUsername       string
	RequesterEmail          string
	RequesterPreAuthorized  bool
	AssigneeIDs             []int
	ReviewerIDs             []int
}

type MergeRequestLifecycleRequest struct {
	DedupeKey       string
	ProjectID       string
	ProjectPath     string
	MergeRequestIID int
	Action          string
	State           string
	HeadSHA         string
}
