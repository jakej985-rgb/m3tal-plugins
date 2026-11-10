package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Share describes a single SMB or NFS network share.
type Share struct {
	Protocol string   `json:"protocol"` // "smb" or "nfs"
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Comment  string   `json:"comment,omitempty"`
	ReadOnly bool     `json:"read_only"`
	GuestOK  bool     `json:"guest_ok,omitempty"`
	Users    []string `json:"users,omitempty"`
	Options  []string `json:"options,omitempty"`
	Clients  []string `json:"clients,omitempty"`
	Builtin  bool     `json:"builtin,omitempty"`
	Raw      string   `json:"raw,omitempty"`
}

// SambaUser describes a local Samba account.
type SambaUser struct {
	Username string `json:"username"`
	UID      string `json:"uid,omitempty"`
}

// ClientSession describes an active SMB client session.
type ClientSession struct {
	Protocol string `json:"protocol"`
	Service  string `json:"service"`
	User     string `json:"user"`
	Machine  string `json:"machine"`
	PID      string `json:"pid,omitempty"`
}

// NFSClient describes an NFS client seen by showmount.
type NFSClient struct {
	Client string   `json:"client"`
	Shares []string `json:"shares"`
}

type pluginConfig struct {
	SmbConfPath string `json:"smb_conf_path"`
	ExportsPath string `json:"exports_path"`
	AutoReload  *bool  `json:"auto_reload"`
}

func defaultPluginConfig() pluginConfig {
	return pluginConfig{
		SmbConfPath: "/etc/samba/smb.conf",
		ExportsPath: "/etc/exports",
		AutoReload:  boolPtr(true),
	}
}

func boolPtr(b bool) *bool { return &b }

func pluginConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "m3tal", "plugins", "samba-nfs", "config.json")
}

func loadPluginConfig() pluginConfig {
	cfg := defaultPluginConfig()
	if path := pluginConfigPath(); path != "" {
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &cfg)
		}
	}
	// Environment overrides win over everything (useful for tests/nonstandard layouts).
	if v := os.Getenv("M3TAL_SMB_CONF"); v != "" {
		cfg.SmbConfPath = v
	}
	if v := os.Getenv("M3TAL_EXPORTS"); v != "" {
		cfg.ExportsPath = v
	}
	return cfg
}

func savePluginConfig(cfg pluginConfig) error {
	path := pluginConfigPath()
	if path == "" {
		return fmt.Errorf("cannot locate the user configuration directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c pluginConfig) isAutoReload() bool {
	return c.AutoReload == nil || *c.AutoReload
}

// validateSmbConfPath restricts the managed smb.conf location to Samba's
// config directory so settings cannot redirect writes at arbitrary files.
func validateSmbConfPath(path string) error {
	if !filepath.IsAbs(path) || strings.Contains(path, "..") {
		return fmt.Errorf("smb_conf_path must be an absolute path without '..': %q", path)
	}
	if filepath.Dir(path) != "/etc/samba" || !strings.HasSuffix(path, ".conf") {
		return fmt.Errorf("smb_conf_path must be a .conf file inside /etc/samba: %q", path)
	}
	return nil
}

// validateExportsPath restricts the managed exports file to the standard
// NFS locations so settings cannot redirect writes at arbitrary files.
func validateExportsPath(path string) error {
	if !filepath.IsAbs(path) || strings.Contains(path, "..") {
		return fmt.Errorf("exports_path must be an absolute path without '..': %q", path)
	}
	if path == "/etc/exports" || strings.HasPrefix(path, "/etc/exports.d/") {
		return nil
	}
	return fmt.Errorf("exports_path must be /etc/exports or a file under /etc/exports.d: %q", path)
}

func backupFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	bak := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102-150405"))
	return os.WriteFile(bak, data, 0644)
}

func runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// commandExists reports whether a binary is on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func serviceActive(name string) bool {
	out, err := runCmd("systemctl", "is-active", name)
	return err == nil && out == "active"
}

func binaryVersion(name string) string {
	out, err := runCmd(name, "--version")
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(out, "\n")
	return strings.TrimSpace(line)
}

// ---------------------------------------------------------------------------
// smb.conf parsing
// ---------------------------------------------------------------------------

var smbShareNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// smbUsernameRe enforces standard POSIX/Samba username constraints (max 32 chars, optional trailing $ for machine accounts).
// It prevents flag injection (leading '-') and dangerous metacharacters in smbpasswd/pdbedit command arguments.
var smbUsernameRe = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.-]{0,31}\$?$`)

func validateSmbUsername(username string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if strings.HasPrefix(username, "-") || !smbUsernameRe.MatchString(username) {
		return fmt.Errorf("invalid Samba username %q: must match %s and not begin with '-'", username, smbUsernameRe.String())
	}
	return nil
}

func isBuiltinShare(name string) bool {
	switch strings.ToLower(name) {
	case "global", "homes", "printers", "print$":
		return true
	default:
		return false
	}
}

// parseSmbConf returns shares defined in an smb.conf file. The [global]
// section and built-in shares are marked Builtin and excluded from Shares
// returned for editing purposes by callers.
func parseSmbConf(path string) ([]Share, map[string]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var shares []Share
	sections := make(map[string]map[string]string)
	var cur *Share
	var curSection string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			curSection = strings.TrimSpace(line[1 : len(line)-1])
			cur = &Share{Protocol: "smb", Name: curSection, Builtin: isBuiltinShare(curSection)}
			sections[curSection] = make(map[string]string)
			shares = append(shares, *cur)
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || curSection == "" {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		sections[curSection][key] = val
		// Update the share we appended (shares holds copies).
		for i := range shares {
			if shares[i].Name == curSection {
				switch strings.ToLower(key) {
				case "path":
					shares[i].Path = val
				case "comment":
					shares[i].Comment = val
				case "read only", "readonly":
					shares[i].ReadOnly = strings.EqualFold(val, "yes") || val == "1" || strings.EqualFold(val, "true")
				case "guest ok":
					shares[i].GuestOK = strings.EqualFold(val, "yes") || val == "1" || strings.EqualFold(val, "true")
				case "valid users", "valid user":
					shares[i].Users = strings.Fields(val)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return shares, sections, nil
}

// smbConfLines returns the raw lines of smb.conf with comments preserved.
func smbConfLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(data), "\n"), nil
}

// writeSmbConf atomically writes new content after taking a timestamped backup.
func writeSmbConf(path string, lines []string) error {
	if _, err := os.Stat(path); err == nil {
		if err := backupFile(path); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
	}
	content := strings.Join(lines, "\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// findSectionRange locates [name] ... up to the next section header or EOF.
// Returns start/end line indexes (end exclusive) or -1 if absent.
func findSectionRange(lines []string, name string) (int, int) {
	target := "[" + name + "]"
	start := -1
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if start >= 0 {
				return start, i
			}
			if strings.EqualFold(trimmed, target) {
				start = i
			}
		}
	}
	if start >= 0 {
		return start, len(lines)
	}
	return -1, -1
}

// removeSmbSection deletes a [name] block (including preceding blank separator
// line if present) from the line slice.
func removeSmbSection(lines []string, name string) ([]string, bool) {
	start, end := findSectionRange(lines, name)
	if start < 0 {
		return lines, false
	}
	// Also swallow one trailing blank line after the section.
	if end < len(lines) && strings.TrimSpace(lines[end]) == "" {
		end++
	}
	out := make([]string, 0, len(lines)-(end-start))
	out = append(out, lines[:start]...)
	out = append(out, lines[end:]...)
	return out, true
}

func yn(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// ---------------------------------------------------------------------------
// smb.conf mutation actions
// ---------------------------------------------------------------------------

func smbReload(cfg pluginConfig) string {
	if !cfg.isAutoReload() {
		return "auto-reload disabled; restart smbd/nmbd manually"
	}
	var notes []string
	if out, err := runCmd("smbcontrol", "all", "reload-config"); err == nil {
		notes = append(notes, "smbcontrol reload-config: "+out)
	} else {
		ctrlOut := strings.TrimSpace(out)
		if out2, err2 := runCmd("systemctl", "reload", "smbd"); err2 == nil {
			notes = append(notes, "systemctl reload smbd: ok (smbcontrol: "+ctrlOut+")")
		} else {
			notes = append(notes, fmt.Sprintf("smbd reload failed: smbcontrol: %s; systemctl: %s", ctrlOut, strings.TrimSpace(out2)))
		}
	}
	return strings.Join(notes, "; ")
}

func CreateSmbShare(cfg pluginConfig, s Share) (string, error) {
	if !smbShareNameRe.MatchString(s.Name) {
		return "", fmt.Errorf("invalid share name %q: use letters, digits, '.', '_' or '-', max 64 chars", s.Name)
	}
	if s.Path == "" {
		return "", fmt.Errorf("path is required")
	}
	for label, v := range map[string]string{"name": s.Name, "path": s.Path, "comment": s.Comment} {
		if strings.ContainsAny(v, "\r\n") {
			return "", fmt.Errorf("%s must not contain newlines", label)
		}
	}
	for _, u := range s.Users {
		if strings.ContainsAny(u, "\r\n \t") {
			return "", fmt.Errorf("invalid user entry %q", u)
		}
	}
	if st, err := os.Stat(s.Path); err != nil || !st.IsDir() {
		return "", fmt.Errorf("path %q is not an existing directory", s.Path)
	}

	lines, err := smbConfLines(cfg.SmbConfPath)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", cfg.SmbConfPath, err)
	}
	if start, _ := findSectionRange(lines, s.Name); start >= 0 {
		return "", fmt.Errorf("share %q already exists in %s", s.Name, cfg.SmbConfPath)
	}

	var section strings.Builder
	fmt.Fprintf(&section, "\n[%s]\n", s.Name)
	if s.Comment != "" {
		fmt.Fprintf(&section, "\tcomment = %s\n", s.Comment)
	}
	fmt.Fprintf(&section, "\tpath = %s\n", s.Path)
	fmt.Fprintf(&section, "\tread only = %s\n", yn(s.ReadOnly))
	fmt.Fprintf(&section, "\tguest ok = %s\n", yn(s.GuestOK))
	if s.GuestOK {
		fmt.Fprintf(&section, "\tmap to guest = %s\n", "Bad User")
	}
	if len(s.Users) > 0 {
		fmt.Fprintf(&section, "\tvalid users = %s\n", strings.Join(s.Users, " "))
	}
	fmt.Fprintf(&section, "\tbrowseable = yes\n")
	fmt.Fprintf(&section, "\tcreate mask = 0664\n")
	fmt.Fprintf(&section, "\tdirectory mask = 0775\n")

	// Ensure file ends with newline before appending.
	if len(lines) > 0 && strings.TrimRight(lines[len(lines)-1], "\r") != "" {
		lines = append(lines, "")
	}
	lines = append(lines, strings.TrimRight(section.String(), "\n"))

	if err := writeSmbConf(cfg.SmbConfPath, lines); err != nil {
		return "", err
	}

	msg := fmt.Sprintf("SMB share [%s] created at %s", s.Name, s.Path)
	if commandExists("testparm") {
		if _, err := runCmd("testparm", "-s"); err == nil {
			msg += " (testparm OK)"
		} else {
			msg += " (testparm reported issues; check smb.conf)"
		}
	}
	return msg + "; " + smbReload(cfg), nil
}

func DeleteSmbShare(cfg pluginConfig, name string) (string, error) {
	lines, err := smbConfLines(cfg.SmbConfPath)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", cfg.SmbConfPath, err)
	}
	newLines, ok := removeSmbSection(lines, name)
	if !ok {
		return "", fmt.Errorf("share %q not found in %s", name, cfg.SmbConfPath)
	}
	if err := writeSmbConf(cfg.SmbConfPath, newLines); err != nil {
		return "", err
	}
	return fmt.Sprintf("SMB share [%s] removed; %s", name, smbReload(cfg)), nil
}

func CreateSmbUser(username, password string) (string, error) {
	if err := validateSmbUsername(username); err != nil {
		return "", err
	}
	if password == "" {
		return "", fmt.Errorf("password is required")
	}
	if strings.ContainsAny(password, "\r\n") {
		return "", fmt.Errorf("password must not contain newlines")
	}
	// smbpasswd -a -s adds a user and sets the password from stdin.
	cmd := exec.Command("smbpasswd", "-a", "-s", username)
	cmd.Stdin = strings.NewReader(password + "\n" + password + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// If the user already exists, fall back to a password change.
		if strings.Contains(strings.ToLower(string(out)), "exists") || strings.Contains(strings.ToLower(string(out)), "already") {
			cmd2 := exec.Command("smbpasswd", "-s", username)
			cmd2.Stdin = strings.NewReader(password + "\n" + password + "\n")
			out2, err2 := cmd2.CombinedOutput()
			if err2 != nil {
				return "", fmt.Errorf("failed to update Samba password: %s (%v)", strings.TrimSpace(string(out2)), err2)
			}
			return fmt.Sprintf("Samba password updated for %s", username), nil
		}
		return "", fmt.Errorf("failed to create Samba user: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	return fmt.Sprintf("Samba user %s created", username), nil
}

func DeleteSmbUser(username string) (string, error) {
	if err := validateSmbUsername(username); err != nil {
		return "", err
	}
	out, err := runCmd("pdbedit", "-x", username)
	if err != nil {
		return "", fmt.Errorf("failed to delete Samba user %s: %s (%v)", username, strings.TrimSpace(out), err)
	}
	return fmt.Sprintf("Samba user %s deleted", username), nil
}

func ListSambaUsers() []SambaUser {
	out, err := runCmd("pdbedit", "-Lw")
	if err != nil {
		// Fallback to non-machine-readable listing.
		out, err = runCmd("pdbedit", "-L")
		if err != nil {
			return nil
		}
		var users []SambaUser
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			name := strings.Fields(line)[0]
			users = append(users, SambaUser{Username: name})
		}
		return users
	}
	var users []SambaUser
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, ":", 3)
		if len(fields) >= 1 && fields[0] != "" {
			users = append(users, SambaUser{Username: fields[0], UID: fields[1]})
		}
	}
	return users
}

// ---------------------------------------------------------------------------
// NFS exports
// ---------------------------------------------------------------------------

type nfsExportLine struct {
	Path    string
	Clients []string // raw "client(options)" tokens
	Raw     string
}

func parseExportsFile(path string) ([]nfsExportLine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var exports []nfsExportLine
	var pending strings.Builder
	scanner := bufio.NewScanner(f)
	flush := func() {
		line := strings.TrimSpace(pending.String())
		pending.Reset()
		if line == "" || strings.HasPrefix(line, "#") {
			return
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return
		}
		exports = append(exports, nfsExportLine{
			Path:    fields[0],
			Clients: fields[1:],
			Raw:     line,
		})
	}
	for scanner.Scan() {
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Handle backslash line continuations.
		if strings.HasSuffix(trimmed, "\\") {
			pending.WriteString(strings.TrimSuffix(trimmed, "\\"))
			pending.WriteString(" ")
			continue
		}
		pending.WriteString(trimmed)
		flush()
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return exports, nil
}

// clientToken splits "host(options)" into host and options.
func clientToken(token string) (host, opts string) {
	open := strings.Index(token, "(")
	if open < 0 {
		return token, ""
	}
	host = token[:open]
	opts = strings.TrimSuffix(token[open+1:], ")")
	return host, opts
}

func ListNFSExports(path string) []Share {
	exports, err := parseExportsFile(path)
	if err != nil {
		return nil
	}
	var shares []Share
	for _, e := range exports {
		for _, tok := range e.Clients {
			host, opts := clientToken(tok)
			s := Share{
				Protocol: "nfs",
				Name:     e.Path,
				Path:     e.Path,
				Options:  strings.Split(opts, ","),
				Clients:  []string{host},
				Raw:      e.Raw,
			}
			if strings.Contains(opts, "ro") && !strings.Contains(opts, "rw") {
				s.ReadOnly = true
			}
			s.GuestOK = strings.Contains(opts, "insecure") || strings.Contains(opts, "no_root_squash")
			shares = append(shares, s)
		}
	}
	return shares
}

func nfsReload(cfg pluginConfig) string {
	if !cfg.isAutoReload() {
		return "auto-reload disabled; run exportfs -ra manually"
	}
	out, err := runCmd("exportfs", "-ra")
	if err != nil {
		return fmt.Sprintf("exportfs -ra failed: %s (%v)", strings.TrimSpace(out), err)
	}
	return "exportfs -ra applied"
}

func CreateNFSExport(cfg pluginConfig, path string, clients []string, options string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if st, err := os.Stat(path); err != nil || !st.IsDir() {
		return "", fmt.Errorf("path %q is not an existing directory", path)
	}
	if len(clients) == 0 {
		clients = []string{"*"}
	}
	if options == "" {
		options = "rw,sync,no_subtree_check"
	}
	if strings.ContainsAny(options, " \t\r\n()") {
		return "", fmt.Errorf("options must not contain whitespace or parentheses: %q", options)
	}

	var toks []string
	for _, c := range clients {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if strings.ContainsAny(c, " \t\r\n()") {
			return "", fmt.Errorf("invalid client %q", c)
		}
		toks = append(toks, fmt.Sprintf("%s(%s)", c, options))
	}
	if len(toks) == 0 {
		return "", fmt.Errorf("at least one client is required")
	}
	newline := strings.Join(append([]string{path}, toks...), " ")

	lines, err := smbConfLines(cfg.ExportsPath) // reuse raw line reader
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("cannot read %s: %w", cfg.ExportsPath, err)
	}
	if err == nil {
		// Refuse duplicates of the exact line.
		for _, l := range lines {
			if strings.TrimSpace(l) == newline {
				return "", fmt.Errorf("export already exists: %s", newline)
			}
		}
		if err := backupFile(cfg.ExportsPath); err != nil {
			return "", fmt.Errorf("backup failed: %w", err)
		}
	}
	// Ensure trailing newline.
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	lines = append(lines, newline)
	if err := os.WriteFile(cfg.ExportsPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("NFS export added: %s; %s", newline, nfsReload(cfg)), nil
}

// joinContinuationLines merges backslash-continued physical lines into
// logical lines so exports entries spanning multiple lines are handled as a
// unit. Comment and blank lines are preserved as-is.
func joinContinuationLines(lines []string) []string {
	var out []string
	var pending strings.Builder
	flush := func() {
		if pending.Len() > 0 {
			out = append(out, pending.String())
			pending.Reset()
		}
	}
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			flush()
			out = append(out, l)
			continue
		}
		if strings.HasSuffix(trimmed, "\\") {
			pending.WriteString(strings.TrimSuffix(trimmed, "\\"))
			pending.WriteString(" ")
			continue
		}
		if pending.Len() > 0 {
			pending.WriteString(trimmed)
			flush()
		} else {
			out = append(out, l)
		}
	}
	flush()
	return out
}

func DeleteNFSExport(cfg pluginConfig, path, client string) (string, error) {
	lines, err := smbConfLines(cfg.ExportsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no exports file at %s", cfg.ExportsPath)
		}
		return "", fmt.Errorf("cannot read %s: %w", cfg.ExportsPath, err)
	}
	lines = joinContinuationLines(lines)

	removed := false
	var out []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out = append(out, l)
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 || fields[0] != path {
			out = append(out, l)
			continue
		}
		if client == "" {
			removed = true
			continue // drop whole line
		}
		var kept []string
		for _, tok := range fields[1:] {
			host, _ := clientToken(tok)
			if host != client && host != strings.TrimSuffix(client, "/32") {
				kept = append(kept, tok)
			} else {
				removed = true
			}
		}
		if len(kept) > 0 {
			out = append(out, strings.Join(append([]string{path}, kept...), " "))
		}
	}
	if !removed {
		return "", fmt.Errorf("no export matching path=%q client=%q found", path, client)
	}
	if err := backupFile(cfg.ExportsPath); err != nil {
		return "", fmt.Errorf("backup failed: %w", err)
	}
	if err := os.WriteFile(cfg.ExportsPath, []byte(strings.Join(out, "\n")), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("NFS export updated; %s", nfsReload(cfg)), nil
}

// ---------------------------------------------------------------------------
// Monitoring
// ---------------------------------------------------------------------------

// parseSmbStatus extracts client sessions and locked-file count from
// `smbstatus` output. Tolerant of column layout differences across versions.
func parseSmbStatus(out string) (sessions []ClientSession, lockedFiles int) {
	lines := strings.Split(out, "\n")
	section := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "service") && strings.Contains(lower, "machine") {
			section = "service"
			continue
		}
		if strings.HasPrefix(lower, "locked files") {
			section = "locked"
			continue
		}
		if strings.HasPrefix(lower, "pid") || strings.HasPrefix(lower, "-") || line == "" {
			if strings.HasPrefix(line, "pid") {
				section = ""
			}
			continue
		}
		switch section {
		case "service":
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				sessions = append(sessions, ClientSession{
					Protocol: "SMB",
					Service:  fields[0],
					PID:      fields[1],
					Machine:  fields[2],
				})
			}
		case "locked":
			if len(strings.Fields(line)) >= 2 {
				lockedFiles++
			}
		}
	}
	return sessions, lockedFiles
}

func GetSmbSessions() ([]ClientSession, int, error) {
	out, err := runCmd("smbstatus")
	if err != nil {
		return nil, 0, err
	}
	sessions, locked := parseSmbStatus(out)
	return sessions, locked, nil
}

func GetNFSClients() []NFSClient {
	out, err := runCmd("showmount", "-a", "localhost")
	if err != nil {
		return nil
	}
	var clients []NFSClient
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			clients = append(clients, NFSClient{Client: fields[0], Shares: fields[1:]})
		}
	}
	return clients
}

// ---------------------------------------------------------------------------
// Mounts
// ---------------------------------------------------------------------------

// validateMountPath validates that a mount point path is absolute and free of
// leading dashes, control characters, or newlines to prevent CLI flag injection.
func validateMountPath(name, path string) error {
	if path == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.HasPrefix(path, "-") {
		return fmt.Errorf("%s must not begin with '-': %q", name, path)
	}
	if strings.ContainsAny(path, "\x00\r\n") {
		return fmt.Errorf("%s contains invalid control characters", name)
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s must be an absolute path: %q", name, path)
	}
	return nil
}

func MountShare(protocol, source, target, options string) (string, error) {
	if source == "" {
		return "", fmt.Errorf("source is required")
	}
	if strings.HasPrefix(source, "-") {
		return "", fmt.Errorf("source must not begin with '-': %q", source)
	}
	if strings.ContainsAny(source, "\x00\r\n") {
		return "", fmt.Errorf("source contains invalid control characters")
	}
	if err := validateMountPath("target mount point", target); err != nil {
		return "", err
	}
	if options != "" {
		if strings.HasPrefix(options, "-") {
			return "", fmt.Errorf("options must not begin with '-': %q", options)
		}
		if strings.ContainsAny(options, " \t\r\n\x00") {
			return "", fmt.Errorf("options must not contain whitespace or control characters: %q", options)
		}
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return "", fmt.Errorf("cannot create mount point %s: %w", target, err)
	}
	var args []string
	switch strings.ToLower(protocol) {
	case "cifs", "smb":
		args = []string{"-t", "cifs", source, target}
	case "nfs":
		args = []string{"-t", "nfs", source, target}
	default:
		return "", fmt.Errorf("unsupported protocol %q (use cifs or nfs)", protocol)
	}
	if options != "" {
		args = append(args, "-o", options)
	}
	out, err := runCmd("mount", args...)
	if err != nil {
		return "", fmt.Errorf("mount failed: %s (%v)", strings.TrimSpace(out), err)
	}
	return fmt.Sprintf("Mounted %s (%s) at %s", source, protocol, target), nil
}

func UnmountShare(target string) (string, error) {
	if err := validateMountPath("target mount point", target); err != nil {
		return "", err
	}
	out, err := runCmd("umount", target)
	if err != nil {
		return "", fmt.Errorf("umount failed: %s (%v)", strings.TrimSpace(out), err)
	}
	return fmt.Sprintf("Unmounted %s", target), nil
}

// restartableServices is the allowlist of systemd units this plugin may
// restart on behalf of the share manager.
var restartableServices = map[string]bool{
	"smbd": true, "nmbd": true, "winbind": true,
	"nfs-server": true, "nfs-kernel-server": true,
}

func RestartServices(names []string) (string, error) {
	if len(names) == 0 {
		names = []string{"smbd", "nmbd", "nfs-server"}
	}
	var results []string
	for _, svc := range names {
		svc = strings.TrimSpace(svc)
		if svc == "" {
			continue
		}
		if !restartableServices[svc] {
			return "", fmt.Errorf("service %q is not managed by this plugin", svc)
		}
		out, err := runCmd("systemctl", "restart", svc)
		if err != nil {
			results = append(results, fmt.Sprintf("%s: FAILED (%s)", svc, strings.TrimSpace(out)))
		} else {
			results = append(results, fmt.Sprintf("%s: restarted", svc))
		}
	}
	if cfg := loadPluginConfig(); cfg.isAutoReload() {
		if out, err := runCmd("exportfs", "-ra"); err == nil {
			results = append(results, "exportfs -ra: ok")
		} else {
			results = append(results, fmt.Sprintf("exportfs -ra: %s", strings.TrimSpace(out)))
		}
	}
	return strings.Join(results, "; "), nil
}

// GetAllShares aggregates SMB and NFS shares for collect.
func GetAllShares(cfg pluginConfig) ([]Share, []SambaUser, []ClientSession, int, []NFSClient, error) {
	var shares []Share
	var users []SambaUser
	var sessions []ClientSession
	var locked int
	var nfsClients []NFSClient

	if commandExists("smbd") || commandExists("testparm") {
		if smbShares, _, err := parseSmbConf(cfg.SmbConfPath); err == nil {
			for _, s := range smbShares {
				if !s.Builtin {
					shares = append(shares, s)
				}
			}
		}
		users = ListSambaUsers()
		if s, l, err := GetSmbSessions(); err == nil {
			sessions = s
			locked = l
		}
	}
	if commandExists("exportfs") || commandExists("showmount") {
		shares = append(shares, ListNFSExports(cfg.ExportsPath)...)
		nfsClients = GetNFSClients()
	}
	return shares, users, sessions, locked, nfsClients, nil
}

// sortedUnique returns a sorted copy of unique non-empty strings.
func sortedUnique(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
