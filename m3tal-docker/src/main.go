package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"unsafe"
)

const (
	CanonicalProcessName = "m3tal-docker"
	PluginName           = "docker-monitor"
	PluginVersion        = "0.3.1"
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

func setProcessName(name string) {
	bytes := append([]byte(name), 0)
	// PR_SET_NAME = 15
	syscall.Syscall6(syscall.SYS_PRCTL, 15, uintptr(unsafe.Pointer(&bytes[0])), 0, 0, 0, 0)
}

func main() {
	setProcessName(CanonicalProcessName)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}

		var req RPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(0, -32700, "Parse error: "+err.Error())
			continue
		}

		handleRequest(&req)
	}
}

func handleRequest(req *RPCRequest) {
	switch req.Method {
	case "m3tal.init":
		sendResult(req.ID, map[string]interface{}{
			"name":    PluginName,
			"version": PluginVersion,
			"capabilities": []string{
				"collector",
				"actions",
				"ui_view",
				"ui_widget",
			},
			"widgets": []map[string]interface{}{
				{
					"id":    "docker-fleet",
					"title": "Containers",
					"size":  []int{2, 1},
					"type":  "card",
				},
			},
			"tabs": []map[string]interface{}{
				{
					"id":             "docker_containers",
					"title":          "Fleet",
					"category":       "containers",
					"category_title": "Containers",
					"shortcut":       "d",
					"description":    "Docker container telemetry, fleet management, and log viewer",
					"order":          20,
				},
			},
			"actions": []map[string]interface{}{
				{
					"id":          "start_container",
					"label":       "Start Container",
					"description": "Start a stopped Docker container",
					"confirm":     false,
				},
				{
					"id":          "stop_container",
					"label":       "Stop Container",
					"description": "Gracefully stop a running Docker container",
					"confirm":     true,
				},
				{
					"id":          "restart_container",
					"label":       "Restart Container",
					"description": "Restart a container",
					"confirm":     false,
				},
				{
					"id":          "prune_images",
					"label":       "Prune Images",
					"description": "Remove unused or dangling Docker images",
					"confirm":     true,
				},
				{
					"id":          "get_logs",
					"label":       "Get Logs",
					"description": "Fetch container logs",
					"confirm":     false,
				},
			},
		})

	case "m3tal.collect":
		containers, err := GetContainers()
		if err != nil {
			sendResult(req.ID, map[string]interface{}{
				"status":  "degraded",
				"message": err.Error(),
				"metrics": []map[string]interface{}{},
				"tables":  []map[string]interface{}{},
			})
			return
		}

		running := 0
		stopped := 0
		rows := make([][]string, 0, len(containers))
		for _, c := range containers {
			if c.State == "running" || strings.HasPrefix(strings.ToLower(c.Status), "up") {
				running++
			} else {
				stopped++
			}
			rows = append(rows, []string{
				c.ID,
				c.Name,
				c.Image,
				c.Status,
				c.Ports,
				c.State,
				c.CPU,
				c.Memory,
			})
		}

		sendResult(req.ID, map[string]interface{}{
			"status": "ok",
			"metrics": []map[string]interface{}{
				{
					"name":     "containers_total",
					"value":    float64(len(containers)),
					"unit":     "containers",
					"category": "docker",
					"status":   "ok",
				},
				{
					"name":     "containers_running",
					"value":    float64(running),
					"unit":     "containers",
					"category": "docker",
					"status":   "ok",
				},
				{
					"name":     "containers_stopped",
					"value":    float64(stopped),
					"unit":     "containers",
					"category": "docker",
					"status":   "ok",
				},
			},
			"tables": []map[string]interface{}{
				{
					"id":      "docker_ps",
					"title":   "🐳 Docker Containers",
					"headers": []string{"ID", "Name", "Image", "Status", "Ports", "State", "CPU", "Memory"},
					"rows":    rows,
				},
			},
		})

	case "m3tal.action":
		var params struct {
			ActionID string                 `json:"action_id"`
			Target   string                 `json:"target"`
			Args     map[string]interface{} `json:"args"`
		}
		_ = json.Unmarshal(req.Params, &params)

		switch params.ActionID {
		case "start_container":
			if err := StartContainer(params.Target); err != nil {
				sendResult(req.ID, map[string]interface{}{
					"success": false,
					"message": "Failed to start container",
					"error":   err.Error(),
				})
			} else {
				sendResult(req.ID, map[string]interface{}{
					"success": true,
					"message": fmt.Sprintf("Container %s started successfully", params.Target),
				})
			}

		case "stop_container":
			if err := StopContainer(params.Target); err != nil {
				sendResult(req.ID, map[string]interface{}{
					"success": false,
					"message": "Failed to stop container",
					"error":   err.Error(),
				})
			} else {
				sendResult(req.ID, map[string]interface{}{
					"success": true,
					"message": fmt.Sprintf("Container %s stopped successfully", params.Target),
				})
			}

		case "restart_container":
			if err := RestartContainer(params.Target); err != nil {
				sendResult(req.ID, map[string]interface{}{
					"success": false,
					"message": "Failed to restart container",
					"error":   err.Error(),
				})
			} else {
				sendResult(req.ID, map[string]interface{}{
					"success": true,
					"message": fmt.Sprintf("Container %s restarted successfully", params.Target),
				})
			}

		case "prune_images":
			msg, err := PruneImages()
			if err != nil {
				sendResult(req.ID, map[string]interface{}{
					"success": false,
					"message": "Failed to prune images",
					"error":   err.Error(),
				})
			} else {
				sendResult(req.ID, map[string]interface{}{
					"success": true,
					"message": msg,
				})
			}

		case "get_logs":
			tail := 50
			if params.Args != nil {
				if t, ok := params.Args["tail"].(float64); ok {
					tail = int(t)
				}
			}
			logs, err := GetLogs(params.Target, tail)
			if err != nil {
				sendResult(req.ID, map[string]interface{}{
					"success": false,
					"message": "Failed to read logs",
					"error":   err.Error(),
				})
			} else {
				sendResult(req.ID, map[string]interface{}{
					"success": true,
					"message": "Logs retrieved",
					"logs":    logs,
				})
			}

		default:
			sendResult(req.ID, map[string]interface{}{
				"success": false,
				"message": "Unknown action: " + params.ActionID,
			})
		}

	case "m3tal.shutdown":
		sendResult(req.ID, map[string]interface{}{
			"status": "terminating",
		})
		os.Exit(0)

	default:
		sendError(req.ID, -32601, "Method not found: "+req.Method)
	}
}

func sendResult(id interface{}, result interface{}) {
	resBytes, _ := json.Marshal(result)
	resp := RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  json.RawMessage(resBytes),
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func sendError(id interface{}, code int, message string) {
	resp := RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}
