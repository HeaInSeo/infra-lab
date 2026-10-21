# Notion Contract

The Agent Report Log is the durable coordination plane. This document describes the row fields `agent-runner` expects. PR1 uses the same contract with a fake JSON adapter.

## Required Fields For Runnable Rows

- `task_id`: stable task identifier.
- `lane`: `operator` or `guardrail`.
- `status`: runnable status for the lane.
- `repository`: exact repository identifier, for example `HeaInSeo/NodeSentinel`.
- `default_head_sha`: default branch HEAD SHA observed by the supervisor.
- `target_branch`: branch the task is allowed to touch.
- `pr`: PR number or URL.
- `role`: launch role/template key.
- `work_category`: category used for safety filters.

Rows missing any required field are skipped and logged.

## Claim Fields

- `session_id`: runner-generated unique session ID.
- `lease_until`: RFC3339 timestamp.
- `checkpoint_at`: RFC3339 timestamp.
- `checkpoint_event_id`: event ID for the last checkpoint.
- `live_resume_capsule`: plain text capsule with repo, branch, PR, phase, and next exact action.

## Event Log Fields

Every state mutation records:

- `event_id`.
- `event_type`.
- `runner_id`.
- `session_id`.
- `state`.
- `timestamp`.

PR1 records these fields only in fake adapter memory and JSON logs.

## Runnable Statuses

PR1 defaults to:

- `실행재개`
- `ready`
- `Ready`

Deployments may configure this later, but status broadening must not bypass lane and safety checks.

## Completion Evidence

After a Codex session exits, the runner checks Notion:

- Completion evidence: task status is `완료`, `완료보고`, `completed`, or has an explicit completion event.
- Blocked evidence: task status is `차단`, `blocked`, `DESIGN_GAP`, or has an explicit blocked event.

If neither is present, the runner does not mark the task complete.

## Atomic Claim Requirement

The real Notion adapter must perform compare-and-set semantics:

- Row still has no valid lease/session.
- Row version or last edited timestamp matches the value read during discovery.
- Claim fields are written in the same operation.

PR1 fake adapter enforces the same semantics in process with a mutex.
