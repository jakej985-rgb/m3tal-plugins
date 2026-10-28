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
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		nonce := r.Header.Get("X-M3TAL-Nonce")
		clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
		if r.Header.Get("X-M3TAL-Proof") != requestProof("test-secret", nonce, clientNonce) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := json.Marshal(remoteResponse{Hostname: "action-test", Telemetry: remoteTelemetry{Hostname: "action-test", Cores: 4}})
		w.Header().Set("X-M3TAL-Proof", responseProof("test-secret", nonce, clientNonce, body))
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
