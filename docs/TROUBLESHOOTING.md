# M3TAL Plugin Troubleshooting

Use this guide when a plugin is missing, fails to start, collects no data, or cannot be installed from APT.

## Quick checks

```bash
systemctl status m3tal-api --no-pager
journalctl -u m3tal-api -n 200 --no-pager
curl -fsS http://127.0.0.1:5050/api/v2/version
curl -fsS http://127.0.0.1:5050/api/v2/plugins
```

If the API requires auth, use an API token:

```bash
curl -fsS -H "X-API-Token: $M3TAL_API_TOKEN" http://127.0.0.1:5050/api/v2/plugins
```

## Plugin not discovered

Check that the plugin directory contains `plugin.yaml` and is under a search path Core scans, such as:

- `/usr/lib/m3tal/plugins/<plugin>/`
- `/usr/local/lib/m3tal/plugins/<plugin>/`
- `~/.local/lib/m3tal/plugins/<plugin>/`
- `~/.config/m3tal/plugins/<plugin>/`

After installing a `.deb`, reload Core:

```bash
sudo systemctl reload-or-restart m3tal-api
```

Then verify:

```bash
curl -fsS http://127.0.0.1:5050/api/v2/plugins | python3 -m json.tool
```

## Plugin installed but state is stopped/error

1. Confirm the manifest `spec.command` path exists inside the plugin directory.
2. Confirm the binary is executable.
3. Check for manifest errors in `journalctl -u m3tal-api`.
4. Confirm required binaries exist (`docker`, `smbd`, `testparm`, etc.).
5. Confirm `spec.process_name` is canonical (`m3tal-<name>`).
6. For root-requiring plugins, verify the package/service permissions are correct.

## `m3tal.init` fails

- Ensure the plugin reads newline-delimited JSON-RPC requests from stdin.
- Ensure responses are written to stdout, one JSON object per line.
- Send all diagnostics to stderr.
- Return `name`, `version`, and `capabilities` in the init result.
- Do not emit non-JSON lines on stdout.

## `m3tal.collect` returns no metrics/tables

- Confirm the plugin declared `collector` in `plugin.yaml`.
- Confirm collect returns `status`, `metrics`, and `tables` in the documented shape.
- Check plugin stderr/logs for host dependency failures such as Docker socket access or missing Samba/NFS tools.
- Call the endpoint directly:

```bash
curl -fsS http://127.0.0.1:5050/api/v2/plugins/<name>/collect | python3 -m json.tool
```

## `m3tal.action` fails

- Confirm the action is declared during `m3tal.init` or in static manifest/tab data.
- Confirm the request uses `action_id`, not `action`.
- Confirm optional `target` and `args` match the plugin's expected schema.
- Check the plugin's returned `success`, `message`, `error`, and `payload` fields.

## APT package not found

```bash
apt-cache policy m3tal-plugin-<name>
apt-cache madison m3tal-plugin-<name>
```

If the package is absent:

1. Confirm `m3tal-apt-key` has indexed the package.
2. Run `sudo apt update`.
3. Confirm the APT source points to `https://jakej985-rgb.github.io/m3tal-apt-key`.
4. Check whether the plugin is still `developing` or `planned` in `PLUGINS.md`; planned plugins are not published.

## Mesh node not appearing

- Confirm `m3tal-plugin-mesh-node` is installed and running.
- Confirm TCP port `9321` (or configured port) is reachable.
- Confirm the pairing token/password matches on sender and reader.
- Confirm mDNS is allowed on the LAN if using discovery.
- Check mesh logs for authentication or telemetry endpoint errors.

## Plugin UI missing

- Confirm `spec.ui.entry` points to a file inside `ui/dist`.
- Confirm the `.deb` installed files under `/usr/lib/m3tal/plugins/<name>/`.
- Check `/api/v2/plugins/tabs` for the route/tab registration.
- Hard-refresh the dashboard or open `/plugins/<metadata.name>/` directly.

## Packaging problems

- Run `make bump-check`, `make build PLUGIN=<name>`, `make test PLUGIN=<name>`, and `make audit PLUGIN=<name>`.
- Confirm `plugin.yaml` version was bumped before packaging.
- Confirm package name follows `m3tal-plugin-<name>` unless a documented exception exists.
- Confirm release workflow dispatched `publish-deb` to `m3tal-apt-key` successfully.
