package main

import (
	"bytes"
	"encoding/base64"
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
	ID            string            `json:"id"`
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
	var newDir *string
	var newLayout *string
	var newBackup *bool
	if value, ok := settings["compose_dir"]; ok {
		dir, valid := value.(string)
		if !valid || strings.TrimSpace(dir) == "" {
			return fmt.Errorf("compose_dir must be a non-empty path")
		}
		expandedDir := expandUserPath(strings.TrimSpace(dir))
		newDir = &expandedDir
	}
	if value, ok := settings["new_stack_layout"]; ok {
		layout, valid := value.(string)
		if !valid || (layout != "per_stack_folder" && layout != "flat") {
			return fmt.Errorf("new_stack_layout must be 'per_stack_folder' or 'flat'")
		}
		newLayout = &layout
	}
	if value, ok := settings["backup_on_save"]; ok {
		backup, valid := value.(bool)
		if !valid {
			return fmt.Errorf("backup_on_save must be a boolean")
		}
		newBackup = &backup
	}
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if newDir != nil {
		composeDirSetting = *newDir
	}
	if newLayout != nil {
		newStackLayout = *newLayout
	}
	if newBackup != nil {
		backupOnSave = *newBackup
	}
	return nil
}

type composeProject struct {
	Name        string `json:"Name"`
	Status      string `json:"Status"`
	ConfigFiles string `json:"ConfigFiles"`
}

const registeredComposeIDPrefix = "compose-project:"

func registeredComposeID(path, projectName string) string {
	encodedPath := base64.RawURLEncoding.EncodeToString([]byte(canonicalPath(path)))
	encodedName := base64.RawURLEncoding.EncodeToString([]byte(projectName))
	return registeredComposeIDPrefix + encodedPath + "." + encodedName
}

func parseRegisteredComposeID(identifier string) (string, string, bool) {
	if !strings.HasPrefix(identifier, registeredComposeIDPrefix) {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(identifier, registeredComposeIDPrefix), ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	pathBytes, pathErr := base64.RawURLEncoding.DecodeString(parts[0])
	nameBytes, nameErr := base64.RawURLEncoding.DecodeString(parts[1])
	if pathErr != nil || nameErr != nil || !filepath.IsAbs(string(pathBytes)) || len(nameBytes) == 0 {
		return "", "", false
	}
	return string(pathBytes), string(nameBytes), true
}

func dockerComposeProjects() ([]composeProject, error) {
	cmd := exec.Command("docker", "compose", "ls", "-a", "--format", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var projects []composeProject
	if err := json.Unmarshal(out, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

var composeProjectsSource = dockerComposeProjects

func resolvedComposeRoot() string {
	root := getBaseComposeDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return canonicalPath(root)
}

func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative)
}

func isOverrideComposeFile(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, ".override.")
}

func allowedComposePath(root, path string) bool {
	if !pathWithin(root, path) || isOverrideComposeFile(filepath.Base(path)) {
		return false
	}
	if isComposeFile(filepath.Base(path)) {
		return true
	}
	return filepath.Dir(path) == root && (strings.EqualFold(filepath.Ext(path), ".yaml") || strings.EqualFold(filepath.Ext(path), ".yml"))
}

func registeredProjectForPath(projects []composeProject, path string) (composeProject, []string, bool) {
	path = canonicalPath(path)
	for _, project := range projects {
		files := normalizeConfigFiles(strings.Split(project.ConfigFiles, ","))
		for _, file := range files {
			if canonicalPath(file) == path {
				return project, files, true
			}
		}
	}
	return composeProject{}, nil, false
}

// resolveStackFiles resolves a stack by its stable Compose-file ID. Legacy
// callers may pass a safe new-stack name, which is resolved only in the root.
func resolveStackFiles(identifier string) ([]string, string, error) {
	root := resolvedComposeRoot()
	projects, _ := composeProjectsSource()
	if primary, projectName, ok := parseRegisteredComposeID(identifier); ok {
		for _, project := range projects {
			files := normalizeConfigFiles(strings.Split(project.ConfigFiles, ","))
			if project.Name == projectName && len(files) > 0 && canonicalPath(files[0]) == canonicalPath(primary) {
				return files, project.Name, nil
			}
		}
		return nil, "", fmt.Errorf("registered Compose project %q not found", projectName)
	}
	if filepath.IsAbs(identifier) {
		path := canonicalPath(identifier)
		if project, files, found := registeredProjectForPath(projects, path); found {
			return files, project.Name, nil
		}
		if !allowedComposePath(root, path) {
			return nil, "", fmt.Errorf("Compose file is not in the search folder or a registered project")
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("Compose file not found: %s", path)
		}
		return defaultComposeFiles(path), flatFileProjectName(path, root, string(content)), nil
	}
	if !validStackName(identifier) {
		return nil, "", fmt.Errorf("invalid stack name")
	}
	for _, candidate := range []string{
		filepath.Join(root, identifier, "compose.yaml"),
		filepath.Join(root, identifier, "compose.yml"),
		filepath.Join(root, identifier, "docker-compose.yaml"),
		filepath.Join(root, identifier, "docker-compose.yml"),
		filepath.Join(root, identifier+".yaml"),
		filepath.Join(root, identifier+".yml"),
	} {
		if _, err := os.Stat(candidate); err == nil && allowedComposePath(root, canonicalPath(candidate)) {
			if project, files, found := registeredProjectForPath(projects, candidate); found {
				return files, project.Name, nil
			}
			content, readErr := os.ReadFile(candidate)
			if readErr != nil {
				return nil, "", readErr
			}
			return defaultComposeFiles(candidate), flatFileProjectName(canonicalPath(candidate), root, string(content)), nil
		}
	}
	for _, project := range projects {
		if project.Name == identifier {
			files := normalizeConfigFiles(strings.Split(project.ConfigFiles, ","))
			if len(files) > 0 {
				return files, project.Name, nil
			}
		}
	}
	return nil, "", fmt.Errorf("Compose project %q not found", identifier)
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

// ListStacks returns all discovered compose stacks.
func ListStacks() ([]StackInfo, error) {
	mockMu.RLock()
	if isMockMode {
		defer mockMu.RUnlock()
		return mockStacks, nil
	}
	mockMu.RUnlock()

	results := make(map[string]StackInfo)
	baseDir := resolvedComposeRoot()
	composeProjects, projectsErr := composeProjectsSource()
	if projectsErr != nil {
		composeProjects = nil
	}
	registeredSecondaries := make(map[string]bool)
	registeredPrimaries := make(map[string]bool)
	for _, project := range composeProjects {
		files := normalizeConfigFiles(strings.Split(project.ConfigFiles, ","))
		if len(files) > 0 {
			registeredPrimaries[canonicalPath(files[0])] = true
		}
		if len(files) > 1 {
			for _, file := range files[1:] {
				registeredSecondaries[canonicalPath(file)] = true
			}
		}
	}
	_ = filepath.WalkDir(baseDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != baseDir && (entry.Type()&os.ModeSymlink != 0 || ignoredComposeDir(entry.Name()) || isSystemComposeDirectory(path) || pathDepth(baseDir, path) > 10) {
				return filepath.SkipDir
			}
			return nil
		}
		flatFile := filepath.Dir(path) == baseDir && !isOverrideComposeFile(entry.Name()) && (strings.EqualFold(filepath.Ext(entry.Name()), ".yaml") || strings.EqualFold(filepath.Ext(entry.Name()), ".yml"))
		if !isComposeFile(entry.Name()) && !flatFile {
			return nil
		}
		if registeredSecondaries[canonicalPath(path)] || registeredPrimaries[canonicalPath(path)] {
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
		if flatFile && len(extractServices(string(content))) == 0 {
			return nil
		}
		name := filepath.Base(filepath.Dir(path))
		if filepath.Dir(path) == baseDir {
			name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if isComposeFile(entry.Name()) {
				name = filepath.Base(baseDir)
			}
		}
		info := inspectStack(canonicalPath(path), name, defaultComposeFiles(path), flatFileProjectName(path, baseDir, string(content)), string(content))
		// Expose a literal top-level Compose name for UI duplicate detection.
		// Local actions still omit -p so Compose can handle interpolation itself.
		if declaredName := extractStaticComposeProjectName(string(content)); declaredName != "" {
			info.ProjectName = declaredName
		}
		results[info.ID] = info
		return nil
	})

	// 2. Discover host-wide compose projects using docker compose ls -a
	if projectsErr == nil {
		for _, cp := range composeProjects {
			files := normalizeConfigFiles(strings.Split(cp.ConfigFiles, ","))
			if len(files) == 0 {
				continue
			}
			content, err := os.ReadFile(files[0])
			if err != nil {
				continue
			}
			id := registeredComposeID(files[0], cp.Name)
			info := inspectStack(id, cp.Name, files, cp.Name, string(content))
			results[id] = info
		}
	}

	// Return actual results (empty slice if none found; never leak mock data into production)
	stacks := make([]StackInfo, 0, len(results))
	for _, stack := range results {
		stacks = append(stacks, stack)
	}
	sort.Slice(stacks, func(i, j int) bool {
		left, right := strings.ToLower(stacks[i].Name), strings.ToLower(stacks[j].Name)
		if left == right {
			return stacks[i].ID < stacks[j].ID
		}
		return left < right
	})
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
	case ".git", "node_modules", "vendor", ".cache", ".local", ".config", ".npm", ".venv", "venv":
		return true
	}
	return strings.HasPrefix(name, ".")
}

func isSystemComposeDirectory(path string) bool {
	switch filepath.Clean(path) {
	case "/dev", "/proc", "/run", "/sys", "/tmp":
		return true
	default:
		return false
	}
}
func pathDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(os.PathSeparator)) + 1
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

func flatFileProjectName(path, root, content string) string {
	if filepath.Dir(path) != root || isComposeFile(filepath.Base(path)) {
		return ""
	}
	var config map[string]interface{}
	if yaml.Unmarshal([]byte(content), &config) != nil {
		return ""
	}
	if _, hasComposeName := config["name"]; hasComposeName {
		return ""
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var normalized strings.Builder
	for _, char := range strings.ToLower(name) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			normalized.WriteRune(char)
		}
	}
	return strings.Trim(normalized.String(), "-_")
}

func extractStaticComposeProjectName(content string) string {
	var config map[string]interface{}
	if yaml.Unmarshal([]byte(content), &config) != nil {
		return ""
	}
	name, ok := config["name"].(string)
	if !ok || strings.Contains(name, "$") {
		return ""
	}
	return strings.TrimSpace(name)
}

func inspectStack(id, name string, files []string, projectName, yamlContent string) StackInfo {
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
		ID:            id,
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
func SaveStack(name, content string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("stack name is required")
	}
	if valid, message, _ := ValidateYAML(content); !valid {
		return "", fmt.Errorf("%s", message)
	}
	mockMu.Lock()
	if isMockMode {
		for i, s := range mockStacks {
			if s.Name == name {
				mockStacks[i].YAMLContent = content
				mockStacks[i].Services = extractServices(content)
				mockStacks[i].ServicesCount = len(mockStacks[i].Services)
				path := mockStacks[i].Path
				mockMu.Unlock()
				return path, nil
			}
		}
		path := filepath.Join("/etc/m3tal/compose", name, "compose.yaml")
		mockStacks = append(mockStacks, StackInfo{
			Name:          name,
			Path:          path,
			Services:      extractServices(content),
			ServicesCount: len(extractServices(content)),
			RunningCount:  0,
			Status:        "stopped",
			UpdatedAt:     time.Now().Format("2006-01-02 15:04:05"),
			YAMLContent:   content,
		})
		mockMu.Unlock()
		return path, nil
	}
	mockMu.Unlock()

	filePath := ""
	mode := os.FileMode(0644)
	if filepath.IsAbs(name) {
		_, _, err := resolveStackFiles(name)
		if err != nil {
			return "", err
		}
		filePath, mode, err = resolveExistingComposeFile(name)
		if err != nil {
			return "", fmt.Errorf("resolve existing Compose file: %w", err)
		}
	} else if _, _, registered := parseRegisteredComposeID(name); registered {
		files, _, err := resolveStackFiles(name)
		if err != nil {
			return "", err
		}
		if len(files) == 0 {
			return "", fmt.Errorf("no Compose file selected")
		}
		filePath, mode, err = resolveExistingComposeFile(files[0])
		if err != nil {
			return "", fmt.Errorf("resolve existing Compose file: %w", err)
		}
	} else {
		if !validStackName(name) {
			return "", fmt.Errorf("invalid stack name")
		}
		root := resolvedComposeRoot()
		settingsMu.RLock()
		layout := newStackLayout
		settingsMu.RUnlock()
		if layout == "flat" {
			filePath = filepath.Join(root, name+".yaml")
		} else {
			filePath = filepath.Join(root, name, "compose.yaml")
		}
		if isOverrideComposeFile(filepath.Base(filePath)) {
			return "", fmt.Errorf("new stack path cannot use a Compose override filename")
		}
		if _, err := os.Lstat(filePath); err == nil {
			return "", fmt.Errorf("a Compose file already exists there; select the existing project to edit it")
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("check new Compose path: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create compose directory: %w", err)
	}
	settingsMu.RLock()
	shouldBackup := backupOnSave
	settingsMu.RUnlock()
	if shouldBackup {
		if old, err := os.ReadFile(filePath); err == nil {
			if err := atomicWriteFile(filePath+".bak", old, mode); err != nil {
				return "", fmt.Errorf("failed to back up compose file: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to read compose file: %w", err)
		}
	}
	if err := atomicWriteFile(filePath, []byte(content), mode); err != nil {
		return "", err
	}
	return canonicalPath(filePath), nil
}

func resolveExistingComposeFile(path string) (string, os.FileMode, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", 0, err
	}
	resolved = canonicalPath(resolved)
	info, err := os.Stat(resolved)
	if err != nil {
		return "", 0, err
	}
	return resolved, info.Mode().Perm(), nil
}

func stackIDForSave(identifier, savedPath string) string {
	if primary, projectName, ok := parseRegisteredComposeID(identifier); ok {
		return registeredComposeID(primary, projectName)
	}
	if filepath.IsAbs(identifier) {
		return canonicalPath(identifier)
	}
	return canonicalPath(savedPath)
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
