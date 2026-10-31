package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func runActionForTest(t *testing.T, id int, params string) RPCResponse {
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

func TestPairThenForgetActionsPersistFleet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}
	if err := saveMeshConfig(meshConfig{PairingToken: "test-secret", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		nonce := r.Header.Get("X-M3TAL-Nonce")
		clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
		fp := testServerFingerprint(server)
		if r.Header.Get("X-M3TAL-Proof") != requestProof("test-secret", nonce, clientNonce, fp) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := json.Marshal(remoteResponse{Hostname: "action-test", Telemetry: remoteTelemetry{Hostname: "action-test", Cores: 4}})
		w.Header().Set("X-M3TAL-Proof", responseProof("test-secret", nonce, clientNonce, body, fp))
		_, _ = w.Write(body)
	}))
	defer server.Close()

	pair := runActionForTest(t, 1, fmt.Sprintf(`{"action":"pair_node","args":{"ip":%q,"name":"action test","role":"server"}}`, server.Listener.Addr().String()))
	if pair.Error != nil {
		t.Fatalf("pair action failed: %s", pair.Error.Message)
	}
	var pairResult struct {
		Node MeshNode `json:"node"`
	}
	encoded, _ := json.Marshal(pair.Result)
	if err := json.Unmarshal(encoded, &pairResult); err != nil || pairResult.Node.ID == "" {
		t.Fatalf("pair action did not return paired node: %s (%v)", encoded, err)
	}
	if peers := loadPeers(); len(peers) != 1 || peers[0].ID != pairResult.Node.ID {
		t.Fatalf("pair action did not persist peer: %+v", peers)
	}

	forget := runActionForTest(t, 2, fmt.Sprintf(`{"action":"forget_node","args":{"node_id":%q}}`, pairResult.Node.ID))
	if forget.Error != nil {
		t.Fatalf("forget action failed: %s", forget.Error.Message)
	}
	if peers := loadPeers(); len(peers) != 0 || len(snapshotPeers()) != 0 {
		t.Fatalf("forget action left peer in fleet: saved=%+v memory=%+v", peers, snapshotPeers())
	}
}

func TestGetSettingsReturnsFlat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveMeshConfig(meshConfig{PairingToken: "test-token", ListenPort: 4242}); err != nil {
		t.Fatal(err)
	}

	resp := runActionForTest(t, 1, `{"action":"get_settings","args":{}}`)
	if resp.Error != nil {
		t.Fatalf("get_settings failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	// Verify flat structure — no nested "data" key
	if _, hasData := result["data"]; hasData {
		t.Fatal("get_settings must not nest fields under \"data\"; Core already wraps the result")
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
	if result["has_pairing_token"] != true {
		t.Fatalf("expected has_pairing_token=true, got %v", result["has_pairing_token"])
	}
	if port, ok := result["listen_port"].(float64); !ok || int(port) != 4242 {
		t.Fatalf("expected listen_port=4242, got %v", result["listen_port"])
	}
	if _, hasPayload := result["payload"]; hasPayload {
		t.Fatal("get_settings must not double-wrap fields under \"payload\"")
	}
}

func TestGetSettingsNoPairingToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveMeshConfig(meshConfig{PairingToken: "", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}

	resp := runActionForTest(t, 1, `{"action":"get_settings","args":{}}`)
	if resp.Error != nil {
		t.Fatalf("get_settings failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["has_pairing_token"] != false {
		t.Fatalf("expected has_pairing_token=false with empty token, got %v", result["has_pairing_token"])
	}
	if port, ok := result["listen_port"].(float64); !ok || int(port) != 9321 {
		t.Fatalf("expected listen_port=9321, got %v", result["listen_port"])
	}
	if _, hasPayload := result["payload"]; hasPayload {
		t.Fatal("get_settings must not double-wrap fields under \"payload\"")
	}
}

func TestGetFleetReturnsFlat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	resp := runActionForTest(t, 1, `{"action":"get_fleet","args":{}}`)
	if resp.Error != nil {
		t.Fatalf("get_fleet failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
	nodes, ok := result["nodes"].([]interface{})
	if !ok {
		t.Fatalf("expected nodes array, got %v", result["nodes"])
	}
	if len(nodes) == 0 {
		t.Fatalf("expected at least local node in fleet, got 0 nodes")
	}
	firstNode, ok := nodes[0].(map[string]interface{})
	if !ok || firstNode["id"] != "node-local" {
		t.Fatalf("expected first node to be node-local, got %v", nodes[0])
	}
	if firstNode["status"] != "online" {
		t.Fatalf("expected node-local status to be online, got %v", firstNode["status"])
	}
	if _, hasPayload := result["payload"]; hasPayload {
		t.Fatal("get_fleet must not double-wrap fields under \"payload\"")
	}
}

func TestPingNodeReturnsFlat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	resp := runActionForTest(t, 1, `{"action":"ping_node","args":{"node_id":"node-local"}}`)
	if resp.Error != nil {
		t.Fatalf("ping_node failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
	if result["node_id"] != "node-local" {
		t.Fatalf("expected node_id=node-local, got %v", result["node_id"])
	}
	if pingMs, ok := result["ping_ms"].(float64); !ok || pingMs != 0 {
		t.Fatalf("expected ping_ms=0, got %v", result["ping_ms"])
	}
	if _, hasPayload := result["payload"]; hasPayload {
		t.Fatal("ping_node must not double-wrap fields under \"payload\"")
	}
}

func TestScanNetworkEmptyTokenReturnsFlat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveMeshConfig(meshConfig{PairingToken: "", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}
	resp := runActionForTest(t, 1, `{"action":"scan_network","args":{}}`)
	if resp.Error != nil {
		t.Fatalf("scan_network failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("expected success=true, got %v", result["success"])
	}
	if _, ok := result["nodes"].([]interface{}); !ok {
		t.Fatalf("expected nodes array, got %v", result["nodes"])
	}
	if msg, ok := result["message"].(string); !ok || msg == "" {
		t.Fatalf("expected prompt message, got %v", result["message"])
	}
	if _, hasPayload := result["payload"]; hasPayload {
		t.Fatal("scan_network must not double-wrap fields under \"payload\"")
	}
}

func TestRequirePairingTokenSettings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Default setting should require pairing token
	resp := runActionForTest(t, 1, `{"action":"get_settings","args":{}}`)
	if resp.Error != nil {
		t.Fatalf("get_settings failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result["require_pairing_token"] != true {
		t.Fatalf("expected require_pairing_token=true by default, got %v", result["require_pairing_token"])
	}

	// Disable token requirement (make optional)
	setResp := runActionForTest(t, 2, `{"action":"set_settings","args":{"require_pairing_token":false}}`)
	if setResp.Error != nil {
		t.Fatalf("set_settings failed: %s", setResp.Error.Message)
	}

	// Verify it was persisted
	resp2 := runActionForTest(t, 3, `{"action":"get_settings","args":{}}`)
	encoded2, _ := json.Marshal(resp2.Result)
	var result2 map[string]interface{}
	if err := json.Unmarshal(encoded2, &result2); err != nil {
		t.Fatal(err)
	}
	if result2["require_pairing_token"] != false {
		t.Fatalf("expected require_pairing_token=false after update, got %v", result2["require_pairing_token"])
	}
}

func TestPairNodeRejectsMissingTokenWhenRequired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	// No token configured, require_pairing_token is true by default
	resp := runActionForTest(t, 1, `{"action":"pair_node","args":{"ip":"127.0.0.1:9321","name":"unauth-node","token":""}}`)
	if resp.Error == nil {
		t.Fatal("expected pair_node to fail when pairing token is required but empty")
	}
	if resp.Error.Code != -32002 {
		t.Fatalf("expected error code -32002, got %d (%s)", resp.Error.Code, resp.Error.Message)
	}
}

