package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMockDockerOperations(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	containers, err := GetContainers()
	if err != nil {
		t.Fatalf("expected no error from mock GetContainers, got %v", err)
	}
	if len(containers) == 0 {
		t.Fatalf("expected mock containers, got 0")
	}

	// Test Stop
	target := containers[0].ID
	if err := StopContainer(target); err != nil {
		t.Fatalf("failed to stop container: %v", err)
	}
	containers, _ = GetContainers()
	if containers[0].State != "exited" {
		t.Errorf("expected state exited, got %s", containers[0].State)
	}

	// Test Start
	if err := StartContainer(target); err != nil {
		t.Fatalf("failed to start container: %v", err)
	}
	containers, _ = GetContainers()
	if containers[0].State != "running" {
		t.Errorf("expected state running, got %s", containers[0].State)
	}

	// Test Restart
	if err := RestartContainer(target); err != nil {
		t.Fatalf("failed to restart container: %v", err)
	}

	// Test Prune
	pruneMsg, err := PruneImages()
	if err != nil {
		t.Fatalf("failed to prune images: %v", err)
	}
	if !strings.Contains(pruneMsg, "reclaimed") {
		t.Errorf("expected reclaimed in pruneMsg, got %s", pruneMsg)
	}

	// Test Logs
	logs, err := GetLogs(target, 10)
	if err != nil {
		t.Fatalf("failed to get logs: %v", err)
	}
	if len(logs) == 0 {
		t.Errorf("expected non-empty logs")
	}
}

func TestRPCHandlerInit(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "m3tal.init",
	}

	// Test that handleRequest doesn't panic
	handleRequest(&req)
}

func TestRPCHandlerCollect(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "m3tal.collect",
	}

	handleRequest(&req)
}

func TestRPCHandlerAction(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	params, _ := json.Marshal(map[string]interface{}{
		"action_id": "prune_images",
	})
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "m3tal.action",
		Params:  params,
	}

	handleRequest(&req)
}
