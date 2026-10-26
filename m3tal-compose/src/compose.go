package main

import (
	"bytes"
	"encoding/json"
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

// resolveStackPath finds the primary compose file for a stack by checking:
// 1. /etc/m3tal/compose/<name>/ (or $M3TAL_COMPOSE_DIR/<name>/)
// 2. Existing file path if name is a direct file path
// 3. Docker Compose native registry (docker compose ls -a)
// 4. Default fallback path in base directory
func resolveStackPath(name string) string {
	baseDir := getBaseComposeDir()
	stackDir := filepath.Join(baseDir, name)

	for _, candidateName := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		candidate := filepath.Join(stackDir, candidateName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	if _, err := os.Stat(name); err == nil {
		return name
	}

	// Query host-wide compose registry for matching project name
	cmd := exec.Command("docker", "compose", "ls", "-a", "--format", "json")
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		var projects []struct {
			Name        string `json:"Name"`
			ConfigFiles string `json:"ConfigFiles"`
		}
		if err := json.Unmarshal(out, &projects); err == nil {
			for _, p := range projects {
				if p.Name == name && p.ConfigFiles != "" {
					files := strings.Split(p.ConfigFiles, ",")
					if len(files) > 0 {
						primary := strings.TrimSpace(files[0])
						if primary != "" {
							return primary
						}
					}
				}
			}
		}
	}

	return filepath.Join(baseDir, name, "compose.yaml")
}

// ListStacks returns all discovered compose stacks.
func ListStacks() ([]StackInfo, error) {
	mockMu.RLock()
	if isMockMode {
		defer mockMu.RUnlock()
		return mockStacks, nil
	}
	mockMu.RUnlock()

	var results []StackInfo
	seenPaths := make(map[string]bool)
	seenNames := make(map[string]bool)

	// 1. Scan local base directory (/etc/m3tal/compose or $M3TAL_COMPOSE_DIR)
	baseDir := getBaseComposeDir()
	if entries, err := os.ReadDir(baseDir); err == nil {
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
			seenPaths[composeFile] = true
			seenNames[entry.Name()] = true
		}
	}

	// 2. Discover host-wide compose projects using docker compose ls -a
	cmd := exec.Command("docker", "compose", "ls", "-a", "--format", "json")
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		var composeProjects []struct {
			Name        string `json:"Name"`
			Status      string `json:"Status"`
			ConfigFiles string `json:"ConfigFiles"`
		}
		if err := json.Unmarshal(out, &composeProjects); err == nil {
			for _, cp := range composeProjects {
				if seenNames[cp.Name] {
					continue
				}
				if cp.ConfigFiles == "" {
					continue
				}
				files := strings.Split(cp.ConfigFiles, ",")
				primaryFile := strings.TrimSpace(files[0])
				if primaryFile == "" || seenPaths[primaryFile] {
					continue
				}

				if content, err := os.ReadFile(primaryFile); err == nil {
					info := inspectStack(cp.Name, primaryFile, string(content))
					results = append(results, info)
					seenPaths[primaryFile] = true
					seenNames[cp.Name] = true
				}
			}
		}
	}

	// Return actual results (empty slice if none found; never leak mock data into production)
	return results, nil
}

func inspectStack(name, path, yamlContent string) StackInfo {
	services := extractServices(yamlContent)
	status := "stopped"
	running := 0

	// Check runtime status with docker compose ps if docker is available
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
		if running >= len(services) && len(services) > 0 {
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
	serviceIndent := -1

	for _, line := range lines {
		// Strip comments
		commentIdx := strings.Index(line, "#")
		cleanLine := line
		if commentIdx >= 0 {
			cleanLine = line[:commentIdx]
		}
		trimmed := strings.TrimSpace(cleanLine)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "services:") {
			after := strings.TrimSpace(strings.TrimPrefix(trimmed, "services:"))
			if strings.HasPrefix(after, "{") {
				inner := strings.Trim(after, "{}")
				parts := strings.Split(inner, ":")
				if len(parts) > 0 {
					svcName := strings.TrimSpace(parts[0])
					svcName = strings.Trim(svcName, `"'`)
					if svcName != "" {
						services = append(services, svcName)
					}
				}
				continue
			}
			inServices = true
			serviceIndent = -1
			continue
		}

		if inServices {
			// Count leading spaces
			leadingSpaces := len(cleanLine) - len(strings.TrimLeft(cleanLine, " "))
			if leadingSpaces == 0 {
				// Reached next top-level root key (e.g., volumes:, networks:)
				inServices = false
				continue
			}

			if serviceIndent == -1 {
				serviceIndent = leadingSpaces
			}

			// If indentation matches the service definition level
			if leadingSpaces == serviceIndent {
				parts := strings.Split(trimmed, ":")
				if len(parts) > 0 {
					svcName := strings.TrimSpace(parts[0])
					svcName = strings.Trim(svcName, `"'`)
					if svcName != "" {
						services = append(services, svcName)
					}
				}
			}
		}
	}
	return services
}

// ValidateYAML performs structure and syntax checks on compose YAML.
func ValidateYAML(content string) (bool, string, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false, "YAML content is empty", nil
	}
	if !strings.Contains(content, "services:") {
		return false, "Missing top-level 'services:' section", nil
	}

	// Basic syntax validation: braces & brackets balancing
	openBraces := strings.Count(content, "{")
	closeBraces := strings.Count(content, "}")
	if openBraces != closeBraces {
		return false, fmt.Sprintf("Mismatched braces: %d '{' vs %d '}'", openBraces, closeBraces), nil
	}
	openBrackets := strings.Count(content, "[")
	closeBrackets := strings.Count(content, "]")
	if openBrackets != closeBrackets {
		return false, fmt.Sprintf("Mismatched brackets: %d '[' vs %d ']'", openBrackets, closeBrackets), nil
	}

	services := extractServices(content)
	if len(services) == 0 {
		return false, "No valid services defined under 'services:' section", nil
	}

	return true, fmt.Sprintf("YAML structure is valid (%d services defined)", len(services)), nil
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

	composePath := resolveStackPath(name)
	if _, err := os.Stat(composePath); err != nil {
		return "", fmt.Errorf("compose file for stack '%s' not found at %s", name, composePath)
	}

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

	composePath := resolveStackPath(name)
	if _, err := os.Stat(composePath); err != nil {
		return "", fmt.Errorf("compose file for stack '%s' not found at %s", name, composePath)
	}

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

	composePath := resolveStackPath(name)
	if _, err := os.Stat(composePath); err != nil {
		return "", fmt.Errorf("compose file for stack '%s' not found at %s", name, composePath)
	}

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

	composePath := resolveStackPath(name)
	if _, err := os.Stat(composePath); err != nil {
		return "", fmt.Errorf("compose file for stack '%s' not found at %s", name, composePath)
	}

	cmd := exec.Command("docker", "compose", "-f", composePath, "logs", fmt.Sprintf("--tail=%d", tail))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose logs failed: %w", err)
	}
	return string(out), nil
}
