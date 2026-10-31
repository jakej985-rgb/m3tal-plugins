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

func TestTelemetryEndpointRejectsRelayedClientAndMismatchedCertFingerprint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveNodeRuntimeConfig(nodeRuntimeConfig{PairingToken: "secret", ListenPort: 9321, BroadcastMDNS: true}); err != nil {
		t.Fatalf("save node config: %v", err)
	}

	// 1. Rejects challenge issued to a different client IP (attacker relay)
	server := httptest.NewServer(http.HandlerFunc(nodeTelemetryHandler))
	defer server.Close()
	nonceRelayed, err := issueNodeChallenge("198.51.100.23") // issued to external IP
	if err != nil {
		t.Fatal(err)
	}
	clientNonce := strings.Repeat("c", 64)
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.Header.Set("X-M3TAL-Nonce", nonceRelayed)
	req.Header.Set("X-M3TAL-Client-Nonce", clientNonce)
	req.Header.Set("X-M3TAL-Proof", nodeRequestProof("secret", nonceRelayed, clientNonce))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("relayed client challenge status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// 2. Over TLS, rejects proof with wrong certificate fingerprint
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(nodeTelemetryHandler))
	defer tlsServer.Close()
	nodeCertFingerprintMu.Lock()
	nodeCertFingerprint = "correct-node-fingerprint"
	nodeCertFingerprintMu.Unlock()

	nonceTLS, err := issueNodeChallenge("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	wrongProofReq, _ := http.NewRequest(http.MethodGet, tlsServer.URL+"/api/v1/telemetry", nil)
	wrongProofReq.Header.Set("X-M3TAL-Nonce", nonceTLS)
	wrongProofReq.Header.Set("X-M3TAL-Client-Nonce", clientNonce)
	wrongProofReq.Header.Set("X-M3TAL-Proof", nodeRequestProof("secret", nonceTLS, clientNonce, "wrong-attacker-fingerprint"))
	resp, err = tlsServer.Client().Do(wrongProofReq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("mismatched cert fingerprint proof status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// 3. Accepts matching certificate fingerprint over TLS
	nonceValid, err := issueNodeChallenge("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	validReq, _ := http.NewRequest(http.MethodGet, tlsServer.URL+"/api/v1/telemetry", nil)
	validReq.Header.Set("X-M3TAL-Nonce", nonceValid)
	validReq.Header.Set("X-M3TAL-Client-Nonce", clientNonce)
	validReq.Header.Set("X-M3TAL-Proof", nodeRequestProof("secret", nonceValid, clientNonce, "correct-node-fingerprint"))
	resp, err = tlsServer.Client().Do(validReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid cert fingerprint proof status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestTelemetryEndpointUnauthenticatedWhenRequirementDisabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaultFalse := false
	if err := saveNodeRuntimeConfig(nodeRuntimeConfig{PairingToken: "", RequirePairingToken: &defaultFalse, ListenPort: 9321, BroadcastMDNS: false}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(nodeTelemetryHandler))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/v1/telemetry")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 for tokenless endpoint with require_pairing_token=false, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload["hostname"] == "" || payload["telemetry"] == nil {
		t.Fatalf("missing node telemetry payload: %v", payload)
	}
}

func TestTelemetryEndpointRESTBearerAndOptionalPassword(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaultTrue := true
	cfg := nodeRuntimeConfig{
		PairingToken:        "my-secret-token",
		Password:            "my-secret-pass",
		RequirePairingToken: &defaultTrue,
		ListenPort:          9321,
	}
	if err := saveNodeRuntimeConfig(cfg); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(nodeTelemetryHandler))
	defer server.Close()

	// 1. Missing credentials -> 401
	resp, err := http.Get(server.URL + "/api/v1/telemetry")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %d", resp.StatusCode)
	}

	// 2. Token only, missing password -> 401
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing password, got %d", resp.StatusCode)
	}

	// 3. Token + wrong password -> 401
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	req.Header.Set("X-M3TAL-Password", "wrong-password")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", resp.StatusCode)
	}

	// 4. Token + correct password -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	req.Header.Set("X-M3TAL-Password", "my-secret-pass")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for valid token and password, got %d", resp.StatusCode)
	}

	// 5. Basic auth fallback -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.SetBasicAuth("my-secret-token", "my-secret-pass")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for valid BasicAuth, got %d", resp.StatusCode)
	}
}

func TestTelemetryEndpointHMACBranchWithPasswordAndRejectEmptyToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaultTrue := true

	// Scenario A: Token + Password configured.
	// HMAC proof alone without password must be rejected!
	cfg := nodeRuntimeConfig{
		PairingToken:        "hmac-secret-token",
		Password:            "node-secret-pass",
		RequirePairingToken: &defaultTrue,
		ListenPort:          9321,
	}
	if err := saveNodeRuntimeConfig(cfg); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/challenge" {
			nodeChallengeHandler(w, r)
			return
		}
		nodeTelemetryHandler(w, r)
	}))
	defer server.Close()

	// Get challenge nonce
	chResp, err := http.Post(server.URL+"/api/v1/challenge", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ch struct {
		Nonce string `json:"nonce"`
	}
	_ = json.NewDecoder(chResp.Body).Decode(&ch)
	chResp.Body.Close()

	cnonce := strings.Repeat("b", 64)
	proof := nodeRequestProof("hmac-secret-token", ch.Nonce, cnonce, "")

	// 1. HMAC proof with NO password -> 401
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.Header.Set("X-M3TAL-Nonce", ch.Nonce)
	req.Header.Set("X-M3TAL-Client-Nonce", cnonce)
	req.Header.Set("X-M3TAL-Proof", proof)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for HMAC proof without password, got %d", resp.StatusCode)
	}

	// Re-issue challenge for next attempt
	chResp, err = http.Post(server.URL+"/api/v1/challenge", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(chResp.Body).Decode(&ch)
	chResp.Body.Close()
	proof = nodeRequestProof("hmac-secret-token", ch.Nonce, cnonce, "")

	// 2. HMAC proof with correct password -> 200 OK
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.Header.Set("X-M3TAL-Nonce", ch.Nonce)
	req.Header.Set("X-M3TAL-Client-Nonce", cnonce)
	req.Header.Set("X-M3TAL-Proof", proof)
	req.Header.Set("X-M3TAL-Password", "node-secret-pass")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for HMAC proof with valid password, got %d", resp.StatusCode)
	}

	// Scenario B: Password-ONLY configured (empty token).
	// An HMAC proof with empty token MUST be rejected!
	cfgPassOnly := nodeRuntimeConfig{
		PairingToken:        "",
		Password:            "node-secret-pass",
		RequirePairingToken: &defaultTrue,
		ListenPort:          9321,
	}
	if err := saveNodeRuntimeConfig(cfgPassOnly); err != nil {
		t.Fatal(err)
	}

	chResp, err = http.Post(server.URL+"/api/v1/challenge", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.NewDecoder(chResp.Body).Decode(&ch)
	chResp.Body.Close()
	emptyProof := nodeRequestProof("", ch.Nonce, cnonce, "")

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.Header.Set("X-M3TAL-Nonce", ch.Nonce)
	req.Header.Set("X-M3TAL-Client-Nonce", cnonce)
	req.Header.Set("X-M3TAL-Proof", emptyProof)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 when attempting HMAC proof against password-only node, got %d", resp.StatusCode)
	}

	// But REST path with correct password on password-only node succeeds
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/v1/telemetry", nil)
	req.Header.Set("X-M3TAL-Password", "node-secret-pass")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for REST password on password-only node, got %d", resp.StatusCode)
	}
}

