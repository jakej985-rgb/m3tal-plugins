# M3TAL Local AI Assistant Prototype (`m3tal-ai`)

[![Status: Developing](https://img.shields.io/badge/Status-Developing-yellow.svg)](https://m3tal.io)
[![Plugin Ready: No](https://img.shields.io/badge/Plugin%20Ready-No-orange.svg)](https://m3tal.io)

`m3tal-ai` is an in-progress M3TAL assistant/RAG prototype. It is **not yet a complete installable M3tal Core plugin** and must not be advertised as `m3tal-plugin-ai` until it has a valid `plugin.yaml`, plugin build/release path, and lifecycle packaging.

## Current implementation

The current code is primarily a Go assistant/RAG command-line prototype:

- Answers questions from local M3tal documentation.
- Can query M3tal Core read-only endpoints when configured.
- Proposes Core actions through deterministic, allowlisted, individually confirmed flows.
- Uses an optional model `Generator` interface for natural-language assistance.
- Keeps safety boundaries: model output is never trusted as an executable command by itself.

## Relationship to `m3tal-llm`

- `m3tal-llm` is the experimental model/training/serving research repository.
- `m3tal-ai` is the assistant/plugin-side integration point.
- A model may help draft responses or proposals, but Core mutations remain deterministic, allowlisted, and confirmed by the operator.

## Missing before plugin release

- `plugin.yaml` manifest
- canonical plugin Makefile/build target
- UI bundle or explicit no-UI manifest decision
- `config/config.schema.json`
- `CHANGELOG.md` release history
- release workflow and `publish-deb` integration
- lifecycle tests for `m3tal.init`, `m3tal.collect`, and `m3tal.action`

## Development

```bash
# From the m3tal-ai directory or repository root, use the Go commands documented
# by the current Makefile/README once a plugin build target exists.
go test ./...
```

Until those pieces exist, treat this directory as assistant research code inside the plugins monorepo, not as a published plugin.
