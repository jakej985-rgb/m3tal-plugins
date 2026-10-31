package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestTestNodeReusesSavedCredentialsWhenChangingAddress(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	// Global default token is DIFFERENT from the peer's custom per-node token
	cfg := meshConfig{PairingToken: "default-mesh-token", ListenPort: 9321}
	if err := saveMeshConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// 1. Initial saved peer with per-peer custom token
	initialNode := MeshNode{
		ID:       "saved-peer-1",
		Name:     "Saved Custom Peer",
		IP:       "127.0.0.1:8888",
		APIToken: "custom-peer-token",
		Password: "custom-peer-pass",
	}
	if err := persistPeerUpsert(initialNode, false, false); err != nil {
		t.Fatal(err)
	}

	// 2. Server at new address accepts custom-peer-token and custom-peer-pass
	var newServer *httptest.Server
	newServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/telemetry" {
			nonce := r.Header.Get("X-M3TAL-Nonce")
			clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
			fp := testServerFingerprint(newServer)
			// Must match the custom-peer-token, not the default-mesh-token!
			if r.Header.Get("X-M3TAL-Proof") != requestProof("custom-peer-token", nonce, clientNonce, fp) {
				http.Error(w, "invalid custom token", http.StatusUnauthorized)
				return
			}
			if r.Header.Get("X-M3TAL-Password") != "custom-peer-pass" {
				http.Error(w, "invalid custom pass", http.StatusUnauthorized)
				return
			}
			body, _ := json.Marshal(remoteResponse{
				Name: "moved-peer", Hostname: "moved-peer",
				Telemetry: remoteTelemetry{Hostname: "moved-peer", Cores: 4, RAMTotalGB: 16, StorageTotalGB: 500},
			})
			w.Header().Set("X-M3TAL-Proof", responseProof("custom-peer-token", nonce, clientNonce, body, fp))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	defer newServer.Close()

	newAddr := newServer.Listener.Addr().String()

	// 3. test_node with new IP and node_id="saved-peer-1", leaving token and password empty
	testReq := fmt.Sprintf(`{"action":"test_node","args":{"node_id":"saved-peer-1","ip":%q}}`, newAddr)
	resp := runActionForTest(t, 22, testReq)
	if resp.Error != nil {
		t.Fatalf("test_node failed to reuse saved credentials for moved peer: %s", resp.Error.Message)
	}
	resMap, _ := resp.Result.(map[string]interface{})
	if resMap["hostname"] != "moved-peer" {
		t.Fatalf("unexpected hostname from test_node: %v", resMap["hostname"])
	}
}
