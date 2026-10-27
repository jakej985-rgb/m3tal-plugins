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
	CanonicalProcessName = "m3tal-compose"
	PluginName           = "compose"
	PluginVersion        = "0.4.0"
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
	var initParams struct {
		Config map[string]interface{} `json:"config"`
	}
	if json.Unmarshal(req.Params, &initParams) == nil && len(initParams.Config) > 0 {
		_ = SetPluginSettings(initParams.Config)
	}
	result := map[string]interface{}{
		"name":         PluginName,
		"version":      PluginVersion,
		"capabilities": []string{"collector", "actions", "ui_view", "settings"},
		"settings": []map[string]interface{}{
			{
				"id": "compose_settings", "title": "Compose Projects", "description": "Project discovery and new stack storage settings",
				"fields": []map[string]interface{}{
					{"key": "compose_dir", "label": "Search folder", "type": "text", "default": "~/docker", "description": "Recursively search for Compose projects in this folder."},
					{"key": "new_stack_layout", "label": "New stack layout", "type": "select", "default": "per_stack_folder", "options": []string{"per_stack_folder", "flat"}, "description": "Choose how new stack files are arranged under the search folder."},
					{"key": "backup_on_save", "label": "Back up before saving", "type": "boolean", "default": true, "description": "Keep a .bak copy when replacing an existing Compose file."},
				},
			},
		},
		"tabs": []map[string]interface{}{
			{
				"id":             "compose_editor",
				"title":          "Compose",
				"category":       "containers",
				"category_title": "Containers",
				"order":          25,
				"shortcut":       "c",
				"description":    "Docker Compose visual editor, syntax validator, and stack lifecycle manager",
			},
		},
		"actions": []map[string]interface{}{
			{
				"id":          "compose_up",
				"label":       "Deploy Stack",
				"description": "Deploy stack in detached mode (docker compose up -d)",
				"confirm":     false,
			},
			{
				"id":          "compose_down",
				"label":       "Stop Stack",
				"description": "Stop and tear down stack services (docker compose down)",
				"confirm":     true,
			},
			{
				"id":          "compose_restart",
				"label":       "Restart Stack",
				"description": "Restart all services in the stack",
				"confirm":     false,
			},
			{
				"id":          "save_stack",
				"label":       "Save Compose File",
				"description": "Save compose.yaml file content to disk",
				"confirm":     false,
			},
			{
				"id":          "validate_yaml",
				"label":       "Validate YAML",
				"description": "Verify Compose syntax and schema validity",
				"confirm":     false,
			},
			{
				"id":          "get_logs",
				"label":       "Get Stack Logs",
				"description": "Fetch live output from all services in stack",
				"confirm":     false,
			},
		},
	}
	sendResponse(req.ID, result)
}

func handleCollect(req *RPCRequest) {
	stacks, err := ListStacks()
	if err != nil {
		sendError(req.ID, -32000, "Collection failed", err.Error())
		return
	}

	runningStacks := 0
	totalServices := 0
	var rows [][]string

	for _, s := range stacks {
		if s.Status == "running" {
			runningStacks++
		}
		totalServices += s.ServicesCount

		servicesStr := fmt.Sprintf("%d (%s)", s.ServicesCount, formatServiceList(s.Services))
		rows = append(rows, []string{
			s.Name,
			servicesStr,
			fmt.Sprintf("%d / %d", s.RunningCount, s.ServicesCount),
			s.Status,
			s.UpdatedAt,
		})
	}

	metrics := []map[string]interface{}{
		{
			"category": "compose",
			"name":     "stacks_total",
			"value":    len(stacks),
			"unit":     "stacks",
			"status":   "ok",
		},
		{
			"category": "compose",
			"name":     "stacks_running",
			"value":    runningStacks,
			"unit":     "stacks",
			"status":   "ok",
		},
		{
			"category": "compose",
			"name":     "services_total",
			"value":    totalServices,
			"unit":     "services",
			"status":   "ok",
		},
	}

	tables := []map[string]interface{}{
		{
			"id":      "compose_stacks",
			"title":   "📦 Docker Compose Stacks",
			"headers": []string{"Stack Name", "Services", "Running", "Status", "Updated"},
			"rows":    rows,
		},
	}

	sendResponse(req.ID, map[string]interface{}{
		"status":  "ok",
		"metrics": metrics,
		"tables":  tables,
		"stacks":  stacks,
	})
}

func formatServiceList(services []string) string {
	if len(services) == 0 {
		return "none"
	}
	if len(services) > 3 {
		return fmt.Sprintf("%s, +%d more", strings.Join(services[:3], ", "), len(services)-3)
	}
	return strings.Join(services, ", ")
}

func handleAction(req *RPCRequest) {
	var params struct {
		ActionID string                 `json:"action_id"`
		Stack    string                 `json:"stack"`
		Content  string                 `json:"content,omitempty"`
		Lines    int                    `json:"lines,omitempty"`
		Args     map[string]interface{} `json:"args,omitempty"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	switch params.ActionID {
	case "set_settings":
		settings := params.Args
		if config, ok := params.Args["config"].(map[string]interface{}); ok {
			settings = config
		}
		if err := SetPluginSettings(settings); err != nil {
			sendError(req.ID, -32602, "Invalid settings", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{"success": true, "message": "Compose settings saved"})
	case "compose_up":
		if params.Stack == "" {
			sendError(req.ID, -32602, "Missing parameter", "stack name required")
			return
		}
		msg, err := ComposeUp(params.Stack)
		if err != nil {
			sendError(req.ID, -32001, "Compose up failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{"success": true, "message": msg})

	case "compose_down":
		if params.Stack == "" {
			sendError(req.ID, -32602, "Missing parameter", "stack name required")
			return
		}
		msg, err := ComposeDown(params.Stack)
		if err != nil {
			sendError(req.ID, -32001, "Compose down failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{"success": true, "message": msg})

	case "compose_restart":
		if params.Stack == "" {
			sendError(req.ID, -32602, "Missing parameter", "stack name required")
			return
		}
		msg, err := ComposeRestart(params.Stack)
		if err != nil {
			sendError(req.ID, -32001, "Compose restart failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{"success": true, "message": msg})

	case "save_stack":
		if params.Stack == "" || params.Content == "" {
			sendError(req.ID, -32602, "Missing parameter", "stack name and content required")
			return
		}
		if err := SaveStack(params.Stack, params.Content); err != nil {
			sendError(req.ID, -32001, "Save stack failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{
			"success":  true,
			"message":  fmt.Sprintf("Stack '%s' saved successfully", params.Stack),
			"stack_id": savedStackID(params.Stack),
		})

	case "validate_yaml":
		valid, msg, err := ValidateYAML(params.Content)
		if err != nil {
			sendError(req.ID, -32001, "Validation failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{"valid": valid, "message": msg})

	case "get_logs":
		if params.Stack == "" {
			sendError(req.ID, -32602, "Missing parameter", "stack name required")
			return
		}
		lines := params.Lines
		if lines <= 0 {
			lines = 50
		}
		logs, err := ComposeLogs(params.Stack, lines)
		if err != nil {
			sendError(req.ID, -32001, "Get logs failed", err.Error())
			return
		}
		sendResponse(req.ID, map[string]interface{}{"success": true, "logs": logs})

	default:
		sendError(req.ID, -32601, "Unknown action", fmt.Sprintf("Action '%s' not recognized", params.ActionID))
	}
}

func sendResponse(id interface{}, result interface{}) {
	res := RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	bytes, _ := json.Marshal(res)
	fmt.Println(string(bytes))
}

func sendError(id interface{}, code int, message string, data interface{}) {
	res := RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	bytes, _ := json.Marshal(res)
	fmt.Println(string(bytes))
}
