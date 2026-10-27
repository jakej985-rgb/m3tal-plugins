package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchRemoteVerifiesAndReadsTelemetry(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := r.Header.Get("X-M3TAL-Nonce")
		if r.Header.Get("X-M3TAL-Proof") != requestProof("shared-secret", nonce) {
			http.Error(w, "invalid pairing token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(remoteResponse{
			Name: "mesh-server", Hostname: "mesh-server",
			Telemetry: remoteTelemetry{Hostname: "mesh-server", Cores: 8, Load1m: 2, RAMTotalGB: 32, StorageTotalGB: 1000},
		})
		w.Header().Set("X-M3TAL-Proof", responseProof("shared-secret", nonce, body))
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
		_, _ = io.WriteString(w, `{"hostname":"spoofed","telemetry":{"cores":4}}`)
	}))
	defer server.Close()
	if _, err := fetchRemote(server.Listener.Addr().String(), "shared-secret", 9321); err == nil {
		t.Fatal("expected unauthenticated telemetry response to be rejected")
	}
}

func TestFetchRemoteRejectsWrongTokenAndIncompatiblePeer(t *testing.T) {
	unauthorized := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestPairThenForgetPeerPersistsFleetChanges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	remoteNodes = []MeshNode{}
	if err := saveMeshConfig(meshConfig{PairingToken: "pair-token", ListenPort: 9321}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := r.Header.Get("X-M3TAL-Nonce")
		if r.Header.Get("X-M3TAL-Proof") != requestProof("pair-token", nonce) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := json.Marshal(remoteResponse{Hostname: "pair-test", Telemetry: remoteTelemetry{Hostname: "pair-test", Cores: 2}})
		w.Header().Set("X-M3TAL-Proof", responseProof("pair-token", nonce, body))
		_, _ = w.Write(body)
	}))
	defer server.Close()

	peer, err := pairAndPersistPeer(server.Listener.Addr().String(), "test peer", "server", "")
	if err != nil {
		t.Fatalf("pair node: %v", err)
	}
	if len(loadPeers()) != 1 || len(snapshotPeers()) != 1 {
		t.Fatal("paired peer was not persisted and published")
	}
	found, err := persistPeerRemoval(peer.ID)
	if err != nil || !found {
		t.Fatalf("forget node: found=%v err=%v", found, err)
	}
	if len(loadPeers()) != 0 || len(snapshotPeers()) != 0 {
		t.Fatal("forgotten peer remains in saved or in-memory fleet")
	}
}
