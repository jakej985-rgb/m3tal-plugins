package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// StackInfo represents metadata and state of a docker compose project.
type StackInfo struct {
	Name          string            `json:"name"`
	Path          string            `json:"path"`
	Services      []string          `json:"services"`
	ServicesCount int               `json:"services_count"`
	RunningCount  int               `json:"running_count"`
	Status        string            `json:"status"` // "running", "stopped", "partial"
	UpdatedAt     string            `json:"updated_at"`
	YAMLContent   string            `json:"yaml_content,omitempty"`
	ConfigFiles   []string          `json:"config_files,omitempty"`
	ProjectName   string            `json:"project_name,omitempty"`
	ServiceStates map[string]string `json:"service_states,omitempty"`
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
	settingsMu        sync.RWMutex
	composeDirSetting string
	newStackLayout    = "per_stack_folder"
	backupOnSave      = true
)

func SetMockMode(mock bool) {
	mockMu.Lock()
	defer mockMu.Unlock()
	isMockMode = mock
}

func getBaseComposeDir() string {
	// Preserve the established environment override for managed installations.
	if dir := os.Getenv("M3TAL_COMPOSE_DIR"); dir != "" {
		return expandUserPath(dir)
	}
	settingsMu.RLock()
	dir := composeDirSetting
	settingsMu.RUnlock()
	if strings.TrimSpace(dir) == "" {
		dir = "~/docker"
	}
	return expandUserPath(dir)
}

func expandUserPath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return filepath.Clean(path)
}

func SetPluginSettings(settings map[string]interface{}) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if value, ok := settings["compose_dir"]; ok {
		dir, valid := value.(string)
		if !valid || strings.TrimSpace(dir) == "" {
			return fmt.Errorf("compose_dir must be a non-empty path")
		}
		composeDirSetting = expandUserPath(strings.TrimSpace(dir))
	}
	if value, ok := settings["new_stack_layout"]; ok {
		layout, valid := value.(string)
		if !valid || (layout != "per_stack_folder" && layout != "flat") {
			return fmt.Errorf("new_stack_layout must be 'per_stack_folder' or 'flat'")
		}
		newStackLayout = layout
	}
	if value, ok := settings["backup_on_save"]; ok {
		backup, valid := value.(bool)
		if !valid {
			return fmt.Errorf("backup_on_save must be a boolean")
		}
		backupOnSave = backup
	}
	return nil
}

// resolveStackPath returns the first Compose file for a discovered stack.
func resolveStackPath(name string) string {
	files, _, err := resolveStackFiles(name)
	if err == nil && len(files) > 0 {
		return files[0]
	}
	return ""
}

func resolveStackFiles(name string) ([]string, string, error) {
	if info, found := findStack(name); found {
		files := info.ConfigFiles
		if len(files) == 0 && info.Path != "" {
			files = []string{info.Path}
		}
		project := info.ProjectName
		if project == "" {
			project = name
		}
		return files, project, nil
	}
	if !validStackName(name) {
		return nil, "", fmt.Errorf("invalid stack name")
	}
	baseDir := getBaseComposeDir()
	stackDir := filepath.Join(baseDir, name)

	for _, candidateName := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		candidate := filepath.Join(stackDir, candidateName)
		if _, err := os.Stat(candidate); err == nil {
			return []string{candidate}, name, nil
		}
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
							return normalizeConfigFiles(files), p.Name, nil
						}
					}
				}
			}
		}
	}

	return nil, "", fmt.Errorf("compose file for stack %q not found", name)
}

func validStackName(name string) bool {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func normalizeConfigFiles(files []string) []string {
	result := make([]string, 0, len(files))
	for _, file := range files {
		if file = strings.TrimSpace(file); file != "" {
			result = append(result, file)
		}
	}
	return result
}

func findStack(name string) (StackInfo, bool) {
	stacks, err := ListStacks()
	if err != nil {
		return StackInfo{}, false
	}
	for _, stack := range stacks {
		if stack.Name == name || stack.ProjectName == name {
			return stack, true
		}
	}
	return StackInfo{}, false
}

// ListStacks returns all discovered compose stacks.
func ListStacks() ([]StackInfo, error) {
	mockMu.RLock()
	if isMockMode {
		defer mockMu.RUnlock()
		return mockStacks, nil
	}
	mockMu.RUnlock()

	results := make(map[string]StackInfo)
	seenNames := make(map[string]bool)
	baseDir := getBaseComposeDir()
	_ = filepath.WalkDir(baseDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != baseDir && (entry.Type()&os.ModeSymlink != 0 || ignoredComposeDir(entry.Name()) || pathDepth(baseDir, path) > 10) {
				return filepath.SkipDir
			}
			return nil
		}
		flatFile := filepath.Dir(path) == baseDir && (strings.EqualFold(filepath.Ext(entry.Name()), ".yaml") || strings.EqualFold(filepath.Ext(entry.Name()), ".yml"))
		if !isComposeFile(entry.Name()) && !flatFile {
			return nil
		}
		// Choose one canonical Compose file per project folder. Flat files are
		// individually named projects, so each YAML file in the root is eligible.
		if !flatFile {
			preferred := ""
			for _, candidateName := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
				candidate := filepath.Join(filepath.Dir(path), candidateName)
				if _, err := os.Stat(candidate); err == nil {
					preferred = candidate
					break
				}
			}
			if preferred != path {
				return nil
			}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		name := filepath.Base(filepath.Dir(path))
		if filepath.Dir(path) == baseDir {
			name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		projectName := extractComposeProjectName(string(content))
		if projectName == "" {
			projectName = normalizeProjectName(name)
		} else {
			projectName = normalizeProjectName(projectName)
		}
		info := inspectStack(name, defaultComposeFiles(path), projectName, string(content))
		key, _ := filepath.Abs(path)
		if key == "" {
			key = filepath.Clean(path)
		}
		results[key] = info
		seenNames[projectName] = true
		return nil
	})

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
				if cp.ConfigFiles == "" {
					continue
				}
				files := normalizeConfigFiles(strings.Split(cp.ConfigFiles, ","))
				if len(files) == 0 {
					continue
				}
				key, _ := filepath.Abs(files[0])
				if key == "" {
					key = filepath.Clean(files[0])
				}
				if seenNames[cp.Name] {
					if _, found := results[key]; !found {
						continue
					}
				}
				content, err := os.ReadFile(files[0])
				if err != nil {
					continue
				}
				info := inspectStack(cp.Name, files, cp.Name, string(content))
				results[key] = info
				seenNames[cp.Name] = true
			}
		}
	}

	// Return actual results (empty slice if none found; never leak mock data into production)
	stacks := make([]StackInfo, 0, len(results))
	for _, stack := range results {
		stacks = append(stacks, stack)
	}
	sort.Slice(stacks, func(i, j int) bool { return strings.ToLower(stacks[i].Name) < strings.ToLower(stacks[j].Name) })
	return stacks, nil
}

func isComposeFile(name string) bool {
	switch strings.ToLower(name) {
	case "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml":
		return true
	}
	return false
}
func ignoredComposeDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".cache", ".local", ".config", ".npm", ".venv", "venv", "proc", "sys", "dev", "run", "tmp":
		return true
	}
	return strings.HasPrefix(name, ".")
}
func pathDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(os.PathSeparator)) + 1
}

func extractComposeProjectName(content string) string {
	var config map[string]interface{}
	if yaml.Unmarshal([]byte(content), &config) != nil {
		return ""
	}
	if name, ok := config["name"].(string); ok {
		return strings.TrimSpace(name)
	}
	return ""
}

func defaultComposeFiles(primary string) []string {
	files := []string{primary}
	candidates := []string{}
	switch filepath.Base(primary) {
	case "compose.yaml":
		candidates = []string{"compose.override.yaml", "compose.override.yml"}
	case "compose.yml":
		candidates = []string{"compose.override.yml", "compose.override.yaml"}
	case "docker-compose.yaml":
		candidates = []string{"docker-compose.override.yaml", "docker-compose.override.yml"}
	case "docker-compose.yml":
		candidates = []string{"docker-compose.override.yml", "docker-compose.override.yaml"}
	}
	for _, candidateName := range candidates {
		candidate := filepath.Join(filepath.Dir(primary), candidateName)
		if _, err := os.Stat(candidate); err == nil {
			files = append(files, candidate)
			break
		}
	}
	return files
}

func normalizeProjectName(name string) string {
	var normalized strings.Builder
	for _, char := range strings.ToLower(strings.TrimSpace(name)) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			normalized.WriteRune(char)
		} else {
			normalized.WriteRune('-')
		}
	}
	return strings.Trim(normalized.String(), "-.")
}

func inspectStack(name string, files []string, projectName, yamlContent string) StackInfo {
	services := extractServices(yamlContent)
	status := "stopped"
	running := 0
	serviceStates := make(map[string]string)

	args := composeArgs(files, projectName)
	args = append(args, "ps", "--format", "{{.Service}}:{{.State}}")
	cmd := exec.Command("docker", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil && out.Len() > 0 {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		for _, line := range lines {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				service := strings.TrimSpace(parts[0])
				state := strings.TrimSpace(parts[1])
				if previous, found := serviceStates[service]; found && !strings.EqualFold(previous, state) {
					serviceStates[service] = "partial"
				} else if !found {
					serviceStates[service] = state
				}
			}
		}
		for _, state := range serviceStates {
			if strings.EqualFold(state, "running") {
				running++
			}
		}
		if running >= len(services) && len(services) > 0 {
			status = "running"
		} else if running > 0 {
			status = "partial"
		}
	}

	path := ""
	if len(files) > 0 {
		path = files[0]
	}
	updatedAt := ""
	if stat, err := os.Stat(path); err == nil {
		updatedAt = stat.ModTime().Format("2006-01-02 15:04:05")
	}
	return StackInfo{
		Name:          name,
		Path:          path,
		ConfigFiles:   files,
		ProjectName:   projectName,
		ServiceStates: serviceStates,
		Services:      services,
		ServicesCount: len(services),
		RunningCount:  running,
		Status:        status,
		UpdatedAt:     updatedAt,
		YAMLContent:   yamlContent,
	}
}

func extractServices(yamlContent string) []string {
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &root); err != nil {
		return nil
	}
	services, ok := root["services"].(map[string]interface{})
	if !ok {
		return nil
	}
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateYAML performs structure and syntax checks on compose YAML.
func ValidateYAML(content string) (bool, string, error) {
	if strings.TrimSpace(content) == "" {
		return false, "YAML content is empty", nil
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return false, "Invalid YAML: " + err.Error(), nil
	}
	services, ok := root["services"].(map[string]interface{})
	if !ok {
		return false, "A top-level 'services' mapping is required", nil
	}
	if len(services) == 0 {
		return false, "At least one service must be defined", nil
	}
	return true, fmt.Sprintf("YAML syntax is valid (%d services defined)", len(services)), nil
}

// SaveStack saves or creates a compose stack definition.
func SaveStack(name, content string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("stack name is required")
	}
	if valid, message, _ := ValidateYAML(content); !valid {
		return fmt.Errorf("%s", message)
	}
	mockMu.Lock()
	if isMockMode {
		for i, s := range mockStacks {
			if s.Name == name {
				mockStacks[i].YAMLContent = content
				mockStacks[i].Services = extractServices(content)
				mockStacks[i].ServicesCount = len(mockStacks[i].Services)
				mockMu.Unlock()
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
		mockMu.Unlock()
		return nil
	}
	mockMu.Unlock()

	filePath := ""
	if existing, found := findStack(name); found {
		filePath = existing.Path
	} else {
		if !validStackName(name) {
			return fmt.Errorf("invalid stack name")
		}
		root := getBaseComposeDir()
		settingsMu.RLock()
		layout := newStackLayout
		settingsMu.RUnlock()
		if layout == "flat" {
			filePath = filepath.Join(root, name+".yaml")
		} else {
			filePath = filepath.Join(root, name, "compose.yaml")
		}
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create compose directory: %w", err)
	}
	settingsMu.RLock()
	shouldBackup := backupOnSave
	settingsMu.RUnlock()
	if shouldBackup {
		if old, err := os.ReadFile(filePath); err == nil {
			if err := atomicWriteFile(filePath+".bak", old, 0644); err != nil {
				return fmt.Errorf("failed to back up compose file: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read compose file: %w", err)
		}
	}
	return atomicWriteFile(filePath, []byte(content), 0644)
}

func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".compose-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("set temporary file mode: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}

func composeArgs(files []string, projectName string) []string {
	args := []string{"compose"}
	if projectName != "" {
		args = append(args, "-p", projectName)
	}
	for _, file := range files {
		args = append(args, "-f", file)
	}
	return args
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

	files, project, err := resolveStackFiles(name)
	if err != nil {
		return "", err
	}
	args := composeArgs(files, project)
	args = append(args, "up", "-d")
	cmd := exec.Command("docker", args...)
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

	files, project, err := resolveStackFiles(name)
	if err != nil {
		return "", err
	}
	args := composeArgs(files, project)
	args = append(args, "down")
	cmd := exec.Command("docker", args...)
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

	files, project, err := resolveStackFiles(name)
	if err != nil {
		return "", err
	}
	args := composeArgs(files, project)
	args = append(args, "restart")
	cmd := exec.Command("docker", args...)
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

	files, project, err := resolveStackFiles(name)
	if err != nil {
		return "", err
	}
	args := composeArgs(files, project)
	args = append(args, "logs", fmt.Sprintf("--tail=%d", tail))
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose logs failed: %w", err)
	}
	return string(out), nil
}
