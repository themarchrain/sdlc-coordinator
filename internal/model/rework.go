package model

import "errors"

const TaskTypeReworkMergeRequest = "rework_merge_request"

var ErrMergeRequestNotManaged = errors.New("merge request is not managed by coordinator")
var ErrMergeRequestReworkRequesterUnauthorized = errors.New("merge request rework requester is not authorized")

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
