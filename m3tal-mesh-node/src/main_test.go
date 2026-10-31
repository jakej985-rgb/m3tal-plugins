package main

import (
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestNodeInit(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "m3tal.init",
	}
	handleRequest(&req)
}

func TestNodeCollect(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "m3tal.collect",
	}
	handleRequest(&req)
}

func TestNodeAction(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{
		"action_id": "beacon_ping",
	})
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "m3tal.action",
		Params:  params,
	}
	handleRequest(&req)
}

func runNodeActionForTest(t *testing.T, id int, params string) RPCResponse {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	request := &RPCRequest{ID: id, Params: json.RawMessage(params)}
	handleAction(request)
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	var response RPCResponse
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatalf("decode action response %q: %v", output, err)
	}
	return response
}

func TestNodeGetSettingsReturnsFlat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveNodeRuntimeConfig(nodeRuntimeConfig{PairingToken: "secret123", ListenPort: 9876, BroadcastMDNS: true}); err != nil {
		t.Fatal(err)
	}

	resp := runNodeActionForTest(t, 1, `{"action_id":"get_settings","args":{}}`)
	if resp.Error != nil {
		t.Fatalf("get_settings failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
	if result["has_pairing_token"] != true {
		t.Fatalf("expected has_pairing_token=true, got %v", result["has_pairing_token"])
	}
	if port, ok := result["listen_port"].(float64); !ok || int(port) != 9876 {
		t.Fatalf("expected listen_port=9876, got %v", result["listen_port"])
	}
	if result["broadcast_mdns"] != true {
		t.Fatalf("expected broadcast_mdns=true, got %v", result["broadcast_mdns"])
	}
	if _, hasPayload := result["payload"]; hasPayload {
		t.Fatal("get_settings must not double-wrap fields under \"payload\"")
	}
}

func TestNodeSetSettingsReturnsFlat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	resp := runNodeActionForTest(t, 1, `{"action_id":"set_settings","args":{"pairing_token":"new-secret","listen_port":8888,"broadcast_mdns":false}}`)
	if resp.Error != nil {
		t.Fatalf("set_settings failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
	if _, hasPayload := result["payload"]; hasPayload {
		t.Fatal("set_settings must not double-wrap fields under \"payload\"")
	}
	cfg := loadNodeRuntimeConfig()
	if cfg.PairingToken != "new-secret" || cfg.ListenPort != 8888 || cfg.BroadcastMDNS != false {
		t.Fatalf("unexpected saved config: %+v", cfg)
	}
}
