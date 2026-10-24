package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StackInfo represents metadata and state of a docker compose project.
type StackInfo struct {
	Name          string   `json:"name"`
	Path          string   `json:"path"`
	Services      []string `json:"services"`
	ServicesCount int      `json:"services_count"`
	RunningCount  int      `json:"running_count"`
	Status        string   `json:"status"` // "running", "stopped", "partial"
	UpdatedAt     string   `json:"updated_at"`
	YAMLContent   string   `json:"yaml_content,omitempty"`
}

var (
	mockMu     sync.RWMutex
	isMockMode = false
	mockStacks = []StackInfo{
		{
			Name:          "proxy-gateway",
			Path:          "/etc/m3tal/compose/proxy-gateway/compose.yaml",
			Services:      []string{"traefik", "authelia", "redis"},
			ServicesCount: 3,
			RunningCount:  3,
			Status:        "running",
			UpdatedAt:     "2026-10-24 10:15:00",
			YAMLContent: `services:
  traefik:
    image: traefik:v3.0
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
  redis:
    image: redis:7-alpine
    restart: unless-stopped
`,
		},
		{
			Name:          "monitoring",
			Path:          "/etc/m3tal/compose/monitoring/compose.yaml",
			Services:      []string{"prometheus", "grafana", "node-exporter"},
			ServicesCount: 3,
			RunningCount:  3,
			Status:        "running",
			UpdatedAt:     "2026-10-24 11:30:00",
			YAMLContent: `services:
  prometheus:
    image: prom/prometheus:latest
    restart: unless-stopped
    ports:
      - "9090:9090"
  grafana:
    image: grafana/grafana:latest
    restart: unless-stopped
    ports:
      - "3000:3000"
`,
		},
	}
)

func SetMockMode(mock bool) {
	mockMu.Lock()
	defer mockMu.Unlock()
	isMockMode = mock
}

func getBaseComposeDir() string {
	if dir := os.Getenv("M3TAL_COMPOSE_DIR"); dir != "" {
		return dir
	}
	return "/etc/m3tal/compose"
}

// ListStacks returns all discovered compose stacks.
func ListStacks() ([]StackInfo, error) {
	mockMu.RLock()
	if isMockMode {
		defer mockMu.RUnlock()
		return mockStacks, nil
	}
	mockMu.RUnlock()

	baseDir := getBaseComposeDir()
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		// Return mock stacks if compose directory not initialized yet
		mockMu.RLock()
		defer mockMu.RUnlock()
		return mockStacks, nil
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		mockMu.RLock()
		defer mockMu.RUnlock()
		return mockStacks, nil
	}

	var results []StackInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stackPath := filepath.Join(baseDir, entry.Name())
		composeFile := ""
		for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
			candidate := filepath.Join(stackPath, name)
			if _, err := os.Stat(candidate); err == nil {
				composeFile = candidate
				break
			}
		}

		if composeFile == "" {
			continue
		}

		content, _ := os.ReadFile(composeFile)
		info := inspectStack(entry.Name(), composeFile, string(content))
		results = append(results, info)
	}

	if len(results) == 0 {
		mockMu.RLock()
		defer mockMu.RUnlock()
		return mockStacks, nil
	}

	return results, nil
}

func inspectStack(name, path, yamlContent string) StackInfo {
	services := extractServices(yamlContent)
	status := "stopped"
	running := 0

	// Check runtime status with docker compose ps if docker available
	cmd := exec.Command("docker", "compose", "-f", path, "ps", "--format", "{{.Service}}:{{.State}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil && out.Len() > 0 {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), "running") {
				running++
			}
		}
		if running == len(services) && len(services) > 0 {
			status = "running"
		} else if running > 0 {
			status = "partial"
		}
	}

	return StackInfo{
		Name:          name,
		Path:          path,
		Services:      services,
		ServicesCount: len(services),
		RunningCount:  running,
		Status:        status,
		UpdatedAt:     time.Now().Format("2006-01-02 15:04:05"),
		YAMLContent:   yamlContent,
	}
}

func extractServices(yamlContent string) []string {
	var services []string
	lines := strings.Split(yamlContent, "\n")
	inServices := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "services:" {
			inServices = true
			continue
		}
		if inServices {
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && trimmed != "" {
				inServices = false
				continue
			}
			// Top-level service indentation is 2 spaces
			if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
				parts := strings.Split(trimmed, ":")
				if len(parts) > 0 && parts[0] != "" && !strings.HasPrefix(parts[0], "#") {
					services = append(services, parts[0])
				}
			}
		}
	}
	return services
}

// ValidateYAML performs lightweight structure and syntax checks on compose YAML.
func ValidateYAML(content string) (bool, string, error) {
	if strings.TrimSpace(content) == "" {
		return false, "YAML content is empty", nil
	}
	if !strings.Contains(content, "services:") {
		return false, "Missing top-level 'services:' section", nil
	}
	return true, "YAML structure is valid", nil
}

// SaveStack saves or creates a compose stack definition.
func SaveStack(name, content string) error {
	mockMu.Lock()
	defer mockMu.Unlock()

	if isMockMode {
		for i, s := range mockStacks {
			if s.Name == name {
				mockStacks[i].YAMLContent = content
				mockStacks[i].Services = extractServices(content)
				mockStacks[i].ServicesCount = len(mockStacks[i].Services)
				return nil
			}
		}
		mockStacks = append(mockStacks, StackInfo{
			Name:          name,
			Path:          filepath.Join("/etc/m3tal/compose", name, "compose.yaml"),
			Services:      extractServices(content),
			ServicesCount: len(extractServices(content)),
			RunningCount:  0,
			Status:        "stopped",
			UpdatedAt:     time.Now().Format("2006-01-02 15:04:05"),
			YAMLContent:   content,
		})
		return nil
	}

	stackDir := filepath.Join(getBaseComposeDir(), name)
	if err := os.MkdirAll(stackDir, 0755); err != nil {
		return fmt.Errorf("failed to create stack directory: %w", err)
	}

	filePath := filepath.Join(stackDir, "compose.yaml")
	return os.WriteFile(filePath, []byte(content), 0644)
}

// ComposeUp starts all services in a stack.
func ComposeUp(name string) (string, error) {
	mockMu.Lock()
	if isMockMode {
		for i, s := range mockStacks {
			if s.Name == name {
				mockStacks[i].Status = "running"
				mockStacks[i].RunningCount = mockStacks[i].ServicesCount
				mockMu.Unlock()
				return fmt.Sprintf("Stack '%s' started successfully (mock)", name), nil
			}
		}
		mockMu.Unlock()
		return "", fmt.Errorf("stack '%s' not found", name)
	}
	mockMu.Unlock()

	composePath := filepath.Join(getBaseComposeDir(), name, "compose.yaml")
	cmd := exec.Command("docker", "compose", "-f", composePath, "up", "-d")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose up failed: %w (output: %s)", err, string(out))
	}
	return string(out), nil
}

// ComposeDown stops all services in a stack.
func ComposeDown(name string) (string, error) {
	mockMu.Lock()
	if isMockMode {
		for i, s := range mockStacks {
			if s.Name == name {
				mockStacks[i].Status = "stopped"
				mockStacks[i].RunningCount = 0
				mockMu.Unlock()
				return fmt.Sprintf("Stack '%s' stopped successfully (mock)", name), nil
			}
		}
		mockMu.Unlock()
		return "", fmt.Errorf("stack '%s' not found", name)
	}
	mockMu.Unlock()

	composePath := filepath.Join(getBaseComposeDir(), name, "compose.yaml")
	cmd := exec.Command("docker", "compose", "-f", composePath, "down")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose down failed: %w (output: %s)", err, string(out))
	}
	return string(out), nil
}

// ComposeRestart restarts all services in a stack.
func ComposeRestart(name string) (string, error) {
	mockMu.Lock()
	if isMockMode {
		mockMu.Unlock()
		return fmt.Sprintf("Stack '%s' restarted successfully (mock)", name), nil
	}
	mockMu.Unlock()

	composePath := filepath.Join(getBaseComposeDir(), name, "compose.yaml")
	cmd := exec.Command("docker", "compose", "-f", composePath, "restart")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose restart failed: %w (output: %s)", err, string(out))
	}
	return string(out), nil
}

// ComposeLogs fetches tail logs for a stack.
func ComposeLogs(name string, tail int) (string, error) {
	mockMu.RLock()
	if isMockMode {
		defer mockMu.RUnlock()
		return fmt.Sprintf("[%s] Mock container log line 1\n[%s] Mock container log line 2\n[%s] Service ready", name, name, name), nil
	}
	mockMu.RUnlock()

	composePath := filepath.Join(getBaseComposeDir(), name, "compose.yaml")
	cmd := exec.Command("docker", "compose", "-f", composePath, "logs", fmt.Sprintf("--tail=%d", tail))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose logs failed: %w", err)
	}
	return string(out), nil
}
