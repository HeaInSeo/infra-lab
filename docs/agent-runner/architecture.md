# agent-runner Architecture

`agent-runner` is a local, single-machine scheduler for Codex work assigned through the Notion Agent Report Log. It is intentionally conservative: it runs one fixed lane, handles one task at a time, and refuses work unless the Notion row contains enough repository and execution metadata to resume safely.

## Goals

- Poll the Agent Report Log for work assigned to one lane: `operator` or `guardrail`.
- Claim at most one eligible task while the runner is idle.
- Record a session ID, lease, checkpoint, event IDs, and a LIVE RESUME CAPSULE back to Notion.
- Validate local repository state before starting any Codex session.
- Launch only predefined Codex templates for the task role.
- Re-check Notion after Codex exits; never mark local completion unless Notion contains completion or blocked evidence.
- Return to `IDLE` after a task reaches a terminal or released state.

## Phase 1 Scope

The first PR implements only local dry-run behavior and a fake Notion adapter:

- CLI program.
- Single machine.
- Single lane per process.
- One task at a time.
- Five-minute polling default.
- Fake Notion read/write adapter backed by JSON test data.
- Local Git repository validation.
- Codex launcher interface with dry-run/fake launch only.
- Structured JSON logs.
- Graceful shutdown.
- Unit tests.
- systemd user service example.

Actual Notion API writes and real Codex process execution are deliberately excluded until the second PR.

## Components

```text
CLI flags/env
  -> Config
  -> Runner loop
     -> NotionAdapter (fake in PR1)
     -> TaskSelector
     -> SafetyValidator
        -> RepoAllowlist
        -> GitInspector
     -> ClaimWriter (dry-run/fake in PR1)
     -> Launcher (dry-run/fake in PR1)
     -> CompletionVerifier
     -> JSON Logger
```

## Trust Boundaries

- Notion task rows are coordination data, not executable authority.
- Local configuration owns repository allowlists and launch templates.
- Shell commands are never accepted from Notion.
- Secrets stay in environment variables or user credential stores; they are redacted from logs and never written to Notion.
- UI/UX, deployment, deletion, main push, merge, and force-push tasks are non-runnable.

## Lane Model

A runner is started with exactly one lane:

- `operator`: may modify only the task's explicit repository and branch.
- `guardrail`: read-only by default; write-capable guardrail tasks require a later explicit design change and are out of PR1.

The runner refuses to start if the lane is not one of those two values.

## Repository Model

Each runnable task must include:

- Repository identifier, such as `HeaInSeo/NodeSentinel`.
- Default HEAD SHA observed by the supervisor.
- Target branch.
- PR identifier or URL.
- Role/template key.

The local runner validates that:

- The repository is in the allowlist.
- The path exists and is a Git repository.
- The default remote HEAD matches the row's expected SHA, or dry-run reports the mismatch.
- The current branch is the task branch when a modifying operator task would run.
- The worktree is clean before a modifying task.

## Out Of Scope Until User Decision

- Webhook execution.
- Public server endpoints.
- Remote shell execution.
- Automatic PR merge.
- Automatic deploy.
- Destructive database schema changes.
- System-wide Notion secret storage.
