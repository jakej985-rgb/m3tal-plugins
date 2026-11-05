# Sentinel's Journal

## 2026-11-05 - Enforce Container Identifier Allowlist Against Command-Line Flag and Argument Injection
**Vulnerability:** Subprocess wrappers invoking `exec.Command("docker", ...)` with unsanitized user-supplied container identifiers allowed command-line argument injection (e.g. passing leading dashes such as `--checkpoint` or `-f` as container names).
**Learning:** Even though `exec.Command` avoids shell metacharacter expansion by passing arguments directly via `execve`, passing arbitrary strings as CLI positional parameters still exposes CLI flag injection vulnerabilities when the CLI utility parses optional flags anywhere in the argument vector.
**Prevention:** Always validate external identifiers with an explicit regex allowlist (such as `^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`) before passing them to external CLI processes, ensuring names begin with an alphanumeric character and contain no command options or illegal path characters.
