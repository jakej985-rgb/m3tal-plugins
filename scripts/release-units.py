#!/usr/bin/env python3
"""Release unit resolver for the M3TAL plugins monorepo.

A release unit is the set of plugin directories that must be version bumped,
built, and published together. It is declared in `release-units.json` at the
repository root.

Most units hold a single plugin. The `mesh` unit holds `m3tal-mesh` and
`m3tal-mesh-node` because `m3tal-plugin-mesh-cluster` is a metapackage over
both, and `scripts/build-cluster-deb.sh` refuses to build it when the node
version trails the mesh version. Members are therefore always aligned to the
highest version in the unit before packaging.

Usage:
  ./scripts/release-units.py --verify
  ./scripts/release-units.py --list                        # matrix for touched units
  ./scripts/release-units.py --list-all                    # matrix for every unit
  ./scripts/release-units.py --list-unit mesh              # matrix for one unit
  ./scripts/release-units.py --unit mesh --plugins         # plugin directories
  ./scripts/release-units.py --unit mesh --cluster         # true|false
  ./scripts/release-units.py --unit mesh --base <sha> --apply [--bump-type patch] [--force]
"""

import argparse
import importlib.util
import json
import os
import re
import subprocess
import sys

ROOT_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
UNITS_PATH = os.path.join(ROOT_DIR, "release-units.json")
SMART_BUMP = os.path.join(ROOT_DIR, "scripts", "smart-bump.py")


def fail(message):
    print(f"Error: {message}", file=sys.stderr)
    sys.exit(1)


def load_smart_bump():
    if not os.path.isfile(SMART_BUMP):
        fail(f"{SMART_BUMP} not found")
    spec = importlib.util.spec_from_file_location("m3tal_smart_bump", SMART_BUMP)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


smart = load_smart_bump()


def load_units():
    if not os.path.isfile(UNITS_PATH):
        fail(f"{UNITS_PATH} not found")
    with open(UNITS_PATH, encoding="utf-8") as f:
        raw = json.load(f)

    units = {}
    assigned = {}
    for name, cfg in raw.items():
        plugins = cfg.get("plugins") if isinstance(cfg, dict) else None
        if not plugins:
            fail(f"release unit '{name}' must list at least one plugin directory")
        for plugin in plugins:
            if not os.path.isfile(os.path.join(ROOT_DIR, plugin, "plugin.yaml")):
                fail(f"release unit '{name}' references '{plugin}', which has no plugin.yaml")
            if plugin in assigned:
                fail(f"'{plugin}' belongs to both '{assigned[plugin]}' and '{name}'")
            assigned[plugin] = name
        units[name] = {"plugins": list(plugins), "cluster": bool(cfg.get("cluster", False))}

    # A plugin with a manifest must be releasable; otherwise its changes would
    # never be detected. Planned directories have no plugin.yaml and are skipped.
    for plugin in smart.find_plugins():
        if plugin not in assigned:
            fail(f"'{plugin}' has a plugin.yaml but no release unit; add it to release-units.json")

    return units


def semver_key(version):
    parts = re.findall(r"\d+", version)
    return tuple(int(p) for p in (parts + ["0", "0", "0"])[:3])


def run(args):
    proc = subprocess.run(
        [sys.executable] + args,
        cwd=ROOT_DIR,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if proc.returncode != 0:
        sys.stderr.write(proc.stdout)
        sys.stderr.write(proc.stderr)
        sys.exit(proc.returncode)
    return proc.stdout


def touched_plugins(base=None):
    # Without an explicit base, smart-bump diffs each plugin against the commit
    # that last touched its plugin.yaml. That makes detection independent of the
    # triggering push, so a superseded queued run cannot lose a release.
    args = [SMART_BUMP] + (["--base", base] if base else []) + ["--list-touched"]
    out = run(args)
    return [p for p in out.split() if p]


def unit_is_touched(unit, touched):
    return bool(set(unit["plugins"]) & set(touched))


def bump_unit(unit, base, bump_type, force):
    bumped = []
    base_args = ["--base", base] if base else []

    if force:
        # Manual dispatch: bump the unit's primary plugin even without changes,
        # then align the rest. Exactly one version increment per dispatch.
        primary = unit["plugins"][0]
        run([SMART_BUMP, "--plugin", primary] + base_args + ["--apply", "--bump-type", bump_type, "--force"])
        bumped.append(primary)
    else:
        for plugin in unit["plugins"]:
            changed = run([SMART_BUMP, "--plugin", plugin] + base_args + ["--list-touched"]).split()
            if not changed:
                print(f"ℹ️ {plugin} unchanged; its version stays as-is.")
                continue
            run([SMART_BUMP, "--plugin", plugin] + base_args + ["--apply", "--bump-type", bump_type])
            bumped.append(plugin)

    if not bumped:
        return

    # Version lockstep: the cluster metapackage requires node >= mesh, and a unit
    # must never publish mismatched versions of its members.
    versions = {p: smart.get_current_version(p) for p in unit["plugins"]}
    target = max(versions.values(), key=semver_key)
    for plugin, version in versions.items():
        if version != target:
            print(f"🔗 Aligning {plugin} {version} -> {target} with its release unit.")
            run([SMART_BUMP, "--plugin", plugin, "--set-version", target, "--apply"])


def main():
    parser = argparse.ArgumentParser(description="Resolve and bump M3TAL release units")
    parser.add_argument("--base", help="Git base ref to compare against")
    parser.add_argument("--unit", help="Release unit name")
    parser.add_argument("--list", action="store_true", help="Print the matrix JSON for units touched since --base")
    parser.add_argument("--list-all", action="store_true", help="Print the matrix JSON for every unit")
    parser.add_argument("--list-unit", metavar="UNIT", help="Print the matrix JSON for one unit")
    parser.add_argument("--plugins", action="store_true", help="Print the plugin directories of --unit")
    parser.add_argument("--cluster", action="store_true", help="Print true/false: does --unit build the cluster metapackage")
    parser.add_argument("--verify", action="store_true", help="Validate release-units.json and print the mapping")
    parser.add_argument("--apply", action="store_true", help="Apply the version bump to the unit")
    parser.add_argument("--bump-type", choices=["auto", "patch", "minor", "major"], default="auto")
    parser.add_argument("--force", action="store_true", help="Bump even when nothing changed (manual dispatch)")
    args = parser.parse_args()

    units = load_units()

    if args.verify:
        for name, unit in units.items():
            extra = " (+ m3tal-plugin-mesh-cluster metapackage)" if unit["cluster"] else ""
            print(f"{name}: {', '.join(unit['plugins'])}{extra}")
        return

    if args.list_all:
        print(json.dumps({"unit": sorted(units)}))
        return

    if args.list_unit:
        if args.list_unit not in units:
            fail(f"unknown release unit '{args.list_unit}' (known: {', '.join(sorted(units))})")
        print(json.dumps({"unit": [args.list_unit]}))
        return

    if args.list:
        touched = touched_plugins(args.base)
        names = sorted(name for name, unit in units.items() if unit_is_touched(unit, touched))
        print(json.dumps({"unit": names}))
        return

    if not args.unit:
        fail("specify --unit (or --list, --list-all, --list-unit, --verify)")
    if args.unit not in units:
        fail(f"unknown release unit '{args.unit}' (known: {', '.join(sorted(units))})")
    unit = units[args.unit]

    if args.plugins:
        print(" ".join(unit["plugins"]))
        return

    if args.cluster:
        print("true" if unit["cluster"] else "false")
        return

    if not args.apply:
        for plugin in unit["plugins"]:
            print(f"{plugin}: {smart.get_current_version(plugin)}")
        return

    bump_unit(unit, args.base, args.bump_type, args.force)
    for plugin in unit["plugins"]:
        print(f"{plugin}: {smart.get_current_version(plugin)}")


if __name__ == "__main__":
    main()
