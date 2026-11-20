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
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	_ "modernc.org/sqlite"
)

var (
	ErrPasswordRequired = errors.New("password required")
	ErrTokenRejected    = errors.New("pairing token was rejected")
	ErrInvalidPassword  = errors.New("invalid password")
)

const meshServiceType = "_m3tal-mesh-node._tcp"

type meshConfig struct {
	PairingToken        string `json:"pairing_token,omitempty"`
	DefaultPassword     string `json:"default_password,omitempty"`
	RequirePairingToken *bool  `json:"require_pairing_token,omitempty"`
	ListenPort          int    `json:"listen_port"`
	DiscoveryEnabled    *bool  `json:"discovery_enabled,omitempty"`
	BroadcastMDNS       *bool  `json:"broadcast_mdns,omitempty"`
	BroadcasterOnly     *bool  `json:"broadcaster_only_mode,omitempty"`
}

func (c meshConfig) IsPairingTokenRequired() bool {
	if c.RequirePairingToken == nil {
		return true
	}
	return *c.RequirePairingToken
}

func (c meshConfig) IsDiscoveryEnabled() bool {
	if c.DiscoveryEnabled == nil {
		return true
	}
	return *c.DiscoveryEnabled
}

func (c meshConfig) IsMDNSEnabled() bool {
	if c.BroadcastMDNS == nil {
		return true
	}
	return *c.BroadcastMDNS
}

func (c meshConfig) IsBroadcasterOnly() bool {
	if c.BroadcasterOnly == nil {
		return false
	}
	return *c.BroadcasterOnly
}

type remoteTelemetry struct {
	Hostname       string  `json:"hostname"`
	Cores          int     `json:"cores"`
	Load1m         float64 `json:"load_1m"`
	RAMUsedGB      float64 `json:"ram_used_gb"`
	RAMTotalGB     float64 `json:"ram_total_gb"`
	StorageUsedGB  float64 `json:"storage_used_gb"`
	StorageTotalGB float64 `json:"storage_total_gb"`
}

type remoteResponse struct {
	Name      string          `json:"name"`
	Hostname  string          `json:"hostname"`
	Telemetry remoteTelemetry `json:"telemetry"`
}

var peerSaveMu sync.Mutex
var peerMutationMu sync.Mutex

var meshHTTPClient = &http.Client{
	Timeout: 4 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12},
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConns:        64,
		MaxIdleConnsPerHost: 8,
	},
}

func newPinnedHTTPClient(expectedFP string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
				VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
					if len(rawCerts) == 0 {
						return fmt.Errorf("no peer certificates presented")
					}
					sum := sha256.Sum256(rawCerts[0])
					got := hex.EncodeToString(sum[:])
					if expectedFP != "" && got != expectedFP {
						return fmt.Errorf("certificate fingerprint mismatch: expected %s, got %s", expectedFP, got)
					}
					return nil
				},
			},
			DisableKeepAlives: true,
		},
	}
}

func meshConfigBaseDir() string {
	// 1. Explicit configuration directory override
	if dir := os.Getenv("M3TAL_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "plugins", "mesh")
	}
	// 2. XDG_CONFIG_HOME override
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "m3tal", "plugins", "mesh")
	}
	// 3. User home directory if valid and not system root
	if home, err := os.UserHomeDir(); err == nil && home != "" && home != "/" && home != "/root" {
		return filepath.Join(home, ".config", "m3tal", "plugins", "mesh")
	}
	// 4. Existing local user configuration path
	if stat, err := os.Stat("/home/m3tal/.config/m3tal/plugins/mesh"); err == nil && stat.IsDir() {
		return "/home/m3tal/.config/m3tal/plugins/mesh"
	}
	if stat, err := os.Stat("/home/m3tal/.config/m3tal"); err == nil && stat.IsDir() {
		return "/home/m3tal/.config/m3tal/plugins/mesh"
	}
	// 5. Preserve legacy root-home configuration from earlier releases
	if home, err := os.UserHomeDir(); err == nil && home == "/root" {
		legacy := filepath.Join(home, ".config", "m3tal", "plugins", "mesh")
		for _, f := range []string{"config.json", "peers.json"} {
			if _, err := os.Stat(filepath.Join(legacy, f)); err == nil {
				return legacy
			}
		}
	}
	// 6. System wide /etc/m3tal configuration directory
	if stat, err := os.Stat("/etc/m3tal/plugins/m3tal-mesh"); err == nil && stat.IsDir() {
		return "/etc/m3tal/plugins/m3tal-mesh"
	}
	if stat, err := os.Stat("/etc/m3tal"); err == nil && stat.IsDir() {
		return "/etc/m3tal/plugins/m3tal-mesh"
	}
	// 6. User home even if /root
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "m3tal", "plugins", "mesh")
	}
	return "/etc/m3tal/plugins/m3tal-mesh"
}

func meshConfigPath() string {
	base := meshConfigBaseDir()
	return filepath.Join(base, "config.json")
}

func saveMeshConfig(cfg meshConfig) error {
	path := meshConfigPath()
	if path == "" {
		return fmt.Errorf("cannot locate the user configuration directory")
	}
	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		return fmt.Errorf("default node port must be between 1 and 65535")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	// Do not persist dynamically inherited Core API key into local plugin config file
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

func coreAPIKeyAllowsMesh(perms []string) bool {
	for _, p := range perms {
		if p == "*" || p == "mesh:access" || strings.HasPrefix(p, "mesh") {
			return true
		}
	}
	return len(perms) == 0
}

func queryStateDBKeys() (primary string, localKeys []string, remoteKeys []string) {
	var candidates []string
	if dbEnv := os.Getenv("M3TAL_STATE_DB"); dbEnv != "" {
		candidates = append(candidates, dbEnv)
	}
	home, _ := os.UserHomeDir()
	isCustomHome := home != "" && home != "/home/m3tal" && home != "/root"
	if isCustomHome {
		candidates = append(candidates,
			filepath.Join(home, ".config", "m3tal", "state.db"),
			filepath.Join(home, "state.db"),
			filepath.Join(home, "data", "state.db"),
		)
	} else {
		candidates = append(candidates,
			"/var/lib/m3tal/state.db",
			"/home/m3tal/apps/M3tal-core/data/state.db",
			"./data/state.db",
		)
		if home != "" {
			candidates = append(candidates,
				filepath.Join(home, ".config", "m3tal", "state.db"),
				filepath.Join(home, "data", "state.db"),
			)
		}
	}

	var dbPath string
	for _, p := range candidates {
		if stat, err := os.Stat(p); err == nil && !stat.IsDir() {
			dbPath = p
			break
		}
	}
	if dbPath == "" {
		return "", nil, nil
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", dbPath))
	if err != nil {
		return "", nil, nil
	}
	defer db.Close()

	rows, err := db.Query("SELECT token, permissions, direction FROM api_keys")
	if err != nil {
		return "", nil, nil
	}
	defer rows.Close()

	for rows.Next() {
		var tok, permsRaw, dir string
		if err := rows.Scan(&tok, &permsRaw, &dir); err != nil {
			continue
		}
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		var perms []string
		_ = json.Unmarshal([]byte(permsRaw), &perms)

		if dir == "remote" {
			remoteKeys = append(remoteKeys, tok)
		} else {
			if coreAPIKeyAllowsMesh(perms) {
				if primary == "" {
					primary = tok
				}
				localKeys = append(localKeys, tok)
			}
		}
	}
	return primary, localKeys, remoteKeys
}

func readAPIKeysFile(path string) (primary string, localKeys []string, remoteKeys []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, nil
	}
	var parsed struct {
		PrimaryToken string            `json:"primary_token"`
		Keys         []coreAPIKeyEntry `json:"keys"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", nil, nil
	}
	primary = strings.TrimSpace(parsed.PrimaryToken)
	for _, k := range parsed.Keys {
		tok := strings.TrimSpace(k.Token)
		if tok == "" {
			continue
		}
		if k.Direction == "remote" {
			remoteKeys = append(remoteKeys, tok)
		} else if coreAPIKeyAllowsMesh(k.Permissions) {
			localKeys = append(localKeys, tok)
		}
	}
	return primary, localKeys, remoteKeys
}

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

	home, _ := os.UserHomeDir()
	isCustomHome := home != "" && home != "/home/m3tal" && home != "/root"

	if dbPrimary, _, _ := queryStateDBKeys(); dbPrimary != "" {
		return dbPrimary
	}

	var keyFiles []string
	if isCustomHome {
		keyFiles = []string{
			filepath.Join(home, ".config", "m3tal", "api_keys.json"),
		}
	} else {
		if home != "" {
			keyFiles = append(keyFiles, filepath.Join(home, ".config", "m3tal", "api_keys.json"))
		}
		keyFiles = append(keyFiles,
			"/home/m3tal/.config/m3tal/api_keys.json",
			"/etc/m3tal/api_keys.json",
		)
	}

	for _, kf := range keyFiles {
		if primary, _, _ := readAPIKeysFile(kf); primary != "" {
			return primary
		}
	}

	var cfgFiles []string
	if isCustomHome {
		cfgFiles = []string{
			filepath.Join(home, ".config", "m3tal", "config.json"),
			filepath.Join(home, ".m3tal", "config"),
		}
	} else {
		if home != "" {
			cfgFiles = append(cfgFiles, filepath.Join(home, ".config", "m3tal", "config.json"))
		}
		cfgFiles = append(cfgFiles,
			"/home/m3tal/.config/m3tal/config.json",
			"/etc/m3tal/config.json",
			"/etc/m3tal/core_settings.json",
		)
	}
	for _, cf := range cfgFiles {
		if data, err := os.ReadFile(cf); err == nil {
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

	if k := os.Getenv("M3TAL_API_KEY"); k != "" {
		add(k)
	}
	if k := os.Getenv("M3TAL_API_TOKEN"); k != "" {
		add(k)
	}
	if k := os.Getenv("API_TOKEN"); k != "" {
		add(k)
	}

	primary, locals, _ := queryStateDBKeys()
	add(primary)
	for _, l := range locals {
		add(l)
	}

	home, _ := os.UserHomeDir()
	isCustomHome := home != "" && home != "/home/m3tal" && home != "/root"

	var keyFiles []string
	if isCustomHome {
		keyFiles = []string{
			filepath.Join(home, ".config", "m3tal", "api_keys.json"),
		}
	} else {
		if home != "" {
			keyFiles = append(keyFiles, filepath.Join(home, ".config", "m3tal", "api_keys.json"))
		}
		keyFiles = append(keyFiles,
			"/home/m3tal/.config/m3tal/api_keys.json",
			"/etc/m3tal/api_keys.json",
		)
	}

	for _, kf := range keyFiles {
		pFile, lFile, _ := readAPIKeysFile(kf)
		add(pFile)
		for _, l := range lFile {
			add(l)
		}
	}
	return out
}

func resolveRemoteAPIKeys() []string {
	seen := make(map[string]bool)
	var out []string
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}

	_, _, remotes := queryStateDBKeys()
	for _, r := range remotes {
		add(r)
	}

	home, _ := os.UserHomeDir()
	isCustomHome := home != "" && home != "/home/m3tal" && home != "/root"

	var keyFiles []string
	if isCustomHome {
		keyFiles = []string{
			filepath.Join(home, ".config", "m3tal", "api_keys.json"),
		}
	} else {
		if home != "" {
			keyFiles = append(keyFiles, filepath.Join(home, ".config", "m3tal", "api_keys.json"))
		}
		keyFiles = append(keyFiles,
			"/home/m3tal/.config/m3tal/api_keys.json",
			"/etc/m3tal/api_keys.json",
		)
	}

	for _, kf := range keyFiles {
		_, _, rFile := readAPIKeysFile(kf)
		for _, r := range rFile {
			add(r)
		}
	}
	return out
}

func (c meshConfig) acceptedTokens() []string {
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

func loadMeshConfig() meshConfig {
	cfg := meshConfig{
		ListenPort: 9321,
	}
	path := meshConfigPath()
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &cfg)
		}
	}
	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		cfg.ListenPort = 9321
	}
	// Inherit Core's managed API key if no explicit cluster token is configured
	if coreKey := resolveCoreAPIKey(); coreKey != "" {
		if cfg.PairingToken == "" || cfg.PairingToken == coreKey {
			cfg.PairingToken = coreKey
		}
	}
	return cfg
}

func peersPath() string {
	home, _ := os.UserHomeDir()
	isCustomHome := home != "" && home != "/home/m3tal" && home != "/root"
	base := meshConfigBaseDir()
	primary := filepath.Join(base, "peers.json")
	if isCustomHome {
		return primary
	}
	if _, err := os.Stat(primary); err == nil {
		return primary
	}
	candidates := []string{
		"/home/m3tal/.config/m3tal/plugins/mesh/peers.json",
		"/etc/m3tal/plugins/m3tal-mesh/peers.json",
	}
	for _, cand := range candidates {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return primary
}

type peerDiskRecord struct {
	MeshNode
	APIToken     string `json:"api_token,omitempty"`
	Password     string `json:"password,omitempty"`
	PasswordOnly bool   `json:"password_only,omitempty"`
}

func (r peerDiskRecord) MarshalJSON() ([]byte, error) {
	type NodeAlias MeshNode
	return json.Marshal(&struct {
		NodeAlias
		APIToken     string `json:"api_token,omitempty"`
		Password     string `json:"password,omitempty"`
		PasswordOnly bool   `json:"password_only,omitempty"`
	}{
		NodeAlias:    NodeAlias(r.MeshNode),
		APIToken:     r.APIToken,
		Password:     r.Password,
		PasswordOnly: r.PasswordOnly,
	})
}

func loadPeers() []MeshNode {
	path := peersPath()
	if path == "" {
		return []MeshNode{}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return []MeshNode{}
	}
	var records []peerDiskRecord
	if json.Unmarshal(b, &records) != nil || records == nil {
		return []MeshNode{}
	}
	peers := make([]MeshNode, len(records))
	for i, r := range records {
		peers[i] = r.MeshNode
		peers[i].APIToken = r.APIToken
		peers[i].Password = r.Password
		peers[i].PasswordOnly = r.PasswordOnly
	}
	return peers
}

func savePeers(peers []MeshNode) error {
	peerSaveMu.Lock()
	defer peerSaveMu.Unlock()
	path := peersPath()
	if path == "" {
		return fmt.Errorf("cannot locate the user configuration directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	records := make([]peerDiskRecord, len(peers))
	for i, p := range peers {
		records[i] = peerDiskRecord{
			MeshNode:     p,
			APIToken:     p.APIToken,
			Password:     p.Password,
			PasswordOnly: p.PasswordOnly,
		}
	}
	b, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func stableNodeID(address, hostname string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(address + "|" + hostname)))
	return "node-" + hex.EncodeToString(sum[:8])
}

func normalizeNodeAddress(address string, defaultPort int) (string, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", fmt.Errorf("node IP address or hostname is required")
	}
	if strings.Contains(address, "://") {
		u, err := url.Parse(address)
		if err != nil || u.Hostname() == "" {
			return "", fmt.Errorf("invalid node address")
		}
		port := u.Port()
		if port == "" {
			port = strconv.Itoa(defaultPort)
		}
		return net.JoinHostPort(u.Hostname(), port), nil
	}
	if host, port, err := net.SplitHostPort(address); err == nil {
		if host == "" {
			return "", fmt.Errorf("invalid node address")
		}
		return net.JoinHostPort(host, port), nil
	}
	return net.JoinHostPort(address, strconv.Itoa(defaultPort)), nil
}

func validMeshNonce(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func newMeshNonce() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func requestProof(token, nonce, clientNonce string, certFingerprint ...string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = io.WriteString(mac, "GET\n/api/v1/telemetry\n"+nonce+"\n"+clientNonce)
	if len(certFingerprint) > 0 && certFingerprint[0] != "" {
		_, _ = io.WriteString(mac, "\n"+certFingerprint[0])
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func responseProof(token, nonce, clientNonce string, body []byte, certFingerprint ...string) string {
	sum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = io.WriteString(mac, "200\n/api/v1/telemetry\n"+nonce+"\n"+clientNonce+"\n"+hex.EncodeToString(sum[:]))
	if len(certFingerprint) > 0 && certFingerprint[0] != "" {
		_, _ = io.WriteString(mac, "\n"+certFingerprint[0])
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// passwordRequestProof authenticates the mesh password without transmitting
// it: HMAC-SHA256 keyed by the password over the single-use challenge nonce.
// Domain-separated ("PASS") from the token proofs. Sending the raw password
// over the default InsecureSkipVerify transport would expose it to MITM.
func passwordRequestProof(password, nonce, clientNonce string, certFingerprint ...string) string {
	mac := hmac.New(sha256.New, []byte(password))
	_, _ = io.WriteString(mac, "PASS\nGET\n/api/v1/telemetry\n"+nonce+"\n"+clientNonce)
	if len(certFingerprint) > 0 && certFingerprint[0] != "" {
		_, _ = io.WriteString(mac, "\n"+certFingerprint[0])
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// passwordResponseProof verifies that the responding server knows the
// password, detecting relay/MITM impersonation on unverified TLS.
func passwordResponseProof(password, nonce, clientNonce string, body []byte, certFingerprint ...string) string {
	sum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(password))
	_, _ = io.WriteString(mac, "PASS\n200\n/api/v1/telemetry\n"+nonce+"\n"+clientNonce+"\n"+hex.EncodeToString(sum[:]))
	if len(certFingerprint) > 0 && certFingerprint[0] != "" {
		_, _ = io.WriteString(mac, "\n"+certFingerprint[0])
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func fetchRemote(address, token string, port int, expectedFingerprint ...string) (MeshNode, error) {
	return fetchRemoteWithAuth(address, token, "", port, expectedFingerprint...)
}

// ErrPasswordPinRequired is returned when a password proof would be sent to a
// peer whose certificate fingerprint is not pinned outside interactive pairing.
var ErrPasswordPinRequired = errors.New("password authentication requires a pinned node fingerprint; re-pair the node")

// fetchRemoteWithAuth never sends a password proof to an unpinned peer: a
// forged node could capture the proof and brute-force the password offline.
func fetchRemoteWithAuth(address, token, password string, port int, expectedFingerprint ...string) (MeshNode, error) {
	return fetchRemoteAuth(address, token, password, port, false, expectedFingerprint...)
}

// fetchRemoteAuth is fetchRemoteWithAuth with an explicit trust-on-first-use
// switch. allowUnpinnedPassword must only be true for a password the user
// typed during an interactive pairing action.
func fetchRemoteAuth(address, token, password string, port int, allowUnpinnedPassword bool, expectedFingerprint ...string) (MeshNode, error) {
	pinned := len(expectedFingerprint) > 0 && expectedFingerprint[0] != ""
	if password != "" && !pinned && !allowUnpinnedPassword {
		return MeshNode{}, fmt.Errorf("%s: %w", address, ErrPasswordPinRequired)
	}
	cfg := loadMeshConfig()
	if token == "" && password == "" && cfg.IsPairingTokenRequired() {
		return MeshNode{}, fmt.Errorf("pairing token is required to authenticate mesh nodes; configure a shared secret in Mesh Settings or enter a pairing token")
	}
	endpoint, err := normalizeNodeAddress(address, port)
	if err != nil {
		return MeshNode{}, err
	}
	started := time.Now()
	var nonce, clientNonce, peerFingerprint string

	if token != "" || password != "" || (len(expectedFingerprint) > 0 && expectedFingerprint[0] != "") {
		challengeReq, err := http.NewRequest(http.MethodPost, "https://"+endpoint+"/api/v1/challenge", nil)
		if err != nil {
			return MeshNode{}, fmt.Errorf("invalid node address: %w", err)
		}
		challengeClient := meshHTTPClient
		if len(expectedFingerprint) > 0 && expectedFingerprint[0] != "" {
			challengeClient = newPinnedHTTPClient(expectedFingerprint[0], 4*time.Second)
		}
		challengeResp, err := challengeClient.Do(challengeReq)
		if err != nil {
			return MeshNode{}, fmt.Errorf("cannot reach %s: %w", address, err)
		}
		defer challengeResp.Body.Close()
		if challengeResp.TLS != nil && len(challengeResp.TLS.PeerCertificates) > 0 {
			sum := sha256.Sum256(challengeResp.TLS.PeerCertificates[0].Raw)
			peerFingerprint = hex.EncodeToString(sum[:])
		}
		if challengeResp.StatusCode != http.StatusOK {
			return MeshNode{}, fmt.Errorf("%s is not a compatible m3tal-mesh-node (HTTP %d)", address, challengeResp.StatusCode)
		}
		var challenge struct {
			Nonce string `json:"nonce"`
		}
		if err := json.NewDecoder(io.LimitReader(challengeResp.Body, 4096)).Decode(&challenge); err != nil {
			return MeshNode{}, fmt.Errorf("%s returned an invalid pairing challenge: %w", address, err)
		}
		if !validMeshNonce(challenge.Nonce) {
			return MeshNode{}, fmt.Errorf("%s returned an invalid pairing challenge", address)
		}
		cNonce, err := newMeshNonce()
		if err != nil {
			return MeshNode{}, fmt.Errorf("could not create pairing challenge: %w", err)
		}
		clientNonce = cNonce
		nonce = challenge.Nonce
	}

	req, err := http.NewRequest(http.MethodGet, "https://"+endpoint+"/api/v1/telemetry", nil)
	if err != nil {
		return MeshNode{}, fmt.Errorf("invalid node address: %w", err)
	}
	if nonce != "" && clientNonce != "" {
		req.Header.Set("X-M3TAL-Nonce", nonce)
		req.Header.Set("X-M3TAL-Client-Nonce", clientNonce)
		if token != "" {
			req.Header.Set("X-M3TAL-Proof", requestProof(token, nonce, clientNonce, peerFingerprint))
		}
	}
	if password != "" {
		// Never send the raw password: the default transport does not
		// authenticate the server, so a MITM could steal it. The HMAC proof
		// binds authentication to the single-use challenge nonce instead.
		if nonce != "" && clientNonce != "" {
			req.Header.Set("X-M3TAL-Password-Proof", passwordRequestProof(password, nonce, clientNonce, peerFingerprint))
		}
	}
	telemetryClient := meshHTTPClient
	if peerFingerprint != "" {
		telemetryClient = newPinnedHTTPClient(peerFingerprint, 4*time.Second)
	} else if len(expectedFingerprint) > 0 && expectedFingerprint[0] != "" {
		telemetryClient = newPinnedHTTPClient(expectedFingerprint[0], 4*time.Second)
	}
	resp, err := telemetryClient.Do(req)
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		return MeshNode{}, fmt.Errorf("cannot reach %s: %w", address, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if resp.Header.Get("X-M3TAL-Auth-Status") == "password_required" {
			if token != "" && nonce != "" && clientNonce != "" {
				expectedProof := responseProof(token, nonce, clientNonce, respBody, peerFingerprint)
				if resp.Header.Get("X-M3TAL-Proof") == expectedProof {
					return MeshNode{Fingerprint: peerFingerprint}, ErrPasswordRequired
				}
			}
		}
		if strings.Contains(strings.ToLower(string(respBody)), "invalid password") {
			return MeshNode{}, fmt.Errorf("%w for %s", ErrInvalidPassword, address)
		}
		return MeshNode{}, fmt.Errorf("%w by %s", ErrTokenRejected, address)
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return MeshNode{}, fmt.Errorf("pairing token is not configured on %s: %w", address, ErrTokenRejected)
	}
	if resp.StatusCode != http.StatusOK {
		return MeshNode{}, fmt.Errorf("%s is not a compatible m3tal-mesh-node (HTTP %d)", address, resp.StatusCode)
	}
	if peerFingerprint == "" && resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		sum := sha256.Sum256(resp.TLS.PeerCertificates[0].Raw)
		peerFingerprint = hex.EncodeToString(sum[:])
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return MeshNode{}, fmt.Errorf("could not read telemetry from %s: %w", address, err)
	}
	if token != "" && nonce != "" && clientNonce != "" {
		proof := resp.Header.Get("X-M3TAL-Proof")
		if proof == "" {
			return MeshNode{}, fmt.Errorf("%s returned unsigned response: %w", address, ErrTokenRejected)
		}
		expected := responseProof(token, nonce, clientNonce, body, peerFingerprint)
		if !hmac.Equal([]byte(expected), []byte(proof)) {
			return MeshNode{}, fmt.Errorf("%s failed mesh node authentication", address)
		}
	}
	if password != "" && nonce != "" && clientNonce != "" {
		// Password-authenticated nodes must prove they know the password;
		// a relay cannot forge this response proof.
		proof := resp.Header.Get("X-M3TAL-Password-Auth-Proof")
		expected := passwordResponseProof(password, nonce, clientNonce, body, peerFingerprint)
		if proof == "" || !hmac.Equal([]byte(expected), []byte(proof)) {
			return MeshNode{}, fmt.Errorf("%s failed mesh node authentication", address)
		}
	}
	var payload remoteResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return MeshNode{}, fmt.Errorf("%s returned invalid telemetry: %w", address, err)
	}
	hostname := payload.Hostname
	if hostname == "" {
		hostname = payload.Telemetry.Hostname
	}
	if hostname == "" {
		return MeshNode{}, fmt.Errorf("%s did not identify itself as an M3TAL mesh node", address)
	}
	name := payload.Name
	if name == "" {
		name = hostname
	}
	t := payload.Telemetry
	cpuload := 0.0
	if t.Cores > 0 {
		cpuload = t.Load1m / float64(t.Cores) * 100
		if cpuload > 100 {
			cpuload = 100
		}
	}
	return MeshNode{
		ID: stableNodeID(endpoint, hostname), Name: name, IP: endpoint, Role: "server", Status: "online",
		Fingerprint: peerFingerprint,
		APIToken:    token,
		Password:    password,
		Cores:       t.Cores, CPULoad: cpuload, RAMUsedGB: t.RAMUsedGB, RAMTotalGB: t.RAMTotalGB,
		StorageUsedGB: t.StorageUsedGB, StorageTotalGB: t.StorageTotalGB, StoragePool: "Root Filesystem",
		PingMs: latency, LastSeen: "just now", IsLocal: false,
	}, nil
}

// effectiveCredentials determines the token and password to use when connecting to a node.
// To prevent exfiltration of mesh-wide secrets to untrusted or unpinned hosts, passwords
// must be configured explicitly on the peer itself; mesh-wide default secrets are never
// transmitted ambiently across routine refresh or ping.
// If passwordOnly is true, default token fallback is suppressed.
func effectiveCredentials(explicitToken, explicitPass string, passwordOnly bool, cfg meshConfig) (tok, pass string) {
	pass = explicitPass
	tok = explicitToken
	if tok == "" && !passwordOnly {
		tok = cfg.PairingToken
	}
	return tok, pass
}

// meshTokenCandidates builds the outbound credential pool for peer
// connections: the effective primary token (explicit per-peer token or the
// inherited pairing token) followed by every remote-direction key registered
// in Core's API Key Manager. Explicit credentials never fall back to the
// manager pool.
func meshTokenCandidates(primary, explicitToken string, passwordOnly bool) []string {
	if explicitToken != "" || passwordOnly {
		if strings.TrimSpace(primary) == "" {
			return nil
		}
		return []string{primary}
	}
	seen := make(map[string]bool)
	var out []string
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	add(primary)
	for _, t := range resolveRemoteAPIKeys() {
		add(t)
	}
	return out
}

// connectAndNegotiateNode attempts to authenticate with a remote node using effective credentials.
// To prevent exfiltration of mesh-wide secrets to untrusted hosts, DefaultPassword is never sent upfront.
// If the remote host returns an authenticated password_required response, it retries with DefaultPassword,
// pinning the peer certificate fingerprint authenticated by the first exchange.
// When no explicit per-peer token is configured, every credential in the pool
// (pairing token + remote API keys) is tried until the peer accepts one.
// If an inherited token was rejected and a password was available, it retries in password-only mode.
// If passwordOnly is specified as true (e.g. for a saved password-only peer), token inheritance is suppressed.
func connectAndNegotiateNode(address, explicitToken, explicitPass string, cfg meshConfig, expectedFingerprint string, passwordOnly bool) (MeshNode, bool, error) {
	return connectAndNegotiateNodeOpts(address, explicitToken, explicitPass, cfg, expectedFingerprint, passwordOnly, false)
}

// connectAndNegotiateNodeOpts allows an unpinned password proof only when
// allowUnpinnedPassword is set (interactive pairing with a user-typed
// password). DefaultPassword retries are always pinned.
func connectAndNegotiateNodeOpts(address, explicitToken, explicitPass string, cfg meshConfig, expectedFingerprint string, passwordOnly, allowUnpinnedPassword bool) (MeshNode, bool, error) {
	allowUnpinnedPassword = allowUnpinnedPassword && explicitPass != ""
	tok, pass := effectiveCredentials(explicitToken, explicitPass, passwordOnly, cfg)
	candidates := meshTokenCandidates(tok, explicitToken, passwordOnly)
	if cfg.IsPairingTokenRequired() && len(candidates) == 0 && pass == "" {
		return MeshNode{}, false, fmt.Errorf("pairing token is required to authenticate remote mesh nodes; configure a shared secret in Mesh Settings or enter a pairing token")
	}
	if len(candidates) == 0 {
		candidates = []string{""}
	}

	var lastErr error
	for _, candidate := range candidates {
		node, err := fetchRemoteAuth(address, candidate, pass, cfg.ListenPort, allowUnpinnedPassword, expectedFingerprint)
		if err == nil {
			pwOnly := candidate == "" && pass != ""
			return node, pwOnly, nil
		}
		lastErr = err

		// Only send default password to unpinned address after an authenticated password_required response,
		// and strictly pin the retry to the peer certificate fingerprint authenticated by the first exchange.
		// A peer authenticated only with a custom per-peer token must not receive mesh DefaultPassword.
		if errors.Is(err, ErrPasswordRequired) && explicitPass == "" && cfg.DefaultPassword != "" && explicitToken == "" && candidate != "" {
			authFP := node.Fingerprint
			if authFP == "" {
				authFP = expectedFingerprint
			}
			if authFP != "" {
				if authNode, authErr := fetchRemoteWithAuth(address, candidate, cfg.DefaultPassword, cfg.ListenPort, authFP); authErr == nil {
					return authNode, false, nil
				}
			}
		}

		// Only move on to the next credential when the node rejected this token;
		// other failures (network, TLS) are not credential problems.
		if errors.Is(err, ErrTokenRejected) {
			continue
		}
		break
	}

	// Only retry in password-only mode when the inherited token was specifically rejected by the node
	if explicitToken == "" && pass != "" && lastErr != nil && errors.Is(lastErr, ErrTokenRejected) {
		if altNode, altErr := fetchRemoteAuth(address, "", pass, cfg.ListenPort, allowUnpinnedPassword, expectedFingerprint); altErr == nil {
			return altNode, true, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no credentials available to authenticate remote mesh node %s", address)
	}
	return MeshNode{}, false, lastErr
}

func pairAndPersistPeer(address, name, role, token, password string) (MeshNode, error) {
	cfg := loadMeshConfig()
	// Interactive pairing: the user-typed password may be proven to the
	// not-yet-pinned peer once (TOFU). The fingerprint is then saved and every
	// later password proof requires it.
	node, passwordOnly, err := connectAndNegotiateNodeOpts(address, token, password, cfg, "", false, true)
	if err != nil {
		return MeshNode{}, err
	}
	if name != "" {
		node.Name = name
	}
	if role != "" {
		node.Role = role
	}
	// Capture the negotiated credentials and authenticated fingerprint before overwriting.
	negToken, negPassword, pinnedFP := node.APIToken, node.Password, node.Fingerprint
	node.APIToken = token
	node.Password = password
	node.PasswordOnly = passwordOnly

	prevPeers := snapshotPeers()
	if err := persistPairedPeer(node); err != nil {
		return MeshNode{}, fmt.Errorf("could not save paired node: %w", err)
	}

	if cfg.PairingToken == "" && token != "" {
		cfg.PairingToken = token
		if err := saveMeshConfig(cfg); err != nil {
			rollbackPeers(prevPeers)
			return MeshNode{}, fmt.Errorf("could not save pairing token: %w", err)
		}
	}

	// Bidirectional auto-registration: asynchronously request the remote peer to register this node
	go func() {
		if err := dispatchBidirectionalPair(address, negToken, negPassword, pinnedFP, passwordOnly, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "m3tal-mesh reverse pairing with %s: %v\n", address, err)
		}
	}()

	return node, nil
}

func discoverAndVerifyPeers() ([]MeshNode, error) {
	cfg := loadMeshConfig()
	if !cfg.IsDiscoveryEnabled() {
		return []MeshNode{}, nil
	}
	if cfg.IsPairingTokenRequired() && cfg.PairingToken == "" {
		return []MeshNode{}, nil
	}
	resolver, err := zeroconf.NewResolver(zeroconf.SelectIPTraffic(zeroconf.IPv4))
	if err != nil {
		return nil, fmt.Errorf("LAN discovery could not start: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	entries := make(chan *zeroconf.ServiceEntry, 32)
	if err := resolver.Browse(ctx, meshServiceType, "local.", entries); err != nil {
		return nil, fmt.Errorf("LAN discovery failed: %w", err)
	}
	addresses := make(map[string]bool)
	pairedAddresses := make(map[string]bool)
	for _, peer := range snapshotPeers() {
		pairedAddresses[strings.ToLower(peer.IP)] = true
	}
	localIPs := make(map[string]bool)
	if interfaces, err := net.InterfaceAddrs(); err == nil {
		for _, address := range interfaces {
			if ipnet, ok := address.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				localIPs[ipnet.IP.String()] = true
			}
		}
	}
	candidates := make([]string, 0)
	candidateHosts := make(map[string]string)
	for {
		select {
		case entry := <-entries:
			if entry == nil {
				continue
			}
			ips := entry.AddrIPv4
			if len(ips) == 0 && entry.HostName != "" {
				if resolved, err := net.LookupIP(strings.TrimSuffix(entry.HostName, ".")); err == nil {
					for _, ip := range resolved {
						if ip.To4() != nil {
							ips = append(ips, ip)
						}
					}
				}
			}
			host := entry.Instance
			if host == "" {
				host = strings.TrimSuffix(entry.HostName, ".")
			}
			for _, ip := range ips {
				if localIPs[ip.String()] || entry.Port < 1 || entry.Port > 65535 {
					continue
				}
				address := net.JoinHostPort(ip.String(), strconv.Itoa(entry.Port))
				if addresses[address] || pairedAddresses[strings.ToLower(address)] {
					continue
				}
				addresses[address] = true
				candidates = append(candidates, address)
				if host != "" {
					candidateHosts[address] = host
				}
			}
		case <-ctx.Done():
			return verifyCandidates(candidates, candidateHosts, meshTokenCandidates(cfg.PairingToken, "", false), cfg.ListenPort), nil
		}
	}
}

func verifyCandidates(candidates []string, candidateHosts map[string]string, tokens []string, port int) []MeshNode {
	var wg sync.WaitGroup
	mu := sync.Mutex{}
	discovered := make([]MeshNode, 0)
	if len(tokens) == 0 {
		tokens = []string{""}
	}
	for _, address := range candidates {
		address := address
		wg.Add(1)
		go func() {
			defer wg.Done()
			var node MeshNode
			var err error
			for _, tok := range tokens {
				node, err = fetchRemoteWithAuth(address, tok, "", port)
				if err == nil || !errors.Is(err, ErrTokenRejected) {
					break
				}
			}
			if err == nil {
				mu.Lock()
				discovered = append(discovered, node)
				mu.Unlock()
			} else if errors.Is(err, ErrPasswordRequired) {
				hostname := ""
				if candidateHosts != nil {
					hostname = candidateHosts[address]
				}
				if hostname == "" {
					hostname = address
				}
				mu.Lock()
				discovered = append(discovered, MeshNode{
					ID:               stableNodeID(address, hostname),
					Name:             hostname,
					IP:               address,
					Role:             "server",
					Status:           "unverified",
					PasswordRequired: true,
				})
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return discovered
}

func rollbackPeers(prev []MeshNode) {
	peerMutationMu.Lock()
	defer peerMutationMu.Unlock()
	_ = savePeers(prev)
	nodeMutex.Lock()
	remoteNodes = prev
	nodeMutex.Unlock()
}

func persistPairedPeer(node MeshNode) error {
	return persistPeerUpsertInternal(node, true, true, true)
}

func persistPeerUpsert(node MeshNode, clearToken, clearPassword bool) error {
	return persistPeerUpsertInternal(node, clearToken, clearPassword, false)
}

func persistPeerUpsertInternal(node MeshNode, clearToken, clearPassword, isPairing bool) error {
	peerMutationMu.Lock()
	defer peerMutationMu.Unlock()
	peers := snapshotPeers()

	existingIDIdx := -1
	for i := range peers {
		if peers[i].ID == node.ID {
			existingIDIdx = i
			break
		}
	}

	// In pairing path, detect conflict if an existing peer already uses this ID at a different IP address
	// (e.g. if a peer previously at this address was moved to another IP via update_node)
	if isPairing && existingIDIdx >= 0 && node.IP != "" && !strings.EqualFold(peers[existingIDIdx].IP, node.IP) {
		return fmt.Errorf("node ID %s conflicts with existing peer %s (%s)", node.ID, peers[existingIDIdx].Name, peers[existingIDIdx].IP)
	}

	targetIdx := existingIDIdx
	if node.IP != "" {
		for i := range peers {
			if strings.EqualFold(peers[i].IP, node.IP) {
				if existingIDIdx >= 0 && existingIDIdx != i {
					// An existing peer is being updated to take another peer's IP
					return fmt.Errorf("IP address %s is already used by node %s (%s)", node.IP, peers[i].Name, peers[i].ID)
				}
				if existingIDIdx < 0 {
					// Re-pairing an existing IP with a new/changed node ID: replace the entry at this IP
					targetIdx = i
				}
				break
			}
		}
	}

	if targetIdx >= 0 {
		old := peers[targetIdx]
		if existingIDIdx >= 0 {
			if !clearToken && node.APIToken == "" && old.APIToken != "" {
				node.APIToken = old.APIToken
			}
			if !clearPassword && node.Password == "" && old.Password != "" {
				node.Password = old.Password
			}
		}
		peers[targetIdx] = node
	} else {
		peers = append(peers, node)
	}
	if err := savePeers(peers); err != nil {
		return err
	}
	nodeMutex.Lock()
	remoteNodes = peers
	nodeMutex.Unlock()
	return nil
}

func persistPeerFingerprint(nodeID, expectedIP, expectedToken, expectedPass, fingerprint string) error {
	if fingerprint == "" {
		return nil
	}
	peerMutationMu.Lock()
	defer peerMutationMu.Unlock()
	peers := loadPeers()
	var modified bool
	for i := range peers {
		if peers[i].ID == nodeID {
			if peers[i].IP != expectedIP || peers[i].APIToken != expectedToken || peers[i].Password != expectedPass {
				// Configuration changed while request in flight; discard stale pin update
				return nil
			}
			if peers[i].Fingerprint == "" {
				peers[i].Fingerprint = fingerprint
				modified = true
			}
			break
		}
	}
	if !modified {
		return nil
	}
	if err := savePeers(peers); err != nil {
		return err
	}
	nodeMutex.Lock()
	for i := range remoteNodes {
		if remoteNodes[i].ID == nodeID && remoteNodes[i].Fingerprint == "" {
			remoteNodes[i].Fingerprint = fingerprint
		}
	}
	nodeMutex.Unlock()
	return nil
}

func persistPeerRemoval(nodeID string) (bool, error) {
	peerMutationMu.Lock()
	defer peerMutationMu.Unlock()
	peers := snapshotPeers()
	updated := make([]MeshNode, 0, len(peers))
	found := false
	for _, peer := range peers {
		if peer.ID == nodeID {
			found = true
			continue
		}
		updated = append(updated, peer)
	}
	if !found {
		return false, nil
	}
	if err := savePeers(updated); err != nil {
		return false, err
	}
	nodeMutex.Lock()
	remoteNodes = updated
	nodeMutex.Unlock()
	return true, nil
}

func snapshotPeers() []MeshNode {
	nodeMutex.RLock()
	defer nodeMutex.RUnlock()
	return append([]MeshNode(nil), remoteNodes...)
}

func refreshRemotePeers() {
	cfg := loadMeshConfig()
	peers := snapshotPeers()

	var wg sync.WaitGroup
	for _, existing := range peers {
		existing := existing
		wg.Add(1)
		go func() {
			defer wg.Done()
			updated, pwOnly, err := connectAndNegotiateNode(existing.IP, existing.APIToken, existing.Password, cfg, existing.Fingerprint, existing.PasswordOnly)
			var pinToPersist string
			nodeMutex.Lock()
			for i := range remoteNodes {
				if remoteNodes[i].ID == existing.ID {
					current := remoteNodes[i]
					if current.IP != existing.IP || current.APIToken != existing.APIToken || current.Password != existing.Password {
						// Configuration changed while request in flight; discard stale result
						break
					}
					name, role := current.Name, current.Role
					if err == nil {
						updated.ID, updated.Name, updated.Role = existing.ID, name, role
						updated.IP = current.IP
						updated.APIToken = current.APIToken
						updated.Password = current.Password
						updated.PasswordOnly = pwOnly
						if updated.Fingerprint == "" && current.Fingerprint != "" {
							updated.Fingerprint = current.Fingerprint
						}
						remoteNodes[i] = updated
						if current.Fingerprint == "" && updated.Fingerprint != "" {
							pinToPersist = updated.Fingerprint
						}
					} else {
						remoteNodes[i].Status = "offline"
						if cfg.IsPairingTokenRequired() && existing.APIToken == "" && existing.Password == "" && cfg.PairingToken == "" {
							remoteNodes[i].LastSeen = "authentication required"
						} else {
							remoteNodes[i].LastSeen = "unreachable"
						}
					}
					break
				}
			}
			nodeMutex.Unlock()
			if pinToPersist != "" {
				_ = persistPeerFingerprint(existing.ID, existing.IP, existing.APIToken, existing.Password, pinToPersist)
			}
		}()
	}
	wg.Wait()
}

var (
	meshShutdownOnce      sync.Once
	meshServer            *http.Server
	meshMDNSServer        *zeroconf.Server
	meshServerMu          sync.Mutex
	meshChallengeMu       sync.Mutex
	meshChallenges        = make(map[string]meshChallengeRecord)
	meshCertFingerprintMu sync.RWMutex
	meshCertFingerprint   string
)

type meshChallengeRecord struct {
	expires time.Time
	client  string
}

func issueMeshChallenge(client string) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(bytes)
	now := time.Now()
	meshChallengeMu.Lock()
	defer meshChallengeMu.Unlock()
	var clientCount int
	var oldestClientNonce string
	var oldestClientExpiry time.Time
	var oldestNonce string
	var oldestExpiry time.Time
	for known, challenge := range meshChallenges {
		if now.After(challenge.expires) {
			delete(meshChallenges, known)
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
		delete(meshChallenges, oldestClientNonce)
	}
	if len(meshChallenges) >= 4096 && oldestNonce != "" {
		delete(meshChallenges, oldestNonce)
	}
	meshChallenges[nonce] = meshChallengeRecord{expires: now.Add(30 * time.Second), client: client}
	return nonce, nil
}

func consumeMeshChallenge(nonce string, client ...string) bool {
	if !validMeshNonce(nonce) {
		return false
	}
	meshChallengeMu.Lock()
	challenge, ok := meshChallenges[nonce]
	if ok {
		delete(meshChallenges, nonce)
	}
	meshChallengeMu.Unlock()
	if !ok || time.Now().After(challenge.expires) {
		return false
	}
	if len(client) > 0 && client[0] != "" && challenge.client != "" && challenge.client != client[0] {
		return false
	}
	return true
}

func pairRequestProof(token, nonce, clientNonce string, certFingerprint ...string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = io.WriteString(mac, "POST\n/api/v1/pair\n"+nonce+"\n"+clientNonce)
	if len(certFingerprint) > 0 && certFingerprint[0] != "" {
		_, _ = io.WriteString(mac, "\n"+certFingerprint[0])
	}
	return hex.EncodeToString(mac.Sum(nil))
}

const (
	meshPassMaxFails   = 5
	meshPassLockout    = 60 * time.Second
	meshPassTrackerMax = 4096
)

type meshPassAttempt struct {
	fails      int
	lockedTill time.Time
}

var (
	meshPassMu    sync.Mutex
	meshPassFails = make(map[string]*meshPassAttempt)
)

func meshPasswordLocked(client string) bool {
	meshPassMu.Lock()
	defer meshPassMu.Unlock()
	a, ok := meshPassFails[client]
	return ok && time.Now().Before(a.lockedTill)
}

func meshPasswordFail(client string) {
	meshPassMu.Lock()
	defer meshPassMu.Unlock()
	if len(meshPassFails) >= meshPassTrackerMax {
		now := time.Now()
		for ip, a := range meshPassFails {
			if now.After(a.lockedTill) && a.fails < meshPassMaxFails {
				delete(meshPassFails, ip)
			}
		}
	}
	a := meshPassFails[client]
	if a == nil {
		a = &meshPassAttempt{}
		meshPassFails[client] = a
	}
	a.fails++
	if a.fails >= meshPassMaxFails {
		a.lockedTill = time.Now().Add(meshPassLockout)
	}
}

func meshPasswordSuccess(client string) {
	meshPassMu.Lock()
	defer meshPassMu.Unlock()
	delete(meshPassFails, client)
}

func setMeshFingerprint(der []byte) {
	sum := sha256.Sum256(der)
	meshCertFingerprintMu.Lock()
	meshCertFingerprint = hex.EncodeToString(sum[:])
	meshCertFingerprintMu.Unlock()
}

// loadOrCreateMeshCertificate keeps a stable TLS identity across restarts so
// peers' pinned fingerprints stay valid. Rotation requires re-pairing.
func loadOrCreateMeshCertificate() (tls.Certificate, error) {
	dir := meshConfigBaseDir()
	certPath := filepath.Join(dir, "tls_cert.pem")
	keyPath := filepath.Join(dir, "tls_key.pem")
	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil && len(cert.Certificate) > 0 {
		if leaf, err := x509.ParseCertificate(cert.Certificate[0]); err == nil && time.Now().Before(leaf.NotAfter) {
			setMeshFingerprint(cert.Certificate[0])
			return cert, nil
		}
	}
	cert, err := createMeshCertificate()
	if err != nil {
		return cert, err
	}
	ecKey, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return cert, nil
	}
	keyDER, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		return cert, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "m3tal-mesh: cannot persist TLS identity: %v\n", err)
		return cert, nil
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "m3tal-mesh: cannot persist TLS key: %v\n", err)
		return cert, nil
	}
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "m3tal-mesh: cannot persist TLS cert: %v\n", err)
	}
	return cert, nil
}

func createMeshCertificate() (tls.Certificate, error) {
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
		hostname = "m3tal-mesh"
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: hostname},
		DNSNames:              []string{hostname},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	setMeshFingerprint(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

func meshHealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hostname, _ := os.Hostname()
	body, err := json.Marshal(map[string]any{
		"name":      hostname,
		"hostname":  hostname,
		"version":   PluginVersion,
		"status":    "online",
	})
	if err != nil {
		http.Error(w, "could not encode health data", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func meshChallengeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		clientIP = r.RemoteAddr
	}
	nonce, err := issueMeshChallenge(clientIP)
	if err != nil {
		http.Error(w, "could not create challenge", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]string{"nonce": nonce}
	meshCertFingerprintMu.RLock()
	fp := meshCertFingerprint
	meshCertFingerprintMu.RUnlock()
	if fp != "" {
		resp["fingerprint"] = fp
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func meshTelemetryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	current := loadMeshConfig()
	acceptedTokens := current.acceptedTokens()
	expectedPass := current.DefaultPassword
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
		meshCertFingerprintMu.RLock()
		fp = meshCertFingerprint
		meshCertFingerprintMu.RUnlock()
	}

	matchedToken := ""
	challengeConsumed := false
	if len(acceptedTokens) > 0 {
		if suppliedProof == "" || nonce == "" || clientNonce == "" {
			http.Error(w, "pairing token required", http.StatusUnauthorized)
			return
		}

		clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			clientIP = r.RemoteAddr
		}

		if !validMeshNonce(clientNonce) || !consumeMeshChallenge(nonce, clientIP) {
			http.Error(w, "invalid pairing challenge", http.StatusUnauthorized)
			return
		}
		challengeConsumed = true

		for _, tok := range acceptedTokens {
			if subtle.ConstantTimeCompare([]byte(suppliedProof), []byte(requestProof(tok, nonce, clientNonce, fp))) == 1 {
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
		if meshPasswordLocked(clientIP) {
			http.Error(w, "too many failed authentication attempts", http.StatusTooManyRequests)
			return
		}
		authed := false
		if passProof := r.Header.Get("X-M3TAL-Password-Proof"); passProof != "" {
			if nonce == "" || clientNonce == "" || !validMeshNonce(clientNonce) ||
				(!challengeConsumed && !consumeMeshChallenge(nonce, clientIP)) {
				meshPasswordFail(clientIP)
				http.Error(w, "invalid pairing challenge", http.StatusUnauthorized)
				return
			}
			expected := passwordRequestProof(expectedPass, nonce, clientNonce, fp)
			if subtle.ConstantTimeCompare([]byte(passProof), []byte(expected)) == 1 {
				authed = true
				passwordProofAuthed = true
			}
		} else {
			suppliedPass := r.Header.Get("X-M3TAL-Password")
			if suppliedPass == "" {
				w.Header().Set("X-M3TAL-Auth-Status", "password_required")
				if suppliedProof != "" && matchedToken != "" {
					w.Header().Set("X-M3TAL-Proof", responseProof(matchedToken, nonce, clientNonce, []byte("password required\n"), fp))
				}
				http.Error(w, "password required", http.StatusUnauthorized)
				return
			}
			if subtle.ConstantTimeCompare([]byte(suppliedPass), []byte(expectedPass)) == 1 {
				authed = true
			}
		}
		if !authed {
			meshPasswordFail(clientIP)
			http.Error(w, "invalid password", http.StatusUnauthorized)
			return
		}
		meshPasswordSuccess(clientIP)
	}

	hostname, _ := os.Hostname()
	body, err := json.Marshal(map[string]any{
		"name":      hostname,
		"hostname":  hostname,
		"version":   PluginVersion,
		"telemetry": getLocalTelemetry(),
	})
	if err != nil {
		http.Error(w, "could not encode telemetry", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if matchedToken != "" {
		w.Header().Set("X-M3TAL-Proof", responseProof(matchedToken, nonce, clientNonce, body, fp))
	}
	if passwordProofAuthed && nonce != "" && clientNonce != "" {
		w.Header().Set("X-M3TAL-Password-Auth-Proof", passwordResponseProof(expectedPass, nonce, clientNonce, body, fp))
	}
	_, _ = w.Write(body)
}

type meshPairRequest struct {
	Name        string `json:"name"`
	Hostname    string `json:"hostname"`
	IP          string `json:"ip"`
	Role        string `json:"role"`
	Token       string `json:"token,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

func meshPairHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	current := loadMeshConfig()
	acceptedTokens := current.acceptedTokens()
	expectedPass := current.DefaultPassword
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
		meshCertFingerprintMu.RLock()
		fp = meshCertFingerprint
		meshCertFingerprintMu.RUnlock()
	}

	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		clientIP = r.RemoteAddr
	}

	matchedToken := ""
	tokenOK := len(acceptedTokens) == 0
	passOK := expectedPass == ""

	// Consume the single-use challenge exactly once for this request.
	challengeOK := nonce != "" && clientNonce != "" && validMeshNonce(clientNonce) && consumeMeshChallenge(nonce, clientIP)

	if !tokenOK && challengeOK && suppliedProof != "" {
		for _, tok := range acceptedTokens {
			pairProof := pairRequestProof(tok, nonce, clientNonce, fp)
			genProof := requestProof(tok, nonce, clientNonce, fp)
			if subtle.ConstantTimeCompare([]byte(suppliedProof), []byte(pairProof)) == 1 ||
				subtle.ConstantTimeCompare([]byte(suppliedProof), []byte(genProof)) == 1 {
				tokenOK = true
				matchedToken = tok
				break
			}
		}
	}

	if !passOK {
		if meshPasswordLocked(clientIP) {
			http.Error(w, "too many failed authentication attempts", http.StatusTooManyRequests)
			return
		}
		if passProof := r.Header.Get("X-M3TAL-Password-Proof"); passProof != "" && challengeOK {
			expected := passwordRequestProof(expectedPass, nonce, clientNonce, fp)
			passOK = subtle.ConstantTimeCompare([]byte(passProof), []byte(expected)) == 1
		}
		if passOK {
			meshPasswordSuccess(clientIP)
		} else {
			meshPasswordFail(clientIP)
		}
	}

	// Every configured credential must be satisfied independently.
	authed := tokenOK && passOK
	if authRequired && !authed {
		http.Error(w, "invalid pairing credentials", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "cannot read request body", http.StatusBadRequest)
		return
	}

	var pairReq meshPairRequest
	if err := json.Unmarshal(body, &pairReq); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	peerIP := pairReq.IP
	if peerIP == "" {
		peerIP = net.JoinHostPort(clientIP, strconv.Itoa(current.ListenPort))
	} else {
		if norm, err := normalizeNodeAddress(peerIP, current.ListenPort); err == nil {
			peerIP = norm
		}
	}

	peerHost := pairReq.Hostname
	if peerHost == "" {
		peerHost = pairReq.Name
	}
	if peerHost == "" {
		peerHost = clientIP
	}
	peerName := pairReq.Name
	if peerName == "" {
		peerName = peerHost
	}
	peerRole := pairReq.Role
	if peerRole == "" {
		peerRole = "server"
	}

	peerToken := pairReq.Token
	if peerToken == "" {
		peerToken = matchedToken
	}

	newNode := MeshNode{
		ID:          stableNodeID(peerIP, peerHost),
		Name:        peerName,
		IP:          peerIP,
		Role:        peerRole,
		Status:      "online",
		Fingerprint: pairReq.Fingerprint,
		APIToken:    peerToken,
		LastSeen:    "just now",
	}

	if err := persistPairedPeer(newNode); err != nil {
		http.Error(w, fmt.Sprintf("could not register peer: %v", err), http.StatusInternalServerError)
		return
	}

	myHost, _ := os.Hostname()
	if myHost == "" {
		myHost = "m3tal-mesh"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"message":     "Node registered successfully in bidirectional mesh",
		"name":        myHost,
		"hostname":    myHost,
		"fingerprint": fp,
	})
}

func startMeshNetworkService() error {
	cfg := loadMeshConfig()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", meshHealthHandler)
	mux.HandleFunc("/api/v1/challenge", meshChallengeHandler)
	mux.HandleFunc("/api/v1/telemetry", meshTelemetryHandler)
	mux.HandleFunc("/api/v1/pair", meshPairHandler)

	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.ListenPort),
		Handler:           mux,
		ReadHeaderTimeout: 4 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       45 * time.Second,
		MaxHeaderBytes:    32 * 1024,
	}

	certificate, err := loadOrCreateMeshCertificate()
	if err != nil {
		return fmt.Errorf("mesh TLS certificate: %w", err)
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	}

	meshServerMu.Lock()
	meshServer = server
	meshServerMu.Unlock()

	// Retry binding with backoff so a legacy m3tal-mesh-node holding the port
	// does not permanently disable the unified broadcaster.
	go func() {
		backoff := 2 * time.Second
		for {
			listener, err := tls.Listen("tcp", server.Addr, tlsCfg)
			if err == nil {
				if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
					fmt.Fprintf(os.Stderr, "m3tal-mesh HTTP service: %v\n", err)
				}
				return
			}
			fmt.Fprintf(os.Stderr, "m3tal-mesh HTTP service port %d: %v (retrying in %s)\n", cfg.ListenPort, err, backoff)
			time.Sleep(backoff)
			if backoff < time.Minute {
				backoff *= 2
			}
		}
	}()

	if cfg.IsMDNSEnabled() {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "m3tal-mesh"
		}
		mdnsServer, err := zeroconf.Register(hostname, meshServiceType, "local.", cfg.ListenPort, []string{"version=" + PluginVersion, "protocol=1"}, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "m3tal-mesh mDNS registration: %v\n", err)
		} else {
			meshServerMu.Lock()
			meshMDNSServer = mdnsServer
			meshServerMu.Unlock()
		}
	}
	return nil
}

func shutdownMeshNetwork() {
	meshShutdownOnce.Do(func() {
		meshServerMu.Lock()
		server, mdnsServer := meshServer, meshMDNSServer
		meshServerMu.Unlock()
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

func outboundIPFor(targetHost string) string {
	conn, err := net.DialTimeout("udp", targetHost, 2*time.Second)
	if err != nil {
		return getLocalIP()
	}
	defer conn.Close()
	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || localAddr == nil || localAddr.IP.IsUnspecified() || localAddr.IP.IsLoopback() {
		return getLocalIP()
	}
	return localAddr.IP.String()
}

func dispatchBidirectionalPair(remoteAddress, token, password, pinnedFP string, passwordOnly bool, cfg meshConfig) error {
	if pinnedFP == "" {
		return fmt.Errorf("refusing reverse pairing without an authenticated peer fingerprint")
	}
	endpoint, err := normalizeNodeAddress(remoteAddress, cfg.ListenPort)
	if err != nil {
		return err
	}
	// Pin every request to the fingerprint authenticated during initial pairing.
	client := newPinnedHTTPClient(pinnedFP, 4*time.Second)
	remoteFP := pinnedFP
	challengeReq, err := http.NewRequest(http.MethodPost, "https://"+endpoint+"/api/v1/challenge", nil)
	if err != nil {
		return err
	}
	challengeResp, err := client.Do(challengeReq)
	if err != nil {
		return err
	}
	defer challengeResp.Body.Close()

	if challengeResp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote node challenge error: HTTP %d", challengeResp.StatusCode)
	}

	var challenge struct {
		Nonce string `json:"nonce"`
	}
	if err := json.NewDecoder(io.LimitReader(challengeResp.Body, 4096)).Decode(&challenge); err != nil || !validMeshNonce(challenge.Nonce) {
		return fmt.Errorf("invalid challenge from %s", endpoint)
	}

	cNonce, err := newMeshNonce()
	if err != nil {
		return err
	}

	myHost, _ := os.Hostname()
	if myHost == "" {
		myHost = "m3tal-mesh"
	}
	localOutboundIP := outboundIPFor(endpoint)
	myAddress := net.JoinHostPort(localOutboundIP, strconv.Itoa(cfg.ListenPort))

	meshCertFingerprintMu.RLock()
	myFP := meshCertFingerprint
	meshCertFingerprintMu.RUnlock()

	// Only offer a return credential this node actually accepts inbound;
	// never forward the remote peer's key back as a raw secret.
	reqPayload := meshPairRequest{
		Name:        myHost,
		Hostname:    myHost,
		IP:          myAddress,
		Role:        "workstation",
		Token:       cfg.PairingToken,
		Fingerprint: myFP,
	}
	body, err := json.Marshal(reqPayload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, "https://"+endpoint+"/api/v1/pair", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-M3TAL-Nonce", challenge.Nonce)
	req.Header.Set("X-M3TAL-Client-Nonce", cNonce)

	if token != "" && !passwordOnly {
		req.Header.Set("X-M3TAL-Proof", pairRequestProof(token, challenge.Nonce, cNonce, remoteFP))
	}
	if password != "" {
		req.Header.Set("X-M3TAL-Password-Proof", passwordRequestProof(password, challenge.Nonce, cNonce, remoteFP))
	}

	pairResp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer pairResp.Body.Close()
	if pairResp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote node pair error: HTTP %d", pairResp.StatusCode)
	}
	return nil
}

