package main

import (
	"encoding/json"
	"io"
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

func TestFetchRemoteVerifiesAndReadsTelemetry(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		nonce := r.Header.Get("X-M3TAL-Nonce")
		clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
		if r.Header.Get("X-M3TAL-Proof") != requestProof("shared-secret", nonce, clientNonce) {
			http.Error(w, "invalid pairing token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(remoteResponse{
			Name: "mesh-server", Hostname: "mesh-server",
			Telemetry: remoteTelemetry{Hostname: "mesh-server", Cores: 8, Load1m: 2, RAMTotalGB: 32, StorageTotalGB: 1000},
		})
		w.Header().Set("X-M3TAL-Proof", responseProof("shared-secret", nonce, clientNonce, body))
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
	if _, err := pairAndPersistPeer("127.0.0.1:1", "unreachable", "server", "secret"); err == nil {
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
	if _, err := pairAndPersistPeer("127.0.0.1:1", "node", "server", "second-token"); err == nil {
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
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		nonce := r.Header.Get("X-M3TAL-Nonce")
		clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
		if r.Header.Get("X-M3TAL-Proof") != requestProof("refresh-token", nonce, clientNonce) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		close(started)
		<-release
		body, _ := json.Marshal(remoteResponse{Hostname: "refresh-test", Telemetry: remoteTelemetry{Hostname: "refresh-test", Cores: 2}})
		w.Header().Set("X-M3TAL-Proof", responseProof("refresh-token", nonce, clientNonce, body))
		_, _ = w.Write(body)
	}))
	defer server.Close()

	remoteNodes = []MeshNode{{ID: "node-refresh", Name: "Old name", Role: "server", IP: server.Listener.Addr().String()}}
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
	fastServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if writeTestChallenge(w, r) {
			return
		}
		nonce := r.Header.Get("X-M3TAL-Nonce")
		clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
		if r.Header.Get("X-M3TAL-Proof") != requestProof("discovery-token", nonce, clientNonce) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := json.Marshal(remoteResponse{Hostname: "fast-node", Telemetry: remoteTelemetry{Hostname: "fast-node", Cores: 4}})
		w.Header().Set("X-M3TAL-Proof", responseProof("discovery-token", nonce, clientNonce, body))
		_, _ = w.Write(body)
	}))
	defer fastServer.Close()

	candidates := []string{slowServer.Listener.Addr().String(), fastServer.Listener.Addr().String()}
	start := time.Now()
	discovered := verifyCandidates(candidates, "discovery-token", 9321)
	elapsed := time.Since(start)
	if len(discovered) != 1 || discovered[0].Name != "fast-node" {
		t.Fatalf("concurrent verification failed to find fast node: %+v", discovered)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("concurrent verification took too long: %v (expected < 5s)", elapsed)
	}
}
