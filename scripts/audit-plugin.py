#!/usr/bin/env python3
"""
M3TAL Plugin Auditor CLI Tool
Automates compliance auditing across all 6 gates of the M3TAL Plugin Standardization Spec.
Outputs human-readable scorecard, detailed findings, and optional JSON/Markdown reports.
"""

import os
import sys
import re
import json
import argparse
from pathlib import Path

try:
    import yaml
except ImportError:
    print("Error: PyYAML is required. Please install with: pip install pyyaml", file=sys.stderr)
    sys.exit(1)

# ANSI Colors
GREEN = "\033[92m"
YELLOW = "\033[93m"
RED = "\033[91m"
BLUE = "\033[94m"
CYAN = "\033[96m"
BOLD = "\033[1m"
RESET = "\033[0m"

VALID_CAPABILITIES = {
    "collector", "actions", "ui_view", "ui_widget", 
    "websocket_stream", "orchestrator", "network_tunnel", "alerts", "settings"
}

SEMVER_REGEX = re.compile(r"^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$")
PROCESS_NAME_REGEX = re.compile(r"^m3tal-[a-z0-9_-]+$")

class PluginAuditor:
    def __init__(self, plugin_dir: Path, verbose: bool = False):
        self.plugin_dir = plugin_dir.resolve()
        self.verbose = verbose
        self.findings = []  # list of (level, gate, message, file, remediation)
        self.gate_results = {
            "Gate 1": {"name": "Manifest & Metadata", "status": "PASS", "details": []},
            "Gate 2": {"name": "Directory Layout & Self-Containment", "status": "PASS", "details": []},
            "Gate 3": {"name": "IPC Protocol (JSON-RPC 2.0)", "status": "PASS", "details": []},
            "Gate 4": {"name": "Security & Sandboxing", "status": "PASS", "details": []},
            "Gate 5": {"name": "Dedicated Maintainer Agent", "status": "PASS", "details": []},
            "Gate 6": {"name": "Build & Packaging Readiness", "status": "PASS", "details": []},
        }
        self.manifest = None
        self.plugin_name = self.plugin_dir.name
        self.plugin_title = self.plugin_dir.name

    def add_finding(self, gate: str, level: str, message: str, file_path: str = "", remediation: str = ""):
        self.findings.append({
            "gate": gate,
            "level": level,
            "message": message,
            "file": file_path,
            "remediation": remediation
        })
        
        # Adjust gate status
        current_status = self.gate_results[gate]["status"]
        if level == "BLOCKER":
            self.gate_results[gate]["status"] = "FAIL"
            self.gate_results[gate]["details"].append(f"❌ {message}")
        elif level == "WARN" and current_status != "FAIL":
            self.gate_results[gate]["status"] = "WARN"
            self.gate_results[gate]["details"].append(f"⚠️ {message}")

    def audit_gate1_manifest(self):
        gate = "Gate 1"
        manifest_file = self.plugin_dir / "plugin.yaml"
        if not manifest_file.exists():
            manifest_file = self.plugin_dir / "plugin.yml"
            
        if not manifest_file.exists():
            self.add_finding(gate, "BLOCKER", "plugin.yaml manifest not found", "plugin.yaml", "Create a standard plugin.yaml conforming to m3tal.io/v1alpha1")
            return

        try:
            with open(manifest_file, "r", encoding="utf-8") as f:
                self.manifest = yaml.safe_load(f)
        except Exception as e:
            self.add_finding(gate, "BLOCKER", f"Failed to parse plugin.yaml: {e}", "plugin.yaml", "Fix YAML syntax errors")
            return

        if not isinstance(self.manifest, dict):
            self.add_finding(gate, "BLOCKER", "plugin.yaml root must be a mapping", "plugin.yaml", "Reformat manifest")
            return

        # Check apiVersion and kind
        if self.manifest.get("apiVersion") != "m3tal.io/v1alpha1":
            self.add_finding(gate, "BLOCKER", f"apiVersion must be 'm3tal.io/v1alpha1', got {self.manifest.get('apiVersion')}", "plugin.yaml", "Set apiVersion: m3tal.io/v1alpha1")
        if self.manifest.get("kind") != "Plugin":
            self.add_finding(gate, "BLOCKER", f"kind must be 'Plugin', got {self.manifest.get('kind')}", "plugin.yaml", "Set kind: Plugin")

        # Check metadata
        meta = self.manifest.get("metadata", {})
        if not isinstance(meta, dict):
            self.add_finding(gate, "BLOCKER", "metadata section is missing or invalid", "plugin.yaml", "Define metadata with name, version, title, etc.")
            meta = {}
            
        self.plugin_name = meta.get("name", self.plugin_name)
        self.plugin_title = meta.get("title", self.plugin_title)

        for req_field in ["name", "version", "title", "description", "author", "license", "icon"]:
            if not meta.get(req_field):
                self.add_finding(gate, "BLOCKER", f"Missing required metadata field: '{req_field}'", "plugin.yaml", f"Add metadata.{req_field}")

        version = str(meta.get("version", ""))
        if version and not SEMVER_REGEX.match(version):
            self.add_finding(gate, "WARN", f"Version '{version}' does not conform strictly to SemVer (X.Y.Z)", "plugin.yaml", "Use standard SemVer (e.g. 0.1.0)")

        # Check spec
        spec = self.manifest.get("spec", {})
        if not isinstance(spec, dict):
            self.add_finding(gate, "BLOCKER", "spec section is missing or invalid", "plugin.yaml", "Define spec section")
            spec = {}

        if not spec.get("command"):
            self.add_finding(gate, "BLOCKER", "spec.command is missing", "plugin.yaml", "Define spec.command pointing to binary entrypoint (e.g. ./bin/m3tal-<name>)")

        proc_name = spec.get("process_name", "")
        if not proc_name:
            self.add_finding(gate, "BLOCKER", "spec.process_name is missing", "plugin.yaml", f"Set spec.process_name: m3tal-{self.plugin_name}")
        elif not PROCESS_NAME_REGEX.match(proc_name):
            self.add_finding(gate, "BLOCKER", f"spec.process_name '{proc_name}' must match pattern '^m3tal-[a-z0-9_-]+$'", "plugin.yaml", f"Change process_name to m3tal-{self.plugin_name}")

        capabilities = spec.get("capabilities", [])
        if not isinstance(capabilities, list):
            self.add_finding(gate, "BLOCKER", "spec.capabilities must be a list", "plugin.yaml", "Specify capabilities as an array")
            capabilities = []
        else:
            for cap in capabilities:
                if cap not in VALID_CAPABILITIES:
                    self.add_finding(gate, "BLOCKER", f"Unknown capability '{cap}'", "plugin.yaml", f"Use valid capability from: {', '.join(sorted(VALID_CAPABILITIES))}")

        # If ui_widget is declared
        if "ui_widget" in capabilities:
            widgets = spec.get("widgets")
            if not widgets or not isinstance(widgets, list) or len(widgets) == 0:
                self.add_finding(gate, "BLOCKER", "Capability 'ui_widget' declared but spec.widgets is missing or empty", "plugin.yaml", "Define spec.widgets list or remove ui_widget capability")
            else:
                for w in widgets:
                    if not isinstance(w, dict) or not w.get("id") or not w.get("title"):
                        self.add_finding(gate, "BLOCKER", f"Widget missing required 'id' or 'title': {w}", "plugin.yaml", "Define id and title for each widget")
                    size = w.get("size")
                    if not isinstance(size, list) or len(size) != 2:
                        self.add_finding(gate, "WARN", f"Widget '{w.get('id')}' size should be [colSpan (1..4), rowSpan (1..3)]", "plugin.yaml", "Set size: [2, 1]")

        # RPC transport check
        rpc = spec.get("rpc", {})
        if rpc.get("protocol") != "jsonrpc-2.0":
            self.add_finding(gate, "BLOCKER", f"spec.rpc.protocol must be 'jsonrpc-2.0', got '{rpc.get('protocol')}'", "plugin.yaml", "Set spec.rpc.protocol: jsonrpc-2.0")
        if rpc.get("transport") not in ["stdio", "unix_socket"]:
            self.add_finding(gate, "BLOCKER", f"spec.rpc.transport must be 'stdio' or 'unix_socket', got '{rpc.get('transport')}'", "plugin.yaml", "Set spec.rpc.transport: stdio")

        if self.gate_results[gate]["status"] == "PASS":
            self.gate_results[gate]["details"].append("Valid m3tal.io/v1alpha1 manifest, process naming, and capabilities.")

    def audit_gate2_layout_self_containment(self):
        gate = "Gate 2"
        # Check changelog
        changelog = self.plugin_dir / "CHANGELOG.md"
        if not changelog.exists():
            self.add_finding(gate, "WARN", "CHANGELOG.md not found", "CHANGELOG.md", "Create CHANGELOG.md following Keep a Changelog standard")

        # Check UI view assets if ui_view capability enabled
        capabilities = self.manifest.get("spec", {}).get("capabilities", []) if self.manifest else []
        if "ui_view" in capabilities:
            ui_dist = self.plugin_dir / "ui" / "dist" / "index.html"
            if not ui_dist.exists():
                self.add_finding(gate, "BLOCKER", "Capability 'ui_view' declared but ui/dist/index.html is missing", "ui/dist/index.html", "Build UI bundle into ui/dist/index.html")
            else:
                try:
                    content = ui_dist.read_text(encoding="utf-8")
                    if 'src="/assets' in content or 'href="/assets' in content or 'src="/' in content:
                        self.add_finding(gate, "WARN", "ui/dist/index.html appears to contain absolute asset paths ('/assets/...') instead of relative ('./assets/...')", "ui/dist/index.html", "Configure frontend bundler with base: './'")
                except Exception:
                    pass

        # Zero Core Footprint / Forbidden imports check
        src_dir = self.plugin_dir / "src"
        if src_dir.exists():
            for root, _, files in os.walk(src_dir):
                for file in files:
                    if file.endswith((".go", ".py", ".rs", ".js", ".ts")):
                        fpath = Path(root) / file
                        try:
                            code = fpath.read_text(encoding="utf-8", errors="ignore")
                            if "github.com/jakej985-rgb/m3tal-core/core" in code:
                                self.add_finding(gate, "BLOCKER", f"Forbidden import of m3tal-core internal package in {fpath.relative_to(self.plugin_dir)}", str(fpath.relative_to(self.plugin_dir)), "Plugins must not import m3tal-core internals; communicate via JSON-RPC or models only.")
                            if "m3tal-core/gui" in code:
                                self.add_finding(gate, "BLOCKER", f"Forbidden reference to m3tal-core/gui in {fpath.relative_to(self.plugin_dir)}", str(fpath.relative_to(self.plugin_dir)), "Plugin UI must reside strictly in plugin ui/ directory.")
                        except Exception:
                            pass

        if self.gate_results[gate]["status"] == "PASS":
            self.gate_results[gate]["details"].append("Clean isolated directory layout with zero core footprint violations.")

    def audit_gate3_ipc_compliance(self):
        gate = "Gate 3"
        # Search for JSON-RPC handlers in src or root backend files
        found_methods = set()
        required_methods = {"m3tal.init", "m3tal.shutdown"}

        capabilities = self.manifest.get("spec", {}).get("capabilities", []) if self.manifest else []
        if "collector" in capabilities:
            required_methods.add("m3tal.collect")
        if "actions" in capabilities or "settings" in capabilities:
            required_methods.add("m3tal.action")

        for root, _, files in os.walk(self.plugin_dir):
            if ".git" in root or "node_modules" in root or "ui/dist" in root:
                continue
            for file in files:
                if file.endswith((".go", ".py", ".rs", ".sh", ".c", ".cpp", ".js")):
                    fpath = Path(root) / file
                    try:
                        code = fpath.read_text(encoding="utf-8", errors="ignore")
                        for m in ["m3tal.init", "m3tal.collect", "m3tal.action", "m3tal.shutdown"]:
                            if m in code:
                                found_methods.add(m)
                    except Exception:
                        pass

        missing_methods = required_methods - found_methods
        if missing_methods:
            for mm in missing_methods:
                self.add_finding(gate, "BLOCKER", f"JSON-RPC method handler '{mm}' not found in source code", "src/", f"Implement '{mm}' handler in JSON-RPC server loop")
        else:
            self.gate_results[gate]["details"].append(f"Implements all required methods: {', '.join(sorted(required_methods))}")

    def audit_gate4_security_sandboxing(self):
        gate = "Gate 4"
        spec = self.manifest.get("spec", {}) if self.manifest else {}
        reqs = spec.get("requirements", {})
        if reqs.get("root") is True:
            self.add_finding(gate, "WARN", "spec.requirements.root is true. Plugin requires elevated root privileges.", "plugin.yaml", "Minimize privileges or document polkit authorization policy")

        # Check for obvious hardcoded secrets or suspicious root commands
        secret_patterns = [
            (re.compile(r'(?i)(?:api_key|secret_key|private_key|auth_token)\s*=\s*["\'][a-zA-Z0-9_\-]{16,}["\']'), "Potential hardcoded secret or token detected")
        ]

        for root, _, files in os.walk(self.plugin_dir):
            if ".git" in root or "node_modules" in root or "ui/dist" in root or "bin" in root:
                continue
            for file in files:
                if file.endswith((".go", ".py", ".sh", ".yaml", ".json", ".env")):
                    fpath = Path(root) / file
                    try:
                        code = fpath.read_text(encoding="utf-8", errors="ignore")
                        for pat, msg in secret_patterns:
                            if pat.search(code):
                                self.add_finding(gate, "WARN", f"{msg} in {fpath.relative_to(self.plugin_dir)}", str(fpath.relative_to(self.plugin_dir)), "Move credentials to environment variables or config.json")
                    except Exception:
                        pass

        if self.gate_results[gate]["status"] == "PASS":
            self.gate_results[gate]["details"].append("Root privileges false and no uncontained credentials detected.")

    def audit_gate5_maintainer_agent(self):
        gate = "Gate 5"
        agent_file = self.plugin_dir / ".agents" / "agents" / "plugin-maintainer" / "agent.md"
        if not agent_file.exists():
            self.add_finding(gate, "WARN", "Dedicated in-repo maintainer agent not found (.agents/agents/plugin-maintainer/agent.md)", ".agents/agents/plugin-maintainer/agent.md", "Scaffold plugin-maintainer/agent.md to safeguard repository maintainability")
        else:
            try:
                content = agent_file.read_text(encoding="utf-8")
                if "name: plugin-maintainer" not in content:
                    self.add_finding(gate, "WARN", "Maintainer agent frontmatter does not define 'name: plugin-maintainer'", str(agent_file.relative_to(self.plugin_dir)), "Set name: plugin-maintainer in agent.md")
                else:
                    self.gate_results[gate]["details"].append("Found dedicated in-repo maintainer agent (.agents/agents/plugin-maintainer/agent.md).")
            except Exception as e:
                self.add_finding(gate, "WARN", f"Could not read maintainer agent: {e}", str(agent_file.relative_to(self.plugin_dir)), "Fix file permissions or syntax")

    def audit_gate6_build_packaging(self):
        gate = "Gate 6"
        makefile = self.plugin_dir / "Makefile"
        if not makefile.exists():
            self.add_finding(gate, "WARN", "Makefile not found at plugin root", "Makefile", "Provide Makefile with 'build' and 'test' targets")
        else:
            try:
                content = makefile.read_text(encoding="utf-8")
                if "build:" not in content:
                    self.add_finding(gate, "WARN", "Makefile missing 'build' target", "Makefile", "Add 'build:' target to Makefile")
                if "test:" not in content and "test-ipc:" not in content:
                    self.add_finding(gate, "WARN", "Makefile missing 'test' target", "Makefile", "Add 'test:' target to Makefile")
            except Exception:
                pass

        if self.gate_results[gate]["status"] == "PASS":
            self.gate_results[gate]["details"].append("Makefile exists with build and test targets.")

    def run_all(self):
        self.audit_gate1_manifest()
        self.audit_gate2_layout_self_containment()
        self.audit_gate3_ipc_compliance()
        self.audit_gate4_security_sandboxing()
        self.audit_gate5_maintainer_agent()
        self.audit_gate6_build_packaging()

        # Overall Status
        has_fail = any(g["status"] == "FAIL" for g in self.gate_results.values())
        has_warn = any(g["status"] == "WARN" for g in self.gate_results.values())
        if has_fail:
            overall = "FAILED"
        elif has_warn:
            overall = "WARNINGS"
        else:
            overall = "PASSED"
        return overall

    def print_terminal_report(self, overall: str):
        print(f"\n{BOLD}{CYAN}══════════════════════════════════════════════════════════════════════════{RESET}")
        print(f"{BOLD}  M3TAL Plugin Audit: {self.plugin_title} ({self.plugin_name}){RESET}")
        print(f"{CYAN}══════════════════════════════════════════════════════════════════════════{RESET}")
        print(f"Target: {self.plugin_dir}")
        status_color = GREEN if overall == "PASSED" else (YELLOW if overall == "WARNINGS" else RED)
        print(f"Overall Status: {status_color}{BOLD}[{overall}]{RESET}\n")

        print(f"{BOLD}Gate Scorecard:{RESET}")
        print(f"{'Gate':<8} | {'Category':<36} | {'Status':<10} | {'Summary'}")
        print("-" * 80)
        for gid, info in self.gate_results.items():
            st = info["status"]
            sc = GREEN if st == "PASS" else (YELLOW if st == "WARN" else RED)
            summary = "; ".join(info["details"]) if info["details"] else ""
            if len(summary) > 40:
                summary = summary[:37] + "..."
            print(f"{gid:<8} | {info['name']:<36} | {sc}{st:<10}{RESET} | {summary}")

        if self.findings:
            print(f"\n{BOLD}Detailed Findings & Action Items:{RESET}")
            for f in self.findings:
                lvl_color = RED if f["level"] == "BLOCKER" else YELLOW
                print(f"  [{lvl_color}{f['level']}{RESET}] {BOLD}{f['gate']}{RESET}: {f['message']}")
                if f["file"]:
                    print(f"         {BLUE}File:{RESET} {f['file']}")
                if f["remediation"]:
                    print(f"         {GREEN}Fix:{RESET}  {f['remediation']}")

        print(f"{CYAN}══════════════════════════════════════════════════════════════════════════{RESET}\n")

    def to_markdown(self, overall: str) -> str:
        md = []
        md.append(f"# M3TAL Plugin Audit Report: {self.plugin_title} (`{self.plugin_name}`)\n")
        md.append(f"**Audit Target**: `{self.plugin_dir}`  ")
        md.append(f"**Spec Version**: `m3tal.io/v1alpha1`  ")
        md.append(f"**Overall Status**: `[{overall}]`\n")
        md.append("## Scorecard Summary\n")
        md.append("| Gate | Category | Status | Details |")
        md.append("|---|---|---|---|")
        for gid, info in self.gate_results.items():
            details = "; ".join(info["details"]) if info["details"] else "Compliant"
            md.append(f"| {gid} | {info['name']} | `{info['status']}` | {details} |")

        md.append("\n## Detailed Findings & Blocking Issues\n")
        if not self.findings:
            md.append("None. All compliance checks passed cleanly.\n")
        else:
            for f in self.findings:
                md.append(f"- **[{f['level']}]**: {f['message']}")
                if f["file"]:
                    md.append(f"  - *File*: `{f['file']}`")
                if f["remediation"]:
                    md.append(f"  - *Remediation*: {f['remediation']}")

        md.append("\n## Recommended Actions\n")
        if not self.findings:
            md.append("1. Proceed to protocol and package verification.")
        else:
            actions = [f['remediation'] for f in self.findings if f['remediation']]
            for idx, act in enumerate(set(actions), 1):
                md.append(f"{idx}. {act}")

        return "\n".join(md)

def main():
    parser = argparse.ArgumentParser(description="Audit M3TAL plugin compliance against standardization spec")
    parser.add_argument("plugin", help="Path to plugin directory or plugin directory name in m3tal-plugins")
    parser.add_argument("--json", action="store_true", help="Output JSON results")
    parser.add_argument("--markdown", action="store_true", help="Output Markdown report")
    parser.add_argument("--strict", action="store_true", help="Treat warnings as failures (exit code 1)")
    args = parser.parse_args()

    target = Path(args.plugin)
    if not target.exists():
        # Check relative to /home/m3tal/apps/m3tal-plugins/
        alt_target = Path("/home/m3tal/apps/m3tal-plugins") / args.plugin
        if alt_target.exists():
            target = alt_target
        else:
            print(f"Error: Plugin directory '{args.plugin}' not found.", file=sys.stderr)
            sys.exit(2)

    auditor = PluginAuditor(target)
    overall = auditor.run_all()

    if args.json:
        data = {
            "plugin_name": auditor.plugin_name,
            "plugin_title": auditor.plugin_title,
            "target": str(auditor.plugin_dir),
            "overall_status": overall,
            "gates": auditor.gate_results,
            "findings": auditor.findings
        }
        print(json.dumps(data, indent=2))
    elif args.markdown:
        print(auditor.to_markdown(overall))
    else:
        auditor.print_terminal_report(overall)

    if overall == "FAILED" or (args.strict and overall == "WARNINGS"):
        sys.exit(1)
    sys.exit(0)

if __name__ == "__main__":
    main()
