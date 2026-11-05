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

func TestInspectContainer(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	info, err := InspectContainer("mock-target")
	if err != nil {
		t.Fatalf("expected no error from InspectContainer, got %v", err)
	}
	if info["Id"] == "" {
		t.Errorf("expected non-empty Id in inspect info")
	}

	// Test inspect action dispatch via RPC
	params, _ := json.Marshal(map[string]interface{}{
		"action_id": "inspect_container",
		"target":    "mock-target",
	})
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "m3tal.action",
		Params:  params,
	}
	handleRequest(&req)
}

func TestTargetValidationRejectsMaliciousInput(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	maliciousTargets := []string{
		"",
		"--checkpoint=foo",
		"-t",
		"; rm -rf /",
		"container $(whoami)",
		"cont/ainer",
		"../../etc/passwd",
		"container name with spaces",
		".hidden",
	}

	for _, target := range maliciousTargets {
		if isValidContainerTarget(target) {
			t.Errorf("expected target %q to be rejected by isValidContainerTarget", target)
		}
		if err := StartContainer(target); err == nil {
			t.Errorf("expected StartContainer to reject %q", target)
		}
		if err := StopContainer(target); err == nil {
			t.Errorf("expected StopContainer to reject %q", target)
		}
		if err := RestartContainer(target); err == nil {
			t.Errorf("expected RestartContainer to reject %q", target)
		}
		if _, err := GetLogs(target, 50); err == nil {
			t.Errorf("expected GetLogs to reject %q", target)
		}
		if _, err := InspectContainer(target); err == nil {
			t.Errorf("expected InspectContainer to reject %q", target)
		}
	}

	validTargets := []string{
		"c1a2b3c4d5e6",
		"m3tal-proxy",
		"my_web_service.1",
		"web-1",
	}
	for _, target := range validTargets {
		if !isValidContainerTarget(target) {
			t.Errorf("expected target %q to be valid", target)
		}
	}
}
