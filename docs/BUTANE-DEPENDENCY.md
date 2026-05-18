# Butane CLI Dependency

## Finding

The `butane` CLI is **NOT** included in Flatcar Container Linux's base image.

Verified against the stable channel package list:

```
curl -sL 'https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_image_packages.txt' | grep -i butane
# (no output — butane not present)
```

## Implications for knuckle

Since knuckle generates Butane YAML and compiles it to Ignition JSON, butane
cannot be assumed to exist on the target system. Two options:

1. **Bundle butane as a Go library** (current approach) — Use
   `github.com/coreos/butane` as a library dependency. This is what
   `internal/ignition` does: it imports butane's Go API to transpile
   Butane configs to Ignition JSON at build time, with zero runtime
   dependency on the `butane` CLI binary.

2. **Ship butane binary alongside knuckle** — Not recommended. Adds
   distribution complexity and version coupling.

## Conclusion

The current architecture is correct: knuckle uses butane as a **Go library
dependency**, not as a runtime CLI tool. No butane binary needs to be
present on the Flatcar system at install time.
