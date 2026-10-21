# agent-runner State Machine

The runner is a single-task state machine. A process can be in only one state at a time.

## States

- `IDLE`: no task is owned by this runner.
- `DISCOVERED`: an eligible candidate was found locally.
- `CLAIMING`: runner is attempting an atomic claim.
- `CLAIMED`: the task row records this runner's session ID and lease.
- `STARTING`: repository checks passed and launch is being prepared.
- `RUNNING`: a Codex session or dry-run launcher is active.
- `CHECKPOINT_STALE`: a task has an expired lease but recent checkpoint ambiguity prevents takeover.
- `COMPLETED`: Notion has completion evidence after the child exits.
- `BLOCKED`: Notion has blocked evidence after the child exits.
- `RECOVERING`: runner is reconciling a session after restart or SIGTERM.
- `RELEASED`: claim was released or no longer belongs to this runner.
- `FAILED`: local runner failure; task is not completed unless Notion says so.

## Transition Summary

```text
IDLE -> DISCOVERED -> CLAIMING -> CLAIMED -> STARTING -> RUNNING
RUNNING -> COMPLETED -> IDLE
RUNNING -> BLOCKED -> IDLE
RUNNING -> FAILED -> RELEASED -> IDLE
DISCOVERED -> CHECKPOINT_STALE -> IDLE
CLAIMING -> IDLE              (atomic claim lost)
STARTING -> FAILED            (safety validation failed)
RECOVERING -> RUNNING         (owned active session found)
RECOVERING -> RELEASED        (no safe session to resume)
```

## Claim Eligibility

A task is eligible only when all are true:

- Lane equals the runner lane.
- Status is runnable for the lane.
- No valid session ID and lease are present.
- Expired leases are paired with stale checkpoint evidence before takeover.
- Repository, default HEAD SHA, target branch, and PR are present.
- Repository is allowed.
- Work category is not UI/UX, deploy, deletion, merge, force-push, or main push.

## Checkpoint Freshness

Lease expiry alone is insufficient for takeover. The runner evaluates checkpoint freshness separately:

- If checkpoint is newer than the configured stale threshold, mark `CHECKPOINT_STALE` and skip.
- If checkpoint is absent, do not assume completion; skip unless the row is explicitly assigned for recovery.
- If checkpoint is stale and the lease is expired, a later PR may implement takeover. PR1 only logs the decision in dry-run.

## Completion Rule

Codex process exit is never completion evidence. The runner checks Notion after exit:

- Completion evidence present: `COMPLETED`.
- Blocked evidence present: `BLOCKED`.
- Neither present: `FAILED` or `RELEASED`, depending on claim ownership and adapter behavior.
