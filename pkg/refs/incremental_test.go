package refs

import (
	"testing"
)

func TestIncrementalGraph_AddRemoveEdge(t *testing.T) {
	g := NewIncrementalGraph()
	g.AddEdge("a", "b")
	g.AddEdge("a", "c")
	g.AddEdge("b", "c")

	if got := g.Targets("a"); len(got) != 2 {
		t.Errorf("targets a = %v, want 2", got)
	}
	if got := g.Backlinks("c"); len(got) != 2 {
		t.Errorf("backlinks c = %v, want 2", got)
	}

	g.RemoveEdge("a", "c")
	if got := g.Targets("a"); len(got) != 1 || got[0] != "b" {
		t.Errorf("targets a after remove = %v", got)
	}
	if got := g.Backlinks("c"); len(got) != 1 || got[0] != "b" {
		t.Errorf("backlinks c after remove = %v", got)
	}
}

func TestIncrementalGraph_UpdateOutgoing(t *testing.T) {
	g := NewIncrementalGraph()
	g.AddEdge("a", "b")
	g.AddEdge("a", "c")

	// pretend a was edited and now points to c, d only
	g.UpdateOutgoing("a", []string{"c", "d"})

	if got := g.Targets("a"); len(got) != 2 {
		t.Fatalf("targets a = %v, want [c d]", got)
	}
	if len(g.Backlinks("b")) != 0 {
		t.Errorf("b still has backlinks: %v", g.Backlinks("b"))
	}
	if len(g.Backlinks("d")) != 1 {
		t.Errorf("d backlinks = %v, want [a]", g.Backlinks("d"))
	}
}

func TestIncrementalGraph_RemoveNode(t *testing.T) {
	g := NewIncrementalGraph()
	g.AddEdge("a", "b")
	g.AddEdge("c", "a")
	g.AddEdge("a", "d")

	g.RemoveNode("a")

	if len(g.Forward["a"]) != 0 || len(g.Reverse["a"]) != 0 {
		t.Errorf("a still in forward/reverse")
	}
	if _, ok := g.Reverse["b"]["a"]; ok {
		t.Errorf("b still has a as backlink")
	}
	if _, ok := g.Forward["c"]["a"]; ok {
		t.Errorf("c still points to a")
	}
}

func TestIncrementalGraph_HasPath(t *testing.T) {
	g := NewIncrementalGraph()
	g.AddEdge("a", "b")
	g.AddEdge("b", "c")
	g.AddEdge("c", "d")

	if !g.HasPath("a", "d") {
		t.Error("a→d path should exist")
	}
	if !g.HasPath("b", "d") {
		t.Error("b→d path should exist")
	}
	if g.HasPath("d", "a") {
		t.Error("d→a path should not exist")
	}
}

// TestIncrementalGraph_HasPath_AddedEdgeWouldCycle verifies the canonical
// cycle-detection-on-insert pattern: HasPath(v, u) is true means adding
// (u, v) creates a cycle.
func TestIncrementalGraph_HasPath_AddedEdgeWouldCycle(t *testing.T) {
	g := NewIncrementalGraph()
	g.AddEdge("a", "b")
	g.AddEdge("b", "c")

	// adding c→a would create a cycle: HasPath(a, c) is true
	if !g.HasPath("a", "c") {
		t.Error("a→c path expected (would-be cycle)")
	}
	// adding d→a would not create a cycle
	if g.HasPath("a", "d") {
		t.Error("a→d path should not exist")
	}
}
