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

func TestApplyStatsOutput(t *testing.T) {
	containers := []Container{
		{ID: "c1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2", Name: "full-hash-container"},
		{ID: "f7e8d9c0b1a2", Name: "short-hash-container"},
		{ID: "shortid", Name: "sub12-char-container"},
		{ID: "unmatched12345", Name: "unmatched-container"},
	}

	statsTSV := "c1a2b3c4d5e6\t12.5%\t256MiB / 8GiB\n" +
		"f7e8d9c0b1a2\t1.2%\t64MiB / 8GiB\n" +
		"shortidprefix\t0.5%\t16MiB / 8GiB\n"

	applyStatsOutput(containers, statsTSV)

	if containers[0].CPU != "12.5%" || containers[0].Memory != "256MiB / 8GiB" {
		t.Errorf("expected container 0 stats to match full ID via 12-char prefix, got cpu=%s mem=%s", containers[0].CPU, containers[0].Memory)
	}
	if containers[1].CPU != "1.2%" || containers[1].Memory != "64MiB / 8GiB" {
		t.Errorf("expected container 1 stats to match exact short ID, got cpu=%s mem=%s", containers[1].CPU, containers[1].Memory)
	}
	if containers[2].CPU != "0.5%" || containers[2].Memory != "16MiB / 8GiB" {
		t.Errorf("expected container 2 stats to match fallback prefix, got cpu=%s mem=%s", containers[2].CPU, containers[2].Memory)
	}
	if containers[3].CPU != "" || containers[3].Memory != "" {
		t.Errorf("expected container 3 stats to remain empty, got cpu=%s mem=%s", containers[3].CPU, containers[3].Memory)
	}
}

func TestValidateContainerTarget(t *testing.T) {
	validTargets := []string{
		"c1a2b3c4d5e6",
		"m3tal-proxy",
		"my_service.v1",
		"container-123",
		"A1b2C3",
		"69a83d735fb29dc0a4cb29bdf934810294827103859203810284719283749281",
		strings.Repeat("a", 129),
	}

	for _, target := range validTargets {
		if err := validateContainerTarget(target); err != nil {
			t.Errorf("validateContainerTarget(%q) returned unexpected error: %v", target, err)
		}
	}

	invalidTargets := []string{
		"",
		"-f",
		"--all",
		"-checkpoint",
		"foo/bar",
		"container;rm -rf /",
		"container|cat",
		"container&echo",
		"container name",
		"container\nnewline",
		".hidden",
		"../../etc/passwd",
	}

	for _, target := range invalidTargets {
		if err := validateContainerTarget(target); err == nil {
			t.Errorf("validateContainerTarget(%q) expected error, got nil", target)
		}
		if err := StartContainer(target); err == nil {
			t.Errorf("StartContainer(%q) expected error, got nil", target)
		}
		if err := StopContainer(target); err == nil {
			t.Errorf("StopContainer(%q) expected error, got nil", target)
		}
		if err := RestartContainer(target); err == nil {
			t.Errorf("RestartContainer(%q) expected error, got nil", target)
		}
		if _, err := GetLogs(target, 50); err == nil {
			t.Errorf("GetLogs(%q) expected error, got nil", target)
		}
		if _, err := InspectContainer(target); err == nil {
			t.Errorf("InspectContainer(%q) expected error, got nil", target)
		}
	}
}

func BenchmarkApplyStatsOutput(b *testing.B) {
	const n = 100
	containers := make([]Container, n)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fullID := strings.Repeat(string(rune('a'+(i%26))), 64)
		containers[i] = Container{ID: fullID}
		sb.WriteString(fullID[:12])
		sb.WriteString("\t2.5%\t128MiB / 16GiB\n")
	}
	statsOutput := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		applyStatsOutput(containers, statsOutput)
	}
}
