# Deploying a Hive Container on Flatcar

This guide covers the host-side prerequisites for running a knuckle/hive
container on Flatcar Container Linux.

## TL;DR — add swap before starting agents

Flatcar Container Linux ships with **no swap configured**. Without swap, the
kernel OOM killer fires the moment multiple agent processes try to start in
parallel — `claude-code` is ~236 MB resident per agent and a 4 GiB VM with no
swap goes red within seconds.

### One-time setup

Run these once on the Flatcar host *before* you start the hive container:

```bash
sudo fallocate -l 512M /var/swapfile
sudo chmod 600 /var/swapfile
sudo mkswap /var/swapfile
sudo swapon /var/swapfile
echo '/var/swapfile swap swap defaults 0 0' | sudo tee -a /etc/fstab
```

Verify:

```bash
swapon --show
free -h
```

### Sizing

| Host RAM | Swap |
|----------|------|
| ≤ 4 GiB  | 512 MiB (matches the certus reference deployment that runs without OOM kills on 4.83) |
| 4–16 GiB | 1–2 GiB |
| > 16 GiB | 4 GiB |

512 MiB is the minimum that has been observed to keep concurrent agent startup
stable. Going larger does no harm.

## Why not just provision it via knuckle?

If you install Flatcar **with knuckle ≥ v0.7.0**, swap is enabled by default
([#95](https://github.com/projectbluefin/knuckle/issues/95)) — you don't need
the manual steps above on fresh installs. The instructions above exist for:

- Hosts installed with knuckle ≤ v0.6.x.
- Hosts where someone passed `--swap=false` (or `{"swap":{"enabled":false}}`
  via headless config) and now wants to add swap back.
- Hosts installed via stock `flatcar-install` / cloud images, which never
  provision swap.

## Context

- **4.83 (certus, working):** 4 GiB RAM + 512 MiB swap — concurrent agents
  start cleanly.
- **4.85 (knuckle ≤ v0.6.x, broken):** 3.8 GiB RAM + no swap → agents killed
  (exit 137) at startup.
- **4.85 (knuckle ≥ v0.7.0):** 3.8 GiB RAM + 4 GiB swap (default) → fine.
