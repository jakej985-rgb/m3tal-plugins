# M3TAL Plugin Release Runbook

This runbook describes how plugins in `m3tal-plugins` are versioned, built, packaged, published to GitHub Releases, and ingested into `m3tal-apt-key`.

## Current release topology

Release automation follows one workflow per releasable plugin, named `Build-<plugin_name>-Deb.yml`:

- `m3tal-docker` → `Build-m3tal-docker-Deb.yml`
- `m3tal-compose` → `Build-m3tal-compose-Deb.yml`
- `m3tal-samba-nfs` → `Build-m3tal-samba-nfs-Deb.yml`
- `m3tal-mesh` → `Build-m3tal-mesh-Deb.yml` — deliberate exception: this single workflow builds `m3tal-mesh`, `m3tal-mesh-node`, and the `m3tal-plugin-mesh-cluster` metapackage together so cluster versions stay aligned.
- Planned plugins (`m3tal-ai`, `m3tal-stacks`, `m3tal-smart`, `m3tal-traefik`, `m3tal-vpn`, `m3tal-bench`) get a `Build-<plugin_name>-Deb.yml` workflow when they receive a `plugin.yaml` and build target.

Every plugin directory also has its own `docs/` folder with a plugin-specific `README.md`.

Do not claim a plugin is released just because code exists. A plugin is `available` only when its Debian package is indexed by `m3tal-apt-key`.

## Versioning

- Plugin versions live in each `plugin.yaml` under `metadata.version`.
- Use semantic versioning without a `v` prefix.
- `scripts/smart-bump.py` infers patch/minor/major bumps from commit messages and touched files.
- Manual releases can use `workflow_dispatch` with `force_bump` set to `auto`, `patch`, `minor`, or `major`.

Useful local commands:

```bash
make bump-check
make bump PLUGIN=m3tal-docker
make build PLUGIN=m3tal-docker
make test PLUGIN=m3tal-docker
make audit PLUGIN=m3tal-docker
make package PLUGIN=m3tal-docker
```

## Build and package

`scripts/build-plugin-deb.sh <plugin-dir>`:

1. Reads `plugin.yaml` for version and package name.
2. Builds the plugin via its Makefile when present.
3. Stages `plugin.yaml`, `bin/`, `ui/dist`, `config/`, docs, and changelog under `/usr/lib/m3tal/plugins/<dir>`.
4. Generates a Debian package named from `metadata.package_name` or `m3tal-plugin-<suffix>`.

The mesh workflow additionally builds `m3tal-plugin-mesh-cluster`, a bundle package that installs the mesh peer components.

## Publish pipeline

For workflow-backed plugins:

1. Push to `main` touches the plugin directory, or manually dispatch the workflow.
2. Smart bump updates `plugin.yaml` and `CHANGELOG.md`.
3. Release commit is pushed back to `main`.
4. `.deb` files are built.
5. Assets are attached to the rolling GitHub Release tag `plugins-latest`.
6. The workflow dispatches `publish-deb` to `jakej985-rgb/m3tal-apt-key` with `package_id`, `version`, `deb_url`, and `allow_replace`.
7. `m3tal-apt-key` validates, signs, indexes, commits, and deploys the repository.

End users install with:

```bash
sudo apt update
sudo apt install -y m3tal-plugin-<name>
```

## Adding release automation to a new plugin

Copy the closest existing workflow and adjust:

- `paths` to the plugin directory.
- Go version and Node version.
- `smart-bump.py --plugin` loop.
- `build-plugin-deb.sh` call.
- `publish-deb` dispatch loop.

For plugins that need privileged host access, document `root: true` and review package/service permissions before publishing.

## Verification checklist

Before considering a release complete:

1. `plugin.yaml` version matches `CHANGELOG.md` and built package version.
2. `make test PLUGIN=<name>` passes.
3. `make audit PLUGIN=<name>` passes.
4. `.deb` installs on a clean Debian/Ubuntu system.
5. Core discovers the plugin after `sudo systemctl reload-or-restart m3tal-api` or `SIGHUP`.
6. `/api/v2/plugins` shows the plugin and `/api/v2/plugins/<name>/collect` returns data.
7. `m3tal-apt-key` validation reports the package in the live index.
8. `catalog.json` and `PLUGINS.md` are updated if status/version/package changed.

## Rollback

- Prefer releasing a higher patch version that fixes the issue.
- If a bad package must be withdrawn, coordinate with `m3tal-apt-key` retention/removal policy; do not manually edit the deployed Pages repository.
- Core plugin state and config remain under user config/state directories; package rollback does not automatically delete plugin data.
