package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestSetSettingsBlankTokenPreservesExisting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveMeshConfig(meshConfig{PairingToken: "existing-secret", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}

	resp := runActionForTest(t, 1, `{"action":"set_settings","args":{"pairing_token":"","listen_port":9500}}`)
	if resp.Error != nil {
		t.Fatalf("set_settings failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result["has_pairing_token"] != true {
		t.Fatalf("expected has_pairing_token=true, got %v", result["has_pairing_token"])
	}

	cfg := loadMeshConfig()
	if cfg.PairingToken != "existing-secret" {
		t.Fatalf("expected PairingToken to be preserved as 'existing-secret', got %q", cfg.PairingToken)
	}
	if cfg.ListenPort != 9500 {
		t.Fatalf("expected ListenPort=9500, got %d", cfg.ListenPort)
	}
}

func TestSetSettingsClearPairingToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveMeshConfig(meshConfig{PairingToken: "existing-secret", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}

	resp := runActionForTest(t, 1, `{"action":"set_settings","args":{"clear_pairing_token":true}}`)
	if resp.Error != nil {
		t.Fatalf("set_settings failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result["has_pairing_token"] != false {
		t.Fatalf("expected has_pairing_token=false after clear, got %v", result["has_pairing_token"])
	}

	cfg := loadMeshConfig()
	if cfg.PairingToken != "" {
		t.Fatalf("expected empty PairingToken, got %q", cfg.PairingToken)
	}
}

func TestSetSettingsReplacementPrioritizedOverClear(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveMeshConfig(meshConfig{PairingToken: "old-secret", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}

	resp := runActionForTest(t, 1, `{"action":"set_settings","args":{"pairing_token":"new-secret","clear_pairing_token":true}}`)
	if resp.Error != nil {
		t.Fatalf("set_settings failed: %s", resp.Error.Message)
	}
	cfg := loadMeshConfig()
	if cfg.PairingToken != "new-secret" {
		t.Fatalf("expected replacement token 'new-secret' to be saved, got %q", cfg.PairingToken)
	}
}

func TestDiscoveryEnabledSetting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	resp := runActionForTest(t, 1, `{"action":"get_settings","args":{}}`)
	if resp.Error != nil {
		t.Fatalf("get_settings failed: %s", resp.Error.Message)
	}
	encoded, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result["discovery_enabled"] != true {
		t.Fatalf("expected discovery_enabled=true by default, got %v", result["discovery_enabled"])
	}

	setResp := runActionForTest(t, 2, `{"action":"set_settings","args":{"discovery_enabled":false}}`)
	if setResp.Error != nil {
		t.Fatalf("set_settings failed: %s", setResp.Error.Message)
	}

	resp2 := runActionForTest(t, 3, `{"action":"get_settings","args":{}}`)
	encoded2, _ := json.Marshal(resp2.Result)
	var result2 map[string]interface{}
	if err := json.Unmarshal(encoded2, &result2); err != nil {
		t.Fatal(err)
	}
	if result2["discovery_enabled"] != false {
		t.Fatalf("expected discovery_enabled=false after update, got %v", result2["discovery_enabled"])
	}

	scanResp := runActionForTest(t, 4, `{"action":"scan_network","args":{}}`)
	if scanResp.Error != nil {
		t.Fatalf("scan_network failed: %s", scanResp.Error.Message)
	}
	encodedScan, _ := json.Marshal(scanResp.Result)
	var scanResult map[string]interface{}
	if err := json.Unmarshal(encodedScan, &scanResult); err != nil {
		t.Fatal(err)
	}
	if msg, ok := scanResult["message"].(string); !ok || !strings.Contains(msg, "disabled") {
		t.Fatalf("expected disabled message, got %v", scanResult["message"])
	}
}

func TestPairNodeSucceedsWithoutTokenWhenRequirementDisabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	defaultFalse := false
	if err := saveMeshConfig(meshConfig{PairingToken: "", RequirePairingToken: &defaultFalse, ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/telemetry" {
			w.Header().Set("Content-Type", "application/json")
			body, _ := json.Marshal(remoteResponse{Hostname: "tokenless-node", Telemetry: remoteTelemetry{Hostname: "tokenless-node", Cores: 8}})
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	pair := runActionForTest(t, 1, fmt.Sprintf(`{"action":"pair_node","args":{"ip":%q,"name":"tokenless","role":"server","token":""}}`, server.Listener.Addr().String()))
	if pair.Error != nil {
		t.Fatalf("expected tokenless pair to succeed when requirement is disabled, got error: %s", pair.Error.Message)
	}
	var pairResult struct {
		Node MeshNode `json:"node"`
	}
	encoded, _ := json.Marshal(pair.Result)
	if err := json.Unmarshal(encoded, &pairResult); err != nil || pairResult.Node.Name != "tokenless" {
		t.Fatalf("unexpected pair result: %s", encoded)
	}
}

func TestActionsTestNodeUpdateNodeAndGetPeers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/telemetry" {
			nonce := r.Header.Get("X-M3TAL-Nonce")
			clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
			fp := testServerFingerprint(server)
			pass := r.Header.Get("X-M3TAL-Password")
			if r.Header.Get("X-M3TAL-Proof") != requestProof("node-secret-tok", nonce, clientNonce, fp) || pass != "node-secret-pwd" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			body, _ := json.Marshal(remoteResponse{
				Name: "testbox", Hostname: "testbox",
				Telemetry: remoteTelemetry{Hostname: "testbox", Cores: 8, RAMTotalGB: 32, StorageTotalGB: 500},
			})
			w.Header().Set("X-M3TAL-Proof", responseProof("node-secret-tok", nonce, clientNonce, body, fp))
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()

	// 1. test_node with wrong password -> fails
	testFailResp := runActionForTest(t, 1, fmt.Sprintf(`{"action":"test_node","args":{"address":%q,"api_token":"node-secret-tok","password":"wrong"}}`, addr))
	if testFailResp.Error == nil {
		t.Fatal("expected test_node to fail with wrong password")
	}

	// 2. test_node with correct credentials using "address" arg -> succeeds
	testOkResp := runActionForTest(t, 2, fmt.Sprintf(`{"action":"test_node","args":{"address":%q,"api_token":"node-secret-tok","password":"node-secret-pwd"}}`, addr))
	if testOkResp.Error != nil {
		t.Fatalf("expected test_node to succeed, got error: %s", testOkResp.Error.Message)
	}

	// 3. pair_node with api_token and password
	pairResp := runActionForTest(t, 3, fmt.Sprintf(`{"action":"pair_node","args":{"ip":%q,"name":"Storage Box","role":"nas","api_token":"node-secret-tok","password":"node-secret-pwd"}}`, addr))
	if pairResp.Error != nil {
		t.Fatalf("pair_node failed: %s", pairResp.Error.Message)
	}
	var pairRes struct {
		Node struct {
			MeshNode
			HasPassword bool `json:"has_password"`
			HasAPIToken bool `json:"has_api_token"`
		} `json:"node"`
	}
	enc, _ := json.Marshal(pairResp.Result)
	_ = json.Unmarshal(enc, &pairRes)
	if pairRes.Node.Name != "Storage Box" || !pairRes.Node.HasPassword || !pairRes.Node.HasAPIToken {
		t.Fatalf("unexpected pair result: %+v", pairRes.Node)
	}
	// Verify plaintext credentials are NOT in pair_node response JSON
	if strings.Contains(string(enc), "node-secret-tok") || strings.Contains(string(enc), "node-secret-pwd") {
		t.Fatalf("pair_node exposed secrets in JSON response: %s", enc)
	}

	// 4. test_node using saved node_id credentials
	nodeID := pairRes.Node.ID
	testSavedResp := runActionForTest(t, 4, fmt.Sprintf(`{"action":"test_node","args":{"node_id":%q}}`, nodeID))
	if testSavedResp.Error != nil {
		t.Fatalf("expected test_node with saved node_id to succeed, got: %s", testSavedResp.Error.Message)
	}

	// 5. get_peers
	peersResp := runActionForTest(t, 5, `{"action":"get_peers","args":{}}`)
	if peersResp.Error != nil {
		t.Fatalf("get_peers failed: %s", peersResp.Error.Message)
	}
	encPeers, _ := json.Marshal(peersResp.Result)
	var peersRes struct {
		Peers []map[string]interface{} `json:"peers"`
	}
	_ = json.Unmarshal(encPeers, &peersRes)
	if len(peersRes.Peers) != 1 || peersRes.Peers[0]["has_password"] != true || peersRes.Peers[0]["has_api_token"] != true {
		t.Fatalf("unexpected get_peers result: %+v", peersRes.Peers)
	}
	if strings.Contains(string(encPeers), "node-secret-tok") || strings.Contains(string(encPeers), "node-secret-pwd") {
		t.Fatalf("get_peers exposed secrets in JSON response: %s", encPeers)
	}

	// 6. update_node
	updateResp := runActionForTest(t, 6, fmt.Sprintf(`{"action":"update_node","args":{"node_id":%q,"name":"Renamed NAS Box","role":"server"}}`, nodeID))
	if updateResp.Error != nil {
		t.Fatalf("update_node failed: %s", updateResp.Error.Message)
	}
	persisted := loadPeers()
	if len(persisted) != 1 || persisted[0].Name != "Renamed NAS Box" || persisted[0].Role != "server" || persisted[0].Password != "node-secret-pwd" {
		t.Fatalf("unexpected persisted peer after update: %+v", persisted)
	}

	// 7. update_node clear_password distinguishes clearing from omitted credentials
	clearResp := runActionForTest(t, 7, fmt.Sprintf(`{"action":"update_node","args":{"node_id":%q,"clear_password":true}}`, nodeID))
	if clearResp.Error != nil {
		t.Fatalf("update_node clear_password failed: %s", clearResp.Error.Message)
	}
	persistedAfterClear := loadPeers()
	if len(persistedAfterClear) != 1 || persistedAfterClear[0].Password != "" {
		t.Fatalf("expected password to be cleared, got: %+v", persistedAfterClear[0])
	}
	// APIToken was omitted, so it should still be preserved
	if persistedAfterClear[0].APIToken != "node-secret-tok" {
		t.Fatalf("expected omitted APIToken to be preserved, got: %s", persistedAfterClear[0].APIToken)
	}

	// 8. update_node with new IP clears Fingerprint
	if persistedAfterClear[0].Fingerprint == "" {
		t.Fatal("expected peer to have pinned fingerprint before IP change")
	}
	ipChangeResp := runActionForTest(t, 8, fmt.Sprintf(`{"action":"update_node","args":{"node_id":%q,"ip":"127.0.0.2:9999"}}`, nodeID))
	if ipChangeResp.Error != nil {
		t.Fatalf("update_node with new IP failed: %s", ipChangeResp.Error.Message)
	}
	persistedAfterIPChange := loadPeers()
	if persistedAfterIPChange[0].Fingerprint != "" {
		t.Fatalf("expected fingerprint to be cleared after IP change, got: %s", persistedAfterIPChange[0].Fingerprint)
	}

	// 9. update_node with an IP already belonging to another peer is rejected
	_ = persistPeerUpsert(MeshNode{ID: "second-peer-id", Name: "Second Node", IP: "127.0.0.3:9999"}, false, false)
	dupIPResp := runActionForTest(t, 9, fmt.Sprintf(`{"action":"update_node","args":{"node_id":"second-peer-id","ip":%q}}`, "127.0.0.2:9999"))
	if dupIPResp.Error == nil {
		t.Fatal("expected update_node with duplicate IP to fail")
	}
	// Verify first peer is not overwritten
	allPeers := snapshotPeers()
	if len(allPeers) != 2 || allPeers[0].ID != nodeID || allPeers[1].ID != "second-peer-id" {
		t.Fatalf("unexpected peers list after duplicate update rejection: %+v", allPeers)
	}
}

func TestUpdateNodeRenegotiatesPasswordOnlyWhenAddressOrCredentialsChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	// Global default token configured in mesh settings
	cfg := meshConfig{PairingToken: "default-mesh-token", ListenPort: 9321}
	if err := saveMeshConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// 1. Initial node is password-only
	initialNode := MeshNode{
		ID:           "target-node-id",
		Name:         "Target Box",
		IP:           "127.0.0.1:8888",
		Password:     "pass-secret",
		PasswordOnly: true,
	}
	if err := persistPeerUpsert(initialNode, false, false); err != nil {
		t.Fatal(err)
	}

	// 2. New address requires default-mesh-token + new-secret
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/telemetry" {
			nonce := r.Header.Get("X-M3TAL-Nonce")
			clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
			fp := testServerFingerprint(server)
			if r.Header.Get("X-M3TAL-Proof") != requestProof("default-mesh-token", nonce, clientNonce, fp) {
				http.Error(w, "invalid token proof", http.StatusUnauthorized)
				return
			}
			if r.Header.Get("X-M3TAL-Password") != "new-secret" {
				http.Error(w, "invalid password", http.StatusUnauthorized)
				return
			}
			body, _ := json.Marshal(remoteResponse{
				Name: "reconfigured-box", Hostname: "reconfigured-box",
				Telemetry: remoteTelemetry{Hostname: "reconfigured-box", Cores: 4, RAMTotalGB: 16, StorageTotalGB: 500},
			})
			w.Header().Set("X-M3TAL-Proof", responseProof("default-mesh-token", nonce, clientNonce, body, fp))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()

	// 3. update_node changing IP and entering new-secret, leaving token blank
	updateReq := fmt.Sprintf(`{"action":"update_node","args":{"node_id":"target-node-id","ip":%q,"password":"new-secret"}}`, addr)
	resp := runActionForTest(t, 20, updateReq)
	if resp.Error != nil {
		t.Fatalf("update_node failed: %s", resp.Error.Message)
	}

	// 4. Verify PasswordOnly is false on disk
	persisted := loadPeers()
	if len(persisted) != 1 {
		t.Fatalf("expected 1 persisted peer, got %d", len(persisted))
	}
	if persisted[0].PasswordOnly {
		t.Fatalf("expected PasswordOnly to be re-negotiated to false, got true")
	}

	// 5. Verify refresh succeeds and keeps the node online with inherited token
	refreshRemotePeers()
	refreshed := snapshotPeers()
	if len(refreshed) != 1 || refreshed[0].Status != "online" {
		t.Fatalf("expected node to remain online on refresh, got: %+v", refreshed)
	}
}

func TestUpdateNodePreservesPasswordOnlyWhenProbeFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	// Global default token configured in mesh settings
	cfg := meshConfig{PairingToken: "default-mesh-token", ListenPort: 9321}
	if err := saveMeshConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// 1. Initial node is password-only
	initialNode := MeshNode{
		ID:           "offline-target-id",
		Name:         "Offline Password Box",
		IP:           "127.0.0.1:8888",
		Password:     "old-secret",
		PasswordOnly: true,
	}
	if err := persistPeerUpsert(initialNode, false, false); err != nil {
		t.Fatal(err)
	}

	// 2. update_node changing password while target is unreachable (no server listening on 127.0.0.1:8888)
	updateReq := `{"action":"update_node","args":{"node_id":"offline-target-id","password":"updated-secret"}}`
	resp := runActionForTest(t, 21, updateReq)
	if resp.Error != nil {
		t.Fatalf("update_node failed: %s", resp.Error.Message)
	}

	// 3. Verify PasswordOnly remains true because probe could not establish a new mode
	persisted := loadPeers()
	if len(persisted) != 1 {
		t.Fatalf("expected 1 persisted peer, got %d", len(persisted))
	}
	if !persisted[0].PasswordOnly {
		t.Fatalf("expected PasswordOnly to be preserved as true on failed probe, got false")
	}
	if persisted[0].Password != "updated-secret" {
		t.Fatalf("expected password to be updated, got: %s", persisted[0].Password)
	}
}

func TestUpdateNodeRenegotiatesPasswordOnlyOnCredentialChangeAlone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	// Global default token configured in mesh settings
	cfg := meshConfig{PairingToken: "default-mesh-token", ListenPort: 9321}
	if err := saveMeshConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// Server requires default-mesh-token and updated-secret
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/telemetry" {
			nonce := r.Header.Get("X-M3TAL-Nonce")
			clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
			fp := testServerFingerprint(server)
			if r.Header.Get("X-M3TAL-Proof") != requestProof("default-mesh-token", nonce, clientNonce, fp) {
				http.Error(w, "invalid token proof", http.StatusUnauthorized)
				return
			}
			if r.Header.Get("X-M3TAL-Password") != "updated-secret" {
				http.Error(w, "invalid password", http.StatusUnauthorized)
				return
			}
			body, _ := json.Marshal(remoteResponse{
				Name: "reconfigured-box", Hostname: "reconfigured-box",
				Telemetry: remoteTelemetry{Hostname: "reconfigured-box", Cores: 4, RAMTotalGB: 16, StorageTotalGB: 500},
			})
			w.Header().Set("X-M3TAL-Proof", responseProof("default-mesh-token", nonce, clientNonce, body, fp))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()

	// Initial node already at addr, saved as password-only
	initialNode := MeshNode{
		ID:           "same-addr-id",
		Name:         "Same Addr Box",
		IP:           addr,
		Password:     "old-secret",
		PasswordOnly: true,
	}
	if err := persistPeerUpsert(initialNode, false, false); err != nil {
		t.Fatal(err)
	}

	// Update node changing only password, address is unchanged and token omitted
	updateReq := `{"action":"update_node","args":{"node_id":"same-addr-id","password":"updated-secret"}}`
	resp := runActionForTest(t, 22, updateReq)
	if resp.Error != nil {
		t.Fatalf("update_node failed: %s", resp.Error.Message)
	}

	// PasswordOnly should be renegotiated to false because server accepted default-mesh-token
	persisted := loadPeers()
	if len(persisted) != 1 {
		t.Fatalf("expected 1 persisted peer, got %d", len(persisted))
	}
	if persisted[0].PasswordOnly {
		t.Fatalf("expected PasswordOnly to be renegotiated to false on credential change, got true")
	}

	// Refresh should keep the node online
	refreshRemotePeers()
	refreshed := snapshotPeers()
	if len(refreshed) != 1 || refreshed[0].Status != "online" {
		t.Fatalf("expected node to remain online on refresh, got: %+v", refreshed)
	}
}

func TestTestNodeDoesNotLeakSavedCredentialsToDifferentAddress(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	// Global default token is DIFFERENT from the peer's custom per-node token
	cfg := meshConfig{PairingToken: "default-mesh-token", ListenPort: 9321}
	if err := saveMeshConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// 1. Initial saved peer with per-peer custom token and password
	initialNode := MeshNode{
		ID:       "saved-peer-1",
		Name:     "Saved Custom Peer",
		IP:       "127.0.0.1:8888",
		APIToken: "custom-peer-token",
		Password: "sensitive-peer-password",
	}
	if err := persistPeerUpsert(initialNode, false, false); err != nil {
		t.Fatal(err)
	}

	// 2. Untrusted server at new address checks credentials sent by client
	var receivedPassword string
	var receivedProof string
	var receivedNonce string
	var receivedClientNonce string
	var newServer *httptest.Server
	newServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/telemetry" {
			receivedPassword = r.Header.Get("X-M3TAL-Password")
			receivedProof = r.Header.Get("X-M3TAL-Proof")
			receivedNonce = r.Header.Get("X-M3TAL-Nonce")
			receivedClientNonce = r.Header.Get("X-M3TAL-Client-Nonce")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	defer newServer.Close()

	newAddr := newServer.Listener.Addr().String()

	// 3. test_node with new IP and node_id="saved-peer-1", leaving token and password empty
	testReq := fmt.Sprintf(`{"action":"test_node","args":{"node_id":"saved-peer-1","ip":%q}}`, newAddr)
	resp := runActionForTest(t, 22, testReq)

	// 4. Verify test_node returned an error (untrusted server rejected probe)
	if resp.Error == nil {
		t.Fatal("expected test_node to report error when probe fails")
	}

	// 5. Verify sensitive password was NOT sent to the untrusted new IP
	if receivedPassword != "" {
		t.Fatalf("security violation: test_node sent password to arbitrary new address: %q", receivedPassword)
	}

	// 6. Verify custom per-peer token was NOT sent (only mesh default token proof may be used)
	fp := testServerFingerprint(newServer)
	customProof := requestProof("custom-peer-token", receivedNonce, receivedClientNonce, fp)
	if receivedProof == customProof {
		t.Fatal("security violation: test_node sent saved per-peer custom token to arbitrary new address")
	}
	defaultProof := requestProof("default-mesh-token", receivedNonce, receivedClientNonce, fp)
	if receivedProof != defaultProof {
		t.Fatalf("expected test_node to fall back to default mesh token proof, got: %s", receivedProof)
	}
}

func TestUpdateNodePreservesCredentialsOnAddressChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	// 1. Initial saved peer with per-peer custom token, password, and pinned fingerprint
	initialNode := MeshNode{
		ID:          "peer-to-move",
		Name:        "Moveable Peer",
		IP:          "127.0.0.1:8888",
		APIToken:    "secret-token-to-keep",
		Password:    "secret-password-to-keep",
		Fingerprint: "saved-cert-fingerprint-9999",
	}
	if err := persistPeerUpsert(initialNode, false, false); err != nil {
		t.Fatal(err)
	}

	// 2. update_node changing IP only without supplying credentials
	updateReq := `{"action":"update_node","args":{"node_id":"peer-to-move","ip":"127.0.0.2:9999"}}`
	resp := runActionForTest(t, 23, updateReq)
	if resp.Error != nil {
		t.Fatalf("update_node failed: %s", resp.Error.Message)
	}

	// 3. Verify credentials are preserved while fingerprint is unpinned on address change
	persisted := loadPeers()
	if len(persisted) != 1 {
		t.Fatalf("expected 1 persisted peer, got %d", len(persisted))
	}
	if persisted[0].APIToken != "secret-token-to-keep" {
		t.Fatalf("expected APIToken to be preserved on address change, got: %s", persisted[0].APIToken)
	}
	if persisted[0].Password != "secret-password-to-keep" {
		t.Fatalf("expected Password to be preserved on address change, got: %s", persisted[0].Password)
	}
	if persisted[0].Fingerprint != "" {
		t.Fatalf("expected Fingerprint to be unpinned on address change, got: %s", persisted[0].Fingerprint)
	}
}

func TestTestNodePreservesPasswordOnlyModeForUnchangedSavedAddress(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	// Server accepts default password, rejects pairing token proof
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/telemetry" {
			if r.Header.Get("X-M3TAL-Proof") != "" {
				http.Error(w, "token rejected", http.StatusUnauthorized)
				return
			}
			if r.Header.Get("X-M3TAL-Password") != "mesh-default-secret" {
				http.Error(w, "invalid password", http.StatusUnauthorized)
				return
			}
			body, _ := json.Marshal(remoteResponse{
				Name: "pw-only-saved-node", Hostname: "pw-only-saved-node",
				Telemetry: remoteTelemetry{Hostname: "pw-only-saved-node", Cores: 4, RAMTotalGB: 16, StorageTotalGB: 500},
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()
	fp := testServerFingerprint(server)

	// Peer originally saved in PasswordOnly mode with password configured on peer
	savedPeer := MeshNode{
		ID:           "saved-pw-node-1",
		Name:         "pw-only-saved-node",
		IP:           addr,
		Fingerprint:  fp,
		PasswordOnly: true,
		APIToken:     "",
		Password:     "mesh-default-secret",
	}
	if err := persistPeerUpsert(savedPeer, false, false); err != nil {
		t.Fatal(err)
	}

	// Now a default token is configured in mesh config
	cfg := meshConfig{PairingToken: "new-mesh-token", DefaultPassword: "mesh-default-secret", ListenPort: 9321}
	if err := saveMeshConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// test_node for the unchanged saved address inherits saved password and mode
	testReq := fmt.Sprintf(`{"action":"test_node","args":{"node_id":"saved-pw-node-1","ip":%q}}`, addr)
	resp := runActionForTest(t, 24, testReq)
	if resp.Error != nil {
		t.Fatalf("test_node failed to preserve PasswordOnly mode for unchanged address: %s", resp.Error.Message)
	}
	resMap, ok := resp.Result.(map[string]interface{})
	if !ok || resMap["success"] != true {
		t.Fatalf("expected successful test_node response, got: %+v", resp.Result)
	}

	// A password-only peer with blank password cannot inherit DefaultPassword and fails test_node
	blankPwPeer := MeshNode{
		ID:           "saved-blank-pw-node",
		Name:         "pw-blank-node",
		IP:           addr,
		Fingerprint:  fp,
		PasswordOnly: true,
		APIToken:     "",
		Password:     "",
	}
	if err := persistPeerUpsert(blankPwPeer, false, false); err != nil {
		t.Fatal(err)
	}
	blankTestReq := fmt.Sprintf(`{"action":"test_node","args":{"node_id":"saved-blank-pw-node","ip":%q}}`, addr)
	blankResp := runActionForTest(t, 25, blankTestReq)
	if blankResp.Error == nil {
		t.Fatal("expected test_node to fail for password-only peer with blank password")
	}
}

func TestTestNodeRenegotiatesPasswordOnlyWhenCredentialsChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/telemetry" {
			nonce := r.Header.Get("X-M3TAL-Nonce")
			clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
			fp := testServerFingerprint(server)
			// Reconfigured server requires default-mesh-token + new-secret
			if r.Header.Get("X-M3TAL-Proof") != requestProof("default-mesh-token", nonce, clientNonce, fp) {
				http.Error(w, "missing or invalid token proof", http.StatusUnauthorized)
				return
			}
			if r.Header.Get("X-M3TAL-Password") != "new-secret" {
				http.Error(w, "invalid password", http.StatusUnauthorized)
				return
			}
			body, _ := json.Marshal(remoteResponse{
				Name: "reconfigured-box", Hostname: "reconfigured-box",
				Telemetry: remoteTelemetry{Hostname: "reconfigured-box", Cores: 4, RAMTotalGB: 16, StorageTotalGB: 500},
			})
			w.Header().Set("X-M3TAL-Proof", responseProof("default-mesh-token", nonce, clientNonce, body, fp))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()
	fp := testServerFingerprint(server)

	// Peer originally saved in PasswordOnly mode
	savedPeer := MeshNode{
		ID:           "saved-reconfigured-node",
		Name:         "saved-node",
		IP:           addr,
		Fingerprint:  fp,
		PasswordOnly: true,
		Password:     "old-secret",
	}
	if err := persistPeerUpsert(savedPeer, false, false); err != nil {
		t.Fatal(err)
	}

	// Default token configured in mesh settings
	cfg := meshConfig{PairingToken: "default-mesh-token", ListenPort: 9321}
	if err := saveMeshConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// 1. test_node with unchanged credentials preserves PasswordOnly=true and fails because token is suppressed
	unchangedReq := fmt.Sprintf(`{"action":"test_node","args":{"node_id":"saved-reconfigured-node","ip":%q}}`, addr)
	unchangedResp := runActionForTest(t, 26, unchangedReq)
	if unchangedResp.Error == nil {
		t.Fatal("expected test_node with unchanged credentials to fail due to token suppression")
	}

	// 2. test_node with updated password in edit modal renegotiates mode, inherits default token, and succeeds
	changedReq := fmt.Sprintf(`{"action":"test_node","args":{"node_id":"saved-reconfigured-node","ip":%q,"password":"new-secret"}}`, addr)
	changedResp := runActionForTest(t, 27, changedReq)
	if changedResp.Error != nil {
		t.Fatalf("expected test_node with changed credentials to renegotiate mode and succeed, got error: %s", changedResp.Error.Message)
	}
	resMap, ok := changedResp.Result.(map[string]interface{})
	if !ok || resMap["success"] != true {
		t.Fatalf("expected successful test_node response, got: %+v", changedResp.Result)
	}
}

func TestUpdateNodeAddressNormalizationPreservesCredentialsAndPin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}
	cfg := meshConfig{PairingToken: "default-mesh-token", ListenPort: 9321}
	if err := saveMeshConfig(cfg); err != nil {
		t.Fatal(err)
	}

	initialNode := MeshNode{
		ID:          "normalized-peer",
		Name:        "Normalized Peer",
		IP:          "127.0.0.1:9321",
		APIToken:    "custom-tok-123",
		Password:    "custom-pass-456",
		Fingerprint: "saved-cert-fingerprint-norm",
	}
	if err := persistPeerUpsert(initialNode, false, false); err != nil {
		t.Fatal(err)
	}

	// Update node from "127.0.0.1:9321" to "127.0.0.1" (same endpoint under default port 9321)
	updateReq := `{"action":"update_node","args":{"node_id":"normalized-peer","ip":"127.0.0.1"}}`
	resp := runActionForTest(t, 25, updateReq)
	if resp.Error != nil {
		t.Fatalf("update_node failed: %s", resp.Error.Message)
	}

	persisted := loadPeers()
	if len(persisted) != 1 {
		t.Fatalf("expected 1 persisted peer, got %d", len(persisted))
	}
	if persisted[0].APIToken != "custom-tok-123" {
		t.Fatalf("expected APIToken to be preserved on normalized address, got: %s", persisted[0].APIToken)
	}
	if persisted[0].Password != "custom-pass-456" {
		t.Fatalf("expected Password to be preserved on normalized address, got: %s", persisted[0].Password)
	}
	if persisted[0].Fingerprint != "saved-cert-fingerprint-norm" {
		t.Fatalf("expected Fingerprint to be preserved on normalized address, got: %s", persisted[0].Fingerprint)
	}
}

func TestMeshInheritsCoreAPIKey(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Case 1: Core API key provided via environment variable
	t.Setenv("M3TAL_API_KEY", "env-core-secret-key-32chars!!")
	cfg := loadMeshConfig()
	if cfg.PairingToken != "env-core-secret-key-32chars!!" {
		t.Fatalf("expected PairingToken to inherit M3TAL_API_KEY, got %q", cfg.PairingToken)
	}

	// Case 2: Core API key written to ~/.config/m3tal/api_keys.json (Core API Key Manager file)
	t.Setenv("M3TAL_API_KEY", "")
	coreDir := filepath.Join(tempHome, ".config", "m3tal")
	_ = os.MkdirAll(coreDir, 0700)
	keyData := []byte(`{"primary_token": "jellyfin-style-generated-key-123"}`)
	_ = os.WriteFile(filepath.Join(coreDir, "api_keys.json"), keyData, 0600)

	cfg2 := loadMeshConfig()
	if cfg2.PairingToken != "jellyfin-style-generated-key-123" {
		t.Fatalf("expected PairingToken to inherit from api_keys.json, got %q", cfg2.PairingToken)
	}

	// Verify get_settings reports using_core_key: true
	resp := runActionForTest(t, 99, `{"action":"get_settings"}`)
	if resp.Error != nil {
		t.Fatalf("get_settings error: %v", resp.Error)
	}
	raw, _ := json.Marshal(resp.Result)
	var settingsMap map[string]interface{}
	_ = json.Unmarshal(raw, &settingsMap)
	if settingsMap["using_core_key"] != true {
		t.Errorf("expected using_core_key=true, got %v", settingsMap["using_core_key"])
	}
}

