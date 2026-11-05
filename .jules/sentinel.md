# Sentinel's Journal - Critical Security Learnings

## 2026-11-05 - Docker Subprocess Argument Injection via Unvalidated Target Parameters
**Vulnerability:** Subprocess wrappers around the `docker` CLI in `m3tal-docker` (`StartContainer`, `StopContainer`, `RestartContainer`, `GetLogs`, `InspectContainer`) took arbitrary user-supplied container identifiers or names and passed them directly as positional arguments to `exec.Command("docker", ... target)`. Attackers could inject CLI flags (e.g. `--checkpoint`, `--time`) or shell metacharacters.
**Learning:** Even when `exec.Command` avoids shell interpretation and splitting, passing untrusted strings directly as positional CLI arguments to utilities like `docker` is vulnerable to CLI argument/flag injection if the argument starts with `-` or contains unexpected control structures.
**Prevention:** Enforce strict whitelisting validation (`isValidContainerTarget`) on all container IDs and names (1-128 alphanumeric characters, dots, underscores, dashes, strictly prohibiting leading hyphens) before invoking external CLI tools or looking up container state.
