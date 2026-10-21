package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"sdlc-coordinator/internal/model"
)

type taskVisualizationGraph struct {
	Nodes []struct {
		ID    model.TaskStatus `json:"id"`
		Label string           `json:"label"`
	} `json:"nodes"`
	Edges []struct {
		From model.TaskStatus `json:"from"`
		To   model.TaskStatus `json:"to"`
	} `json:"edges"`
}

func TestTaskGraphMatchesStateMachine(t *testing.T) {
	if err := validateTaskVisualizationGraph(NewStateMachine(), readTaskVisualizationGraph(t)); err != nil {
		t.Fatal(err)
	}
}

func TestTaskGraphDetectsMissingTargetState(t *testing.T) {
	machine := NewStateMachine()
	machine.allowed[model.TaskStatusRunning][model.TaskStatus("future_terminal")] = struct{}{}
	if err := validateTaskVisualizationGraph(machine, readTaskVisualizationGraph(t)); err == nil {
		t.Fatal("graph validation missed a state that appears only as a transition target")
	}
}

func readTaskVisualizationGraph(t *testing.T) taskVisualizationGraph {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "web", "src", "content", "task-state-machine.json"))
	if err != nil {
		t.Fatalf("read task graph: %v", err)
	}
	var graph taskVisualizationGraph
	if err := json.Unmarshal(raw, &graph); err != nil {
		t.Fatalf("decode task graph: %v", err)
	}
	return graph
}

func validateTaskVisualizationGraph(machine *StateMachine, graph taskVisualizationGraph) error {
	known := make(map[model.TaskStatus]bool)
	for from, targets := range machine.allowed {
		known[from] = true
		for to := range targets {
			known[to] = true
		}
	}
	nodes := make(map[model.TaskStatus]bool)
	for _, node := range graph.Nodes {
		if nodes[node.ID] || node.Label == "" {
			return fmt.Errorf("duplicate node or missing label: %q", node.ID)
		}
		if !known[node.ID] {
			return fmt.Errorf("graph has unknown task status %q", node.ID)
		}
		nodes[node.ID] = true
	}
	for status := range known {
		if !nodes[status] {
			return fmt.Errorf("graph is missing task status %q", status)
		}
	}
	edges := make(map[[2]model.TaskStatus]bool)
	for _, edge := range graph.Edges {
		key := [2]model.TaskStatus{edge.From, edge.To}
		if !nodes[edge.From] || !nodes[edge.To] || edges[key] || edge.From == edge.To {
			return fmt.Errorf("invalid or duplicate edge: %s -> %s", edge.From, edge.To)
		}
		edges[key] = true
	}
	for from := range known {
		for to := range known {
			if from == to {
				continue
			}
			if got, want := edges[[2]model.TaskStatus{from, to}], machine.CanTransition(from, to); got != want {
				return fmt.Errorf("graph edge %s -> %s = %v, state machine allows %v", from, to, got, want)
			}
		}
	}
	return nil
}
