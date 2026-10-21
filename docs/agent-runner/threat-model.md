# agent-runner Threat Model

## Assets

- Local source trees and uncommitted user changes.
- GitHub branches and PR integrity.
- Notion Agent Report Log state.
- Codex credentials and other local secrets.
- Operator attention and task ordering.

## Primary Risks

## Arbitrary Command Execution

Risk: a Notion row injects shell text that the runner executes.

Mitigation:

- Notion rows select a template key only.
- Templates are local code/config, not row-provided commands.
- PR1 implements only a dry-run/fake launcher.

## Duplicate Work Or PR Collisions

Risk: two runners modify the same repository or PR.

Mitigation:

- One runner handles one task.
- Atomic claim with session ID and lease.
- Repository allowlist.
- Refuse runnable tasks without repository, branch, and PR.
- Refuse valid existing session/lease.
- Verify open PR state before creating any future PR.

## Lease Theft

Risk: an active runner loses network temporarily and another runner steals the task.

Mitigation:

- Lease expiry is not enough.
- Checkpoint freshness must also be stale.
- PR1 does not implement takeover; it only reports stale/ambiguous states.

## Secret Disclosure

Risk: logs or Notion updates leak tokens.

Mitigation:

- Structured logs redact known secret-looking keys.
- Notion rows receive event IDs and state summaries only.
- No system-wide secret storage is introduced.

## Unsafe Work Categories

Risk: automation starts UI/UX, deployment, deletion, merge, or force-push tasks.

Mitigation:

- Hard denylist in safety validation.
- Guardrail lane is read-only by default.
- Operator lane can only use the task branch.

## Local Dirty State Loss

Risk: runner overwrites user work.

Mitigation:

- No cleanup commands.
- No checkout/reset in PR1.
- Dirty modifying worktrees are rejected.

## Fake Completion

Risk: Codex exits successfully but did not record evidence.

Mitigation:

- Exit code is not a terminal decision.
- Runner re-reads Notion evidence after launch and refuses to mark completion itself.
