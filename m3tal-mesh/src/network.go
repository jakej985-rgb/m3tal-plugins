package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	PairingToken string `json:"pairing_token"`
	ListenPort   int    `json:"listen_port"`
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
	cfg := meshConfig{ListenPort: 9321}
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

func loadPeers() []MeshNode {
	path := peersPath()
	if path == "" {
		return []MeshNode{}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return []MeshNode{}
	}
	var peers []MeshNode
	if json.Unmarshal(b, &peers) != nil || peers == nil {
		return []MeshNode{}
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
	b, err := json.MarshalIndent(peers, "", "  ")
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
	if strings.Count(address, ":") > 1 && net.ParseIP(address) != nil {
		return net.JoinHostPort(address, strconv.Itoa(defaultPort)), nil
	}
	return net.JoinHostPort(address, strconv.Itoa(defaultPort)), nil
}

func fetchRemote(address, token string, port int) (MeshNode, error) {
	if token == "" {
		return MeshNode{}, fmt.Errorf("set the pairing token in Mesh Settings first")
	}
	endpoint, err := normalizeNodeAddress(address, port)
	if err != nil {
		return MeshNode{}, err
	}
	client := &http.Client{Timeout: 4 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "http://"+endpoint+"/api/v1/telemetry", nil)
	if err != nil {
		return MeshNode{}, fmt.Errorf("invalid node address: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	started := time.Now()
	resp, err := client.Do(req)
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
	var payload remoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
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
		Cores: t.Cores, CPULoad: cpuload, RAMUsedGB: t.RAMUsedGB, RAMTotalGB: t.RAMTotalGB,
		StorageUsedGB: t.StorageUsedGB, StorageTotalGB: t.StorageTotalGB, StoragePool: "Root Filesystem",
		PingMs: latency, LastSeen: "just now", IsLocal: false,
	}, nil
}

func pairAndPersistPeer(address, name, role, token string) (MeshNode, error) {
	cfg := loadMeshConfig()
	if token == "" {
		token = cfg.PairingToken
	}
	node, err := fetchRemote(address, token, cfg.ListenPort)
	if err != nil {
		return MeshNode{}, err
	}
	if name != "" {
		node.Name = name
	}
	if role != "" {
		node.Role = role
	}
	if token != cfg.PairingToken {
		cfg.PairingToken = token
		if err := saveMeshConfig(cfg); err != nil {
			return MeshNode{}, fmt.Errorf("could not save pairing token: %w", err)
		}
	}
	upsertPeer(node)
	if err := savePeers(snapshotPeers()); err != nil {
		return MeshNode{}, fmt.Errorf("could not save paired node: %w", err)
	}
	return node, nil
}

func discoverAndVerifyPeers() ([]MeshNode, error) {
	cfg := loadMeshConfig()
	if cfg.PairingToken == "" {
		return nil, fmt.Errorf("set the pairing token in Mesh Settings before scanning")
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
	var discovered []MeshNode
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
				node, err := fetchRemote(address, cfg.PairingToken, cfg.ListenPort)
				if err == nil {
					discovered = append(discovered, node)
				}
			}
		case <-ctx.Done():
			return discovered, nil
		}
	}
}

func snapshotPeers() []MeshNode {
	nodeMutex.RLock()
	defer nodeMutex.RUnlock()
	return append([]MeshNode(nil), remoteNodes...)
}

func upsertPeer(node MeshNode) {
	nodeMutex.Lock()
	defer nodeMutex.Unlock()
	for i := range remoteNodes {
		if remoteNodes[i].ID == node.ID || strings.EqualFold(remoteNodes[i].IP, node.IP) {
			remoteNodes[i] = node
			return
		}
	}
	remoteNodes = append(remoteNodes, node)
}

func refreshRemotePeers() {
	cfg := loadMeshConfig()
	peers := snapshotPeers()
	for _, existing := range peers {
		updated, err := fetchRemote(existing.IP, cfg.PairingToken, cfg.ListenPort)
		nodeMutex.Lock()
		for i := range remoteNodes {
			if remoteNodes[i].ID == existing.ID {
				if err == nil {
					updated.ID = existing.ID
					updated.Name = existing.Name
					updated.Role = existing.Role
					remoteNodes[i] = updated
				} else {
					remoteNodes[i].Status = "offline"
					remoteNodes[i].LastSeen = "unreachable"
				}
				break
			}
		}
		nodeMutex.Unlock()
	}
}
