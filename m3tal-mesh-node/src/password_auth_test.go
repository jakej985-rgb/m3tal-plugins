package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupPasswordNode points the real node config loader at a temp HOME with
// the given token/password and resets the password-attempt tracker.
func setupPasswordNode(t *testing.T, token, password string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("M3TAL_API_KEY", "")
	t.Setenv("M3TAL_API_TOKEN", "")
	t.Setenv("API_TOKEN", "")
	path := nodeConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(nodeRuntimeConfig{PairingToken: token, Password: password, ListenPort: 9321})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	nodePassMu.Lock()
	nodePassFails = make(map[string]*nodePassAttempt)
	nodePassMu.Unlock()
}

// doTelemetry issues a fresh challenge and calls the real nodeTelemetryHandler
// with token and/or password proofs, as the m3tal-mesh client does.
func doTelemetry(t *testing.T, ip, token, password string) *httptest.ResponseRecorder {
	t.Helper()
	nonce, err := issueNodeChallenge(ip)
	if err != nil {
		t.Fatal(err)
	}
	clientNonce := strings.Repeat("ab", 32)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
	req.RemoteAddr = ip + ":40000"
	req.Header.Set("X-M3TAL-Nonce", nonce)
	req.Header.Set("X-M3TAL-Client-Nonce", clientNonce)
	if token != "" {
		req.Header.Set("X-M3TAL-Proof", nodeRequestProof(token, nonce, clientNonce))
	}
	if password != "" {
		req.Header.Set("X-M3TAL-Password-Proof", nodePasswordProof(password, nonce, clientNonce))
	}
	rec := httptest.NewRecorder()
	nodeTelemetryHandler(rec, req)
	return rec
}

func TestDualAuthAcceptsTokenAndPasswordProofs(t *testing.T) {
	setupPasswordNode(t, "mesh-token", "secret")
	rec := doTelemetry(t, "10.0.0.1", "mesh-token", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("dual-auth: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-M3TAL-Proof") == "" {
		t.Fatal("missing token response proof")
	}
	if rec.Header().Get("X-M3TAL-Password-Auth-Proof") == "" {
		t.Fatal("missing password response proof")
	}
}

func TestPasswordOnlyLockoutAfterFailures(t *testing.T) {
	setupPasswordNode(t, "", "secret")
	for i := 0; i < nodePassMaxFails; i++ {
		if rec := doTelemetry(t, "10.0.0.2", "", "wrong"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i, rec.Code)
		}
	}
	if rec := doTelemetry(t, "10.0.0.2", "", "secret"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 while locked out, got %d", rec.Code)
	}
	if rec := doTelemetry(t, "10.0.0.3", "", "secret"); rec.Code != http.StatusOK {
		t.Fatalf("other IP should be unaffected, got %d", rec.Code)
	}
}

func TestPasswordLockoutExpiryResetsFailureCount(t *testing.T) {
	setupPasswordNode(t, "", "secret")
	for i := 0; i < nodePassMaxFails; i++ {
		doTelemetry(t, "10.0.0.4", "", "wrong")
	}
	nodePassMu.Lock()
	nodePassFails["10.0.0.4"].lockedTill = time.Now().Add(-time.Second)
	nodePassMu.Unlock()

	if rec := doTelemetry(t, "10.0.0.4", "", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after expiry, got %d", rec.Code)
	}
	if rec := doTelemetry(t, "10.0.0.4", "", "secret"); rec.Code != http.StatusOK {
		t.Fatalf("one failure after expiry must not relock; got %d", rec.Code)
	}
}

func TestInvalidChallengeDoesNotCountTowardLockout(t *testing.T) {
	setupPasswordNode(t, "", "secret")
	for i := 0; i < nodePassMaxFails*2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
		req.RemoteAddr = "10.0.0.5:40000"
		req.Header.Set("X-M3TAL-Nonce", strings.Repeat("00", 32))
		req.Header.Set("X-M3TAL-Client-Nonce", strings.Repeat("ab", 32))
		req.Header.Set("X-M3TAL-Password-Proof", "bogus")
		rec := httptest.NewRecorder()
		nodeTelemetryHandler(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for invalid challenge, got %d", rec.Code)
		}
	}
	if rec := doTelemetry(t, "10.0.0.5", "", "secret"); rec.Code != http.StatusOK {
		t.Fatalf("invalid challenges must not lock out the IP; got %d", rec.Code)
	}
}

func TestPasswordBeginBoundsConcurrentAttempts(t *testing.T) {
	setupPasswordNode(t, "", "secret")
	ip := "10.0.0.6"
	for i := 0; i < nodePassMaxFails; i++ {
		if !nodePasswordBegin(ip) {
			t.Fatalf("reservation %d unexpectedly refused", i)
		}
	}
	if nodePasswordBegin(ip) {
		t.Fatal("in-flight attempts exceeded the failure budget")
	}
	nodePasswordRelease(ip)
	if !nodePasswordBegin(ip) {
		t.Fatal("released slot should be reusable")
	}
}
