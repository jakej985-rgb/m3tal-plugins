package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
)

const meshServiceType = "_m3tal-mesh-node._tcp"

type meshConfig struct {
	PairingToken        string `json:"pairing_token,omitempty"`
	DefaultPassword     string `json:"default_password,omitempty"`
	RequirePairingToken *bool  `json:"require_pairing_token,omitempty"`
	ListenPort          int    `json:"listen_port"`
	DiscoveryEnabled    *bool  `json:"discovery_enabled,omitempty"`
}

func (c *meshConfig) UnmarshalJSON(data []byte) error {
	type Alias meshConfig
	var aux struct {
		Alias
		DefaultAPIToken string `json:"default_api_token,omitempty"`
		APIToken        string `json:"api_token,omitempty"`
		RequireAuth     *bool  `json:"require_auth,omitempty"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*c = meshConfig(aux.Alias)
	if c.PairingToken == "" {
		if aux.APIToken != "" {
			c.PairingToken = aux.APIToken
		} else if aux.DefaultAPIToken != "" {
			c.PairingToken = aux.DefaultAPIToken
		}
	}
	if c.RequirePairingToken == nil && aux.RequireAuth != nil {
		c.RequirePairingToken = aux.RequireAuth
	}
	return nil
}

func (c meshConfig) GetDefaultAPIToken() string {
	return c.PairingToken
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

func meshConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "m3tal", "plugins", "mesh", "config.json")
}

func saveMeshConfig(cfg meshConfig) error {
	path := meshConfigPath()
	if path == "" {
		return fmt.Errorf("cannot locate the user configuration directory")
	}
	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		return fmt.Errorf("default node port must be between 1 and 65535")
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
	return cfg
}

func peersPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "m3tal", "plugins", "mesh", "peers.json")
}

type peerDiskRecord struct {
	MeshNode
	APIToken string `json:"api_token,omitempty"`
	Password string `json:"password,omitempty"`
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
		peers[i].HasPassword = peers[i].Password != ""
		peers[i].HasAPIToken = peers[i].APIToken != ""
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
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	records := make([]peerDiskRecord, len(peers))
	for i, p := range peers {
		records[i] = peerDiskRecord{
			MeshNode: p,
			APIToken: p.APIToken,
			Password: p.Password,
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

func fetchRemote(address, token string, port int, expectedFingerprint ...string) (MeshNode, error) {
	return fetchRemoteWithAuth(address, token, "", port, expectedFingerprint...)
}

func fetchRemoteWithAuth(address, token, password string, port int, expectedFingerprint ...string) (MeshNode, error) {
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

	if token != "" || (len(expectedFingerprint) > 0 && expectedFingerprint[0] != "") {
		challengeReq, err := http.NewRequest(http.MethodPost, "https://"+endpoint+"/api/v1/challenge", nil)
		if err != nil {
			return MeshNode{}, fmt.Errorf("invalid node address: %w", err)
		}
		challengeResp, err := meshHTTPClient.Do(challengeReq)
		if err != nil {
			return MeshNode{}, fmt.Errorf("cannot reach %s: %w", address, err)
		}
		defer challengeResp.Body.Close()
		if challengeResp.TLS != nil && len(challengeResp.TLS.PeerCertificates) > 0 {
			sum := sha256.Sum256(challengeResp.TLS.PeerCertificates[0].Raw)
			peerFingerprint = hex.EncodeToString(sum[:])
		}
		if len(expectedFingerprint) > 0 && expectedFingerprint[0] != "" {
			if peerFingerprint != "" && peerFingerprint != expectedFingerprint[0] {
				return MeshNode{}, fmt.Errorf("%s certificate fingerprint mismatch: expected %s, got %s", address, expectedFingerprint[0], peerFingerprint)
			}
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
	if token != "" && nonce != "" && clientNonce != "" {
		req.Header.Set("X-M3TAL-Nonce", nonce)
		req.Header.Set("X-M3TAL-Client-Nonce", clientNonce)
		req.Header.Set("X-M3TAL-Proof", requestProof(token, nonce, clientNonce, peerFingerprint))
	}
	if password != "" {
		req.Header.Set("X-M3TAL-Password", password)
	}
	resp, err := meshHTTPClient.Do(req)
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		return MeshNode{}, fmt.Errorf("cannot reach %s: %w", address, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return MeshNode{}, fmt.Errorf("pairing token was rejected by %s", address)
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return MeshNode{}, fmt.Errorf("pairing token is not configured on %s", address)
	}
	if resp.StatusCode != http.StatusOK {
		return MeshNode{}, fmt.Errorf("%s is not a compatible m3tal-mesh-node (HTTP %d)", address, resp.StatusCode)
	}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		sum := sha256.Sum256(resp.TLS.PeerCertificates[0].Raw)
		telemetryFP := hex.EncodeToString(sum[:])
		if peerFingerprint != "" && telemetryFP != peerFingerprint {
			return MeshNode{}, fmt.Errorf("%s certificate fingerprint changed during connection", address)
		}
		peerFingerprint = telemetryFP
	}
	if len(expectedFingerprint) > 0 && expectedFingerprint[0] != "" {
		if peerFingerprint != "" && peerFingerprint != expectedFingerprint[0] {
			return MeshNode{}, fmt.Errorf("%s certificate fingerprint mismatch: expected %s, got %s", address, expectedFingerprint[0], peerFingerprint)
		}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return MeshNode{}, fmt.Errorf("could not read telemetry from %s: %w", address, err)
	}
	if token != "" && nonce != "" && clientNonce != "" {
		expected := responseProof(token, nonce, clientNonce, body, peerFingerprint)
		if !hmac.Equal([]byte(expected), []byte(resp.Header.Get("X-M3TAL-Proof"))) {
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
		HasPassword: password != "",
		HasAPIToken: token != "",
		Cores: t.Cores, CPULoad: cpuload, RAMUsedGB: t.RAMUsedGB, RAMTotalGB: t.RAMTotalGB,
		StorageUsedGB: t.StorageUsedGB, StorageTotalGB: t.StorageTotalGB, StoragePool: "Root Filesystem",
		PingMs: latency, LastSeen: "just now", IsLocal: false,
	}, nil
}

func pairAndPersistPeer(address, name, role, token, password string) (MeshNode, error) {
	cfg := loadMeshConfig()
	pass := password
	if token == "" {
		token = cfg.GetDefaultAPIToken()
	}
	if pass == "" && cfg.DefaultPassword != "" {
		pass = cfg.DefaultPassword
	}
	if cfg.IsPairingTokenRequired() && token == "" && pass == "" {
		return MeshNode{}, fmt.Errorf("pairing token is required to authenticate remote mesh nodes; configure a shared secret in Mesh Settings or enter a pairing token")
	}
	if cfg.PairingToken != "" && token != "" && token != cfg.PairingToken && pass == "" {
		return MeshNode{}, fmt.Errorf("this mesh uses one shared pairing token; configure the same token in Mesh Settings before pairing")
	}
	node, err := fetchRemoteWithAuth(address, token, pass, cfg.ListenPort)
	if err != nil {
		return MeshNode{}, err
	}
	if name != "" {
		node.Name = name
	}
	if role != "" {
		node.Role = role
	}
	node.APIToken = token
	node.Password = pass
	node.HasPassword = pass != ""
	node.HasAPIToken = token != ""

	if cfg.PairingToken == "" && token != "" {
		cfg.PairingToken = token
		if err := saveMeshConfig(cfg); err != nil {
			return MeshNode{}, fmt.Errorf("could not save pairing token: %w", err)
		}
	}
	if err := persistPeerUpsert(node); err != nil {
		return MeshNode{}, fmt.Errorf("could not save paired node: %w", err)
	}
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
			}
		case <-ctx.Done():
			return verifyCandidates(candidates, cfg.PairingToken, cfg.ListenPort), nil
		}
	}
}

func verifyCandidates(candidates []string, token string, port int) []MeshNode {
	var wg sync.WaitGroup
	mu := sync.Mutex{}
	discovered := make([]MeshNode, 0)
	for _, address := range candidates {
		address := address
		wg.Add(1)
		go func() {
			defer wg.Done()
			node, err := fetchRemote(address, token, port)
			if err == nil {
				mu.Lock()
				discovered = append(discovered, node)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return discovered
}

func persistPeerUpsert(node MeshNode) error {
	peerMutationMu.Lock()
	defer peerMutationMu.Unlock()
	peers := snapshotPeers()
	updated := false
	for i := range peers {
		if peers[i].ID == node.ID || strings.EqualFold(peers[i].IP, node.IP) {
			if !node.ClearAPIToken && node.APIToken == "" && peers[i].APIToken != "" {
				node.APIToken = peers[i].APIToken
			}
			if !node.ClearPassword && node.Password == "" && peers[i].Password != "" {
				node.Password = peers[i].Password
			}
			node.HasPassword = node.Password != ""
			node.HasAPIToken = node.APIToken != ""
			peers[i] = node
			updated = true
			break
		}
	}
	if !updated {
		node.HasPassword = node.Password != ""
		node.HasAPIToken = node.APIToken != ""
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
	defaultToken := cfg.GetDefaultAPIToken()

	var wg sync.WaitGroup
	for _, existing := range peers {
		existing := existing
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok := existing.APIToken
			if tok == "" {
				tok = defaultToken
			}
			pass := existing.Password
			if pass == "" {
				pass = cfg.DefaultPassword
			}
			if cfg.IsPairingTokenRequired() && tok == "" && pass == "" {
				nodeMutex.Lock()
				for i := range remoteNodes {
					if remoteNodes[i].ID == existing.ID {
						remoteNodes[i].Status = "offline"
						remoteNodes[i].LastSeen = "authentication required"
						break
					}
				}
				nodeMutex.Unlock()
				return
			}

			updated, err := fetchRemoteWithAuth(existing.IP, tok, pass, cfg.ListenPort, existing.Fingerprint)
			nodeMutex.Lock()
			defer nodeMutex.Unlock()
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
						updated.HasPassword = current.HasPassword
						updated.HasAPIToken = current.HasAPIToken
						remoteNodes[i] = updated
					} else {
						remoteNodes[i].Status = "offline"
						remoteNodes[i].LastSeen = "unreachable"
					}
					break
				}
			}
		}()
	}
	wg.Wait()
}
