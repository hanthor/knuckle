# Sysext Catalog Reference

knuckle presents all available [Flatcar Bakery](https://github.com/flatcar/sysext-bakery) system extensions
during installation. This document is the reference for every extension — what it does, its support tier,
and any caveats you need to know before selecting it.

Full usage guides and Butane config snippets are at **[extensions.flatcar.org](https://extensions.flatcar.org)**.

---

## Support Tiers

| Tier | Meaning |
|---|---|
| **Flatcar Integrated** | Directly used or tested by the Flatcar team (e.g. ClusterAPI nodes, built-in component overrides). Full docs and automated updates. |
| **Bakery Maintained** | In bakery CI with regular automated releases and documentation. Community-maintained, production-ready. |
| **Experimental** | Emerging technology or extensions with documented build/deployment caveats. Evaluate carefully before production use. |

> Tiers describe integration level and maintenance status — not a hardware certification or
> quality guarantee. All extensions are open source and community-auditable.

---

## Container Runtime

| Extension | Version Source | Tier | Description |
|---|---|---|---|
| **containerd** | [GitHub](https://github.com/containerd/containerd) | Flatcar Integrated | Replace or upgrade Flatcar's built-in container runtime. Masks the built-in containerd sysext on install. |
| **crio** | [GitHub](https://github.com/cri-o/cri-o) | Flatcar Integrated | OCI container runtime purpose-built for Kubernetes. Use with `--cri-socket=unix:///var/run/crio/crio.sock`. |
| **docker** | [docker.com](https://docker.com) | Flatcar Integrated | Full Docker engine (daemon, CLI, containerd, runc). Overrides and masks Flatcar's built-in Docker and containerd. |
| **docker-buildx** | [GitHub](https://github.com/docker/buildx) | Bakery Maintained | Multi-platform image builder plugin. Requires docker sysext or built-in Docker. ⚠ |
| **docker-compose** | [GitHub](https://github.com/docker/compose) | Bakery Maintained | Multi-container Compose plugin. Requires docker sysext or built-in Docker. ⚠ |
| **nerdctl** | [GitHub](https://github.com/containerd/nerdctl) | Bakery Maintained | Docker-compatible CLI for containerd. Without CNI plugins, only `--net host` mode is available. ⚠ |

### Caveats — Container Runtime

- **docker-buildx / docker-compose**: require the `docker` sysext or Flatcar's built-in Docker.
- **nerdctl**: requires `containerd` (built-in Flatcar or `containerd` sysext). Without CNI plugins built in, networking is `--net host` only.

---

## Orchestration

| Extension | Version Source | Tier | Description |
|---|---|---|---|
| **kubernetes** | [kubernetes.io](https://kubernetes.io) | Flatcar Integrated | Ships kubelet, kubeadm, kubectl. Used by Flatcar ClusterAPI (CAPI). Full guide at Flatcar Kubernetes docs. ⚠ |
| **cilium** | [cilium.io](https://cilium.io) | Flatcar Integrated | eBPF-powered Kubernetes networking, security, and observability. Ships the cilium CLI + service unit. |
| **k3s** | [k3s.io](https://k3s.io) | Flatcar Integrated | Lightweight CNCF-certified Kubernetes for edge and resource-limited nodes. Server or agent role selected via Butane. ⚠ |
| **rke2** | [rke2.io](https://docs.rke2.io) | Flatcar Integrated | Rancher's security-hardened Kubernetes distribution. Server/agent unit selected via Butane. ⚠ |
| **consul** | [hashicorp.com](https://github.com/hashicorp/consul) | Bakery Maintained | Service mesh, discovery, health checking, and KV store. Configured as server by default. |
| **nomad** | [hashicorp.com](https://github.com/hashicorp/nomad) | Bakery Maintained | Workload orchestrator for containers, VMs, and binaries. Configured as server by default. |

### Caveats — Orchestration

- **kubernetes / k3s / rke2**: `systemd-sysupdate` is supported **within the same minor version only**
  (e.g. v1.36.x → v1.36.y). Cross-minor updates (v1.35.x → v1.36.x) must be done manually.

---

## Networking

| Extension | Version Source | Tier | Description |
|---|---|---|---|
| **tailscale** | [tailscale.com](https://tailscale.com) | Flatcar Integrated | Zero-config WireGuard mesh VPN. Auto-starts at boot (v1.78.1+). Run `tailscale up` to authenticate. |
| **bird** | [bird.network.cz](https://bird.network.cz) | Bakery Maintained | BGP, OSPF, RIP, BFD routing daemon. No default config — supply `/etc/bird/` via Butane. ⚠ |
| **keepalived** | [keepalived.org](https://keepalived.org) | Bakery Maintained | VRRP high-availability daemon with virtual IP failover. Provide `/etc/keepalived/keepalived.conf` via Butane. |
| **nebula** | [GitHub](https://github.com/slackhq/nebula) | Bakery Maintained | Encrypted overlay VPN mesh (originated at Slack). Requires a Nebula CA and per-node certificates. ⚠ |

### Caveats — Networking

- **bird**: no default configuration — you must supply `/etc/bird/` config files via Butane before the service will start.
- **nebula**: requires a Nebula CA, per-node certificates, and `/etc/nebula/config.yaml` — none included. Supply via Butane.

---

## Security

| Extension | Version Source | Tier | Description |
|---|---|---|---|
| **falco** | [falco.org](https://falco.org) | Bakery Maintained | Runtime security monitoring via modern eBPF probe. Default rules included. Custom rules at `/etc/falco/falco_rules.local.yaml`. |
| **ig** | [inspektor-gadget.io](https://www.inspektor-gadget.io) | Bakery Maintained | Inspektor Gadget: eBPF tools for cluster and host inspection. Ships `ig` CLI and `gadgetctl`. |
| **opkssh** | [GitHub](https://github.com/openpubkey/opkssh) | Bakery Maintained | SSH via OIDC identities (e.g. `alice@example.com`) using the OpenPubKey protocol. Replaces long-lived SSH keys. |

---

## GPU / Accelerators

| Extension | Version Source | Tier | Description |
|---|---|---|---|
| **nvidia-runtime** | [GitHub](https://github.com/NVIDIA/nvidia-container-toolkit) | Bakery Maintained | NVIDIA Container Toolkit — GPU passthrough for Docker/containerd containers. ⚠⚠⚠ |

### Caveats — nvidia-runtime (read carefully)

1. **Kernel module NOT included.** The NVIDIA kernel driver must be installed separately using the
   [Flatcar NVIDIA customization guide](https://www.flatcar.org/docs/latest/setup/customization/using-nvidia/).
   Without the kernel module, GPU passthrough will not work.

2. **No CUDA on the host.** This sysext installs `nvidia-container-runtime`, `nvidia-ctk`, and
   `libnvidia-container` — the userspace toolkit that bridges the container runtime to the kernel driver.
   CUDA libraries (`libcuda.so`, `libcublas.so`, etc.) are **not** installed on the host. They come from
   your container images (e.g. `nvidia/cuda:12.x`).

3. **x86-64 only.** No arm64 builds are available from the Flatcar Bakery. Selecting this extension on
   an arm64 installation will have no effect.

---

## AI / ML

| Extension | Version Source | Tier | Description |
|---|---|---|---|
| **ollama** | [ollama.com](https://ollama.com) | Bakery Maintained | Local LLM server with OpenAI-compatible REST API. Starts at boot. ⚠ |
| **llamaedge** | [GitHub](https://github.com/LlamaEdge/LlamaEdge) | Experimental | LLM inference API server built on WasmEdge. Requires matching wasmedge sysext version. ⚠⚠ |

### Caveats — AI / ML

- **ollama**: the Ollama API is **publicly accessible by default**. Set `OLLAMA_HOST` to restrict access
  before deploying to any network-accessible node.
- **llamaedge**:
  - Requires the `wasmedge` sysext with an **exactly matching version** — the plugin is version-specific.
    Check the [bakery docs](https://extensions.flatcar.org/extensions/llamaedge.html) for the required wasmedge version.
  - Not automatically built in bakery CI — releases may lag behind upstream.
  - Does not start at boot — invoke with `wasmedge run /usr/lib/wasmedge/wasm/llama-api-server.wasm`.

---

## System

| Extension | Version Source | Tier | Description |
|---|---|---|---|
| **chrony** | [chrony-project.org](https://chrony-project.org) | Bakery Maintained | NTP client/server for precise clock synchronization. Default config uses the Flatcar NTP pool. |

---

## WebAssembly

| Extension | Version Source | Tier | Description |
|---|---|---|---|
| **wasmtime** | [wasmtime.dev](https://wasmtime.dev) | Experimental | Fast, secure WebAssembly runtime from the Bytecode Alliance. No default service unit — add via Butane. ⚠ |
| **wasmedge** | [wasmedge.org](https://wasmedge.org) | Experimental | Lightweight WebAssembly runtime (WASI + extensions). Required by llamaedge. No default service unit. ⚠ |
| **wasmcloud** | [wasmcloud.com](https://wasmcloud.com) | Experimental | Distributed WebAssembly platform with bundled NATS. Both services start at boot. |

### Caveats — WebAssembly

- **wasmtime / wasmedge**: no default service unit is included. Add a custom `systemd` unit via Butane to
  run Wasm workloads at boot.

---

## Utilities

| Extension | Version Source | Tier | Description |
|---|---|---|---|
| **btop** | [GitHub](https://github.com/aristocratos/btop) | Bakery Maintained | Interactive resource monitor (CPU, memory, disk, network). SSH into the node to use it. |
| **tilde** | [GitHub](https://github.com/jkbr/tilde) | Bakery Maintained | Terminal text editor with familiar GUI keybindings (Ctrl+S, Ctrl+C, Ctrl+Z). |

---

## Updating This Document

This file is maintained alongside `internal/bakery/descriptions.go`. When the Flatcar Bakery adds a
new extension:

1. Add an entry to `extensionCatalog` in `descriptions.go`.
2. Add the extension name to `allKnownExtensions` in `descriptions_test.go`.
3. Add a row to the appropriate table in this file.
4. Run `just ci` to confirm all gates pass.
