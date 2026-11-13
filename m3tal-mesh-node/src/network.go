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
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const meshServiceType = "_m3tal-mesh-node._tcp"

type nodeRuntimeConfig struct {
	PairingToken        string `json:"pairing_token,omitempty"`
	Password            string `json:"password,omitempty"`
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
	// Do not persist dynamically inherited Core API key to config file
	cfgToSave := cfg
	if coreKey := resolveCoreAPIKey(); coreKey != "" && cfgToSave.PairingToken == coreKey {
		cfgToSave.PairingToken = ""
	}
	b, err := json.MarshalIndent(cfgToSave, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// resolveCoreAPIKey retrieves the central API key configured in M3TAL Core.
func resolveCoreAPIKey() string {
	if k := os.Getenv("M3TAL_API_KEY"); k != "" {
		return k
	}
	if k := os.Getenv("M3TAL_API_TOKEN"); k != "" {
		return k
	}
	if k := os.Getenv("API_TOKEN"); k != "" {
		return k
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		keyFile := filepath.Join(home, ".config", "m3tal", "api_keys.json")
		if data, err := os.ReadFile(keyFile); err == nil {
			var parsed struct {
				PrimaryToken string `json:"primary_token"`
			}
			if err := json.Unmarshal(data, &parsed); err == nil && parsed.PrimaryToken != "" {
				return parsed.PrimaryToken
			}
		}
		coreCfgFile := filepath.Join(home, ".config", "m3tal", "config.json")
		if data, err := os.ReadFile(coreCfgFile); err == nil {
			var parsed struct {
				APIToken string `json:"api_token"`
			}
			if err := json.Unmarshal(data, &parsed); err == nil && parsed.APIToken != "" {
				return parsed.APIToken
			}
		}
		legacyCfg := filepath.Join(home, ".m3tal", "config")
		if data, err := os.ReadFile(legacyCfg); err == nil {
			var parsed struct {
				APIToken string `json:"api_token"`
			}
			if err := json.Unmarshal(data, &parsed); err == nil && parsed.APIToken != "" {
				return parsed.APIToken
			}
		}
	}
	return ""
}

type coreAPIKeyEntry struct {
	Token       string   `json:"token"`
	Permissions []string `json:"permissions"`
	Direction   string   `json:"direction"`
}

// coreAPIKeyAllowsMesh reports whether an exported API key may authenticate
// mesh federation. Accepts full access ("*"), the dedicated mesh scope, any
// "mesh*" scope, or empty permissions (legacy keys).
func coreAPIKeyAllowsMesh(perms []string) bool {
	for _, p := range perms {
		if p == "*" || p == "mesh:access" || strings.HasPrefix(p, "mesh") {
			return true
		}
	}
	return len(perms) == 0
}

// resolveCoreAPIKeys returns every API key token exported by M3TAL Core's API
// Key Manager that is permitted for mesh federation, including the primary
// token for backward compatibility. Remote-direction keys are credentials for
// other machines and are never accepted.
func resolveCoreAPIKeys() []string {
	seen := make(map[string]bool)
	var out []string
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		keyFile := filepath.Join(home, ".config", "m3tal", "api_keys.json")
		if data, err := os.ReadFile(keyFile); err == nil {
			var parsed struct {
				PrimaryToken string            `json:"primary_token"`
				Keys         []coreAPIKeyEntry `json:"keys"`
			}
			if err := json.Unmarshal(data, &parsed); err == nil {
				add(parsed.PrimaryToken)
				for _, k := range parsed.Keys {
					if k.Direction == "remote" {
						continue
					}
					if !coreAPIKeyAllowsMesh(k.Permissions) {
						continue
					}
					add(k.Token)
				}
			}
		}
	}
	return out
}

// acceptedTokens returns all tokens this node will accept: the explicitly
// configured/inherited pairing token plus every mesh-capable key from Core's
// API Key Manager, deduplicated.
func (c nodeRuntimeConfig) acceptedTokens() []string {
	seen := make(map[string]bool)
	var out []string
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	add(c.PairingToken)
	for _, t := range resolveCoreAPIKeys() {
		add(t)
	}
	return out
}

func loadNodeRuntimeConfig() nodeRuntimeConfig {
	cfg := nodeRuntimeConfig{
		ListenPort:    9321,
		BroadcastMDNS: true,
	}
	path := nodeConfigPath()
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &cfg)
		}
	}
	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		cfg.ListenPort = 9321
	}
	// Inherit Core's central API key if no explicit per-node override is configured
	if coreKey := resolveCoreAPIKey(); coreKey != "" {
		if cfg.PairingToken == "" || cfg.PairingToken == coreKey {
			cfg.PairingToken = coreKey
		}
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
	acceptedTokens := current.acceptedTokens()
	expectedPass := current.Password
	authRequired := current.IsPairingTokenRequired()

	if authRequired && len(acceptedTokens) == 0 && expectedPass == "" {
		http.Error(w, "authentication is required but neither API token nor password is configured on this node", http.StatusServiceUnavailable)
		return
	}

	suppliedProof := r.Header.Get("X-M3TAL-Proof")
	nonce := r.Header.Get("X-M3TAL-Nonce")
	clientNonce := r.Header.Get("X-M3TAL-Client-Nonce")
	var fp string
	if r.TLS != nil {
		nodeCertFingerprintMu.RLock()
		fp = nodeCertFingerprint
		nodeCertFingerprintMu.RUnlock()
	}

	matchedToken := ""
	if len(acceptedTokens) > 0 {
		if suppliedProof == "" || nonce == "" || clientNonce == "" {
			http.Error(w, "pairing token required", http.StatusUnauthorized)
			return
		}

		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			clientIP = r.RemoteAddr
		}

		if !validNodeNonce(clientNonce) || !consumeNodeChallenge(nonce, clientIP) {
			http.Error(w, "invalid pairing challenge", http.StatusUnauthorized)
			return
		}

		// Multiple API keys may be live at once: accept the proof if it was
		// computed with any authorized token from Core's API Key Manager.
		for _, tok := range acceptedTokens {
			if subtle.ConstantTimeCompare([]byte(suppliedProof), []byte(nodeRequestProof(tok, nonce, clientNonce, fp))) == 1 {
				matchedToken = tok
				break
			}
		}
		if matchedToken == "" {
			http.Error(w, "invalid pairing token", http.StatusUnauthorized)
			return
		}
	}

	passwordProofAuthed := false
	if expectedPass != "" {
		clientIP, _, ipErr := net.SplitHostPort(r.RemoteAddr)
		if ipErr != nil {
			clientIP = r.RemoteAddr
		}
		// Atomically check the lockout and reserve an attempt slot so
		// concurrent requests cannot exceed the failure budget.
		if !nodePasswordBegin(clientIP) {
			http.Error(w, "too many failed authentication attempts", http.StatusTooManyRequests)
			return
		}
		authed := false
		if suppliedProof := r.Header.Get("X-M3TAL-Password-Proof"); suppliedProof != "" {
			// Proof-based password auth: the client proves knowledge of the
			// password via HMAC over the single-use challenge nonce, so the
			// secret never crosses the wire (safe even on unverified TLS)
			// and each guess consumes a fresh nonce.
			// The token branch already validated and consumed this challenge
			// when tokens are configured; consuming it again would always fail.
			// Invalid challenges are not password guesses, so they do not
			// count toward the lockout.
			challengeOK := matchedToken != ""
			if !challengeOK {
				challengeOK = nonce != "" && clientNonce != "" && validNodeNonce(clientNonce) &&
					consumeNodeChallenge(nonce, clientIP)
			}
			if !challengeOK {
				nodePasswordRelease(clientIP)
				http.Error(w, "invalid pairing challenge", http.StatusUnauthorized)
				return
			}
			expected := nodePasswordProof(expectedPass, nonce, clientNonce, fp)
			if subtle.ConstantTimeCompare([]byte(suppliedProof), []byte(expected)) == 1 {
				authed = true
				passwordProofAuthed = true
			}
		} else {
			// Legacy raw-password header (older clients). Still rate-limited;
			// new clients must use X-M3TAL-Password-Proof.
			suppliedPass := r.Header.Get("X-M3TAL-Password")
			if suppliedPass == "" {
				nodePasswordRelease(clientIP)
				w.Header().Set("X-M3TAL-Auth-Status", "password_required")
				if suppliedProof != "" && matchedToken != "" {
					w.Header().Set("X-M3TAL-Proof", nodeResponseProof(matchedToken, nonce, clientNonce, []byte("password required\n"), fp))
				}
				http.Error(w, "password required", http.StatusUnauthorized)
				return
			}
			if subtle.ConstantTimeCompare([]byte(suppliedPass), []byte(expectedPass)) == 1 {
				authed = true
			}
		}
		if !authed {
			nodePasswordFail(clientIP)
			http.Error(w, "invalid password", http.StatusUnauthorized)
			return
		}
		nodePasswordSuccess(clientIP)
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
	if matchedToken != "" {
		w.Header().Set("X-M3TAL-Proof", nodeResponseProof(matchedToken, nonce, clientNonce, body, fp))
	}
	if passwordProofAuthed && nonce != "" && clientNonce != "" {
		// Prove to the client that this server knows the password, so a
		// relay/MITM on the unauthenticated TLS channel cannot impersonate
		// the node to a client holding the real credential.
		w.Header().Set("X-M3TAL-Password-Auth-Proof", nodePasswordResponseProof(expectedPass, nonce, clientNonce, body, fp))
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

// nodePasswordProof authenticates a password without transmitting it:
// HMAC-SHA256 keyed by the password over the single-use challenge nonce.
// Domain-separated ("PASS") from the token proofs.
func nodePasswordProof(password, nonce, clientNonce string, certFingerprint ...string) string {
	mac := hmac.New(sha256.New, []byte(password))
	_, _ = io.WriteString(mac, "PASS\nGET\n/api/v1/telemetry\n"+nonce+"\n"+clientNonce)
	if len(certFingerprint) > 0 && certFingerprint[0] != "" {
		_, _ = io.WriteString(mac, "\n"+certFingerprint[0])
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// nodePasswordResponseProof lets a password-authenticated client verify the
// responding server actually knows the password.
func nodePasswordResponseProof(password, nonce, clientNonce string, body []byte, certFingerprint ...string) string {
	sum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(password))
	_, _ = io.WriteString(mac, "PASS\n200\n/api/v1/telemetry\n"+nonce+"\n"+clientNonce+"\n"+hex.EncodeToString(sum[:]))
	if len(certFingerprint) > 0 && certFingerprint[0] != "" {
		_, _ = io.WriteString(mac, "\n"+certFingerprint[0])
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// Password-attempt rate limiting: failed password proofs/raw passwords are
// counted per client IP; 5 failures lock the IP out for 60 seconds. Bounds
// online guessing even though each proof consumes a single-use nonce.
const (
	nodePassMaxFails   = 5
	nodePassLockout    = 60 * time.Second
	nodePassTrackerMax = 4096
)

type nodePassAttempt struct {
	fails      int
	inflight   int
	lockedTill time.Time
}

var (
	nodePassMu    sync.Mutex
	nodePassFails = make(map[string]*nodePassAttempt)
)

// nodePasswordBegin atomically checks the lockout and reserves one attempt
// slot. Failures plus in-flight attempts never exceed nodePassMaxFails, so
// parallel requests cannot test more guesses than the budget allows. Every
// true result must be settled by exactly one of nodePasswordFail,
// nodePasswordSuccess, or nodePasswordRelease.
func nodePasswordBegin(client string) bool {
	nodePassMu.Lock()
	defer nodePassMu.Unlock()
	now := time.Now()
	a := nodePassFails[client]
	if a == nil {
		if len(nodePassFails) >= nodePassTrackerMax {
			// Evict idle entries that are not actively locked out.
			for ip, e := range nodePassFails {
				if e.inflight == 0 && !now.Before(e.lockedTill) {
					delete(nodePassFails, ip)
				}
			}
		}
		if len(nodePassFails) >= nodePassTrackerMax {
			// Tracker saturated by active lockouts: fail closed rather
			// than grow unbounded or allow untracked guesses.
			return false
		}
		a = &nodePassAttempt{}
		nodePassFails[client] = a
	} else if !a.lockedTill.IsZero() && !now.Before(a.lockedTill) {
		// Previous lockout expired: start a fresh failure window.
		a.fails = 0
		a.lockedTill = time.Time{}
	}
	if now.Before(a.lockedTill) || a.fails+a.inflight >= nodePassMaxFails {
		return false
	}
	a.inflight++
	return true
}

// nodePasswordRelease returns a reserved slot for a request that did not
// actually test a password (e.g. invalid challenge, no credential supplied).
func nodePasswordRelease(client string) {
	nodePassMu.Lock()
	defer nodePassMu.Unlock()
	a := nodePassFails[client]
	if a == nil {
		return
	}
	if a.inflight > 0 {
		a.inflight--
	}
	if a.fails == 0 && a.inflight == 0 && a.lockedTill.IsZero() {
		delete(nodePassFails, client)
	}
}

func nodePasswordFail(client string) {
	nodePassMu.Lock()
	defer nodePassMu.Unlock()
	a := nodePassFails[client]
	if a == nil {
		return
	}
	if a.inflight > 0 {
		a.inflight--
	}
	a.fails++
	if a.fails >= nodePassMaxFails {
		a.lockedTill = time.Now().Add(nodePassLockout)
	}
}

func nodePasswordSuccess(client string) {
	nodePassMu.Lock()
	defer nodePassMu.Unlock()
	a := nodePassFails[client]
	if a == nil {
		return
	}
	if a.inflight > 0 {
		a.inflight--
	}
	a.fails = 0
	a.lockedTill = time.Time{}
	if a.inflight == 0 {
		delete(nodePassFails, client)
	}
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
