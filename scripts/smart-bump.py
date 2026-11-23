#!/usr/bin/env python3
"""
scripts/smart-bump.py
Smart Semantic Version Bumper and Change Detector for M3TAL Plugins Monorepo.

Determines if a plugin directory has been touched since its last release or git base ref.
If touched:
  - Calculates Semantic Version bump (feat: -> minor, fix:/docs:/chore: -> patch, BREAKING -> major)
  - Updates <plugin>/plugin.yaml
  - Appends new version entry to <plugin>/CHANGELOG.md
If untouched:
  - Skips bumping and skips packaging.
"""

import sys
import os
import re
import subprocess
import argparse
from datetime import datetime

ROOT_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def run_git(args, cwd=ROOT_DIR):
    proc = subprocess.run(["git"] + args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, cwd=cwd)
    if proc.returncode != 0:
        return ""
    return proc.stdout.strip()


def find_plugins():
    plugins = []
    for item in sorted(os.listdir(ROOT_DIR)):
        item_path = os.path.join(ROOT_DIR, item)
        # Must be real directory (not symlink), not hidden, not scripts/dist
        if os.path.isdir(item_path) and not os.path.islink(item_path) and not item.startswith(".") and item not in ["scripts", "dist"]:
            if os.path.isfile(os.path.join(item_path, "plugin.yaml")):
                plugins.append(item)
    return plugins


def get_current_version(plugin):
    manifest_path = os.path.join(ROOT_DIR, plugin, "plugin.yaml")
    if not os.path.isfile(manifest_path):
        return "0.1.0"
    with open(manifest_path, "r", encoding="utf-8") as f:
        content = f.read()
    match = re.search(r'^\s*version:\s*["\']?([^"\'\s]+)["\']?', content, re.MULTILINE)
    if match:
        return match.group(1).lstrip("v")
    return "0.1.0"


def find_last_bump_commit(plugin):
    manifest_rel = os.path.join(plugin, "plugin.yaml")
    # First check if there is a plugin tag e.g. <plugin>-v<version>
    curr_ver = get_current_version(plugin)
    tag_name = f"{plugin}-v{curr_ver}"
    tag_commit = run_git(["rev-list", "-n", "1", tag_name])
    if tag_commit:
        return tag_commit

    # Otherwise find the commit that touched plugin.yaml
    out = run_git(["log", "-n", "1", "--format=%H", "--", manifest_rel])
    if out:
        return out
    return ""


def get_changes_for_plugin(plugin, base_ref=None):
    plugin_rel = plugin
    commits = []
    
    # 1. Uncommitted changes in working tree
    status_out = run_git(["status", "--porcelain", "--", plugin_rel])
    has_uncommitted = bool(status_out.strip())

    # 2. Determine base commit to check
    if not base_ref:
        base_ref = find_last_bump_commit(plugin)

    if base_ref:
        # Check commits between base_ref and HEAD that touch this plugin
        range_str = f"{base_ref}..HEAD"
        raw = run_git(["log", range_str, "--pretty=format:%h%x1f%s%x1f%b%x1e", "--", plugin_rel])
        if raw:
            for entry in raw.split("\x1e"):
                parts = entry.strip().split("\x1f")
                if len(parts) >= 2 and parts[0].strip():
                    commits.append({
                        "sha": parts[0].strip(),
                        "subject": parts[1].strip(),
                        "body": parts[2].strip() if len(parts) > 2 else ""
                    })

    # Also check git diff files directly
    diff_files = []
    if base_ref:
        diff_out = run_git(["diff", "--name-only", f"{base_ref}..HEAD", "--", plugin_rel])
        if diff_out:
            diff_files = [f.strip() for f in diff_out.split("\n") if f.strip()]

    is_touched = has_uncommitted or len(commits) > 0 or len(diff_files) > 0
    return {
        "touched": is_touched,
        "has_uncommitted": has_uncommitted,
        "commits": commits,
        "diff_files": diff_files,
        "base_ref": base_ref
    }


def calculate_bump_type(commits, force_bump=None):
    if force_bump in ["major", "minor", "patch"]:
        return force_bump

    has_major = False
    has_minor = False

    for c in commits:
        subj = c["subject"].lower()
        body = c["body"].lower()
        full = f"{subj}\n{body}"

        if "breaking change" in full or "!:" in subj:
            has_major = True
            break
        if subj.startswith("feat:") or subj.startswith("feat(") or "feature" in subj:
            has_minor = True

    if has_major:
        return "major"
    if has_minor:
        return "minor"
    return "patch"


def bump_semver(version, bump_type):
    parts = version.split(".")
    major = int(parts[0]) if len(parts) > 0 and parts[0].isdigit() else 0
    minor = int(parts[1]) if len(parts) > 1 and parts[1].isdigit() else 1
    patch = int(parts[2]) if len(parts) > 2 and parts[2].isdigit() else 0

    if bump_type == "major":
        return f"{major + 1}.0.0"
    elif bump_type == "minor":
        return f"{major}.{minor + 1}.0"
    else:  # patch
        return f"{major}.{minor}.{patch + 1}"


def update_plugin_manifest(plugin, new_version):
    manifest_path = os.path.join(ROOT_DIR, plugin, "plugin.yaml")
    with open(manifest_path, "r", encoding="utf-8") as f:
        content = f.read()

    new_content = re.sub(
        r'(^\s*version:\s*)["\']?[^"\'\n]+["\']?',
        rf'\g<1>"{new_version}"',
        content,
        flags=re.MULTILINE
    )
    with open(manifest_path, "w", encoding="utf-8") as f:
        f.write(new_content)


def update_plugin_changelog(plugin, new_version, commits, note=None):
    changelog_path = os.path.join(ROOT_DIR, plugin, "CHANGELOG.md")
    if not os.path.isfile(changelog_path):
        return

    with open(changelog_path, "r", encoding="utf-8") as f:
        existing = f.read()

    # Don't duplicate if header already exists
    if f"## [{new_version}]" in existing:
        return

    today = datetime.now().strftime("%Y-%m-%d")
    header = f"## [{new_version}] - {today}\n\n### 🚀 Changes & Improvements\n"

    items = []
    if note:
        items.append(f"- {note}")
    elif commits:
        for c in commits:
            items.append(f"- **{c['subject']}** (`{c['sha']}`)")
    else:
        items.append(f"- Automated version bump to {new_version} for touched plugin assets.")

    new_section = header + "\n".join(items) + "\n\n---\n\n"

    # Insert after first intro divider or title
    if "---" in existing:
        parts = existing.split("---", 1)
        updated = parts[0] + "---\n\n" + new_section + parts[1].lstrip("\n")
    else:
        updated = existing + "\n\n" + new_section

    with open(changelog_path, "w", encoding="utf-8") as f:
        f.write(updated)


def main():
    parser = argparse.ArgumentParser(description="Smart plugin change detector and version bumper")
    parser.add_argument("--plugin", help="Specific plugin directory (or all if omitted)")
    parser.add_argument("--base", help="Git base ref to compare against (e.g. HEAD~1, origin/main)")
    parser.add_argument("--apply", action="store_true", help="Apply version bump to plugin.yaml and CHANGELOG.md")
    parser.add_argument("--force", action="store_true", help="Force bump even if no change is detected")
    parser.add_argument("--bump-type", choices=["auto", "patch", "minor", "major"], default="auto")
    parser.add_argument("--set-version", metavar="VERSION", help="Set an exact MAJOR.MINOR.PATCH version (used to align release units)")
    parser.add_argument("--list-touched", action="store_true", help="Print only space-separated list of touched plugins")
    args = parser.parse_args()

    all_plugins = find_plugins()
    if args.plugin:
        if args.plugin not in all_plugins:
            print(f"Error: plugin '{args.plugin}' not found in {all_plugins}", file=sys.stderr)
            sys.exit(1)
        target_plugins = [args.plugin]
    else:
        target_plugins = all_plugins

    if args.set_version:
        if not args.plugin:
            print("Error: --set-version requires --plugin (refusing to rewrite every manifest).", file=sys.stderr)
            sys.exit(1)
        new_ver = args.set_version.strip().lstrip("v")
        if not re.fullmatch(r"\d+\.\d+\.\d+", new_ver):
            print(f"Error: --set-version expects MAJOR.MINOR.PATCH (got '{args.set_version}').", file=sys.stderr)
            sys.exit(1)
        for p in target_plugins:
            curr_ver = get_current_version(p)
            if curr_ver == new_ver:
                print(f"📦 Plugin: {p}\n   Version:  {curr_ver} (unchanged)")
                continue
            print(f"📦 Plugin: {p}\n   Version:  {curr_ver} -> {new_ver} (set)")
            if args.apply:
                update_plugin_manifest(p, new_ver)
                update_plugin_changelog(p, new_ver, [], note=f"Aligned to release unit version {new_ver}.")
                print(f"   ✅ Applied version {new_ver} to {p}/plugin.yaml and {p}/CHANGELOG.md")
        return

    touched_plugins = []

    for p in target_plugins:
        curr_ver = get_current_version(p)
        info = get_changes_for_plugin(p, base_ref=args.base)

        is_touched = info["touched"] or args.force

        if is_touched:
            touched_plugins.append(p)
            bump_type = calculate_bump_type(info["commits"], force_bump=args.bump_type if args.bump_type != "auto" else None)
            new_ver = bump_semver(curr_ver, bump_type)

            if not args.list_touched:
                print(f"📦 Plugin: {p}")
                print(f"   Status:   TOUCHED (commits: {len(info['commits'])}, uncommitted: {info['has_uncommitted']})")
                print(f"   Version:  {curr_ver} -> {new_ver} ({bump_type})")

            if args.apply:
                update_plugin_manifest(p, new_ver)
                update_plugin_changelog(p, new_ver, info["commits"])
                if not args.list_touched:
                    print(f"   ✅ Applied bump to {p}/plugin.yaml and {p}/CHANGELOG.md")
        else:
            if not args.list_touched:
                print(f"📦 Plugin: {p}")
                print(f"   Status:   UNTOUCHED (version stays at {curr_ver})")
                print(f"   Action:   Skipping bump and build.")

    if args.list_touched:
        print(" ".join(touched_plugins))


if __name__ == "__main__":
    main()
