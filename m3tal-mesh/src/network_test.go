package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetchRemoteVerifiesAndReadsTelemetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer shared-secret" {
			http.Error(w, "invalid pairing token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(remoteResponse{
			Name: "mesh-server", Hostname: "mesh-server",
			Telemetry: remoteTelemetry{Hostname: "mesh-server", Cores: 8, Load1m: 2, RAMTotalGB: 32, StorageTotalGB: 1000},
		})
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

func TestFetchRemoteRejectsWrongTokenAndIncompatiblePeer(t *testing.T) {
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid pairing token", http.StatusUnauthorized)
	}))
	defer unauthorized.Close()
	if _, err := fetchRemote(unauthorized.Listener.Addr().String(), "bad", 9321); err == nil || err.Error() != "pairing token was rejected by "+unauthorized.Listener.Addr().String() {
		t.Fatalf("expected token rejection, got %v", err)
	}

	incompatible := httptest.NewServer(http.NotFoundHandler())
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
