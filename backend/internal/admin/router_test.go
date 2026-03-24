package admin

import (
	"testing"

	"harborshield/backend/internal/storage"
)

func TestShouldBlockNodeStateChangeWhenLastHealthyActiveNodeWouldBeDemoted(t *testing.T) {
	current := &storage.Node{
		ID:            "node-a",
		Status:        "healthy",
		OperatorState: "active",
	}
	nodes := []storage.Node{
		{
			ID:            "node-a",
			Status:        "healthy",
			OperatorState: "active",
		},
		{
			ID:            "node-b",
			Status:        "offline",
			OperatorState: "maintenance",
		},
	}

	if !shouldBlockNodeStateChange(current, "maintenance", nodes) {
		t.Fatal("expected last healthy active node demotion to be blocked")
	}
	if !shouldBlockNodeStateChange(current, "draining", nodes) {
		t.Fatal("expected last healthy active node drain to be blocked")
	}
}

func TestShouldBlockNodeStateChangeAllowsDemotionWithAnotherHealthyActiveNode(t *testing.T) {
	current := &storage.Node{
		ID:            "node-a",
		Status:        "healthy",
		OperatorState: "active",
	}
	nodes := []storage.Node{
		{
			ID:            "node-a",
			Status:        "healthy",
			OperatorState: "active",
		},
		{
			ID:            "node-b",
			Status:        "healthy",
			OperatorState: "active",
		},
	}

	if shouldBlockNodeStateChange(current, "maintenance", nodes) {
		t.Fatal("expected demotion to stay allowed when another healthy active node exists")
	}
}

func TestShouldBlockNodeStateChangeAllowsSafeCases(t *testing.T) {
	tests := []struct {
		name           string
		current        *storage.Node
		requestedState string
		nodes          []storage.Node
	}{
		{
			name:           "nil current node",
			current:        nil,
			requestedState: "maintenance",
			nodes:          []storage.Node{},
		},
		{
			name: "already non-active",
			current: &storage.Node{
				Status:        "healthy",
				OperatorState: "maintenance",
			},
			requestedState: "active",
			nodes: []storage.Node{
				{Status: "healthy", OperatorState: "maintenance"},
			},
		},
		{
			name: "current node unhealthy",
			current: &storage.Node{
				Status:        "offline",
				OperatorState: "active",
			},
			requestedState: "maintenance",
			nodes: []storage.Node{
				{Status: "offline", OperatorState: "active"},
			},
		},
		{
			name: "staying active",
			current: &storage.Node{
				Status:        "healthy",
				OperatorState: "active",
			},
			requestedState: "active",
			nodes: []storage.Node{
				{Status: "healthy", OperatorState: "active"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if shouldBlockNodeStateChange(test.current, test.requestedState, test.nodes) {
				t.Fatal("expected safeguard helper to allow this state change")
			}
		})
	}
}
