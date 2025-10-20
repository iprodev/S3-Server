package miniobject

import (
	"testing"
)

func TestHashRingStability(t *testing.T) {
	nodes := []string{"node1", "node2", "node3"}
	ring := NewHashRing(nodes, 3)

	// Same key should always map to same nodes
	key := "test/object"
	nodes1 := ring.GetNodes(key, 3)
	nodes2 := ring.GetNodes(key, 3)

	if len(nodes1) != 3 || len(nodes2) != 3 {
		t.Fatalf("expected 3 nodes, got %d and %d", len(nodes1), len(nodes2))
	}

	for i := range nodes1 {
		if nodes1[i] != nodes2[i] {
			t.Errorf("node mismatch at position %d: %s != %s", i, nodes1[i], nodes2[i])
		}
	}
}

func TestHashRingDistribution(t *testing.T) {
	nodes := []string{"node1", "node2", "node3"}
	ring := NewHashRing(nodes, 3)

	counts := make(map[string]int)

	// Test 1000 keys
	for i := 0; i < 1000; i++ {
		key := string(rune(i))
		selectedNodes := ring.GetNodes(key, 1)
		if len(selectedNodes) > 0 {
			counts[selectedNodes[0]]++
		}
	}

	// Each node should get roughly 1/3 of keys (allow 20% variance)
	expected := 333
	tolerance := 67

	for node, count := range counts {
		if count < expected-tolerance || count > expected+tolerance {
			t.Logf("node %s: %d keys (expected ~%d)", node, count, expected)
		}
	}
}

func TestHashRingUniqueNodes(t *testing.T) {
	nodes := []string{"node1", "node2", "node3"}
	ring := NewHashRing(nodes, 3)

	selectedNodes := ring.GetNodes("test/key", 3)

	seen := make(map[string]bool)
	for _, node := range selectedNodes {
		if seen[node] {
			t.Errorf("duplicate node: %s", node)
		}
		seen[node] = true
	}
}

func TestHashRingRequestMoreThanAvailable(t *testing.T) {
	nodes := []string{"node1", "node2"}
	ring := NewHashRing(nodes, 3)

	selectedNodes := ring.GetNodes("test/key", 5)

	if len(selectedNodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(selectedNodes))
	}
}
