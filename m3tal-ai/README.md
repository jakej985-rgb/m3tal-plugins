# M3TAL Local AI & LLM Engine Plugin (`m3tal-ai`)

[![Status: Planned](https://img.shields.io/badge/Status-Planned-orange.svg)](https://m3tal.io)
[![Process Identity: m3tal-ai](https://img.shields.io/badge/Process-m3tal--ai-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-ai](https://img.shields.io/badge/Package-m3tal--plugin--ai-blue.svg)](https://m3tal.io)

The **M3TAL Local AI & LLM Engine Plugin** (`m3tal-ai`) provides unified local large language model inference orchestration (Ollama, vLLM, llama.cpp), GPU VRAM allocation telemetry, asynchronous job queuing, and a built-in model hub for self-hosted artificial intelligence workloads.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-ai`
- **Package Name**: `m3tal-plugin-ai`
- **Dashboard UI Route**: `/ai`
- **Capabilities**: `inference_runner`, `job_queue`, `ui_view`

---

## 2. Planned Features & Scope

- **Inference Runtime Adapter**: Native orchestration of Ollama, vLLM, and llama.cpp backends.
- **Model Catalog & Pull Manager**: One-click download, quantization selection, and lifecycle management for open weights (Llama, Gemma, Mistral, Qwen, DeepSeek).
- **GPU & VRAM Telemetry**: Live NVIDIA / AMD GPU utilization, VRAM saturation curves, temperature, and power consumption stats.
- **Unified OpenAI-Compatible Proxy**: Exposes standard `/v1/chat/completions` and `/v1/embeddings` endpoints authenticated with M3TAL API tokens.
- **Interactive Playground**: Built-in chat interface with system prompt configuration and token throughput benchmarks (tokens/sec).

---

## 3. IPC Methods Roadmap

| Method | Description |
|---|---|
| `m3tal.init` | Handshake announcing AI runtime capabilities and GPU accelerators. |
| `m3tal.collect` | Collects loaded models, VRAM usage, queue depth, and inference throughput. |
| `m3tal.action` | Dispatches `pull_model`, `unload_model`, `delete_model`, and `benchmark_inference`. |
| `m3tal.shutdown` | Safely flushes active inference queries and releases allocated VRAM. |

---

## 4. Debian Installation (When Released)

```bash
sudo apt update
sudo apt install -y m3tal-plugin-ai
```
