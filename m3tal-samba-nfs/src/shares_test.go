package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleSmbConf = `[global]
   workgroup = WORKGROUP
   server string = %h server

[homes]
   comment = Home Directories
   browseable = no

[public]
   comment = Public Files
   path = /srv/public
   read only = no
   guest ok = yes
   valid users = alice bob

[backup]
   path = /mnt/backups
   read only = yes
   guest ok = no
`

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestParseSmbConf(t *testing.T) {
	path := writeTempFile(t, "smb.conf", sampleSmbConf)
	shares, sections, err := parseSmbConf(path)
	if err != nil {
		t.Fatalf("parseSmbConf: %v", err)
	}
	if len(shares) != 4 {
		t.Fatalf("expected 4 sections, got %d", len(shares))
	}

	var pub *Share
	for i := range shares {
		if shares[i].Name == "public" {
			pub = &shares[i]
		}
	}
	if pub == nil {
		t.Fatal("public share not found")
	}
	if pub.Path != "/srv/public" {
		t.Errorf("path = %q, want /srv/public", pub.Path)
	}
	if pub.ReadOnly {
		t.Error("public should not be read-only")
	}
	if !pub.GuestOK {
		t.Error("public should allow guests")
	}
	if len(pub.Users) != 2 || pub.Users[0] != "alice" {
		t.Errorf("users = %v, want [alice bob]", pub.Users)
	}
	if pub.Builtin {
		t.Error("public should not be builtin")
	}

	// global must be marked builtin
	foundGlobal := false
	for _, s := range shares {
		if s.Name == "global" {
			foundGlobal = true
			if !s.Builtin {
				t.Error("global should be builtin")
			}
		}
	}
	if !foundGlobal {
		t.Error("global section missing")
	}
	if sections["public"]["guest ok"] != "yes" {
		t.Error("sections map missing guest ok key")
	}
}

func TestCreateAndDeleteSmbShare(t *testing.T) {
	dir := t.TempDir()
	smbPath := filepath.Join(dir, "smb.conf")
	shareDir := filepath.Join(dir, "sharedata")
	if err := os.MkdirAll(shareDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(smbPath, []byte(sampleSmbConf), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := pluginConfig{SmbConfPath: smbPath, ExportsPath: filepath.Join(dir, "exports"), AutoReload: boolPtr(false)}

	msg, err := CreateSmbShare(cfg, Share{
		Protocol: "smb", Name: "media", Path: shareDir,
		Comment: "Media share", ReadOnly: false, GuestOK: true, Users: []string{"alice"},
	})
	if err != nil {
		t.Fatalf("CreateSmbShare: %v", err)
	}
	if !strings.Contains(msg, "media") {
		t.Errorf("unexpected message: %s", msg)
	}

	data, _ := os.ReadFile(smbPath)
	content := string(data)
	for _, want := range []string{"[media]", "path = " + shareDir, "guest ok = yes", "valid users = alice"} {
		if !strings.Contains(content, want) {
			t.Errorf("smb.conf missing %q", want)
		}
	}
	// Backup must exist.
	matches, _ := filepath.Glob(smbPath + ".bak-*")
	if len(matches) == 0 {
		t.Error("expected timestamped backup of smb.conf")
	}

	// Duplicate create must fail.
	if _, err := CreateSmbShare(cfg, Share{Protocol: "smb", Name: "media", Path: shareDir}); err == nil {
		t.Error("expected duplicate share error")
	}

	// Invalid names rejected.
	for _, bad := range []string{"", "bad name]", "[x]", strings.Repeat("a", 70)} {
		if _, err := CreateSmbShare(cfg, Share{Protocol: "smb", Name: bad, Path: shareDir}); err == nil {
			t.Errorf("expected error for name %q", bad)
		}
	}

	msg, err = DeleteSmbShare(cfg, "media")
	if err != nil {
		t.Fatalf("DeleteSmbShare: %v", err)
	}
	if !strings.Contains(msg, "removed") {
		t.Errorf("unexpected delete message: %s", msg)
	}
	data, _ = os.ReadFile(smbPath)
	if strings.Contains(string(data), "[media]") {
		t.Error("media section still present after delete")
	}
	// Other sections intact.
	if !strings.Contains(string(data), "[public]") || !strings.Contains(string(data), "[global]") {
		t.Error("unrelated sections damaged")
	}
	if _, err := DeleteSmbShare(cfg, "media"); err == nil {
		t.Error("expected not-found error on second delete")
	}
}

const sampleExports = `# NFS exports
/srv/nfs 192.168.1.0/24(rw,sync,no_subtree_check) 10.0.0.5(ro)
/srv/backup *(ro,sync)
`

func TestParseExportsAndListNFS(t *testing.T) {
	path := writeTempFile(t, "exports", sampleExports)
	exports, err := parseExportsFile(path)
	if err != nil {
		t.Fatalf("parseExportsFile: %v", err)
	}
	if len(exports) != 2 {
		t.Fatalf("expected 2 export lines, got %d", len(exports))
	}
	if exports[0].Path != "/srv/nfs" || len(exports[0].Clients) != 2 {
		t.Errorf("unexpected first export: %+v", exports[0])
	}

	shares := ListNFSExports(path)
	if len(shares) != 3 { // /srv/nfs has 2 clients, /srv/backup has 1
		t.Fatalf("expected 3 client shares, got %d", len(shares))
	}
	var roFound, rwFound bool
	for _, s := range shares {
		if s.Protocol != "nfs" {
			t.Errorf("protocol = %q", s.Protocol)
		}
		if s.ReadOnly {
			roFound = true
		} else {
			rwFound = true
		}
	}
	if !roFound || !rwFound {
		t.Errorf("expected mix of ro/rw shares, got %+v", shares)
	}
}

func TestCreateAndDeleteNFSExport(t *testing.T) {
	dir := t.TempDir()
	exportsPath := filepath.Join(dir, "exports")
	shareDir := filepath.Join(dir, "nfsdata")
	if err := os.MkdirAll(shareDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exportsPath, []byte(sampleExports), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := pluginConfig{SmbConfPath: filepath.Join(dir, "smb.conf"), ExportsPath: exportsPath, AutoReload: boolPtr(false)}

	msg, err := CreateNFSExport(cfg, shareDir, []string{"192.168.1.50", "10.0.0.9"}, "rw,sync")
	if err != nil {
		t.Fatalf("CreateNFSExport: %v", err)
	}
	if !strings.Contains(msg, "NFS export added") {
		t.Errorf("unexpected message: %s", msg)
	}
	data, _ := os.ReadFile(exportsPath)
	content := string(data)
	if !strings.Contains(content, shareDir+"(rw,sync)") && !strings.Contains(content, "192.168.1.50(rw,sync)") {
		t.Errorf("exports missing new entry:\n%s", content)
	}
	if strings.Count(content, shareDir) != 1 {
		t.Errorf("expected path once with both client tokens, got:\n%s", content)
	}
	if strings.Count(content, "(rw,sync)") != 2 {
		t.Errorf("expected both clients with rw,sync options, got:\n%s", content)
	}

	// Duplicate exact line rejected.
	if _, err := CreateNFSExport(cfg, shareDir, []string{"192.168.1.50", "10.0.0.9"}, "rw,sync"); err == nil {
		t.Error("expected duplicate export error")
	}

	// Delete one client only.
	msg, err = DeleteNFSExport(cfg, shareDir, "192.168.1.50")
	if err != nil {
		t.Fatalf("DeleteNFSExport client: %v", err)
	}
	_ = msg
	data, _ = os.ReadFile(exportsPath)
	content = string(data)
	if strings.Contains(content, "192.168.1.50") {
		t.Error("client still present after delete")
	}
	if !strings.Contains(content, "10.0.0.9") {
		t.Error("other client wrongly removed")
	}

	// Delete remaining clients (whole path).
	if _, err := DeleteNFSExport(cfg, shareDir, ""); err != nil {
		t.Fatalf("DeleteNFSExport path: %v", err)
	}
	data, _ = os.ReadFile(exportsPath)
	if strings.Contains(string(data), shareDir) {
		t.Error("path still present after full delete")
	}
	if !strings.Contains(string(data), "/srv/nfs") {
		t.Error("unrelated export damaged")
	}
}

func TestExportsContinuationAndComments(t *testing.T) {
	content := "# comment\n/srv/x 10.0.0.1(rw) \\\n  10.0.0.2(ro)\n\n/srv/y *\n"
	path := writeTempFile(t, "exports", content)
	exports, err := parseExportsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(exports) != 2 {
		t.Fatalf("expected 2 exports, got %d: %+v", len(exports), exports)
	}
	if len(exports[0].Clients) != 2 {
		t.Errorf("continuation not joined, clients=%v", exports[0].Clients)
	}
}

func TestParseSmbStatus(t *testing.T) {
	out := `Samba version 4.17.0
PID     Username     Group        Machine                                   Protocol  Version  Encryption           Signing
----------------------------------------------------------------------------------------------------------------------------------------
1234    alice        users        192.168.1.20                               SMB3_11   -        -                    partial(AES-128-GMAC)

Service      pid     machine       Connected at                     Encryption   Signing
---------------------------------------------------------------------------------------------
public       1234    192.168.1.20   Wed Nov  4 10:00:00 2026            -            -
media        1235    10.0.0.7        Wed Nov  4 10:05:00 2026            -            -

Locked files:
PID    User   DenyMode   Access      R/W        Oplock           SharePath   Name   Time
-----------------------------------------------------------------------------------------------
1234   alice  DENY_NONE  0x120089    RDONLY     BATCH           /srv/public file.txt  Wed Nov 4 10:01:00 2026
1234   alice  DENY_NONE  0x120089    RDONLY     BATCH           /srv/public other.txt Wed Nov 4 10:02:00 2026
`
	sessions, locked := parseSmbStatus(out)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d: %+v", len(sessions), sessions)
	}
	if sessions[0].Service != "public" || sessions[0].Machine != "192.168.1.20" {
		t.Errorf("unexpected session: %+v", sessions[0])
	}
	if locked != 2 {
		t.Errorf("expected 2 locked files, got %d", locked)
	}
}

func TestIsBuiltinShare(t *testing.T) {
	for _, n := range []string{"global", "homes", "printers", "print$", "GLOBAL"} {
		if !isBuiltinShare(n) {
			t.Errorf("%q should be builtin", n)
		}
	}
	for _, n := range []string{"public", "media", "backup"} {
		if isBuiltinShare(n) {
			t.Errorf("%q should not be builtin", n)
		}
	}
}

func TestRemoveSmbSection(t *testing.T) {
	lines := strings.Split(sampleSmbConf, "\n")
	out, ok := removeSmbSection(lines, "public")
	if !ok {
		t.Fatal("section not removed")
	}
	joined := strings.Join(out, "\n")
	if strings.Contains(joined, "[public]") {
		t.Error("public section still present")
	}
	if !strings.Contains(joined, "[backup]") || !strings.Contains(joined, "[global]") {
		t.Error("other sections damaged")
	}
	if _, ok := removeSmbSection(out, "nonexistent"); ok {
		t.Error("expected false for missing section")
	}
}

func TestSortedUnique(t *testing.T) {
	got := sortedUnique([]string{"b", "a", "b", "", "c", "a"})
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestCreateSmbShareRejectsNewlines(t *testing.T) {
	dir := t.TempDir()
	smbPath := filepath.Join(dir, "smb.conf")
	shareDir := filepath.Join(dir, "sharedata")
	if err := os.MkdirAll(shareDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(smbPath, []byte(sampleSmbConf), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := pluginConfig{SmbConfPath: smbPath, ExportsPath: filepath.Join(dir, "exports"), AutoReload: boolPtr(false)}

	for _, s := range []Share{
		{Protocol: "smb", Name: "evil", Path: shareDir, Comment: "x\n\tadmin users = *"},
		{Protocol: "smb", Name: "evil\n[global]", Path: shareDir},
		{Protocol: "smb", Name: "evil2", Path: shareDir, Users: []string{"alice\nbob"}},
	} {
		if _, err := CreateSmbShare(cfg, s); err == nil {
			t.Errorf("expected rejection for %+v", s)
		}
	}
	data, _ := os.ReadFile(smbPath)
	if strings.Contains(string(data), "admin users") {
		t.Error("injected directive written to smb.conf")
	}
}

func TestConfigPathValidation(t *testing.T) {
	for _, p := range []string{"/etc/samba/smb.conf", "/etc/samba/alt.conf"} {
		if err := validateSmbConfPath(p); err != nil {
			t.Errorf("validateSmbConfPath(%q): %v", p, err)
		}
	}
	for _, p := range []string{"/etc/passwd", "/etc/samba/../shadow", "relative.conf", "/etc/samba/x.txt"} {
		if err := validateSmbConfPath(p); err == nil {
			t.Errorf("validateSmbConfPath(%q) should fail", p)
		}
	}
	for _, p := range []string{"/etc/exports", "/etc/exports.d/media.exports"} {
		if err := validateExportsPath(p); err != nil {
			t.Errorf("validateExportsPath(%q): %v", p, err)
		}
	}
	for _, p := range []string{"/etc/shadow", "/etc/exports/../passwd", "exports"} {
		if err := validateExportsPath(p); err == nil {
			t.Errorf("validateExportsPath(%q) should fail", p)
		}
	}
}

func TestRestartServicesAllowlist(t *testing.T) {
	if _, err := RestartServices([]string{"ssh"}); err == nil {
		t.Error("expected rejection of non-allowlisted service")
	}
	if _, err := RestartServices([]string{"smbd", "evil.service"}); err == nil {
		t.Error("expected rejection when any service is not allowlisted")
	}
}

func TestValidateSmbUsername(t *testing.T) {
	valid := []string{"alice", "bob_1", "carol.d", "user-name", "workstation$", "Admin", "_svc"}
	for _, u := range valid {
		if err := validateSmbUsername(u); err != nil {
			t.Errorf("validateSmbUsername(%q) expected valid, got error: %v", u, err)
		}
	}

	invalid := []string{
		"",              // empty
		"-a",            // flag injection
		"--delete",      // flag injection
		"user name",     // contains whitespace
		"user;cat",      // command injection character
		"user\nname",    // newline injection
		"user`id`",      // backtick
		"user$(whoami)", // command substitution
		"user/sub",      // slash
		"user\\sub",     // backslash
		"a_very_long_username_that_exceeds_thirty_two_characters_limit", // > 32 chars
	}
	for _, u := range invalid {
		if err := validateSmbUsername(u); err == nil {
			t.Errorf("validateSmbUsername(%q) expected failure, got nil", u)
		}
	}

	// Test user creation rejects invalid username and password with newlines
	if _, err := CreateSmbUser("-flag", "validpass"); err == nil {
		t.Error("CreateSmbUser should reject usernames starting with '-'")
	}
	if _, err := CreateSmbUser("validuser", "pass\nnewline"); err == nil {
		t.Error("CreateSmbUser should reject passwords containing newlines")
	}

	// Test user deletion rejects invalid username
	if _, err := DeleteSmbUser("-flag"); err == nil {
		t.Error("DeleteSmbUser should reject usernames starting with '-'")
	}
}

func TestCreateNFSExportRejectsBadTokens(t *testing.T) {
	dir := t.TempDir()
	exportsPath := filepath.Join(dir, "exports")
	shareDir := filepath.Join(dir, "nfsdata")
	if err := os.MkdirAll(shareDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exportsPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := pluginConfig{SmbConfPath: filepath.Join(dir, "smb.conf"), ExportsPath: exportsPath, AutoReload: boolPtr(false)}

	if _, err := CreateNFSExport(cfg, shareDir, []string{"*"}, "rw *(ro)"); err == nil {
		t.Error("expected rejection of options containing whitespace/parens")
	}
	if _, err := CreateNFSExport(cfg, shareDir, []string{"10.0.0.1 extra"}, "rw"); err == nil {
		t.Error("expected rejection of client containing whitespace")
	}
	data, _ := os.ReadFile(exportsPath)
	if strings.Contains(string(data), shareDir) {
		t.Error("rejected export was written")
	}
}

func TestDeleteNFSExportContinuation(t *testing.T) {
	dir := t.TempDir()
	exportsPath := filepath.Join(dir, "exports")
	shareDir := filepath.Join(dir, "nfsdata")
	if err := os.MkdirAll(shareDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# header\n" + shareDir + " 10.0.0.1(rw) \\\n  10.0.0.2(ro)\n/srv/other *\n"
	if err := os.WriteFile(exportsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := pluginConfig{SmbConfPath: filepath.Join(dir, "smb.conf"), ExportsPath: exportsPath, AutoReload: boolPtr(false)}

	if _, err := DeleteNFSExport(cfg, shareDir, "10.0.0.1"); err != nil {
		t.Fatalf("DeleteNFSExport on continued line: %v", err)
	}
	data, _ := os.ReadFile(exportsPath)
	out := string(data)
	if strings.Contains(out, "10.0.0.1") {
		t.Error("continued-line client still present")
	}
	if !strings.Contains(out, "10.0.0.2") || !strings.Contains(out, "/srv/other") {
		t.Errorf("unrelated content damaged:\n%s", out)
	}
}
