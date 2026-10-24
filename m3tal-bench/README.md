# M3TAL System Benchmarks Plugin (`m3tal-bench`)

[![Status: Planned](https://img.shields.io/badge/Status-Planned-orange.svg)](https://m3tal.io)
[![Process Identity: m3tal-bench](https://img.shields.io/badge/Process-m3tal--bench-brightgreen.svg)](https://m3tal.io)
[![Package: m3tal-plugin-bench](https://img.shields.io/badge/Package-m3tal--plugin--bench-blue.svg)](https://m3tal.io)

The **M3TAL System Benchmarks Plugin** (`m3tal-bench`) provides one-click hardware performance benchmarking for multi-threaded CPU computational limits, GPU compute matrices, disk I/O throughput (sequential and 4K random read/write), and network loopback saturation.

---

## 1. Architectural Overview & Identity

- **Canonical Process Identity**: `m3tal-bench`
- **Package Name**: `m3tal-plugin-bench`
- **Dashboard UI Route**: `/diagnostics/benchmark`
- **Capabilities**: `benchmark_runner`, `ui_view`

---

## 2. Planned Features & Scope

- **CPU Multi-Core Tests**: Single-thread and multi-thread integer and floating point stress tests with thermal throttle detection.
- **Disk I/O Benchmarking**: High-fidelity read/write bandwidth and IOPS benchmarks via `fio` emulation (sequential read/write and random 4K Q1T1 / Q32T1).
- **GPU Compute & Matrix Multiplies**: Tensor operations and FP16/FP32/INT8 compute evaluation for available GPUs.
- **Historic Benchmark Ledger**: Store and compare benchmark runs across hardware upgrades, kernel versions, and storage configurations.
- **Safe Auto-Throttling**: Automatically terminates tests if CPU or GPU junction temperatures cross safe operational thresholds.

---

## 3. IPC Methods Roadmap

| Method | Description |
|---|---|
| `m3tal.init` | Handshake detecting available benchmark tools (`sysbench`, `fio`) and hardware specs. |
| `m3tal.collect` | Collects historic benchmark results, currently executing test stage, and progress %. |
| `m3tal.action` | Dispatches `start_benchmark`, `stop_benchmark`, and `export_results`. |
| `m3tal.shutdown` | Safely aborts running benchmarks and restores system to idle. |

---

## 4. Debian Installation (When Released)

```bash
sudo apt update
sudo apt install -y m3tal-plugin-bench
```
