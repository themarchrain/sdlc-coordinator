package orchestrator

import (
	"sdlc-coordinator/internal/model"
	"testing"
)

func TestInterruptedRunIsTerminal(t *testing.T) {
	if !isTerminalRunStatus(model.RunStatusInterrupted) {
		t.Fatal("interrupted run was not terminal")
	}
}
