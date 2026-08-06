# Failure And Recovery

## PR1 Scope

PR1 handles graceful shutdown and local dry-run state. Full crash recovery and lease renewal are second-phase work.

## SIGTERM

On SIGTERM or SIGINT:

- Stop discovering new tasks.
- Let the active launcher receive context cancellation.
- Log a shutdown event with event ID.
- Do not mark the task complete.
- Do not clean worktrees or delete branches.

## Restart

PR1 starts in `IDLE` and reads fake adapter state. It does not reclaim real Notion work.

The future real adapter must:

- Find rows with this runner ID/session ID.
- Enter `RECOVERING`.
- Validate the live resume capsule.
- Continue only if the local state and Notion checkpoint match.

## Lease Expiry

Lease expiry does not imply safe takeover.

Safe takeover requires:

- Expired lease.
- Stale checkpoint.
- No fresher event from the owning session.
- Repository state still matches the row.

PR1 logs stale and ambiguous rows but does not take them.

## Local Git Failures

Repository validation failure prevents launch:

- Missing allowlist entry.
- Missing repository path.
- Default HEAD mismatch.
- Missing branch/PR.
- Dirty worktree for modifying operator tasks.

The runner logs `FAILED` locally and leaves Notion completion untouched.

## Notion Write Failures

PR1 fake adapter returns errors directly to the runner. The runner logs a failed event and returns to `IDLE` without pretending the claim succeeded.

The real adapter must persist retryable write failures as events when possible.
