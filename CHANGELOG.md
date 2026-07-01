# Changelog

## v0.7.0 - 2026-07-02

### Added

- Added local stdio MCP support for typed infra-lab tools.
- Added JSON contract output for `ilab --json` read-only commands.
- Added MCP operation lifecycle tools: approve, cancel, status, logs, lock list, and stale lock unlock.
- Added approved operation flows for addon install, env up, env down, env clean, rebuild, and addon uninstall.
- Added profile write tools and audit/operation records.
- Added snapshot, health summary, plan-only, plan store, and fingerprint support.
- Added remote live validation notes for MCP operation workflows.

### Changed

- `make build` and `make install` now inject `VERSION`, git commit, and build date into `ilab version`.
- MCP addon install now routes `metrics-server` through the base addon path and keeps other addons on the optional path.
- MCP env clean now targets the requested env explicitly.

### Validated

- `make test-contract`
- `make test-mcp`
- Remote MCP read-only smoke against the Tailscale-connected lab host.
- Remote addon install prepare/approve/commit repeated 3 times against `test-wizard-env`.
- Remote `mcp-live-multipass` env up/down/clean prepare/approve/commit success path.

