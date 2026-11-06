# m3tal-ai Documentation

`m3tal-ai` is an in-progress assistant/RAG prototype, not yet a complete installable Core plugin.

## Current state

- Go assistant/RAG code exists under this directory.
- No `plugin.yaml`, plugin Makefile, UI bundle decision, config schema, changelog release history, or release workflow yet.
- Intended role: deterministic M3tal assistant with optional model-backed proposals.
- Model relationship: `m3tal-llm` is research/serving; `m3tal-ai` is the assistant/plugin-side consumer.

## Before plugin release

Add `plugin.yaml`, canonical build target, `config/config.schema.json`, `CHANGELOG.md`, lifecycle tests, and `.github/workflows/Build-m3tal-ai-Deb.yml`.

See the plugin [`README.md`](../README.md) and repository [`docs/PLUGIN_AUTHORING.md`](../../docs/PLUGIN_AUTHORING.md).
