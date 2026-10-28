package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelemetryEndpointRequiresConfiguredToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveNodeRuntimeConfig(nodeRuntimeConfig{PairingToken: "mesh-secret", ListenPort: 9321, BroadcastMDNS: true}); err != nil {
		t.Fatalf("save node config: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(nodeTelemetryHandler))
	defer server.Close()

	unauthorized, err := http.Get(server.URL + "/api/v1/telemetry")
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized request status = %d, want %d", unauthorized.StatusCode, http.StatusUnauthorized)
	}

	nonce, err := issueNodeChallenge("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	clientNonce := strings.Repeat("b", 64)
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.Header.Set("X-M3TAL-Nonce", nonce)
	req.Header.Set("X-M3TAL-Client-Nonce", clientNonce)
	req.Header.Set("X-M3TAL-Proof", nodeRequestProof("mesh-secret", nonce, clientNonce))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authenticated request status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["hostname"] == "" || payload["telemetry"] == nil {
		t.Fatalf("missing node identity or telemetry: %#v", payload)
	}
	if resp.Header.Get("X-M3TAL-Proof") != nodeResponseProof("mesh-secret", nonce, clientNonce, body) {
		t.Fatal("authenticated response proof is missing or invalid")
	}

	replay, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	replay.Header.Set("X-M3TAL-Nonce", nonce)
	replay.Header.Set("X-M3TAL-Client-Nonce", clientNonce)
	replay.Header.Set("X-M3TAL-Proof", nodeRequestProof("mesh-secret", nonce, clientNonce))
	replayResp, err := http.DefaultClient.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer replayResp.Body.Close()
	if replayResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replayed challenge status = %d, want %d", replayResp.StatusCode, http.StatusUnauthorized)
	}

	nonce, err = issueNodeChallenge("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	missingClient, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	missingClient.Header.Set("X-M3TAL-Nonce", nonce)
	missingClient.Header.Set("X-M3TAL-Proof", nodeRequestProof("mesh-secret", nonce, clientNonce))
	missingResp, err := http.DefaultClient.Do(missingClient)
	if err != nil {
		t.Fatal(err)
	}
	defer missingResp.Body.Close()
	if missingResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing client nonce status = %d, want %d", missingResp.StatusCode, http.StatusUnauthorized)
	}
}

func TestTelemetryEndpointDisabledUntilTokenConfigured(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(nodeTelemetryHandler))
	defer server.Close()
	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured endpoint status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}
