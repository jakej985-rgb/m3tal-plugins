package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
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
	PairingToken  string `json:"pairing_token"`
	ListenPort    int    `json:"listen_port"`
	BroadcastMDNS bool   `json:"broadcast_mdns"`
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
	cfg := nodeRuntimeConfig{ListenPort: 9321, BroadcastMDNS: true}
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

func startNodeNetworkService() {
	cfg := loadNodeRuntimeConfig()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", nodeTelemetryHandler)
	mux.HandleFunc("/api/v1/telemetry", nodeTelemetryHandler)

	server := &http.Server{Addr: ":" + strconv.Itoa(cfg.ListenPort), Handler: mux, ReadHeaderTimeout: 4 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
			go func() {
				<-nodeShutdown
				mdnsServer.Shutdown()
			}()
		}
	}

	go func() {
		<-nodeShutdown
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
}

func nodeTelemetryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	current := loadNodeRuntimeConfig()
	if current.PairingToken == "" {
		http.Error(w, "pairing token is not configured on this node", http.StatusServiceUnavailable)
		return
	}
	supplied := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if subtle.ConstantTimeCompare([]byte(supplied), []byte(current.PairingToken)) != 1 {
		http.Error(w, "invalid pairing token", http.StatusUnauthorized)
		return
	}
	hostname, _ := os.Hostname()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name": hostname, "hostname": hostname, "version": PluginVersion,
		"telemetry": getTelemetry(),
	})
}

var nodeShutdown = make(chan struct{})
var nodeShutdownOnce sync.Once

func shutdownNodeNetwork() {
	nodeShutdownOnce.Do(func() { close(nodeShutdown) })
}
