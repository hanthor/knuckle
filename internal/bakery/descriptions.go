package bakery

// Support tier constants for Flatcar Bakery extensions.
// These describe the integration and maintenance level, not a quality certification.
const (
	// TierIntegrated marks extensions directly used or tested by the Flatcar team
	// (e.g. Flatcar ClusterAPI nodes, built-in component overrides).
	TierIntegrated = "Flatcar Integrated"

	// TierMaintained marks extensions in bakery CI with regular automated releases
	// and full documentation at extensions.flatcar.org.
	TierMaintained = "Bakery Maintained"

	// TierExperimental marks emerging or niche extensions that may have build
	// irregularities, manual steps, or documented deployment caveats.
	TierExperimental = "Experimental"
)

// ExtensionMeta holds curated display metadata for a Flatcar Bakery extension.
// All data is static and compiled into the binary — no network call is needed.
type ExtensionMeta struct {
	// Category is the functional group (e.g. "Container Runtime", "Networking").
	Category string

	// SupportTier is one of the Tier* constants above.
	SupportTier string

	// Short is a ~80-character one-line description shown in the list row.
	Short string

	// Long is a 3–5 sentence description shown in the detail panel.
	Long string

	// Caveats are ⚠ warnings shown in the detail panel. Nil if none.
	// Sourced from official bakery docs at https://extensions.flatcar.org.
	Caveats []string
}

// extensionCatalog is the static curated catalog of all Flatcar Bakery extensions.
// Descriptions are sourced from the official bakery docs at https://extensions.flatcar.org.
// When the bakery adds new extensions not present here, Lookup returns ok=false and
// viewSysext groups them under "Other" with the raw GitHub release body as description.
var extensionCatalog = map[string]ExtensionMeta{
	"bird": {
		Category:    "Networking",
		SupportTier: TierMaintained,
		Short:       "BIRD Internet Routing Daemon — BGP, OSPF, RIP, BFD for Flatcar nodes",
		Long:        "BIRD is a full-featured Internet routing daemon supporting BGP, OSPF, RIP, and BFD protocols. Ships the bird daemon and birdc CLI tool. No default configuration is provided — you must supply /etc/bird/ config files via Butane. Suitable for bare-metal nodes that participate in datacenter routing.",
		Caveats:     []string{"No default configuration included — /etc/bird/ config files must be provided via Butane before starting."},
	},
	"btop": {
		Category:    "Utilities",
		SupportTier: TierMaintained,
		Short:       "btop — interactive resource monitor with CPU, memory, disk, and network stats",
		Long:        "btop is a terminal-based resource monitor displaying CPU usage, memory, disk I/O, and network statistics in an interactive TUI. Useful for diagnosing performance issues on a running Flatcar node. Not a server daemon — connect to the node via SSH to use it.",
		Caveats:     nil,
	},
	"chrony": {
		Category:    "System",
		SupportTier: TierMaintained,
		Short:       "Chrony NTP client/server — accurate time synchronization for Flatcar",
		Long:        "Chrony implements the Network Time Protocol for precise clock synchronization. Ships a default configuration using the Flatcar NTP pool (0–3.flatcar.pool.ntp.org). Includes systemd-sysupdate support for automated version updates. The default config can be replaced via Butane.",
		Caveats:     nil,
	},
	"cilium": {
		Category:    "Networking",
		SupportTier: TierIntegrated,
		Short:       "Cilium CLI — eBPF-based Kubernetes networking, security, and observability",
		Long:        "Cilium provides eBPF-powered networking, security policy enforcement, and observability for Kubernetes clusters. Ships the cilium CLI and a service unit that runs cilium at boot. Additional installation flags can be passed via the CILIUM_INSTALL_ARGS environment variable. Pair with the kubernetes sysext for a complete cluster setup.",
		Caveats:     nil,
	},
	"consul": {
		Category:    "Orchestration",
		SupportTier: TierMaintained,
		Short:       "HashiCorp Consul — service mesh, discovery, health checking, and KV store",
		Long:        "Consul provides service discovery, distributed health checking, KV storage, and service mesh capabilities. Ships the consul binary and a service unit configured as a server by default. The default configuration can be modified or replaced via your Butane config.",
		Caveats:     nil,
	},
	"containerd": {
		Category:    "Container Runtime",
		SupportTier: TierIntegrated,
		Short:       "containerd — replace or upgrade Flatcar's built-in container runtime",
		Long:        "Ships a custom containerd build that overrides the containerd included in Flatcar's OS image. Use this to upgrade, downgrade, or test a specific containerd version independently of the OS. Includes a service unit and a basic containerd.toml configuration. The built-in Flatcar containerd sysext is automatically masked on install.",
		Caveats:     nil,
	},
	"crio": {
		Category:    "Container Runtime",
		SupportTier: TierIntegrated,
		Short:       "CRI-O — lightweight OCI container runtime purpose-built for Kubernetes",
		Long:        "CRI-O is an OCI-compliant container runtime designed for Kubernetes. Pass --cri-socket=unix:///var/run/crio/crio.sock to kubeadm to use it as the cluster runtime. Ships a service unit and basic configuration. Pair with the kubernetes sysext for a complete Kubernetes node.",
		Caveats:     nil,
	},
	"docker": {
		Category:    "Container Runtime",
		SupportTier: TierIntegrated,
		Short:       "Docker engine — replace or upgrade Flatcar's built-in Docker",
		Long:        "Ships the full Docker engine (daemon, CLI, containerd, runc) from official upstream Docker static binaries. Overrides both Docker and containerd built into Flatcar's OS image — both are automatically masked on install. Docker starts via socket activation; containerd starts at boot.",
		Caveats:     nil,
	},
	"docker-buildx": {
		Category:    "Container Runtime",
		SupportTier: TierMaintained,
		Short:       "Docker Buildx — multi-platform image builder plugin for Docker",
		Long:        "Adds the docker buildx subcommand for building multi-platform container images using BuildKit. Ships only the buildx plugin binary — not a standalone daemon. Requires the docker sysext or Flatcar's built-in Docker.",
		Caveats:     []string{"Requires the docker sysext or Flatcar built-in Docker."},
	},
	"docker-compose": {
		Category:    "Container Runtime",
		SupportTier: TierMaintained,
		Short:       "Docker Compose — multi-container application definition and orchestration",
		Long:        "Adds the docker compose subcommand for defining and running multi-container applications from a Compose YAML file. Ships only the compose plugin binary. Requires the docker sysext or Flatcar's built-in Docker.",
		Caveats:     []string{"Requires the docker sysext or Flatcar built-in Docker."},
	},
	"falco": {
		Category:    "Security",
		SupportTier: TierMaintained,
		Short:       "Falco — cloud-native runtime security monitoring via eBPF",
		Long:        "Falco detects unexpected application behavior and security policy violations on Linux hosts and in containers using the modern eBPF probe. Ships Falco, default rules files, and a service unit. Custom rules can be added at /etc/falco/falco_rules.local.yaml or via the Falco artifact-follower plugin.",
		Caveats:     nil,
	},
	"ig": {
		Category:    "Security",
		SupportTier: TierMaintained,
		Short:       "Inspektor Gadget — eBPF-based cluster and host inspection toolkit",
		Long:        "Inspektor Gadget is a collection of eBPF-powered tools for data collection and system inspection on Kubernetes clusters and Linux hosts. Ships both the ig CLI (runs gadgets on hosts) and gadgetctl (manages gadgets in daemon mode). Useful for debugging, tracing, and performance analysis.",
		Caveats:     nil,
	},
	"k3s": {
		Category:    "Orchestration",
		SupportTier: TierIntegrated,
		Short:       "K3s — lightweight CNCF-certified Kubernetes for edge and resource-limited nodes",
		Long:        "K3s is a CNCF-certified Kubernetes distribution optimized for low-resource environments. Can run as a server (control plane) or agent (worker) — the active service unit is chosen at provisioning time via Butane. No default unit is pre-enabled. Well-suited for edge, IoT, and single-node deployments.",
		Caveats:     []string{"Sysupdate within same minor version only (e.g. v1.32.x → v1.32.y). Cross-minor updates (v1.31.x → v1.32.x) must be done manually."},
	},
	"keepalived": {
		Category:    "Networking",
		SupportTier: TierMaintained,
		Short:       "Keepalived — VRRP high-availability daemon with virtual IP failover",
		Long:        "Keepalived implements VRRP (Virtual Router Redundancy Protocol) for building high-availability clusters with automatic virtual IP failover. Ships a statically compiled keepalived binary. Provide your own /etc/keepalived/keepalived.conf configuration via Butane.",
		Caveats:     nil,
	},
	"kubernetes": {
		Category:    "Orchestration",
		SupportTier: TierIntegrated,
		Short:       "Kubernetes — production container orchestration, used by Flatcar ClusterAPI",
		Long:        "Ships kubelet, kubeadm, and kubectl for deploying and managing Kubernetes clusters on Flatcar. Used by the Flatcar project for ClusterAPI (CAPI) node provisioning. A full deployment guide is available at the Flatcar Kubernetes docs. Supports both control-plane and worker node configurations.",
		Caveats:     []string{"Sysupdate within same minor version only (e.g. v1.36.x → v1.36.y). Cross-minor updates (v1.35.x → v1.36.x) must be done manually."},
	},
	"llamaedge": {
		Category:    "AI / ML",
		SupportTier: TierExperimental,
		Short:       "LlamaEdge — LLM inference API server built on WasmEdge (requires wasmedge)",
		Long:        "LlamaEdge provides a lightweight LLM inference server running WebAssembly via WasmEdge. The llamaedge and wasmedge sysext versions must match exactly — llamaedge includes a WasmEdge plugin that is version-specific. Does not start at boot. Run with: wasmedge run /usr/lib/wasmedge/wasm/llama-api-server.wasm.",
		Caveats: []string{
			"Requires the wasmedge sysext with an exactly matching version — check bakery docs for the required wasmedge version.",
			"Not automatically built in bakery CI — releases may lag behind upstream.",
			"Does not start at boot — must be invoked manually after provisioning.",
		},
	},
	"nebula": {
		Category:    "Networking",
		SupportTier: TierMaintained,
		Short:       "Nebula — scalable encrypted overlay VPN mesh (originated at Slack)",
		Long:        "Nebula creates a secure overlay network between nodes using a custom tunneling protocol. Requires a Nebula CA, per-node certificates, and a /etc/nebula/config.yaml configuration — none are included in the sysext. Provide the config and key material via your Butane configuration.",
		Caveats:     []string{"Requires a Nebula CA and per-node certificate — not included. Supply /etc/nebula/config.yaml and key files via Butane."},
	},
	"nerdctl": {
		Category:    "Container Runtime",
		SupportTier: TierMaintained,
		Short:       "nerdctl — Docker-compatible CLI for containerd",
		Long:        "nerdctl provides a Docker-compatible command-line interface for containerd, including Compose support. Works with Flatcar's built-in containerd or the containerd sysext. CNI plugins are optional — without them, nerdctl operates in --net host mode only.",
		Caveats:     []string{"Requires containerd (Flatcar built-in or containerd sysext). Without CNI plugins, only --net host networking mode is available."},
	},
	"nomad": {
		Category:    "Orchestration",
		SupportTier: TierMaintained,
		Short:       "HashiCorp Nomad — flexible workload orchestrator for containers, VMs, and binaries",
		Long:        "Nomad is a workload orchestrator that schedules containers, VMs, and standalone binaries across a cluster. Ships the nomad binary and a service unit configured as a server by default. The default configuration can be modified or replaced via your Butane config.",
		Caveats:     nil,
	},
	"nvidia-runtime": {
		Category:    "GPU / Accelerators",
		SupportTier: TierMaintained,
		Short:       "NVIDIA Container Toolkit — GPU passthrough for containers. No kernel module or host CUDA.",
		Long:        "Ships nvidia-container-runtime, nvidia-ctk, and libnvidia-container — the NVIDIA Container Toolkit userspace stack. Enables GPU access inside Docker and containerd containers via CDI (Container Device Interface). CUDA libraries are provided by your container images, not installed on the host. The NVIDIA kernel module must be installed separately using the Flatcar NVIDIA customization guide.",
		Caveats: []string{
			"NVIDIA kernel module NOT included — install separately via the Flatcar NVIDIA customization guide (flatcar.org/docs).",
			"No CUDA on the host — CUDA libraries come from your container images, not this sysext.",
			"x86-64 only — no arm64 builds are available from the bakery.",
		},
	},
	"ollama": {
		Category:    "AI / ML",
		SupportTier: TierMaintained,
		Short:       "Ollama — run LLMs locally with an OpenAI-compatible REST API",
		Long:        "Ollama provides a local LLM server with an OpenAI-compatible REST API. Ships the ollama binary and a service unit that starts at boot. Model storage, configuration, and runtime library paths are configurable via HOME, OLLAMA_MODELS, and OLLAMA_RUNNERS_DIR environment variables.",
		Caveats:     []string{"The Ollama API is publicly accessible by default — set OLLAMA_HOST to restrict access before deploying to a network-accessible node."},
	},
	"opkssh": {
		Category:    "Security",
		SupportTier: TierMaintained,
		Short:       "opkssh — SSH via OpenID Connect identities instead of long-lived static keys",
		Long:        "opkssh enables SSH authentication using OIDC identities (e.g. alice@example.com) via the OpenPubKey protocol. It generates SSH public keys containing PK Tokens and configures sshd to verify them. Eliminates long-lived static SSH keys and ties SSH access to your identity provider's session lifecycle.",
		Caveats:     nil,
	},
	"rke2": {
		Category:    "Orchestration",
		SupportTier: TierIntegrated,
		Short:       "RKE2 — Rancher's security-hardened Kubernetes distribution",
		Long:        "RKE2 is Rancher's next-generation Kubernetes distribution with a focus on security and compliance. Ships service units for both server (control plane) and agent (worker) roles — no service is pre-enabled; select the correct one via Butane. Supports automated patch-level updates via systemd-sysupdate.",
		Caveats:     []string{"Sysupdate within same minor version only (e.g. v1.32.x → v1.32.y). Cross-minor updates (v1.31.x → v1.32.x) must be done manually."},
	},
	"tailscale": {
		Category:    "Networking",
		SupportTier: TierIntegrated,
		Short:       "Tailscale — zero-config WireGuard mesh VPN for secure node-to-node networking",
		Long:        "Tailscale creates an encrypted WireGuard mesh network between your nodes with no manual key exchange or firewall rules required. Ships tailscale, tailscaled, and a service unit that auto-starts at boot (v1.78.1+). Authenticate nodes with tailscale up after provisioning.",
		Caveats:     nil,
	},
	"tilde": {
		Category:    "Utilities",
		SupportTier: TierMaintained,
		Short:       "tilde — terminal text editor with familiar GUI keybindings (Ctrl+S, Ctrl+C)",
		Long:        "tilde is a terminal text editor designed to feel familiar to users of GUI editors. Uses standard keybindings: Ctrl+S to save, Ctrl+C to copy, Ctrl+X to cut, Ctrl+Z to undo. Useful for quick on-node config file edits without learning vi or nano.",
		Caveats:     nil,
	},
	"wasmcloud": {
		Category:    "WebAssembly",
		SupportTier: TierExperimental,
		Short:       "wasmCloud — distributed WebAssembly application platform with NATS messaging",
		Long:        "wasmCloud is a platform for running WebAssembly components at scale across distributed infrastructure. Ships the wasmcloud host and a bundled NATS server, with service units that start both at boot. Supports custom NATS leaf-node config and environment overrides for lattice and credential management.",
		Caveats:     nil,
	},
	"wasmedge": {
		Category:    "WebAssembly",
		SupportTier: TierExperimental,
		Short:       "WasmEdge — lightweight WebAssembly runtime for cloud-native and edge workloads",
		Long:        "WasmEdge is a lightweight, high-performance WebAssembly runtime supporting WASI and WasmEdge extensions. Does not ship a default service unit — add a custom unit via Butane to run Wasm workloads at boot. Required as a dependency by the llamaedge sysext.",
		Caveats:     []string{"No default service unit — add a custom systemd unit via Butane to run Wasm workloads at boot."},
	},
	"wasmtime": {
		Category:    "WebAssembly",
		SupportTier: TierExperimental,
		Short:       "Wasmtime — fast, secure WebAssembly runtime from the Bytecode Alliance",
		Long:        "Wasmtime is a standalone WebAssembly runtime from the Bytecode Alliance, focused on standards compliance, security sandbox isolation, and performance. Supports WASI and the Component Model. Does not ship a default service unit — add custom units via Butane to run Wasm workloads at boot.",
		Caveats:     []string{"No default service unit — add a custom systemd unit via Butane to run Wasm workloads at boot."},
	},
}

// Lookup returns curated metadata for the named extension.
// Returns the metadata and true if found; zero ExtensionMeta and false if not in catalog.
// Unknown extensions should be displayed with Category "Other" and the raw bakery description.
func Lookup(name string) (ExtensionMeta, bool) {
	meta, ok := extensionCatalog[name]
	return meta, ok
}

// CaveatsFor returns curated caveats for the named extension.
// Returns nil if there are no caveats or the extension is not in the catalog.
func CaveatsFor(name string) []string {
	if meta, ok := extensionCatalog[name]; ok {
		return meta.Caveats
	}
	return nil
}
