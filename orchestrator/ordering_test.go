package orchestrator

import (
	"testing"

	"github.com/user/golovebox/core"
	"github.com/user/golovebox/orchestrator/dag"
)

// makeIntent builds a core.Intent with the given step kinds.
func makeIntent(kinds ...core.StepKind) core.Intent {
	steps := make([]core.Step, len(kinds))
	for i, k := range kinds {
		steps[i] = core.Step{Kind: k, Title: "test"}
	}
	return core.Intent{Steps: steps}
}

func TestEnforceWorkflowOrdering_branchBeforeEdits(t *testing.T) {
	d := dag.New("test")
	// Manually add a branch node and an edit node with wrong ordering.
	branch := &dag.Node{ID: "branch", Type: dag.TypeBranch, State: dag.StatePending, Task: "branch", Dependencies: []string{}}
	edit := &dag.Node{ID: "edit1", Type: dag.TypeTask, State: dag.StatePending, Task: "edit", Dependencies: []string{}}
	push := &dag.Node{ID: "push", Type: dag.TypePush, State: dag.StatePending, Task: "push", Dependencies: []string{}}
	_ = d.AddNode(branch)
	_ = d.AddNode(edit)
	_ = d.AddNode(push)

	intents := []core.Intent{makeIntent(core.KindBranch, core.KindPush)}
	enforceWorkflowOrdering(d, intents)

	// After enforcement, edit node must depend on branch node.
	editNode := d.Nodes["edit1"]
	hasBranchDep := false
	for _, dep := range editNode.Dependencies {
		if dep == "branch" {
			hasBranchDep = true
		}
	}
	if !hasBranchDep {
		t.Fatalf("edit node dependencies = %v, want to include 'branch'", editNode.Dependencies)
	}

	// Push must depend on all task nodes.
	pushNode := d.Nodes["push"]
	hasEditDep := false
	for _, dep := range pushNode.Dependencies {
		if dep == "edit1" {
			hasEditDep = true
		}
	}
	if !hasEditDep {
		t.Fatalf("push node dependencies = %v, want to include 'edit1'", pushNode.Dependencies)
	}
}

func TestEnforceWorkflowOrdering_noOpWhenNoBranchPushPR(t *testing.T) {
	d := dag.New("test2")
	task := &dag.Node{ID: "task1", Type: dag.TypeTask, State: dag.StatePending, Task: "impl", Dependencies: []string{}}
	_ = d.AddNode(task)

	intents := []core.Intent{makeIntent(core.KindEdit)}
	before := len(d.Nodes)
	enforceWorkflowOrdering(d, intents)
	if len(d.Nodes) != before {
		t.Fatalf("enforceWorkflowOrdering mutated DAG when no branch/push/pr: %d nodes -> %d", before, len(d.Nodes))
	}
}
