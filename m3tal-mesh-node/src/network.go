package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const meshServiceType = "_m3tal-mesh-node._tcp"

type nodeRuntimeConfig struct {
	PairingToken        string `json:"pairing_token"`
	RequirePairingToken *bool  `json:"require_pairing_token,omitempty"`
	ListenPort          int    `json:"listen_port"`
	BroadcastMDNS       bool   `json:"broadcast_mdns"`
}

func (c nodeRuntimeConfig) IsPairingTokenRequired() bool {
	if c.RequirePairingToken == nil {
		return true
	}
	return *c.RequirePairingToken
}

func nodeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "m3tal", "plugins", "mesh-node", "config.json")
}

func saveNodeRuntimeConfig(cfg nodeRuntimeConfig) error {
	path := nodeConfigPath()
	if path == "" {
		return fmt.Errorf("cannot locate the user configuration directory")
	}
	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		return fmt.Errorf("telemetry port must be between 1 and 65535")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadNodeRuntimeConfig() nodeRuntimeConfig {
	cfg := nodeRuntimeConfig{
		ListenPort:    9321,
		BroadcastMDNS: true,
	}
	path := nodeConfigPath()
	if path == "" {
		return cfg
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(b, &cfg)
	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		cfg.ListenPort = 9321
	}
	return cfg
}

func startNodeNetworkService() error {
	cfg := loadNodeRuntimeConfig()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", nodeTelemetryHandler)
	mux.HandleFunc("/api/v1/challenge", nodeChallengeHandler)
	mux.HandleFunc("/api/v1/telemetry", nodeTelemetryHandler)

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.ListenPort),
		Handler:           mux,
		ReadHeaderTimeout: 4 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       45 * time.Second,
		MaxHeaderBytes:    32 * 1024,
	}
	certificate, err := createNodeCertificate()
	if err != nil {
		return fmt.Errorf("mesh-node TLS certificate: %w", err)
	}
	listener, err := tls.Listen("tcp", server.Addr, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("mesh-node HTTP service: %w", err)
	}
	nodeServerMu.Lock()
	nodeServer = server
	nodeServerMu.Unlock()
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "mesh-node HTTP service: %v\n", err)
		}
	}()

	if cfg.BroadcastMDNS {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "m3tal-mesh-node"
		}
		mdnsServer, err := zeroconf.Register(hostname, meshServiceType, "local.", cfg.ListenPort, []string{"version=" + PluginVersion, "protocol=1"}, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mesh-node mDNS registration: %v\n", err)
		} else {
			nodeServerMu.Lock()
			nodeMDNSServer = mdnsServer
			nodeServerMu.Unlock()
		}
	}
	return nil
}

var (
	nodeCertFingerprintMu sync.RWMutex
	nodeCertFingerprint   string
)

func createNodeCertificate() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "m3tal-mesh-node"
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hostname},
		DNSNames:     []string{hostname},
		NotBefore:    now.Add(-time.Minute), NotAfter: now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	sum := sha256.Sum256(der)
	nodeCertFingerprintMu.Lock()
	nodeCertFingerprint = hex.EncodeToString(sum[:])
	nodeCertFingerprintMu.Unlock()
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

func nodeTelemetryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	current := loadNodeRuntimeConfig()
	if current.IsPairingTokenRequired() && current.PairingToken == "" {
		http.Error(w, "pairing token is not configured on this node", http.StatusServiceUnavailable)
		return
	}
	var nonce, clientNonce, fp string
	if current.PairingToken != "" {
		nonce = r.Header.Get("X-M3TAL-Nonce")
		clientNonce = r.Header.Get("X-M3TAL-Client-Nonce")
		supplied := r.Header.Get("X-M3TAL-Proof")

		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			clientIP = r.RemoteAddr
		}

		if !validNodeNonce(clientNonce) || !consumeNodeChallenge(nonce, clientIP) {
			http.Error(w, "invalid pairing challenge", http.StatusUnauthorized)
			return
		}

		if r.TLS != nil {
			nodeCertFingerprintMu.RLock()
			fp = nodeCertFingerprint
			nodeCertFingerprintMu.RUnlock()
		}

		if subtle.ConstantTimeCompare([]byte(supplied), []byte(nodeRequestProof(current.PairingToken, nonce, clientNonce, fp))) != 1 {
			http.Error(w, "invalid pairing token", http.StatusUnauthorized)
			return
		}
	}

	hostname, _ := os.Hostname()
	body, err := json.Marshal(map[string]any{
		"name": hostname, "hostname": hostname, "version": PluginVersion,
		"telemetry": getTelemetry(),
	})
	if err != nil {
		http.Error(w, "could not encode telemetry", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if current.PairingToken != "" {
		w.Header().Set("X-M3TAL-Proof", nodeResponseProof(current.PairingToken, nonce, clientNonce, body, fp))
	}
	_, _ = w.Write(body)
}

func nodeChallengeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		clientIP = r.RemoteAddr
	}
	nonce, err := issueNodeChallenge(clientIP)
	if err != nil {
		http.Error(w, "could not create challenge", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]string{"nonce": nonce}
	nodeCertFingerprintMu.RLock()
	fp := nodeCertFingerprint
	nodeCertFingerprintMu.RUnlock()
	if fp != "" {
		resp["fingerprint"] = fp
	}
	_ = json.NewEncoder(w).Encode(resp)
}

var (
	nodeShutdownOnce sync.Once
	nodeServer       *http.Server
	nodeMDNSServer   *zeroconf.Server
	nodeServerMu     sync.Mutex
	nodeChallengeMu  sync.Mutex
	nodeChallenges   = make(map[string]nodeChallenge)
)

type nodeChallenge struct {
	expires time.Time
	client  string
}

func issueNodeChallenge(client string) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(bytes)
	now := time.Now()
	nodeChallengeMu.Lock()
	defer nodeChallengeMu.Unlock()
	var clientCount int
	var oldestClientNonce string
	var oldestClientExpiry time.Time
	var oldestNonce string
	var oldestExpiry time.Time
	for known, challenge := range nodeChallenges {
		if now.After(challenge.expires) {
			delete(nodeChallenges, known)
			continue
		}
		if oldestNonce == "" || challenge.expires.Before(oldestExpiry) {
			oldestNonce, oldestExpiry = known, challenge.expires
		}
		if challenge.client == client {
			clientCount++
			if oldestClientNonce == "" || challenge.expires.Before(oldestClientExpiry) {
				oldestClientNonce, oldestClientExpiry = known, challenge.expires
			}
		}
	}
	if clientCount >= 16 && oldestClientNonce != "" {
		delete(nodeChallenges, oldestClientNonce)
	}
	if len(nodeChallenges) >= 4096 && oldestNonce != "" {
		delete(nodeChallenges, oldestNonce)
	}
	nodeChallenges[nonce] = nodeChallenge{expires: now.Add(30 * time.Second), client: client}
	return nonce, nil
}

func validNodeNonce(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func consumeNodeChallenge(nonce string, client ...string) bool {
	if !validNodeNonce(nonce) {
		return false
	}
	nodeChallengeMu.Lock()
	challenge, ok := nodeChallenges[nonce]
	if ok {
		delete(nodeChallenges, nonce)
	}
	nodeChallengeMu.Unlock()
	if !ok || time.Now().After(challenge.expires) {
		return false
	}
	if len(client) > 0 && client[0] != "" && challenge.client != "" && challenge.client != client[0] {
		return false
	}
	return true
}

func nodeRequestProof(token, nonce, clientNonce string, certFingerprint ...string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = io.WriteString(mac, "GET\n/api/v1/telemetry\n"+nonce+"\n"+clientNonce)
	if len(certFingerprint) > 0 && certFingerprint[0] != "" {
		_, _ = io.WriteString(mac, "\n"+certFingerprint[0])
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func nodeResponseProof(token, nonce, clientNonce string, body []byte, certFingerprint ...string) string {
	sum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = io.WriteString(mac, "200\n/api/v1/telemetry\n"+nonce+"\n"+clientNonce+"\n"+hex.EncodeToString(sum[:]))
	if len(certFingerprint) > 0 && certFingerprint[0] != "" {
		_, _ = io.WriteString(mac, "\n"+certFingerprint[0])
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func shutdownNodeNetwork() {
	nodeShutdownOnce.Do(func() {
		nodeServerMu.Lock()
		server, mdnsServer := nodeServer, nodeMDNSServer
		nodeServerMu.Unlock()
		if mdnsServer != nil {
			mdnsServer.Shutdown()
		}
		if server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
		}
	})
}
