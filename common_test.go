package main

import "testing"

func TestNewNode(t *testing.T) {
	fakeArgs := &btcdArgs{}
	node, err := NewNodeFromArgs(fakeArgs, nil, nil)
	if err != nil {
		t.Errorf("NewNodeFromArgs error: %v", err)
	}
	defer node.Shutdown()
	if err := node.Start(); err == nil {
		t.Errorf("node.Start error: %v", err)
	}
	if err := node.Connect(); err == nil {
		t.Errorf("node.Connect error: %v", err)
	}
}
