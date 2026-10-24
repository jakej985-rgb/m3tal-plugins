package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMockComposeOperations(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	stacks, err := ListStacks()
	if err != nil {
		t.Fatalf("expected no error from mock ListStacks, got %v", err)
	}
	if len(stacks) == 0 {
		t.Fatalf("expected mock stacks, got 0")
	}

	target := stacks[0].Name

	// Test ValidateYAML
	valid, msg, err := ValidateYAML(stacks[0].YAMLContent)
	if err != nil || !valid {
		t.Fatalf("expected valid YAML, got %v (%s)", err, msg)
	}

	invalidContent := "foo: bar"
	valid, _, _ = ValidateYAML(invalidContent)
	if valid {
		t.Errorf("expected invalid YAML for content missing services:, got valid")
	}

	// Test Stop
	downMsg, err := ComposeDown(target)
	if err != nil {
		t.Fatalf("failed to stop compose stack: %v", err)
	}
	if !strings.Contains(downMsg, "stopped") {
		t.Errorf("expected stopped in downMsg, got %s", downMsg)
	}

	// Test Start
	upMsg, err := ComposeUp(target)
	if err != nil {
		t.Fatalf("failed to start compose stack: %v", err)
	}
	if !strings.Contains(upMsg, "started") {
		t.Errorf("expected started in upMsg, got %s", upMsg)
	}

	// Test Restart
	reMsg, err := ComposeRestart(target)
	if err != nil {
		t.Fatalf("failed to restart compose stack: %v", err)
	}
	if !strings.Contains(reMsg, "restarted") {
		t.Errorf("expected restarted in reMsg, got %s", reMsg)
	}

	// Test SaveStack
	newYaml := "services:\n  web:\n    image: nginx:alpine\n"
	if err := SaveStack("new-stack", newYaml); err != nil {
		t.Fatalf("failed to save stack: %v", err)
	}

	// Test Logs
	logs, err := ComposeLogs(target, 10)
	if err != nil {
		t.Fatalf("failed to get compose logs: %v", err)
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
		"action_id": "validate_yaml",
		"content":   "services:\n  app:\n    image: node:20\n",
	})
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "m3tal.action",
		Params:  params,
	}

	handleRequest(&req)
}
