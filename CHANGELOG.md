# Changelog

## v0.7.3 - 2026-07-02

### Added

- Added `Env.TerraformResourceCount()`; `ilab env list`/`ilab doctor` (text, `--json`, and MCP) now report a `stale` field / `STALE_ENV_EMPTY_STATE` finding for envs whose `state/` directory exists but whose terraform state has zero resources.
- Added a `DUPLICATE_NAME_PREFIX` finding to `ilab doctor` when multiple managed envs share the same `name_prefix`.

### Fixed

- Fixed `ilab env list`/`doctor` silently listing destroyed-but-uncleaned envs as live managed environments. (#24)
- Fixed `FindEnvForVM` resolving a VM name to the wrong env when multiple envs share a `name_prefix`; it now disambiguates using live terraform state and errors on genuine (multiple-live-match) ambiguity instead of guessing. (#25)

### Validated

- `make test-go`
- End-to-end on remote host: `ilab env list`/`doctor` correctly flag `remote-seoy-libvirt-flannel` as stale; `ilab vm version lab-master-0` still resolves correctly to `test-wizard-env` despite the shared `name_prefix=lab`.

## v0.7.2 - 2026-07-02

### Added

- Added `infra-lab-mcp --setup` menu option to auto-register the MCP server with Claude Code CLI (`claude mcp add`, scope: user), matching the existing Codex registration flow.

### Fixed

- Fixed `InstallCodexMCP`/`InstallClaudeMCP` reporting "already registered" without checking whether the registered command path/env matched the current binary; both now always remove and re-add so a rebuilt or moved binary's registration self-heals instead of going stale. (#21)
- Fixed the setup menu re-running `bootstrap()` (and respawning `ilab version`/`ilab capabilities`) on every menu selection; the setup check now runs once per menu session and is reused across choices. (#22)

### Validated

- `make test-mcp`
- End-to-end on remote host: fresh Claude Code registration, idempotent re-registration, and stale-registration self-heal for both Codex and Claude Code.

## v0.7.1 - 2026-07-02

### Added

- Added `infra-lab-mcp --setup` interactive setup menu.
- Added `infra-lab-mcp --doctor` setup readiness check.
- Added `infra_lab.setup_check` tool output details for binaries, readiness, and tool category summary.
- Added `infra_lab.what_can_i_do` tool for categorized MCP capability discovery and safe execution flows.
- Added MCP tool discovery sprint design documentation.

### Changed

- Updated MCP user guide with setup menu, setup check field explanations, and tool catalog documentation.
- Updated agent workflow guidance to call `setup_check` and `what_can_i_do` before summarizing available actions.

### Validated

- `make test-mcp`
- Local MCP `tools/list`
- Local MCP `infra_lab.what_can_i_do`
- Local `infra-lab-mcp --doctor`

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
