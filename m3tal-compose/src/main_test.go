package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestYAMLValidationAndFlowStyleServices(t *testing.T) {
	valid, _, err := ValidateYAML("services: { api: { image: 'nginx:alpine' }, cache: { image: redis } }\n")
	if err != nil || !valid {
		t.Fatalf("expected valid flow-style YAML: valid=%v err=%v", valid, err)
	}
	services := extractServices("services: { api: { image: 'nginx:alpine' }, cache: { image: redis } }\n")
	if len(services) != 2 || services[0] != "api" || services[1] != "cache" {
		t.Fatalf("unexpected services: %#v", services)
	}
	valid, _, _ = ValidateYAML("services:\n  api: [\n")
	if valid {
		t.Fatal("expected malformed YAML to fail validation")
	}
	valid, _, _ = ValidateYAML("services: {}\n")
	if valid {
		t.Fatal("expected an empty services mapping to fail validation")
	}
}

func TestConfiguredRecursiveDiscoveryAndSaveExisting(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	root := t.TempDir()
	composePath := filepath.Join(root, "nested", "api service", "compose.yaml")
	if err := os.MkdirAll(filepath.Dir(composePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(composePath, []byte("name: api-project\nservices:\n  old:\n    image: alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(composePath), "docker-compose.yml"), []byte("services:\n  ignored:\n    image: alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	overridePath := filepath.Join(filepath.Dir(composePath), "compose.override.yaml")
	if err := os.WriteFile(overridePath, []byte("services:\n  old:\n    environment:\n      MODE: test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	flatPath := filepath.Join(root, "flat-stack.yaml")
	if err := os.WriteFile(flatPath, []byte("services:\n  flat:\n    image: alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	settingsMu.Lock()
	oldRoot, oldLayout, oldBackup := composeDirSetting, newStackLayout, backupOnSave
	composeDirSetting, newStackLayout, backupOnSave = root, "per_stack_folder", true
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		composeDirSetting, newStackLayout, backupOnSave = oldRoot, oldLayout, oldBackup
		settingsMu.Unlock()
	}()

	stacks, err := ListStacks()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	flatFound := false
	for _, stack := range stacks {
		if stack.Name == "api service" && stack.ProjectName == "api-project" && stack.Path == composePath && len(stack.ConfigFiles) == 2 && stack.ConfigFiles[1] == overridePath {
			found = true
		}
		if stack.Name == "flat-stack" && stack.Path == flatPath {
			flatFound = true
		}
	}
	if !found {
		t.Fatalf("recursive discovery did not return existing compose project at %s", composePath)
	}
	if !flatFound {
		t.Fatalf("flat Compose file was not discovered: %s", flatPath)
	}

	content := "services:\n  api:\n    image: nginx:alpine\n"
	if err := SaveStack("api service", content); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("expected save to update discovered file; got %q", got)
	}
	if _, err := os.Stat(composePath + ".bak"); err != nil {
		t.Fatalf("expected backup of existing file: %v", err)
	}
}

func TestNewStackLayoutsAndPathValidation(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	root := t.TempDir()
	settingsMu.Lock()
	oldRoot, oldLayout, oldBackup := composeDirSetting, newStackLayout, backupOnSave
	composeDirSetting, backupOnSave = root, false
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		composeDirSetting, newStackLayout, backupOnSave = oldRoot, oldLayout, oldBackup
		settingsMu.Unlock()
	}()
	content := "services:\n  app:\n    image: nginx\n"
	settingsMu.Lock()
	newStackLayout = "per_stack_folder"
	settingsMu.Unlock()
	if err := SaveStack("folder-app", content); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "folder-app", "compose.yaml")); err != nil {
		t.Fatalf("per-stack layout not created: %v", err)
	}
	settingsMu.Lock()
	newStackLayout = "flat"
	settingsMu.Unlock()
	if err := SaveStack("flat-app", content); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "flat-app.yaml")); err != nil {
		t.Fatalf("flat layout not created: %v", err)
	}
	if err := SaveStack("../escape", content); err == nil {
		t.Fatal("path traversal stack name was accepted")
	}
}

func TestSettingsEnvelopeAndComposeArguments(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	settingsMu.RLock()
	oldRoot, oldLayout, oldBackup := composeDirSetting, newStackLayout, backupOnSave
	settingsMu.RUnlock()
	defer func() {
		settingsMu.Lock()
		composeDirSetting, newStackLayout, backupOnSave = oldRoot, oldLayout, oldBackup
		settingsMu.Unlock()
	}()
	params, err := json.Marshal(map[string]interface{}{"action_id": "set_settings", "args": map[string]interface{}{"config": map[string]interface{}{"compose_dir": "~/docker-test", "new_stack_layout": "flat", "backup_on_save": false}}})
	if err != nil {
		t.Fatal(err)
	}
	req := RPCRequest{JSONRPC: "2.0", ID: 5, Method: "m3tal.action", Params: params}
	handleRequest(&req)
	if !strings.HasSuffix(getBaseComposeDir(), "docker-test") {
		t.Fatalf("tilde path was not expanded: %s", getBaseComposeDir())
	}
	args := composeArgs([]string{"/srv/app/compose.yaml", "/srv/app/compose.override.yaml"}, "app-prod")
	want := []string{"compose", "-p", "app-prod", "-f", "/srv/app/compose.yaml", "-f", "/srv/app/compose.override.yaml"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected compose arguments: %#v", args)
	}
	if got := normalizeProjectName("Monster-Lab"); got != "monster-lab" {
		t.Fatalf("unexpected normalized Compose project name: %q", got)
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
