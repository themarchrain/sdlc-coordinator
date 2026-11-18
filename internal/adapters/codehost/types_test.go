package codehost

import (
	"strings"
	"testing"
)

func TestNormalizeMergeRequestState(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		merged bool
		want   MergeRequestState
	}{
		{name: "opened is open", raw: "opened", want: MergeRequestStateOpen},
		{name: "locked is open", raw: "locked", want: MergeRequestStateOpen},
		{name: "open is open", raw: "open", want: MergeRequestStateOpen},
		{name: "closed is closed", raw: "closed", want: MergeRequestStateClosed},
		{name: "merged state is merged", raw: "merged", want: MergeRequestStateMerged},
		{name: "merged flag wins over closed", raw: "closed", merged: true, want: MergeRequestStateMerged},
		{name: "case and blanks are ignored", raw: "  CLOSED ", want: MergeRequestStateClosed},
		{name: "empty is unknown", raw: "", want: MergeRequestStateUnknown},
		{name: "unrecognised is unknown", raw: "reopening", want: MergeRequestStateUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeMergeRequestState(test.raw, test.merged); got != test.want {
				t.Fatalf("NormalizeMergeRequestState(%q, %v) = %q, want %q", strings.TrimSpace(test.raw), test.merged, got, test.want)
			}
		})
	}
	if MergeRequestStateOpen == MergeRequestStateUnknown {
		t.Fatal("open must not equal unknown")
	}
}
