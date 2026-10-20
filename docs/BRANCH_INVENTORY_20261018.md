# Branch inventory — 2026-10-18

This is a dated maintenance snapshot, not a task authorization or runtime claim.
The repository policy is `main` plus at most one ACTIVE modifying branch/PR.

| State | Branch | Exact preserved input | Disposition / next trigger |
| --- | --- | --- | --- |
| Integration | `main` | `05d761f2c7b9c54c6b24fc3b47336ac79f786a6c` | Unchanged by this cleanup |
| ACTIVE lane | `fix/notion-projection-guard-20261012` / PR #41 | consolidation `b88563954f80a7c16612b3c661606c10334e9649` | Sole modifying lane; continue existing PR #41 and its current-head gates |
| PARKED | `agent-runner-dry-run` / PR #39 | `829777a860de1f3c4a2a3f6d19d7919ca52a7c89` | Preserve five unresolved P2 findings. After #41 lane release, fresh scope/ownership review determines same-PR correction; no automatic resume |
| PARKED | `fix/queue-source-sync-20261010` | `6f3fc2c781bd9c065fe30287b6e883554c63239e` | Preserve queue-sync and wrapper-integrity commits. After #41 lane release, reconcile against landed code and the approved deployed source before selecting only the remaining work |
| Consolidated | former `fix/npg-transport-recovery-20261013` / PR #42 | `c353d1779927ac7e29deb21f81e61c5e742d6313` | Normally merged into #41's branch, preserving both parents; source branch automatically deleted |

## Retired MCP stage branches

Original commits were preserved by the remote tags below before expected-head, atomic branch deletion. The stage implementation patches match the replacement history in PR #16; the final `mcp` subtree and seven stage execution documents match exactly. PR #16's reviewed and merged trees both equal `7f4dc45a833c040a8c61f366d742075a79700608`. The five nonmatching original history commits belong to the earlier JSON contract now carried by squash PR #8; no original commit is discarded by these archive tags.

| Former branch | Preserved commit | Remote archive tag |
| --- | --- | --- |
| `codex/stage1-readonly-mcp` | `a14d779ef3a5a26080068e90bf7583c99a78fb16` | `archive/20261018/codex/stage1-readonly-mcp` |
| `codex/stage2-snapshot-mcp` | `24420a781373d1ae7eddc1a21382a4cffb671010` | `archive/20261018/codex/stage2-snapshot-mcp` |
| `codex/stage3-plan-only-mcp` | `43331aab29193d7ec828323dba251d10ed80d683` | `archive/20261018/codex/stage3-plan-only-mcp` |
| `codex/stage4-profile-write-mcp` | `1ca2506feca0ed2c42af929dc70c071c7b1371b5` | `archive/20261018/codex/stage4-profile-write-mcp` |
| `codex/stage5-approved-safe-mutation` | `320adbbf70f423afb363640260a074018b14a919` | `archive/20261018/codex/stage5-approved-safe-mutation` |
| `codex/stage6-approved-env-up` | `abb7eb9bebadb0240534b839a2775f82d2e00700` | `archive/20261018/codex/stage6-approved-env-up` |
| `codex/stage7-destructive-operations` | `e11111ddf11b40bbf444a9e9b9e642f6192c4198` | `archive/20261018/codex/stage7-destructive-operations` |

## Validation

The #41/#42 integration was tested in an isolated checkout on Node 22.22.0: 72 tests passed, zero failed. Relative to reviewed recovery head `c353d177`, the integration adds only the two preserved verification documents from #41. This cleanup additionally records repository workflow and branch inventory documents; it does not modify executable code, deployed files, the Notion journal, services or timers. Final remote inventory: four heads, one selected modifying lane, two preserved PARKED branches and seven verified archive tags.
