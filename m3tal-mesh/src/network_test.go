package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestChallenge(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/v1/challenge" {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"nonce": strings.Repeat("a", 64)})
	return true
}

func testServerFingerprint(server *httptest.Server) string {
	if server != nil && server.TLS != nil && len(server.TLS.Certificates) > 0 && len(server.TLS.Certificates[0].Certificate) > 0 {
		sum := sha256.Sum256(server.TLS.Certificates[0].Certificate[0])
		return hex.EncodeToString(sum[:])
	}
	return ""
}

func testTLSCert(t *testing.T, commonName string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestFetchRemoteVerifiesAndReadsTelemetry(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		nonce := r.Header.Get("X-M3TAL-Nonce")
		clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
		fp := testServerFingerprint(server)
		if r.Header.Get("X-M3TAL-Proof") != requestProof("shared-secret", nonce, clientNonce, fp) {
			http.Error(w, "invalid pairing token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(remoteResponse{
			Name: "mesh-server", Hostname: "mesh-server",
			Telemetry: remoteTelemetry{Hostname: "mesh-server", Cores: 8, Load1m: 2, RAMTotalGB: 32, StorageTotalGB: 1000},
		})
		w.Header().Set("X-M3TAL-Proof", responseProof("shared-secret", nonce, clientNonce, body, fp))
		_, _ = w.Write(body)
	}))
	defer server.Close()

	node, err := fetchRemote(server.Listener.Addr().String(), "shared-secret", 9321)
	if err != nil {
		t.Fatalf("fetchRemote returned error: %v", err)
	}
	if node.Name != "mesh-server" || node.Cores != 8 || node.Status != "online" {
		t.Fatalf("unexpected remote node: %+v", node)
	}
	if node.Fingerprint != testServerFingerprint(server) {
		t.Fatalf("expected node fingerprint %s, got %s", testServerFingerprint(server), node.Fingerprint)
	}
}

func TestFetchRemoteRejectsUnauthenticatedTelemetry(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		_, _ = io.WriteString(w, `{"hostname":"spoofed","telemetry":{"cores":4}}`)
	}))
	defer server.Close()
	if _, err := fetchRemote(server.Listener.Addr().String(), "shared-secret", 9321); err == nil {
		t.Fatal("expected unauthenticated telemetry response to be rejected")
	}
}

func TestFetchRemoteRejectsWrongTokenAndIncompatiblePeer(t *testing.T) {
	unauthorized := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		http.Error(w, "invalid pairing token", http.StatusUnauthorized)
	}))
	defer unauthorized.Close()
	if _, err := fetchRemote(unauthorized.Listener.Addr().String(), "bad", 9321); err == nil || err.Error() != "pairing token was rejected by "+unauthorized.Listener.Addr().String() {
		t.Fatalf("expected token rejection, got %v", err)
	}

	incompatible := httptest.NewTLSServer(http.NotFoundHandler())
	defer incompatible.Close()
	if _, err := fetchRemote(incompatible.Listener.Addr().String(), "secret", 9321); err == nil {
		t.Fatal("expected incompatible peer to be rejected")
	}
}

func TestFetchRemoteRejectsEmptyTokenWhenRequired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Default config requires pairing token
	if _, err := fetchRemote("127.0.0.1:9321", "", 9321); err == nil {
		t.Fatal("expected fetchRemote to reject empty token when required")
	}
}

func TestRefreshRemotePeersAuthenticationRequiredWhenTokenMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaultTrue := true
	if err := saveMeshConfig(meshConfig{PairingToken: "", RequirePairingToken: &defaultTrue, ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}
	remoteNodes = []MeshNode{{ID: "node-test", Name: "Node Test", IP: "127.0.0.1:9321", Status: "online"}}
	refreshRemotePeers()
	peers := snapshotPeers()
	if len(peers) != 1 || peers[0].Status != "offline" || peers[0].LastSeen != "authentication required" {
		t.Fatalf("expected node to be marked offline with 'authentication required', got: %+v", peers)
	}
}

func TestPeerPersistenceAcrossRestart(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	peers := []MeshNode{{ID: "node-one", Name: "server", IP: "192.0.2.10:9321", Status: "online"}}
	if err := savePeers(peers); err != nil {
		t.Fatalf("savePeers returned error: %v", err)
	}
	reloaded := loadPeers()
	if len(reloaded) != 1 || reloaded[0].ID != peers[0].ID || reloaded[0].IP != peers[0].IP {
		t.Fatalf("peer state did not persist: %+v", reloaded)
	}
	info, err := os.Stat(peersPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("peer store permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestFailedPairDoesNotAddOrPersistPeer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}
	if _, err := pairAndPersistPeer("127.0.0.1:1", "unreachable", "server", "secret", ""); err == nil {
		t.Fatal("expected unreachable node to fail pairing")
	}
	if peers := snapshotPeers(); len(peers) != 0 {
		t.Fatalf("failed pairing added a peer: %+v", peers)
	}
	if _, err := os.Stat(peersPath()); !os.IsNotExist(err) {
		t.Fatalf("failed pairing should not create peer state, stat error = %v", err)
	}
}

func TestPairingDoesNotReplaceSharedToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveMeshConfig(meshConfig{PairingToken: "first-token", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}
	if _, err := pairAndPersistPeer("127.0.0.1:1", "node", "server", "second-token", ""); err == nil {
		t.Fatal("expected a conflicting shared token to be rejected")
	}
	if got := loadMeshConfig().PairingToken; got != "first-token" {
		t.Fatalf("pairing changed the configured token to %q", got)
	}
}

func TestPeerMemoryChangesOnlyAfterPersistence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	remoteNodes = []MeshNode{{ID: "existing", IP: "192.0.2.1:9321"}}
	path := peersPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := persistPeerUpsert(MeshNode{ID: "new", IP: "192.0.2.2:9321"}); err == nil {
		t.Fatal("expected peer persistence to fail")
	}
	if got := snapshotPeers(); len(got) != 1 || got[0].ID != "existing" {
		t.Fatalf("failed save mutated in-memory fleet: %+v", got)
	}
	if found, err := persistPeerRemoval("existing"); err == nil || found {
		t.Fatalf("expected failed removal save, got found=%v err=%v", found, err)
	}
	if got := snapshotPeers(); len(got) != 1 || got[0].ID != "existing" {
		t.Fatalf("failed removal mutated in-memory fleet: %+v", got)
	}
}

func TestRefreshPreservesMetadataFromConcurrentRepair(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveMeshConfig(meshConfig{PairingToken: "refresh-token", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		nonce := r.Header.Get("X-M3TAL-Nonce")
		clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
		fp := testServerFingerprint(server)
		if r.Header.Get("X-M3TAL-Proof") != requestProof("refresh-token", nonce, clientNonce, fp) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		close(started)
		<-release
		body, _ := json.Marshal(remoteResponse{Hostname: "refresh-test", Telemetry: remoteTelemetry{Hostname: "refresh-test", Cores: 2}})
		w.Header().Set("X-M3TAL-Proof", responseProof("refresh-token", nonce, clientNonce, body, fp))
		_, _ = w.Write(body)
	}))
	defer server.Close()

	remoteNodes = []MeshNode{{ID: "node-refresh", Name: "Old name", Role: "server", IP: server.Listener.Addr().String(), Fingerprint: testServerFingerprint(server)}}
	done := make(chan struct{})
	go func() {
		refreshRemotePeers()
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not reach the peer")
	}
	repaired := snapshotPeers()[0]
	repaired.Name, repaired.Role = "Updated name", "workstation"
	if err := persistPeerUpsert(repaired); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not finish")
	}
	peers := snapshotPeers()
	if len(peers) != 1 || peers[0].Name != "Updated name" || peers[0].Role != "workstation" {
		t.Fatalf("refresh overwrote repaired metadata: %+v", peers)
	}
}

func TestFetchRemoteRejectsReplayedResponseProof(t *testing.T) {
	capturedBody, _ := json.Marshal(remoteResponse{Hostname: "mesh-server", Telemetry: remoteTelemetry{Hostname: "mesh-server", Cores: 4}})
	capturedProof := responseProof("shared-secret", strings.Repeat("a", 64), strings.Repeat("b", 64), capturedBody)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-M3TAL-Proof", capturedProof)
		_, _ = w.Write(capturedBody)
	}))
	defer server.Close()
	if _, err := fetchRemote(server.Listener.Addr().String(), "shared-secret", 9321); err == nil {
		t.Fatal("expected a replayed response proof to be rejected")
	}
}

func TestDiscoveryConcurrentVerification(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveMeshConfig(meshConfig{PairingToken: "discovery-token", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}
	slowServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		time.Sleep(5 * time.Second)
		http.Error(w, "too slow", http.StatusRequestTimeout)
	}))
	defer slowServer.Close()
	var fastServer *httptest.Server
	fastServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		nonce := r.Header.Get("X-M3TAL-Nonce")
		clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
		fp := testServerFingerprint(fastServer)
		if r.Header.Get("X-M3TAL-Proof") != requestProof("discovery-token", nonce, clientNonce, fp) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := json.Marshal(remoteResponse{Hostname: "fast-node", Telemetry: remoteTelemetry{Hostname: "fast-node", Cores: 4}})
		w.Header().Set("X-M3TAL-Proof", responseProof("discovery-token", nonce, clientNonce, body, fp))
		_, _ = w.Write(body)
	}))
	defer fastServer.Close()

	candidates := []string{slowServer.Listener.Addr().String(), fastServer.Listener.Addr().String()}
	start := time.Now()
	discovered := verifyCandidates(candidates, "discovery-token", "", 9321)
	elapsed := time.Since(start)
	if len(discovered) != 1 || discovered[0].Name != "fast-node" {
		t.Fatalf("concurrent verification failed to find fast node: %+v", discovered)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("concurrent verification took too long: %v (expected < 5s)", elapsed)
	}
}

func TestFetchRemoteRejectsRelayedChallengeToDifferentServer(t *testing.T) {
	var legitServer *httptest.Server
	legitServer = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		nonce := r.Header.Get("X-M3TAL-Nonce")
		clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
		fp := testServerFingerprint(legitServer)
		if r.Header.Get("X-M3TAL-Proof") != requestProof("shared-secret", nonce, clientNonce, fp) {
			http.Error(w, "invalid pairing token", http.StatusUnauthorized)
			return
		}
		body, _ := json.Marshal(remoteResponse{Hostname: "legit", Telemetry: remoteTelemetry{Hostname: "legit", Cores: 4}})
		w.Header().Set("X-M3TAL-Proof", responseProof("shared-secret", nonce, clientNonce, body, fp))
		_, _ = w.Write(body)
	}))
	legitCert := testTLSCert(t, "legit-server")
	legitServer.TLS = &tls.Config{Certificates: []tls.Certificate{legitCert}}
	legitServer.StartTLS()
	defer legitServer.Close()

	attackerServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/challenge" {
			resp, err := legitServer.Client().Post(legitServer.URL+"/api/v1/challenge", "application/json", nil)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer resp.Body.Close()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.Copy(w, resp.Body)
			return
		}
		relayedReq, _ := http.NewRequest(http.MethodGet, legitServer.URL+"/api/v1/telemetry", nil)
		relayedReq.Header.Set("X-M3TAL-Nonce", r.Header.Get("X-M3TAL-Nonce"))
		relayedReq.Header.Set("X-M3TAL-Client-Nonce", r.Header.Get("X-M3TAL-Client-Nonce"))
		relayedReq.Header.Set("X-M3TAL-Proof", r.Header.Get("X-M3TAL-Proof"))
		resp, err := legitServer.Client().Do(relayedReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	attackerCert := testTLSCert(t, "attacker-server")
	attackerServer.TLS = &tls.Config{Certificates: []tls.Certificate{attackerCert}}
	attackerServer.StartTLS()
	defer attackerServer.Close()

	_, err := fetchRemote(attackerServer.Listener.Addr().String(), "shared-secret", 9321)
	if err == nil {
		t.Fatal("expected relayed challenge to different TLS server to be rejected")
	}
}

func TestFetchRemoteRESTTokenAndOptionalPassword(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	nodeMutex.Lock()
	remoteNodes = []MeshNode{}
	nodeMutex.Unlock()

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
			// Must receive HMAC proof (not raw Bearer token) and password
			if r.Header.Get("X-M3TAL-Proof") != requestProof("node-api-token", nonce, clientNonce, fp) || pass != "node-pass" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			body, _ := json.Marshal(remoteResponse{
				Name: "auth-server", Hostname: "auth-server",
				Telemetry: remoteTelemetry{Hostname: "auth-server", Cores: 16, RAMTotalGB: 64, StorageTotalGB: 2000},
			})
			w.Header().Set("X-M3TAL-Proof", responseProof("node-api-token", nonce, clientNonce, body, fp))
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// 1. Valid token and password via HMAC
	node, err := fetchRemoteWithAuth(server.Listener.Addr().String(), "node-api-token", "node-pass", 9321)
	if err != nil {
		t.Fatalf("fetchRemoteWithAuth failed: %v", err)
	}
	if node.Name != "auth-server" || node.Cores != 16 || !node.HasPassword || !node.HasAPIToken {
		t.Fatalf("unexpected node payload: %+v", node)
	}
	// Verify JSON marshaling does NOT expose token or password
	nodeJSON, _ := json.Marshal(node)
	if strings.Contains(string(nodeJSON), "node-api-token") || strings.Contains(string(nodeJSON), "node-pass") {
		t.Fatalf("MeshNode JSON serialization leaked secrets: %s", nodeJSON)
	}

	// 2. Wrong password -> error
	_, err = fetchRemoteWithAuth(server.Listener.Addr().String(), "node-api-token", "wrong-pass", 9321)
	if err == nil {
		t.Fatal("expected failure with wrong password")
	}

	// 3. Challenge failure must fail closed (not fallback to insecure request)
	badChallengeServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/challenge" {
			http.Error(w, "challenge failed", http.StatusInternalServerError)
			return
		}
		http.Error(w, "should not be reached", http.StatusOK)
	}))
	defer badChallengeServer.Close()
	_, err = fetchRemoteWithAuth(badChallengeServer.Listener.Addr().String(), "node-api-token", "node-pass", 9321)
	if err == nil {
		t.Fatal("expected fetchRemoteWithAuth to fail closed when challenge fails")
	}

	// 4. Password-only peer (token == "", password != "")
	passOnlyServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/challenge" {
			n, _ := newMeshNonce()
			_ = json.NewEncoder(w).Encode(map[string]string{"nonce": n})
			return
		}
		if r.URL.Path == "/api/v1/telemetry" {
			if r.Header.Get("X-M3TAL-Password") != "only-pass" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(remoteResponse{
				Name: "pass-only-server", Hostname: "pass-only-server",
				Telemetry: remoteTelemetry{Hostname: "pass-only-server", Cores: 4, RAMTotalGB: 16, StorageTotalGB: 500},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer passOnlyServer.Close()

	passNode, err := fetchRemoteWithAuth(passOnlyServer.Listener.Addr().String(), "", "only-pass", 9321)
	if err != nil {
		t.Fatalf("fetchRemoteWithAuth on password-only server failed: %v", err)
	}
	if passNode.Name != "pass-only-server" || !passNode.HasPassword || passNode.HasAPIToken {
		t.Fatalf("unexpected password-only node: %+v", passNode)
	}

	// 5. Pair and persist
	paired, err := pairAndPersistPeer(server.Listener.Addr().String(), "Auth Box", "nas", "node-api-token", "node-pass")
	if err != nil {
		t.Fatalf("pairAndPersistPeer failed: %v", err)
	}
	if paired.Name != "Auth Box" || paired.Role != "nas" || !paired.HasPassword {
		t.Fatalf("unexpected paired node: %+v", paired)
	}

	// 6. Verify peers persistence
	peers := loadPeers()
	if len(peers) != 1 || peers[0].Password != "node-pass" || peers[0].APIToken != "node-api-token" {
		t.Fatalf("expected persisted peer with credentials, got: %+v", peers)
	}
}

func TestPairingWithBlankCredentialsInheritsDefaultsAndFollowsRotatedToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	currentSharedToken := "initial-token"
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/telemetry" {
			nonce := r.Header.Get("X-M3TAL-Nonce")
			clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
			fp := testServerFingerprint(server)
			if r.Header.Get("X-M3TAL-Proof") != requestProof(currentSharedToken, nonce, clientNonce, fp) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			body, _ := json.Marshal(remoteResponse{
				Name: "default-peer", Hostname: "default-peer",
				Telemetry: remoteTelemetry{Hostname: "default-peer", Cores: 4, RAMTotalGB: 16, StorageTotalGB: 200},
			})
			w.Header().Set("X-M3TAL-Proof", responseProof(currentSharedToken, nonce, clientNonce, body, fp))
			_, _ = w.Write(body)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	if err := saveMeshConfig(meshConfig{PairingToken: "initial-token", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}

	// Pair with blank credentials
	addr := server.Listener.Addr().String()
	paired, err := pairAndPersistPeer(addr, "Default Peer", "worker", "", "")
	if err != nil {
		t.Fatalf("pairAndPersistPeer failed: %v", err)
	}
	if paired.APIToken != "" || paired.Password != "" || paired.HasAPIToken || paired.HasPassword {
		t.Fatalf("expected empty credential overrides on peer, got: %+v", paired)
	}

	// Rotate shared token on both mesh config and server
	currentSharedToken = "rotated-token"
	if err := saveMeshConfig(meshConfig{PairingToken: "rotated-token", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}

	// Refresh remote peers - should use the rotated default token
	refreshRemotePeers()
	peers := snapshotPeers()
	if len(peers) != 1 || peers[0].Status != "online" {
		t.Fatalf("expected peer to refresh successfully with rotated token, got: %+v", peers)
	}
}

func TestDiscoveryVerifyCandidatesUsesDefaultPassword(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		if r.URL.Path == "/api/v1/telemetry" {
			if r.Header.Get("X-M3TAL-Password") != "discovery-secret-pwd" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(remoteResponse{
				Name: "discovered-box", Hostname: "discovered-box",
				Telemetry: remoteTelemetry{Hostname: "discovered-box", Cores: 8, RAMTotalGB: 32, StorageTotalGB: 1000},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	addr := server.Listener.Addr().String()
	discovered := verifyCandidates([]string{addr}, "", "discovery-secret-pwd", 9321)
	if len(discovered) != 1 || discovered[0].Name != "discovered-box" {
		t.Fatalf("expected candidate to be verified using default password, got: %+v", discovered)
	}
}

func TestRePairingExistingIPUpdatesAndReplacesPeer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}

	// 1. Initial peer
	p1 := MeshNode{ID: "node-old-id", Name: "Old Hostname", IP: "192.168.1.100:9321", APIToken: "tok-1"}
	if err := persistPeerUpsert(p1); err != nil {
		t.Fatalf("persistPeerUpsert failed: %v", err)
	}

	// 2. Re-pairing same IP with new ID (hostname change) replaces the entry
	p1Renamed := MeshNode{ID: "node-new-id", Name: "Renamed Hostname", IP: "192.168.1.100:9321", APIToken: "tok-2"}
	if err := persistPeerUpsert(p1Renamed); err != nil {
		t.Fatalf("re-pairing with new ID failed: %v", err)
	}
	peers := snapshotPeers()
	if len(peers) != 1 || peers[0].ID != "node-new-id" || peers[0].Name != "Renamed Hostname" || peers[0].APIToken != "tok-2" {
		t.Fatalf("expected re-pairing to replace existing entry, got: %+v", peers)
	}

	// 3. Second peer
	p2 := MeshNode{ID: "node-2", Name: "Second Host", IP: "192.168.1.200:9321"}
	if err := persistPeerUpsert(p2); err != nil {
		t.Fatalf("persistPeerUpsert for second peer failed: %v", err)
	}

	// 4. Updating second peer to use first peer's IP must fail with duplicate IP error
	p2Conflicting := MeshNode{ID: "node-2", Name: "Second Host", IP: "192.168.1.100:9321"}
	if err := persistPeerUpsert(p2Conflicting); err == nil {
		t.Fatal("expected duplicate IP error when existing peer changes IP to another peer's IP")
	}

	// Verify fleet remains clean with 2 peers
	finalPeers := snapshotPeers()
	if len(finalPeers) != 2 || finalPeers[0].ID != "node-new-id" || finalPeers[1].ID != "node-2" {
		t.Fatalf("fleet corrupted after rejected duplicate IP: %+v", finalPeers)
	}
}



