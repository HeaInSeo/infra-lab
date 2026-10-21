# Codex Launch Contract

`agent-runner` never launches arbitrary shell from Notion. A Notion row chooses a local template key, and the local runner expands that template with validated fields.

## PR1 Behavior

PR1 includes only:

- `dry-run` launcher: logs the intended launch and returns exit code 0.
- `fake` launcher for tests.

No real Codex process is executed in PR1.

## Future Real Launch Inputs

The second PR may add a real launcher with these validated inputs:

- `session_id`.
- `lane`.
- `role`.
- `repository`.
- `repo_path`.
- `default_head_sha`.
- `target_branch`.
- `pr`.
- `task_id`.
- `resume_capsule`.

## Template Rules

- Template names are local allowlisted constants.
- Templates define argv arrays, not shell strings.
- No shell interpolation.
- Environment allowlist is explicit.
- Secrets are passed through environment only when needed and are redacted from logs.

## Denied Actions

The launcher must not run tasks involving:

- UI/UX implementation.
- Deployment.
- Data deletion.
- Main push.
- Merge.
- Force push.
- Remote shell execution.

## Exit Handling

Exit code is local evidence only. After exit, the runner re-reads Notion:

- If Notion says completed: terminal `COMPLETED`.
- If Notion says blocked: terminal `BLOCKED`.
- Otherwise: local `FAILED` or `RELEASED`, never completed.
