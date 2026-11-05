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
	CanonicalProcessName = "m3tal-samba-nfs"
	PluginName           = "samba-nfs"
	PluginVersion        = "0.1.0"
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
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents standard JSON-RPC 2.0 error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
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
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}

		var req RPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(nil, -32700, "Parse error: "+err.Error())
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
		sendResult(req.ID, map[string]interface{}{"status": "ok"})
		os.Exit(0)
	default:
		sendError(req.ID, -32601, "Method not found: "+req.Method)
	}
}

func handleInit(req *RPCRequest) {
	cfg := loadPluginConfig()
	sendResult(req.ID, map[string]interface{}{
		"name":         PluginName,
		"version":      PluginVersion,
		"capabilities": []string{"collector", "actions", "ui_view", "settings"},
		"services": map[string]interface{}{
			"samba_installed":   commandExists("smbd"),
			"samba_version":     binaryVersion("smbd"),
			"smbd_active":       serviceActive("smbd"),
			"nmbd_active":       serviceActive("nmbd"),
			"nfs_installed":     commandExists("exportfs") || commandExists("showmount"),
			"nfs_server_active": serviceActive("nfs-server") || serviceActive("nfs-kernel-server"),
			"config_paths": map[string]string{
				"smb_conf": cfg.SmbConfPath,
				"exports":  cfg.ExportsPath,
			},
		},
		"tabs": []map[string]interface{}{
			{
				"id":             "samba_nfs_shares",
				"title":          "Shares",
				"category":       "storage",
				"category_title": "Storage",
				"shortcut":       "s",
				"order":          40,
				"description":    "Samba (SMB/CIFS) and NFS share manager with users, permissions, and live client monitoring",
			},
		},
		"settings": []map[string]interface{}{
			{
				"id":          "samba_nfs_config",
				"title":       "Share Manager",
				"description": "Configuration file locations and reload behavior for Samba and NFS share management",
				"fields": []map[string]interface{}{
					{"key": "smb_conf_path", "label": "Samba Configuration Path", "type": "text", "default": "/etc/samba/smb.conf", "description": "Path to smb.conf managed by this plugin"},
					{"key": "exports_path", "label": "NFS Exports Path", "type": "text", "default": "/etc/exports", "description": "Path to the NFS exports file managed by this plugin"},
					{"key": "auto_reload", "label": "Reload Services After Changes", "type": "boolean", "default": true, "description": "Automatically reload/restart smbd, nmbd, and nfs-server after share changes"},
				},
			},
		},
		"actions": []map[string]interface{}{
			{"id": "get_shares", "label": "Refresh Shares", "description": "Fetch all SMB shares, NFS exports, users, and client connections", "confirm": false},
			{"id": "create_smb_share", "label": "Create SMB Share", "description": "Add a new [share] section to smb.conf", "confirm": false},
			{"id": "delete_smb_share", "label": "Delete SMB Share", "description": "Remove a share section from smb.conf", "confirm": true},
			{"id": "create_nfs_export", "label": "Create NFS Export", "description": "Append an export entry to /etc/exports", "confirm": false},
			{"id": "delete_nfs_export", "label": "Delete NFS Export", "description": "Remove an export entry (optionally per client) from /etc/exports", "confirm": true},
			{"id": "create_smb_user", "label": "Create Samba User", "description": "Add a Samba user account with a password", "confirm": false},
			{"id": "delete_smb_user", "label": "Delete Samba User", "description": "Remove a Samba user account", "confirm": true},
			{"id": "set_smb_password", "label": "Set Samba Password", "description": "Change a Samba user's password", "confirm": false},
			{"id": "mount_share", "label": "Mount Network Share", "description": "Mount a remote CIFS (SMB) or NFS share to a local mount point", "confirm": false},
			{"id": "unmount_share", "label": "Unmount Share", "description": "Unmount a network share from a local mount point", "confirm": false},
			{"id": "restart_services", "label": "Restart Share Services", "description": "Restart smbd, nmbd, and nfs-server", "confirm": true},
			{"id": "get_settings", "label": "Get Settings", "description": "Read plugin configuration", "confirm": false},
			{"id": "set_settings", "label": "Save Settings", "description": "Save plugin configuration", "confirm": false},
		},
	})
}

func handleCollect(req *RPCRequest) {
	cfg := loadPluginConfig()
	shares, users, sessions, lockedFiles, nfsClients, err := GetAllShares(cfg)
	if err != nil {
		sendResult(req.ID, map[string]interface{}{
			"status":  "degraded",
			"message": err.Error(),
			"metrics": []map[string]interface{}{},
			"tables":  []map[string]interface{}{},
		})
		return
	}

	smbCount, nfsCount := 0, 0
	shareRows := make([][]string, 0, len(shares))
	for _, s := range shares {
		if s.Protocol == "smb" {
			smbCount++
		} else {
			nfsCount++
		}
		detail := s.Path
		if s.Protocol == "nfs" {
			if len(s.Clients) > 0 {
				detail = fmt.Sprintf("%s → %s", s.Path, strings.Join(s.Clients, ", "))
			}
		} else {
			parts := []string{s.Path}
			if s.ReadOnly {
				parts = append(parts, "read-only")
			}
			if s.GuestOK {
				parts = append(parts, "guest")
			}
			if len(s.Users) > 0 {
				parts = append(parts, "users: "+strings.Join(s.Users, ", "))
			}
			detail = strings.Join(parts, " · ")
		}
		access := "read/write"
		if s.ReadOnly {
			access = "read-only"
		}
		shareRows = append(shareRows, []string{s.Protocol, s.Name, detail, access})
	}

	userRows := make([][]string, 0, len(users))
	for _, u := range users {
		userRows = append(userRows, []string{u.Username, u.UID})
	}

	sessionRows := make([][]string, 0, len(sessions))
	for _, s := range sessions {
		sessionRows = append(sessionRows, []string{s.Service, s.Machine, s.PID, s.Protocol})
	}

	nfsClientRows := make([][]string, 0, len(nfsClients))
	for _, c := range nfsClients {
		nfsClientRows = append(nfsClientRows, []string{c.Client, strings.Join(c.Shares, ", ")})
	}

	sendResult(req.ID, map[string]interface{}{
		"status":       "ok",
		"shares":       shares,
		"samba_users":  users,
		"smb_sessions": sessions,
		"locked_files": lockedFiles,
		"nfs_clients":  nfsClients,
		"metrics": []map[string]interface{}{
			{"category": "shares", "name": "shares_total", "value": len(shares), "unit": "shares", "status": "ok"},
			{"category": "shares", "name": "smb_shares", "value": smbCount, "unit": "shares", "status": "ok"},
			{"category": "shares", "name": "nfs_exports", "value": nfsCount, "unit": "exports", "status": "ok"},
			{"category": "shares", "name": "samba_users", "value": len(users), "unit": "users", "status": "ok"},
			{"category": "shares", "name": "active_connections", "value": len(sessions), "unit": "clients", "status": "ok"},
			{"category": "shares", "name": "locked_files", "value": lockedFiles, "unit": "files", "status": "ok"},
		},
		"tables": []map[string]interface{}{
			{
				"id":      "shares",
				"title":   "📁 Network Shares",
				"headers": []string{"Protocol", "Name", "Path / Clients", "Access"},
				"rows":    shareRows,
			},
			{
				"id":      "samba_users",
				"title":   "👤 Samba Users",
				"headers": []string{"Username", "UID"},
				"rows":    userRows,
			},
			{
				"id":      "smb_sessions",
				"title":   "🔌 Active SMB Sessions",
				"headers": []string{"Share", "Client", "PID", "Protocol"},
				"rows":    sessionRows,
			},
			{
				"id":      "nfs_clients",
				"title":   "🌐 NFS Clients",
				"headers": []string{"Client", "Mounted Exports"},
				"rows":    nfsClientRows,
			},
		},
	})
}

func strArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func boolArg(args map[string]interface{}, key string) bool {
	if args == nil {
		return false
	}
	if v, ok := args[key].(bool); ok {
		return v
	}
	return false
}

func strListArg(args map[string]interface{}, key string) []string {
	raw := strArg(args, key)
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func handleAction(req *RPCRequest) {
	var params struct {
		ActionID string                 `json:"action_id"`
		Action   string                 `json:"action"`
		Target   string                 `json:"target"`
		Args     map[string]interface{} `json:"args"`
	}
	if req.Params != nil {
		_ = json.Unmarshal(req.Params, &params)
	}
	// Flat args fallback: core may pass params without an "args" wrapper.
	args := params.Args
	if args == nil && len(req.Params) > 0 {
		var flat map[string]interface{}
		if json.Unmarshal(req.Params, &flat) == nil {
			args = flat
			delete(args, "action_id")
			delete(args, "action")
			delete(args, "target")
			delete(args, "args")
		}
	}

	action := params.ActionID
	if action == "" {
		action = params.Action
	}
	if action == "" {
		if v := strArg(args, "action_id"); v != "" {
			action = v
		} else if v := strArg(args, "action"); v != "" {
			action = v
		}
	}
	cfg := loadPluginConfig()

	fail := func(format string, a ...interface{}) {
		sendResult(req.ID, map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf(format, a...),
		})
	}
	ok := func(msg string, extra map[string]interface{}) {
		res := map[string]interface{}{"success": true, "message": msg}
		for k, v := range extra {
			res[k] = v
		}
		sendResult(req.ID, res)
	}

	switch action {
	case "get_shares":
		shares, users, sessions, locked, nfsClients, err := GetAllShares(cfg)
		if err != nil {
			fail("Failed to collect shares: %s", err)
			return
		}
		ok("Shares retrieved", map[string]interface{}{
			"shares": shares, "samba_users": users, "smb_sessions": sessions,
			"locked_files": locked, "nfs_clients": nfsClients,
		})

	case "create_smb_share":
		name := strArg(args, "name")
		if name == "" {
			name = params.Target
		}
		share := Share{
			Protocol: "smb",
			Name:     name,
			Path:     strArg(args, "path"),
			Comment:  strArg(args, "comment"),
			ReadOnly: boolArg(args, "read_only"),
			GuestOK:  boolArg(args, "guest_ok"),
			Users:    strListArg(args, "users"),
		}
		msg, err := CreateSmbShare(cfg, share)
		if err != nil {
			fail("%s", err)
			return
		}
		ok(msg, nil)

	case "delete_smb_share":
		name := strArg(args, "name")
		if name == "" {
			name = params.Target
		}
		msg, err := DeleteSmbShare(cfg, name)
		if err != nil {
			fail("%s", err)
			return
		}
		ok(msg, nil)

	case "create_nfs_export":
		path := strArg(args, "path")
		clients := strListArg(args, "clients")
		if len(clients) == 0 {
			if c := strArg(args, "client"); c != "" {
				clients = []string{c}
			}
		}
		options := strArg(args, "options")
		msg, err := CreateNFSExport(cfg, path, clients, options)
		if err != nil {
			fail("%s", err)
			return
		}
		ok(msg, nil)

	case "delete_nfs_export":
		path := strArg(args, "path")
		client := strArg(args, "client")
		msg, err := DeleteNFSExport(cfg, path, client)
		if err != nil {
			fail("%s", err)
			return
		}
		ok(msg, nil)

	case "create_smb_user":
		username := strArg(args, "username")
		password := strArg(args, "password")
		msg, err := CreateSmbUser(username, password)
		if err != nil {
			fail("%s", err)
			return
		}
		ok(msg, nil)

	case "delete_smb_user":
		username := strArg(args, "username")
		if username == "" {
			username = params.Target
		}
		msg, err := DeleteSmbUser(username)
		if err != nil {
			fail("%s", err)
			return
		}
		ok(msg, nil)

	case "set_smb_password":
		username := strArg(args, "username")
		password := strArg(args, "password")
		if username == "" || password == "" {
			fail("username and password are required")
			return
		}
		msg, err := CreateSmbUser(username, password) // smbpasswd -a falls back to password change
		if err != nil {
			fail("%s", err)
			return
		}
		ok(msg, nil)

	case "mount_share":
		protocol := strArg(args, "protocol")
		source := strArg(args, "source")
		target := strArg(args, "target")
		options := strArg(args, "options")
		msg, err := MountShare(protocol, source, target, options)
		if err != nil {
			fail("%s", err)
			return
		}
		ok(msg, nil)

	case "unmount_share":
		target := strArg(args, "target")
		if target == "" {
			target = params.Target
		}
		msg, err := UnmountShare(target)
		if err != nil {
			fail("%s", err)
			return
		}
		ok(msg, nil)

	case "restart_services":
		var services []string
		if v, ok := args["services"].([]interface{}); ok {
			for _, s := range v {
				if str, ok := s.(string); ok {
					services = append(services, str)
				}
			}
		} else if s := strArg(args, "services"); s != "" {
			services = strings.Split(s, ",")
		}
		msg, err := RestartServices(services)
		if err != nil {
			fail("%s", err)
			return
		}
		ok(msg, nil)

	case "get_settings":
		ok("Settings retrieved", map[string]interface{}{
			"settings": map[string]interface{}{
				"smb_conf_path": cfg.SmbConfPath,
				"exports_path":  cfg.ExportsPath,
				"auto_reload":   cfg.isAutoReload(),
			},
		})

	case "set_settings":
		newCfg := cfg
		if v := strArg(args, "smb_conf_path"); v != "" {
			newCfg.SmbConfPath = v
		}
		if v := strArg(args, "exports_path"); v != "" {
			newCfg.ExportsPath = v
		}
		if v, ok := args["auto_reload"].(bool); ok {
			newCfg.AutoReload = boolPtr(v)
		}
		if err := savePluginConfig(newCfg); err != nil {
			fail("Failed to save settings: %s", err)
			return
		}
		ok("Settings saved", map[string]interface{}{
			"settings": map[string]interface{}{
				"smb_conf_path": newCfg.SmbConfPath,
				"exports_path":  newCfg.ExportsPath,
				"auto_reload":   newCfg.isAutoReload(),
			},
		})

	default:
		fail("Unknown action: %s", action)
	}
}

func sendResult(id interface{}, result interface{}) {
	resBytes, _ := json.Marshal(result)
	resp := RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  resBytes,
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
