package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

	nonce := fmt.Sprintf("%064x", 1)
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.Header.Set("X-M3TAL-Nonce", nonce)
	req.Header.Set("X-M3TAL-Proof", nodeRequestProof("mesh-secret", nonce))
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
	if resp.Header.Get("X-M3TAL-Proof") != nodeResponseProof("mesh-secret", nonce, body) {
		t.Fatal("authenticated response proof is missing or invalid")
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
