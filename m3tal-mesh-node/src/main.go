package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	CanonicalProcessName = "m3tal-mesh-node"
	PluginName           = "mesh-node"
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

type NodeTelemetry struct {
	Hostname       string  `json:"hostname"`
	Cores          int     `json:"cores"`
	Load1m         float64 `json:"load_1m"`
	Load5m         float64 `json:"load_5m"`
	Load15m        float64 `json:"load_15m"`
	RAMUsedGB      float64 `json:"ram_used_gb"`
	RAMTotalGB     float64 `json:"ram_total_gb"`
	StorageUsedGB  float64 `json:"storage_used_gb"`
	StorageTotalGB float64 `json:"storage_total_gb"`
	OS             string  `json:"os"`
	Arch           string  `json:"arch"`
}

func setProcessName(name string) {
	bytes := append([]byte(name), 0)
	// PR_SET_NAME = 15
	syscall.Syscall6(syscall.SYS_PRCTL, 15, uintptr(unsafe.Pointer(&bytes[0])), 0, 0, 0, 0)
}

func main() {
	setProcessName(CanonicalProcessName)
	if err := startNodeNetworkService(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		shutdownNodeNetwork()
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
		shutdownNodeNetwork()
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
		"tabs":         []map[string]interface{}{},
		"actions": []map[string]interface{}{
			{
				"id":          "get_telemetry",
				"label":       "Get Telemetry",
				"description": "Fetch real-time hardware telemetry payload from this node",
				"confirm":     false,
			},
			{
				"id":          "beacon_ping",
				"label":       "Beacon Ping",
				"description": "Respond to mesh discovery cluster beacon",
				"confirm":     false,
			},
			{"id": "get_settings", "label": "Get Mesh Node Settings", "description": "Read sender configuration for its plugin settings page", "confirm": false},
			{"id": "set_settings", "label": "Save Mesh Node Settings", "description": "Save sender configuration", "confirm": false},
		},
	}
	sendResponse(req.ID, result)
}

func getTelemetry() NodeTelemetry {
	hostname, _ := os.Hostname()
	cores := runtime.NumCPU()

	var load1, load5, load15 float64
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			load1, _ = strconv.ParseFloat(parts[0], 64)
			load5, _ = strconv.ParseFloat(parts[1], 64)
			load15, _ = strconv.ParseFloat(parts[2], 64)
		}
	}

	var totalRAM, availRAM float64
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					val, _ := strconv.ParseFloat(fields[1], 64)
					totalRAM = val / 1024.0 / 1024.0 // to GB
				}
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					val, _ := strconv.ParseFloat(fields[1], 64)
					availRAM = val / 1024.0 / 1024.0 // to GB
				}
			}
		}
	}

	ramUsed := totalRAM - availRAM
	if ramUsed < 0 {
		ramUsed = 0
	}

	var totalStorage, usedStorage float64
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		totalBytes := stat.Blocks * uint64(stat.Bsize)
		freeBytes := stat.Bfree * uint64(stat.Bsize)
		totalStorage = float64(totalBytes) / 1024.0 / 1024.0 / 1024.0
		usedStorage = float64(totalBytes-freeBytes) / 1024.0 / 1024.0 / 1024.0
	}

	return NodeTelemetry{
		Hostname:       hostname,
		Cores:          cores,
		Load1m:         load1,
		Load5m:         load5,
		Load15m:        load15,
		RAMUsedGB:      ramUsed,
		RAMTotalGB:     totalRAM,
		StorageUsedGB:  usedStorage,
		StorageTotalGB: totalStorage,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
	}
}

func handleCollect(req *RPCRequest) {
	t := getTelemetry()

	result := map[string]interface{}{
		"status":    "ok",
		"telemetry": t,
		"metrics": []map[string]interface{}{
			{
				"category": "node",
				"name":     "cpu_cores",
				"value":    t.Cores,
				"unit":     "cores",
				"status":   "ok",
			},
			{
				"category": "node",
				"name":     "load_1m",
				"value":    t.Load1m,
				"unit":     "load",
				"status":   "ok",
			},
			{
				"category": "node",
				"name":     "ram_used_gb",
				"value":    t.RAMUsedGB,
				"unit":     "GB",
				"status":   "ok",
			},
			{
				"category": "node",
				"name":     "ram_total_gb",
				"value":    t.RAMTotalGB,
				"unit":     "GB",
				"status":   "ok",
			},
			{
				"category": "node",
				"name":     "storage_used_gb",
				"value":    t.StorageUsedGB,
				"unit":     "GB",
				"status":   "ok",
			},
			{
				"category": "node",
				"name":     "storage_total_gb",
				"value":    t.StorageTotalGB,
				"unit":     "GB",
				"status":   "ok",
			},
		},
		"tables": []map[string]interface{}{
			{
				"id":    "node_status",
				"title": "📡 Node Local Hardware Telemetry",
				"headers": []string{
					"Metric",
					"Value",
				},
				"rows": [][]string{
					{"Hostname", t.Hostname},
					{"CPU Topology", fmt.Sprintf("%d Cores (Load: %.2f, %.2f, %.2f)", t.Cores, t.Load1m, t.Load5m, t.Load15m)},
					{"Memory", fmt.Sprintf("%.2f / %.2f GB", t.RAMUsedGB, t.RAMTotalGB)},
					{"Root Filesystem", fmt.Sprintf("%.1f / %.1f GB", t.StorageUsedGB, t.StorageTotalGB)},
					{"Platform", fmt.Sprintf("%s / %s", t.OS, t.Arch)},
				},
			},
		},
	}

	sendResponse(req.ID, result)
}

func handleAction(req *RPCRequest) {
	var params struct {
		ActionID string         `json:"action_id"`
		Args     map[string]any `json:"args"`
	}

	if req.Params != nil {
		_ = json.Unmarshal(req.Params, &params)
	}

	switch params.ActionID {
	case "get_telemetry":
		t := getTelemetry()
		sendResponse(req.ID, map[string]interface{}{
			"success":   true,
			"telemetry": t,
		})

	case "beacon_ping":
		hostname, _ := os.Hostname()
		sendResponse(req.ID, map[string]interface{}{
			"success":  true,
			"hostname": hostname,
			"status":   "online",
			"version":  PluginVersion,
		})
	case "set_settings":
		cfg := loadNodeRuntimeConfig()
		if clear, ok := params.Args["clear_pairing_token"].(bool); ok && clear {
			cfg.PairingToken = ""
		} else if value, ok := params.Args["pairing_token"].(string); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				cfg.PairingToken = trimmed
			}
		}
		if value, ok := params.Args["require_pairing_token"].(bool); ok {
			cfg.RequirePairingToken = &value
		}
		if value, ok := params.Args["listen_port"].(float64); ok {
			cfg.ListenPort = int(value)
		}
		if value, ok := params.Args["broadcast_mdns"].(bool); ok {
			cfg.BroadcastMDNS = value
		}
		if err := saveNodeRuntimeConfig(cfg); err != nil {
			sendError(req.ID, -32010, "Settings save failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{
			"success":               true,
			"message":               "Settings saved. Restart the plugin if you changed its port or LAN discovery setting.",
			"has_pairing_token":     cfg.PairingToken != "",
			"require_pairing_token": cfg.IsPairingTokenRequired(),
			"listen_port":           cfg.ListenPort,
			"broadcast_mdns":        cfg.BroadcastMDNS,
		})
	case "get_settings":
		cfg := loadNodeRuntimeConfig()
		sendResponse(req.ID, map[string]interface{}{
			"success":               true,
			"has_pairing_token":     cfg.PairingToken != "",
			"require_pairing_token": cfg.IsPairingTokenRequired(),
			"listen_port":           cfg.ListenPort,
			"broadcast_mdns":        cfg.BroadcastMDNS,
		})

	default:
		sendError(req.ID, -32601, "Unknown action", fmt.Sprintf("Unsupported action_id: %s", params.ActionID))
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
