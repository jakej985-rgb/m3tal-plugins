package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMockComposeOperations(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	stacks, err := ListStacks()
	if err != nil {
		t.Fatalf("expected no error from mock ListStacks, got %v", err)
	}
	if len(stacks) == 0 {
		t.Fatalf("expected mock stacks, got 0")
	}

	target := stacks[0].Name

	// Test ValidateYAML
	valid, msg, err := ValidateYAML(stacks[0].YAMLContent)
	if err != nil || !valid {
		t.Fatalf("expected valid YAML, got %v (%s)", err, msg)
	}

	invalidContent := "foo: bar"
	valid, _, _ = ValidateYAML(invalidContent)
	if valid {
		t.Errorf("expected invalid YAML for content missing services:, got valid")
	}

	// Test Stop
	downMsg, err := ComposeDown(target)
	if err != nil {
		t.Fatalf("failed to stop compose stack: %v", err)
	}
	if !strings.Contains(downMsg, "stopped") {
		t.Errorf("expected stopped in downMsg, got %s", downMsg)
	}

	// Test Start
	upMsg, err := ComposeUp(target)
	if err != nil {
		t.Fatalf("failed to start compose stack: %v", err)
	}
	if !strings.Contains(upMsg, "started") {
		t.Errorf("expected started in upMsg, got %s", upMsg)
	}

	// Test Restart
	reMsg, err := ComposeRestart(target)
	if err != nil {
		t.Fatalf("failed to restart compose stack: %v", err)
	}
	if !strings.Contains(reMsg, "restarted") {
		t.Errorf("expected restarted in reMsg, got %s", reMsg)
	}

	// Test SaveStack
	newYaml := "services:\n  web:\n    image: nginx:alpine\n"
	if _, err := SaveStack("new-stack", newYaml); err != nil {
		t.Fatalf("failed to save stack: %v", err)
	}

	// Test Logs
	logs, err := ComposeLogs(target, 10)
	if err != nil {
		t.Fatalf("failed to get compose logs: %v", err)
	}
	if len(logs) == 0 {
		t.Errorf("expected non-empty logs")
	}
}

func TestYAMLValidationAndFlowStyleServices(t *testing.T) {
	valid, _, err := ValidateYAML("services: { api: { image: 'nginx:alpine' }, cache: { image: redis } }\n")
	if err != nil || !valid {
		t.Fatalf("expected valid flow-style YAML: valid=%v err=%v", valid, err)
	}
	services := extractServices("services: { api: { image: 'nginx:alpine' }, cache: { image: redis } }\n")
	if len(services) != 2 || services[0] != "api" || services[1] != "cache" {
		t.Fatalf("unexpected services: %#v", services)
	}
	valid, _, _ = ValidateYAML("services:\n  api: [\n")
	if valid {
		t.Fatal("expected malformed YAML to fail validation")
	}
	valid, _, _ = ValidateYAML("services: {}\n")
	if valid {
		t.Fatal("expected an empty services mapping to fail validation")
	}
}

func TestConfiguredRecursiveDiscoveryAndSaveExisting(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	oldProjectsSource := composeProjectsSource
	composeProjectsSource = func() ([]composeProject, error) { return nil, nil }
	defer func() { composeProjectsSource = oldProjectsSource }()
	root := t.TempDir()
	composePath := filepath.Join(root, "nested", "api service", "compose.yaml")
	if err := os.MkdirAll(filepath.Dir(composePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(composePath, []byte("name: api-project\nservices:\n  old:\n    image: alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(composePath, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(composePath), "docker-compose.yml"), []byte("services:\n  ignored:\n    image: alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	overridePath := filepath.Join(filepath.Dir(composePath), "compose.override.yaml")
	if err := os.WriteFile(overridePath, []byte("services:\n  old:\n    environment:\n      MODE: test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	flatPath := filepath.Join(root, "flat-stack.yaml")
	if err := os.WriteFile(flatPath, []byte("services:\n  flat:\n    image: alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	rootComposePath := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(rootComposePath, []byte("services:\n  root-app:\n    image: alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.override.yaml"), []byte("services:\n  ignored-override:\n    image: alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "settings.yaml"), []byte("theme: dark\n"), 0644); err != nil {
		t.Fatal(err)
	}
	devComposePath := filepath.Join(root, "dev", "compose.yaml")
	if err := os.MkdirAll(filepath.Dir(devComposePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(devComposePath, []byte("services:\n  dev-app:\n    image: alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	settingsMu.Lock()
	oldRoot, oldLayout, oldBackup := composeDirSetting, newStackLayout, backupOnSave
	composeDirSetting, newStackLayout, backupOnSave = root, "per_stack_folder", true
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		composeDirSetting, newStackLayout, backupOnSave = oldRoot, oldLayout, oldBackup
		settingsMu.Unlock()
	}()

	stacks, err := ListStacks()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	flatFound := false
	rootComposeFound := false
	devStackFound := false
	var existing StackInfo
	for _, stack := range stacks {
		if stack.Name == "api service" && stack.ProjectName == "api-project" && stack.Path == composePath && stack.ID == canonicalPath(composePath) && len(stack.ConfigFiles) == 2 && stack.ConfigFiles[1] == overridePath {
			found = true
			existing = stack
		}
		if stack.Name == "flat-stack" && stack.Path == flatPath {
			flatFound = true
		}
		if stack.Name == filepath.Base(root) && stack.Path == rootComposePath {
			rootComposeFound = true
		}
		if stack.Name == "dev" && stack.Path == devComposePath {
			devStackFound = true
		}
		if stack.Name == "settings" || stack.Name == "compose.override" || stack.Name == "ignored-override" {
			t.Fatalf("unrelated/override YAML was listed as a project: %#v", stack)
		}
	}
	if !found {
		t.Fatalf("recursive discovery did not return existing compose project at %s", composePath)
	}
	if !flatFound {
		t.Fatalf("flat Compose file was not discovered: %s", flatPath)
	}
	if got := flatFileProjectName(filepath.Join(root, "api service.yaml"), root, "services: { web: { image: nginx } }\n"); got != "apiservice" {
		t.Fatalf("flat file project name does not match Compose normalization: %q", got)
	}
	if got := flatFileProjectName(filepath.Join(root, "dynamic.yaml"), root, "name: ${STACK_NAME:-prod}\nservices: { web: { image: nginx } }\n"); got != "" {
		t.Fatalf("dynamic Compose project name should be left to Compose, got %q", got)
	}
	if !rootComposeFound {
		t.Fatal("valid root compose.yaml was not discovered with the root folder name")
	}
	if !devStackFound {
		t.Fatal("Compose project in a folder named dev was incorrectly excluded")
	}

	content := "services:\n  api:\n    image: nginx:alpine\n"
	// A new stack named after an existing file's Compose name must not update that file.
	if _, err := SaveStack("api-project", content); err != nil {
		t.Fatal(err)
	}
	untouched, err := os.ReadFile(composePath)
	if err != nil || string(untouched) != "name: api-project\nservices:\n  old:\n    image: alpine\n" {
		t.Fatalf("new-stack alias overwrote existing project: content=%q err=%v", untouched, err)
	}
	if _, err := SaveStack(existing.ID, content); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("expected save to update discovered file; got %q", got)
	}
	if _, err := os.Stat(composePath + ".bak"); err != nil {
		t.Fatalf("expected backup of existing file: %v", err)
	}
	for _, path := range []string{composePath, composePath + ".bak"} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("expected restrictive mode 0600 on %s, got %04o", path, got)
		}
	}
}

func TestRegisteredSecondaryComposeFileIsNotStandaloneAndSavePreservesTarget(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	root := t.TempDir()
	primary := filepath.Join(root, "app", "compose.yaml")
	secondary := filepath.Join(root, "production.yaml")
	for path, content := range map[string]string{
		primary:   "services:\n  app:\n    image: alpine\n",
		secondary: "services:\n  production:\n    image: nginx\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	settingsMu.Lock()
	oldRoot, oldLayout, oldBackup := composeDirSetting, newStackLayout, backupOnSave
	composeDirSetting, newStackLayout, backupOnSave = root, "per_stack_folder", true
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		composeDirSetting, newStackLayout, backupOnSave = oldRoot, oldLayout, oldBackup
		settingsMu.Unlock()
	}()
	oldProjectsSource := composeProjectsSource
	composeProjectsSource = func() ([]composeProject, error) {
		return []composeProject{{Name: "app", ConfigFiles: primary + "," + secondary}}, nil
	}
	defer func() { composeProjectsSource = oldProjectsSource }()

	stacks, err := ListStacks()
	if err != nil {
		t.Fatal(err)
	}
	if len(stacks) != 1 || stacks[0].ID != registeredComposeID(primary, "app") {
		t.Fatalf("registered secondary file appeared as a standalone stack: %#v", stacks)
	}
	updated := "services:\n  production:\n    image: busybox\n"
	if _, err := SaveStack(secondary, updated); err != nil {
		t.Fatal(err)
	}
	primaryContent, err := os.ReadFile(primary)
	if err != nil {
		t.Fatal(err)
	}
	secondaryContent, err := os.ReadFile(secondary)
	if err != nil {
		t.Fatal(err)
	}
	if string(primaryContent) != "services:\n  app:\n    image: alpine\n" || string(secondaryContent) != updated {
		t.Fatalf("save did not preserve the selected secondary file: primary=%q secondary=%q", primaryContent, secondaryContent)
	}
	backupInfo, err := os.Stat(secondary + ".bak")
	if err != nil || backupInfo.Mode().Perm() != 0600 {
		t.Fatalf("secondary file backup did not preserve restrictive permissions: info=%v err=%v", backupInfo, err)
	}
}

func TestSaveFollowsComposeFileSymlinkAndKeepsLink(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "source-compose.yaml")
	original := "services:\n  source:\n    image: alpine\n"
	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "compose.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	settingsMu.Lock()
	oldRoot, oldBackup := composeDirSetting, backupOnSave
	composeDirSetting, backupOnSave = root, true
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		composeDirSetting, backupOnSave = oldRoot, oldBackup
		settingsMu.Unlock()
	}()
	oldProjectsSource := composeProjectsSource
	composeProjectsSource = func() ([]composeProject, error) { return nil, nil }
	defer func() { composeProjectsSource = oldProjectsSource }()

	stacks, err := ListStacks()
	if err != nil || len(stacks) != 1 {
		t.Fatalf("failed to discover symlinked Compose file: stacks=%#v err=%v", stacks, err)
	}
	updated := "services:\n  updated:\n    image: nginx\n"
	savedPath, err := SaveStack(stacks[0].ID, updated)
	if err != nil {
		t.Fatal(err)
	}
	linkInfo, err := os.Lstat(link)
	if err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("saving replaced the Compose symlink: info=%v err=%v", linkInfo, err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != updated {
		t.Fatalf("save did not update the symlink target: content=%q err=%v", content, err)
	}
	backup, err := os.Stat(target + ".bak")
	if err != nil || backup.Mode().Perm() != 0600 {
		t.Fatalf("target backup did not preserve file mode: info=%v err=%v", backup, err)
	}
	if canonicalPath(savedPath) != canonicalPath(target) {
		t.Fatalf("SaveStack returned %q, want resolved target %q", savedPath, target)
	}
}

func TestSaveStackReturnsCreatedFilePath(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	root := t.TempDir()
	settingsMu.Lock()
	oldRoot, oldLayout, oldBackup := composeDirSetting, newStackLayout, backupOnSave
	composeDirSetting, newStackLayout, backupOnSave = root, "per_stack_folder", false
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		composeDirSetting, newStackLayout, backupOnSave = oldRoot, oldLayout, oldBackup
		settingsMu.Unlock()
	}()
	oldProjectsSource := composeProjectsSource
	composeProjectsSource = func() ([]composeProject, error) { return nil, nil }
	defer func() { composeProjectsSource = oldProjectsSource }()
	savedPath, err := SaveStack("fresh-project", "services:\n  app:\n    image: nginx\n")
	if err != nil {
		t.Fatal(err)
	}
	want := canonicalPath(filepath.Join(root, "fresh-project", "compose.yaml"))
	if got := canonicalPath(savedPath); got != want {
		t.Fatalf("saved stack ID mismatch: got %q, want %q", got, want)
	}
}

func TestFlatSaveReturnsItsWrittenPathWhenFolderNameIsOccupied(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	root := t.TempDir()
	existingPath := filepath.Join(root, "foo", "compose.yaml")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0755); err != nil {
		t.Fatal(err)
	}
	original := "services:\n  app:\n    image: alpine\n"
	if err := os.WriteFile(existingPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	settingsMu.Lock()
	oldRoot, oldLayout, oldBackup := composeDirSetting, newStackLayout, backupOnSave
	composeDirSetting, newStackLayout, backupOnSave = root, "flat", false
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		composeDirSetting, newStackLayout, backupOnSave = oldRoot, oldLayout, oldBackup
		settingsMu.Unlock()
	}()
	oldProjectsSource := composeProjectsSource
	composeProjectsSource = func() ([]composeProject, error) {
		return []composeProject{{Name: "prod", ConfigFiles: existingPath}}, nil
	}
	defer func() { composeProjectsSource = oldProjectsSource }()
	savedPath, err := SaveStack("foo", "services:\n  fresh:\n    image: nginx\n")
	if err != nil {
		t.Fatal(err)
	}
	want := canonicalPath(filepath.Join(root, "foo.yaml"))
	if got := canonicalPath(savedPath); got != want {
		t.Fatalf("saved flat stack ID points to the wrong project: got %q, want %q", got, want)
	}
	got, err := os.ReadFile(existingPath)
	if err != nil || string(got) != original {
		t.Fatalf("flat save modified the existing folder project: content=%q err=%v", got, err)
	}
}

func TestRegisteredProjectsSharingConfigHaveDistinctActionIDs(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	root := t.TempDir()
	primary := filepath.Join(root, "shared", "compose.yaml")
	if err := os.MkdirAll(filepath.Dir(primary), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primary, []byte("services:\n  app:\n    image: alpine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	settingsMu.Lock()
	oldRoot := composeDirSetting
	composeDirSetting = root
	settingsMu.Unlock()
	defer func() { settingsMu.Lock(); composeDirSetting = oldRoot; settingsMu.Unlock() }()
	oldProjectsSource := composeProjectsSource
	composeProjectsSource = func() ([]composeProject, error) {
		return []composeProject{
			{Name: "alpha", ConfigFiles: primary},
			{Name: "beta", ConfigFiles: primary},
		}, nil
	}
	defer func() { composeProjectsSource = oldProjectsSource }()

	stacks, err := ListStacks()
	if err != nil {
		t.Fatal(err)
	}
	if len(stacks) != 2 || stacks[0].ID == stacks[1].ID {
		t.Fatalf("projects sharing a file did not get distinct identities: %#v", stacks)
	}
	for _, stack := range stacks {
		_, projectName, err := resolveStackFiles(stack.ID)
		if err != nil || projectName != stack.ProjectName {
			t.Fatalf("action ID resolved to the wrong Compose project: stack=%#v project=%q err=%v", stack, projectName, err)
		}
		if got := stackIDForSave(stack.ID, primary); got != stack.ID {
			t.Fatalf("saving a registered project would lose its project identity: got %q, want %q", got, stack.ID)
		}
	}
}

func TestNewStackLayoutsAndPathValidation(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	root := t.TempDir()
	settingsMu.Lock()
	oldRoot, oldLayout, oldBackup := composeDirSetting, newStackLayout, backupOnSave
	composeDirSetting, backupOnSave = root, false
	settingsMu.Unlock()
	defer func() {
		settingsMu.Lock()
		composeDirSetting, newStackLayout, backupOnSave = oldRoot, oldLayout, oldBackup
		settingsMu.Unlock()
	}()
	content := "services:\n  app:\n    image: nginx\n"
	settingsMu.Lock()
	newStackLayout = "per_stack_folder"
	settingsMu.Unlock()
	if _, err := SaveStack("folder-app", content); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "folder-app", "compose.yaml")); err != nil {
		t.Fatalf("per-stack layout not created: %v", err)
	}
	settingsMu.Lock()
	newStackLayout = "flat"
	settingsMu.Unlock()
	if _, err := SaveStack("flat-app", content); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "flat-app.yaml")); err != nil {
		t.Fatalf("flat layout not created: %v", err)
	}
	if _, err := SaveStack("../escape", content); err == nil {
		t.Fatal("path traversal stack name was accepted")
	}
}

func TestSymlinkSearchRootAndDuplicateStackIDs(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	oldProjectsSource := composeProjectsSource
	composeProjectsSource = func() ([]composeProject, error) { return nil, nil }
	defer func() { composeProjectsSource = oldProjectsSource }()
	target := t.TempDir()
	first := filepath.Join(target, "a", "app", "compose.yaml")
	second := filepath.Join(target, "b", "app", "compose.yaml")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("services:\n  web:\n    image: nginx\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "docker-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	settingsMu.Lock()
	oldRoot := composeDirSetting
	composeDirSetting = link
	settingsMu.Unlock()
	defer func() { settingsMu.Lock(); composeDirSetting = oldRoot; settingsMu.Unlock() }()
	stacks, err := ListStacks()
	if err != nil {
		t.Fatal(err)
	}
	var matches []StackInfo
	for _, stack := range stacks {
		if stack.Name == "app" {
			matches = append(matches, stack)
		}
	}
	if len(matches) != 2 {
		t.Fatalf("expected both duplicate display names, got %d (%#v)", len(matches), stacks)
	}
	if matches[0].ID == "" || matches[1].ID == "" || matches[0].ID == matches[1].ID {
		t.Fatalf("expected distinct stable stack IDs, got %#v", matches)
	}
	if matches[0].ID >= matches[1].ID {
		t.Fatalf("duplicate stacks are not deterministically ordered: %q then %q", matches[0].ID, matches[1].ID)
	}
	updated := "services:\n  first-only:\n    image: nginx\n"
	if _, err := SaveStack(matches[0].ID, updated); err != nil {
		t.Fatal(err)
	}
	firstContent, err := os.ReadFile(matches[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	secondContent, err := os.ReadFile(matches[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstContent) != updated || strings.Contains(string(secondContent), "first-only") {
		t.Fatalf("path ID did not target only its project: first=%q second=%q", firstContent, secondContent)
	}
}

func TestSettingsEnvelopeAndComposeArguments(t *testing.T) {
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	settingsMu.RLock()
	oldRoot, oldLayout, oldBackup := composeDirSetting, newStackLayout, backupOnSave
	settingsMu.RUnlock()
	defer func() {
		settingsMu.Lock()
		composeDirSetting, newStackLayout, backupOnSave = oldRoot, oldLayout, oldBackup
		settingsMu.Unlock()
	}()
	settingsMu.Lock()
	composeDirSetting, newStackLayout, backupOnSave = "/original/root", "per_stack_folder", true
	settingsMu.Unlock()
	if err := SetPluginSettings(map[string]interface{}{"compose_dir": "/partial/change", "new_stack_layout": "unsupported"}); err == nil {
		t.Fatal("expected invalid layout to fail")
	}
	if got := getBaseComposeDir(); got != "/original/root" {
		t.Fatalf("invalid settings partially changed the root: %s", got)
	}
	settingsMu.RLock()
	gotLayout := newStackLayout
	settingsMu.RUnlock()
	if gotLayout != "per_stack_folder" {
		t.Fatalf("invalid settings partially changed layout: %s", gotLayout)
	}
	params, err := json.Marshal(map[string]interface{}{"action_id": "set_settings", "args": map[string]interface{}{"config": map[string]interface{}{"compose_dir": "~/docker-test", "new_stack_layout": "flat", "backup_on_save": false}}})
	if err != nil {
		t.Fatal(err)
	}
	req := RPCRequest{JSONRPC: "2.0", ID: 5, Method: "m3tal.action", Params: params}
	handleRequest(&req)
	if !strings.HasSuffix(getBaseComposeDir(), "docker-test") {
		t.Fatalf("tilde path was not expanded: %s", getBaseComposeDir())
	}
	args := composeArgs([]string{"/srv/app/compose.yaml", "/srv/app/compose.override.yaml"}, "app-prod")
	want := []string{"compose", "-p", "app-prod", "-f", "/srv/app/compose.yaml", "-f", "/srv/app/compose.override.yaml"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected compose arguments: %#v", args)
	}
	// Local stacks must leave project-name interpolation to Docker Compose.
	root := t.TempDir()
	t.Setenv("M3TAL_COMPOSE_DIR", "")
	settingsMu.Lock()
	composeDirSetting = root
	settingsMu.Unlock()
	path := filepath.Join(root, "my folder", "compose.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("name: ${STACK_NAME:-prod}\nservices:\n  app:\n    image: nginx\n"), 0644); err != nil {
		t.Fatal(err)
	}
	oldProjectsSource := composeProjectsSource
	lookupCount := 0
	composeProjectsSource = func() ([]composeProject, error) { lookupCount++; return nil, nil }
	defer func() { composeProjectsSource = oldProjectsSource }()
	files, project, err := resolveStackFiles(path)
	if err != nil {
		t.Fatal(err)
	}
	if project != "" {
		t.Fatalf("local Compose name expression was forced as project name %q", project)
	}
	if lookupCount != 1 {
		t.Fatalf("target resolution performed %d registry lookups, want 1", lookupCount)
	}
	if got := composeArgs(files, project); strings.Contains(strings.Join(got, " "), "-p") {
		t.Fatalf("local Compose interpolation was overridden: %#v", got)
	}
}

func TestRPCHandlerInit(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "m3tal.init",
	}

	handleRequest(&req)
}

func TestRPCHandlerCollect(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "m3tal.collect",
	}

	handleRequest(&req)
}

func TestRPCHandlerAction(t *testing.T) {
	SetMockMode(true)
	defer SetMockMode(false)

	params, _ := json.Marshal(map[string]interface{}{
		"action_id": "validate_yaml",
		"content":   "services:\n  app:\n    image: node:20\n",
	})
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "m3tal.action",
		Params:  params,
	}

	handleRequest(&req)
}
