package operationmarker

import (
	"regexp"
	"strings"
)

const currentPrefix = "agent-coordinator-operation"

var markerPattern = regexp.MustCompile(`<!-- (?:[a-z0-9-]+-)?coordinator-operation:[^>]+ -->`)

func New(dedupeKey string) string {
	return "<!-- " + currentPrefix + ":" + strings.TrimSpace(dedupeKey) + " -->"
}

func Contains(body string) bool {
	return markerPattern.MatchString(body)
}
