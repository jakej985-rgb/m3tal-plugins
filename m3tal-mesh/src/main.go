package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	CanonicalProcessName = "m3tal-mesh"
	PluginName           = "mesh"
	PluginVersion        = "0.1.4"
)

// RPCRequest represents standard JSON-RPC 2.0 request.
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCResponse represents standard JSON-RPC 2.0 response.
type RPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents standard JSON-RPC 2.0 error.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MeshNode represents a machine connected to the personal cluster.
type MeshNode struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	IP               string  `json:"ip"`
	Role             string  `json:"role"`   // "workstation", "server", "nas", "edge"
	Status           string  `json:"status"` // "online", "warning", "offline", "unverified"
	PasswordRequired bool    `json:"password_required,omitempty"`
	PasswordOnly     bool    `json:"-"`
	Fingerprint      string  `json:"fingerprint,omitempty"`
	APIToken         string  `json:"-"`
	Password         string  `json:"-"`
	Cores            int     `json:"cores"`
	CPULoad          float64 `json:"cpu_load"`
	RAMUsedGB        float64 `json:"ram_used_gb"`
	RAMTotalGB       float64 `json:"ram_total_gb"`
	StorageUsedGB    float64 `json:"storage_used_gb"`
	StorageTotalGB   float64 `json:"storage_total_gb"`
	StoragePool      string  `json:"storage_pool,omitempty"`
	GPUName          string  `json:"gpu_name,omitempty"`
	GPUVRAMUsedGB    float64 `json:"gpu_vram_used_gb,omitempty"`
	GPUVRAMTotalGB   float64 `json:"gpu_vram_total_gb,omitempty"`
	GPUTempC         int     `json:"gpu_temp_c,omitempty"`
	PingMs           int     `json:"ping_ms"`
	LastSeen         string  `json:"last_seen"`
	IsLocal          bool    `json:"is_local"`
}

// MarshalJSON derives has_password and has_api_token dynamically at marshal time.
func (n MeshNode) MarshalJSON() ([]byte, error) {
	type Alias MeshNode
	return json.Marshal(&struct {
		Alias
		HasPassword bool `json:"has_password"`
		HasAPIToken bool `json:"has_api_token"`
	}{
		Alias:       Alias(n),
		HasPassword: n.Password != "",
		HasAPIToken: n.APIToken != "",
	})
}

var (
	nodeMutex   sync.RWMutex
	remoteNodes = make([]MeshNode, 0)
	refreshMu   sync.Mutex
)

func setProcessName(name string) {
	bytes := append([]byte(name), 0)
	// PR_SET_NAME = 15
	syscall.Syscall6(syscall.SYS_PRCTL, 15, uintptr(unsafe.Pointer(&bytes[0])), 0, 0, 0, 0)
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ipNet.IP.To4() != nil {
					return ipNet.IP.String()
				}
			}
		}
	}
	return "127.0.0.1"
}

func getCPULoad(cores int) float64 {
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 1 {
			if load1, err := strconv.ParseFloat(parts[0], 64); err == nil && cores > 0 {
				pct := (load1 / float64(cores)) * 100.0
				if pct > 100.0 {
					pct = 100.0
				}
				return pct
			}
		}
	}
	return 0.0
}

func getRAM() (float64, float64) {
	var totalRAM, availRAM float64
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					val, _ := strconv.ParseFloat(fields[1], 64)
					totalRAM = val / 1024.0 / 1024.0
				}
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					val, _ := strconv.ParseFloat(fields[1], 64)
					availRAM = val / 1024.0 / 1024.0
				}
			}
		}
	}
	usedRAM := totalRAM - availRAM
	if usedRAM < 0 {
		usedRAM = 0
	}
	return usedRAM, totalRAM
}

func getStorage() (float64, float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		totalBytes := stat.Blocks * uint64(stat.Bsize)
		freeBytes := stat.Bfree * uint64(stat.Bsize)
		totalStorage := float64(totalBytes) / 1024.0 / 1024.0 / 1024.0
		usedStorage := float64(totalBytes-freeBytes) / 1024.0 / 1024.0 / 1024.0
		return usedStorage, totalStorage
	}
	return 0, 0
}

func getGPU() (string, float64, float64, int) {
	out, err := exec.Command("nvidia-smi", "--query-gpu=name,memory.total,memory.used,temperature.gpu", "--format=csv,noheader,nounits").Output()
	if err == nil {
		line := strings.TrimSpace(string(out))
		if line != "" {
			parts := strings.Split(line, ",")
			if len(parts) >= 4 {
				name := strings.TrimSpace(parts[0])
				totalMB, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
				usedMB, _ := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
				temp, _ := strconv.Atoi(strings.TrimSpace(parts[3]))
				return name, usedMB / 1024.0, totalMB / 1024.0, temp
			}
		}
	}
	return "", 0, 0, 0
}

func getLocalNode() MeshNode {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "Local Host"
	}
	cores := runtime.NumCPU()
	cpuPct := getCPULoad(cores)
	ramUsed, ramTotal := getRAM()
	diskUsed, diskTotal := getStorage()
	gpuName, gpuUsed, gpuTotal, gpuTemp := getGPU()

	return MeshNode{
		ID:             "node-local",
		Name:           fmt.Sprintf("%s (Host Workstation)", hostname),
		IP:             getLocalIP(),
		Role:           "workstation",
		Status:         "online",
		Cores:          cores,
		CPULoad:        math.Round(cpuPct*10) / 10,
		RAMUsedGB:      math.Round(ramUsed*10) / 10,
		RAMTotalGB:     math.Round(ramTotal*10) / 10,
		StorageUsedGB:  math.Round(diskUsed*10) / 10,
		StorageTotalGB: math.Round(diskTotal*10) / 10,
		StoragePool:    "Root NVMe/SSD",
		GPUName:        gpuName,
		GPUVRAMUsedGB:  math.Round(gpuUsed*10) / 10,
		GPUVRAMTotalGB: math.Round(gpuTotal*10) / 10,
		GPUTempC:       gpuTemp,
		PingMs:         0,
		LastSeen:       "now",
		IsLocal:        true,
	}
}

func getAllNodes() []MeshNode {
	if refreshMu.TryLock() {
		go func() {
			defer refreshMu.Unlock()
			refreshRemotePeers()
		}()
	}
	nodeMutex.RLock()
	defer nodeMutex.RUnlock()

	all := make([]MeshNode, 0, 1+len(remoteNodes))
	all = append(all, getLocalNode())
	all = append(all, remoteNodes...)
	return all
}

func main() {
	setProcessName(CanonicalProcessName)
	remoteNodes = loadPeers()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req RPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		handleRequest(&req)
	}
}

func handleRequest(req *RPCRequest) {
	switch req.Method {
	case "m3tal.init":
		handleInit(req)
	case "m3tal.collect":
		handleCollect(req)
	case "m3tal.action":
		handleAction(req)
	case "m3tal.shutdown":
		sendResponse(req.ID, map[string]interface{}{"status": "ok"})
		os.Exit(0)
	default:
		sendError(req.ID, -32601, "Method not found", fmt.Sprintf("Unsupported method: %s", req.Method))
	}
}

func handleInit(req *RPCRequest) {
	result := map[string]interface{}{
		"name":         PluginName,
		"version":      PluginVersion,
		"capabilities": []string{"collector", "actions", "ui_view", "settings"},
		"tabs": []map[string]interface{}{
			{
				"id":             "mesh_fleet",
				"title":          "Mesh Fleet",
				"category":       "fleet",
				"category_title": "Mesh Fleet",
				"order":          30,
				"shortcut":       "m",
				"description":    "Personal Cloud & Local Network Fleet Federation, peer discovery, and multi-node hardware telemetry reader",
			},
		},
		"actions": []map[string]interface{}{
			{
				"id":          "pair_node",
				"label":       "Pair Node",
				"description": "Authenticate and connect to a remote m3tal-mesh-node peer",
				"confirm":     false,
			},
			{
				"id":          "forget_node",
				"label":       "Forget Node",
				"description": "Remove a node from the personal mesh cluster",
				"confirm":     true,
			},
			{
				"id":          "ping_node",
				"label":       "Ping Node",
				"description": "Measure latency and verify peer reachability",
				"confirm":     false,
			},
			{
				"id":          "scan_network",
				"label":       "Scan Network",
				"description": "Broadcast mDNS/beacon to discover local M3TAL instances on LAN",
				"confirm":     false,
			},
			{"id": "get_settings", "label": "Get Mesh Settings", "description": "Read local Mesh pairing configuration", "confirm": false},
			{"id": "set_settings", "label": "Save Mesh Settings", "description": "Save local Mesh pairing configuration", "confirm": false},
			{"id": "get_fleet", "label": "Refresh Mesh Fleet", "description": "Fetch current local and remote node telemetry for Mesh Fleet", "confirm": false},
			{"id": "test_node", "label": "Test Node Connection", "description": "Test connectivity and credentials for a remote node", "confirm": false},
			{"id": "update_node", "label": "Update Node", "description": "Update credentials or metadata for a connected node", "confirm": false},
			{"id": "get_peers", "label": "Get Saved Peers", "description": "Fetch persisted peer connection entries", "confirm": false},
		},
	}
	sendResponse(req.ID, result)
}

func handleCollect(req *RPCRequest) {
	allNodes := getAllNodes()

	var totalCores int
	var totalRAM float64
	var totalStorage float64
	var onlineCount int

	tableRows := make([][]string, 0, len(allNodes))
	for _, n := range allNodes {
		totalCores += n.Cores
		totalRAM += n.RAMTotalGB
		totalStorage += n.StorageTotalGB
		if n.Status == "online" {
			onlineCount++
		}

		gpuDisplay := "None"
		if n.GPUName != "" {
			gpuDisplay = fmt.Sprintf("%s (%d°C)", n.GPUName, n.GPUTempC)
		}

		tableRows = append(tableRows, []string{
			n.Name,
			n.IP,
			n.Role,
			fmt.Sprintf("%.1f%% (%d cores)", n.CPULoad, n.Cores),
			fmt.Sprintf("%.1f / %.1f GB", n.RAMUsedGB, n.RAMTotalGB),
			fmt.Sprintf("%.0f / %.0f GB (%s)", n.StorageUsedGB, n.StorageTotalGB, n.StoragePool),
			gpuDisplay,
			fmt.Sprintf("%d ms", n.PingMs),
			n.Status,
		})
	}

	result := map[string]interface{}{
		"status": "ok",
		"nodes":  allNodes,
		"metrics": []map[string]interface{}{
			{
				"category": "mesh",
				"name":     "nodes_total",
				"value":    len(allNodes),
				"unit":     "nodes",
				"status":   "ok",
			},
			{
				"category": "mesh",
				"name":     "nodes_online",
				"value":    onlineCount,
				"unit":     "nodes",
				"status":   "ok",
			},
			{
				"category": "mesh",
				"name":     "fleet_cores_total",
				"value":    totalCores,
				"unit":     "cores",
				"status":   "ok",
			},
			{
				"category": "mesh",
				"name":     "fleet_ram_total_gb",
				"value":    totalRAM,
				"unit":     "GB",
				"status":   "ok",
			},
			{
				"category": "mesh",
				"name":     "fleet_storage_total_gb",
				"value":    totalStorage,
				"unit":     "GB",
				"status":   "ok",
			},
		},
		"tables": []map[string]interface{}{
			{
				"id":    "mesh_nodes",
				"title": "🌐 Connected Mesh Nodes",
				"headers": []string{
					"Node Name",
					"IP Address",
					"Role",
					"CPU Load",
					"Memory",
					"Storage",
					"GPU",
					"Latency",
					"Status",
				},
				"rows": tableRows,
			},
		},
	}

	sendResponse(req.ID, result)
}

func handleAction(req *RPCRequest) {
	var raw struct {
		ActionID string                 `json:"action_id"`
		Action   string                 `json:"action"`
		Target   string                 `json:"target"`
		Args     map[string]interface{} `json:"args"`
		NodeID   string                 `json:"node_id"`
		Name     string                 `json:"name"`
		IP       string                 `json:"ip"`
		Address  string                 `json:"address"`
		Role     string                 `json:"role"`
		Token    string                 `json:"token"`
		APIToken string                 `json:"api_token"`
		Password string                 `json:"password"`
	}

	if req.Params != nil {
		_ = json.Unmarshal(req.Params, &raw)
	}

	action := raw.ActionID
	if action == "" {
		action = raw.Action
	}

	name := raw.Name
	ip := raw.IP
	if ip == "" {
		ip = raw.Address
	}
	role := raw.Role
	nodeID := raw.NodeID
	pairToken := raw.Token
	apiToken := raw.APIToken
	password := raw.Password

	if raw.Args != nil {
		if v, ok := raw.Args["name"].(string); ok && name == "" {
			name = v
		}
		if v, ok := raw.Args["ip"].(string); ok && ip == "" {
			ip = v
		}
		if v, ok := raw.Args["address"].(string); ok && ip == "" {
			ip = v
		}
		if v, ok := raw.Args["role"].(string); ok && role == "" {
			role = v
		}
		if v, ok := raw.Args["node_id"].(string); ok && nodeID == "" {
			nodeID = v
		}
		if v, ok := raw.Args["token"].(string); ok && pairToken == "" {
			pairToken = v
		}
		if v, ok := raw.Args["api_token"].(string); ok && apiToken == "" {
			apiToken = v
		}
		if v, ok := raw.Args["password"].(string); ok && password == "" {
			password = v
		}
	}
	if raw.Target != "" && nodeID == "" {
		nodeID = raw.Target
	}

	effectiveToken := apiToken
	if effectiveToken == "" {
		effectiveToken = pairToken
	}

	switch action {
	case "pair_node":
		if ip == "" {
			sendError(req.ID, -32602, "Invalid params", "Node IP address is required")
			return
		}
		cfg := loadMeshConfig()
		normIP, err := normalizeNodeAddress(ip, cfg.ListenPort)
		if err != nil {
			sendError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
		newNode, err := pairAndPersistPeer(normIP, name, role, effectiveToken, password)
		if err != nil {
			sendError(req.ID, -32002, "Node pairing failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Paired and verified %s (%s)", newNode.Name, newNode.IP),
			"node":    newNode,
		})

	case "test_node":
		var savedPeer *MeshNode
		if nodeID != "" {
			for _, peer := range snapshotPeers() {
				if peer.ID == nodeID {
					copy := peer
					savedPeer = &copy
					break
				}
			}
		}
		if ip == "" && savedPeer != nil {
			ip = savedPeer.IP
		}
		if ip == "" {
			sendError(req.ID, -32602, "Invalid params", "Node IP address is required")
			return
		}
		cfg := loadMeshConfig()
		normIP, normErr := normalizeNodeAddress(ip, cfg.ListenPort)
		if normErr == nil {
			ip = normIP
		}
		samePeerIP := false
		if savedPeer != nil {
			normSaved, _ := normalizeNodeAddress(savedPeer.IP, cfg.ListenPort)
			samePeerIP = strings.EqualFold(ip, normSaved)
		}
		explicitTok := effectiveToken
		if explicitTok == "" && samePeerIP {
			explicitTok = savedPeer.APIToken
		}
		explicitPass := password
		if explicitPass == "" && samePeerIP {
			explicitPass = savedPeer.Password
		}
		var expectedFP string
		savedPasswordOnly := false
		if samePeerIP {
			expectedFP = savedPeer.Fingerprint
			savedPasswordOnly = savedPeer.PasswordOnly
		}
		testedNode, _, err := connectAndNegotiateNode(ip, explicitTok, explicitPass, cfg, expectedFP, savedPasswordOnly)
		if err != nil {
			sendError(req.ID, -32002, "Connection test failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{
			"success":          true,
			"message":          fmt.Sprintf("Connection successful: %s responded in %dms", testedNode.Name, testedNode.PingMs),
			"hostname":         testedNode.Name,
			"ping_ms":          testedNode.PingMs,
			"cores":            testedNode.Cores,
			"ram_total_gb":     testedNode.RAMTotalGB,
			"storage_total_gb": testedNode.StorageTotalGB,
		})

	case "update_node":
		if nodeID == "" {
			sendError(req.ID, -32602, "Invalid params", "node_id is required")
			return
		}
		peers := snapshotPeers()
		var foundNode *MeshNode
		for i := range peers {
			if peers[i].ID == nodeID {
				copyNode := peers[i]
				foundNode = &copyNode
				break
			}
		}
		if foundNode == nil {
			sendError(req.ID, -32602, "Not found", fmt.Sprintf("Node %s not found", nodeID))
			return
		}
		if name != "" {
			foundNode.Name = name
		}
		if role != "" {
			foundNode.Role = role
		}
		cfg := loadMeshConfig()
		addressChanged := false
		if ip != "" {
			normUpdateIP, err := normalizeNodeAddress(ip, cfg.ListenPort)
			if err != nil {
				sendError(req.ID, -32602, "Invalid params", err.Error())
				return
			}
			normCurrent, _ := normalizeNodeAddress(foundNode.IP, cfg.ListenPort)
			if !strings.EqualFold(normUpdateIP, normCurrent) {
				foundNode.Fingerprint = ""
				addressChanged = true
			}
			foundNode.IP = normUpdateIP
		}
		clearTok, _ := raw.Args["clear_api_token"].(bool)
		clearPass, _ := raw.Args["clear_password"].(bool)
		if addressChanged {
			if effectiveToken == "" {
				clearTok = true
			}
			if password == "" {
				clearPass = true
			}
		}
		credsChanged := false
		if effectiveToken != "" {
			foundNode.APIToken = effectiveToken
			credsChanged = true
		} else if clearTok {
			foundNode.APIToken = ""
			credsChanged = true
		}
		if password != "" {
			foundNode.Password = password
			credsChanged = true
		} else if clearPass {
			foundNode.Password = ""
			credsChanged = true
		}
		if addressChanged || credsChanged {
			cfg := loadMeshConfig()
			probePasswordOnly := foundNode.PasswordOnly
			if addressChanged {
				probePasswordOnly = false
			}
			testedNode, pwOnly, err := connectAndNegotiateNode(foundNode.IP, foundNode.APIToken, foundNode.Password, cfg, foundNode.Fingerprint, probePasswordOnly)
			if err == nil {
				foundNode.PasswordOnly = pwOnly
				if testedNode.Fingerprint != "" {
					foundNode.Fingerprint = testedNode.Fingerprint
				}
				foundNode.Status = "online"
			} else if foundNode.APIToken != "" {
				foundNode.PasswordOnly = false
			}
		}
		if err := persistPeerUpsert(*foundNode, clearTok, clearPass); err != nil {
			sendError(req.ID, -32003, "Could not save updated node", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Updated node %s", foundNode.Name),
			"node":    *foundNode,
		})

	case "forget_node":
		if nodeID == "" {
			sendError(req.ID, -32602, "Invalid params", "node_id is required")
			return
		}
		if nodeID == "node-local" {
			sendError(req.ID, -32602, "Invalid operation", "Cannot forget the local workstation host")
			return
		}

		found, err := persistPeerRemoval(nodeID)
		if err != nil {
			sendError(req.ID, -32003, "Could not save fleet changes", err.Error())
			return
		}
		if !found {
			sendError(req.ID, -32602, "Not found", fmt.Sprintf("Node %s not found", nodeID))
			return
		}

		sendResponse(req.ID, map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Node %s removed from mesh cluster", nodeID),
		})

	case "ping_node":
		if nodeID == "node-local" {
			sendResponse(req.ID, map[string]interface{}{
				"success": true,
				"node_id": nodeID,
				"ping_ms": 0,
				"status":  "online",
			})
			return
		}

		var target *MeshNode
		for _, peer := range snapshotPeers() {
			if peer.ID == nodeID {
				copy := peer
				target = &copy
				break
			}
		}
		if target == nil {
			sendError(req.ID, -32602, "Not found", fmt.Sprintf("Node %s not found", nodeID))
			return
		}
		cfg := loadMeshConfig()
		tok, pass := effectiveCredentials(target.APIToken, target.Password, target.PasswordOnly, cfg, target.Fingerprint)
		updated, err := fetchRemoteWithAuth(target.IP, tok, pass, cfg.ListenPort, target.Fingerprint)
		if err != nil {
			sendError(req.ID, -32004, "Node ping failed", err.Error())
			return
		}
		updated.ID, updated.Name, updated.Role = target.ID, target.Name, target.Role
		updated.APIToken = target.APIToken
		updated.Password = target.Password
		updated.PasswordOnly = target.PasswordOnly
		if err := persistPeerUpsert(updated, false, false); err != nil {
			sendError(req.ID, -32005, "Could not save node status", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{
			"success": true,
			"node_id": nodeID,
			"ping_ms": updated.PingMs,
			"status":  updated.Status,
		})

	case "scan_network":
		cfg := loadMeshConfig()
		if !cfg.IsDiscoveryEnabled() {
			sendResponse(req.ID, map[string]interface{}{
				"success":    true,
				"discovered": 0,
				"nodes":      []MeshNode{},
				"message":    "LAN discovery via mDNS is disabled in Mesh Settings.",
			})
			return
		}
		if cfg.IsPairingTokenRequired() && cfg.PairingToken == "" && cfg.DefaultPassword == "" {
			sendResponse(req.ID, map[string]interface{}{
				"success":    true,
				"discovered": 0,
				"nodes":      []MeshNode{},
				"message":    "Set the shared pairing token or default password in Mesh Settings to verify discovered nodes.",
			})
			return
		}
		discovered, err := discoverAndVerifyPeers()
		if err != nil {
			sendError(req.ID, -32001, "LAN scan failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{
			"success":    true,
			"discovered": len(discovered),
			"nodes":      discovered,
		})
	case "set_settings":
		cfg := loadMeshConfig()
		if value, ok := raw.Args["pairing_token"].(string); ok && strings.TrimSpace(value) != "" {
			cfg.PairingToken = strings.TrimSpace(value)
		} else if clear, ok := raw.Args["clear_pairing_token"].(bool); ok && clear {
			cfg.PairingToken = ""
		}

		if value, ok := raw.Args["default_password"].(string); ok && strings.TrimSpace(value) != "" {
			cfg.DefaultPassword = strings.TrimSpace(value)
		} else if clear, ok := raw.Args["clear_password"].(bool); ok && clear {
			cfg.DefaultPassword = ""
		}

		if value, ok := raw.Args["require_pairing_token"].(bool); ok {
			cfg.RequirePairingToken = &value
		}
		if value, ok := raw.Args["discovery_enabled"].(bool); ok {
			cfg.DiscoveryEnabled = &value
		}
		if value, ok := raw.Args["listen_port"].(float64); ok {
			cfg.ListenPort = int(value)
		}
		if err := saveMeshConfig(cfg); err != nil {
			sendError(req.ID, -32010, "Settings save failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{
			"success":               true,
			"message":               "Mesh settings saved",
			"has_password":          cfg.DefaultPassword != "",
			"has_pairing_token":     cfg.PairingToken != "",
			"require_pairing_token": cfg.IsPairingTokenRequired(),
			"discovery_enabled":     cfg.IsDiscoveryEnabled(),
			"listen_port":           cfg.ListenPort,
		})
	case "get_fleet":
		sendResponse(req.ID, map[string]interface{}{
			"success": true,
			"nodes":   getAllNodes(),
		})
	case "get_peers":
		peers := snapshotPeers()
		sanitized := make([]map[string]interface{}, 0, len(peers))
		for _, p := range peers {
			sanitized = append(sanitized, map[string]interface{}{
				"id":               p.ID,
				"name":             p.Name,
				"ip":               p.IP,
				"role":             p.Role,
				"status":           p.Status,
				"ping_ms":          p.PingMs,
				"last_seen":        p.LastSeen,
				"has_password":     p.Password != "",
				"has_api_token":    p.APIToken != "",
				"cores":            p.Cores,
				"ram_total_gb":     p.RAMTotalGB,
				"storage_total_gb": p.StorageTotalGB,
			})
		}
		sendResponse(req.ID, map[string]interface{}{
			"success": true,
			"peers":   sanitized,
		})
	case "get_settings":
		cfg := loadMeshConfig()
		sendResponse(req.ID, map[string]interface{}{
			"success":               true,
			"has_password":          cfg.DefaultPassword != "",
			"has_pairing_token":     cfg.PairingToken != "",
			"require_pairing_token": cfg.IsPairingTokenRequired(),
			"discovery_enabled":     cfg.IsDiscoveryEnabled(),
			"listen_port":           cfg.ListenPort,
		})

	default:
		sendError(req.ID, -32601, "Unknown action", fmt.Sprintf("Unsupported action_id: %s", action))
	}
}

func sendResponse(id interface{}, result interface{}) {
	resp := RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	bytes, _ := json.Marshal(resp)
	fmt.Println(string(bytes))
}

func sendError(id interface{}, code int, message string, data interface{}) {
	resp := RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	bytes, _ := json.Marshal(resp)
	fmt.Println(string(bytes))
}
