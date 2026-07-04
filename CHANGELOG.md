# Changelog

## v0.7.5 - 2026-07-04

### Added

- Added the `lustre-lab` OpenTofu root module for an isolated single-node Lustre lab VM, including a libvirt pool, Rocky base image volume, OS disk, dedicated Lustre target disk, cloud-init seed, and VM domain.
- Added `infra_lab.tool_catalog`, a read-only MCP introspection tool that reports the actual registered MCP tools with category, risk, destructive status, approval requirement, source, stage, and required capability gates.
- Added MCP tool catalog documentation and included `lustre-lab` in HCL validation.

### Fixed

- Updated the Lustre lab module to the current `dmacvicar/libvirt` provider schema and added provider lock coverage for the new module.
- Ignored local `terraform.tfvars` and `tofu.tfvars` files to match the documented secret-handling guidance.

### Validated

- `make test-mcp`
- `make test-contract`
- `make test-go`
- `make lint-hcl`
- GitHub Actions checks on PR #17.

## v0.7.4 - 2026-07-02

### Added

- Added `Env.ReadOSRelease()`; `ilab vm version` (text, `--json`, and `infra_lab.vm_version` over MCP) now reports guest OS info (`os.id`, `os.prettyName`, `os.versionId`, `os.versionCodename`) read live from `/etc/os-release`, distinct from a profile's requested `vm.osImage`.
- Added a `Backend` field to `VMInfo`; `ilab doctor`'s "VMs (all backends)" table, `ilab vm list --all`, and `infra_lab.doctor`/`infra_lab.vm_list_all` now show which provider (multipass/libvirt) reported each VM, including unmanaged ones.

### Fixed

- Fixed `ListAllVMs` computing VM→env attribution with its own first-prefix-match loop instead of the disambiguation logic added for #25; `ilab doctor`'s VM table was still misattributing VMs to a stale env when name_prefixes collided. Extracted `resolveEnvForVMName` as the single shared resolver used by both `FindEnvForVM` and `ListAllVMs`. (#23)

### Validated

- `make test-go`
- End-to-end on remote host via the actual MCP JSON-RPC protocol: `infra_lab.vm_version` returns correct guest OS info for `lab-master-0`; `infra_lab.doctor` shows `backend: libvirt` for all VMs (including unmanaged `tori-lustre-lab`) and correctly attributes managed VMs to `test-wizard-env` instead of the stale env.

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
