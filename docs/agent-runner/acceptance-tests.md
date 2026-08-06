# Acceptance Tests

PR1 acceptance is limited to local dry-run and fake adapter behavior.

## Required Tests

- Runner refuses invalid lanes.
- Runner selects only tasks for its configured lane.
- Runner skips rows with valid session ID and lease.
- Runner skips expired-lease rows when checkpoint is still fresh.
- Runner skips rows missing repository, default HEAD SHA, branch, or PR.
- Runner skips UI/UX, deploy, deletion, main push, merge, and force-push work categories.
- Runner enforces repository allowlist.
- Runner validates local Git default HEAD.
- Runner claims only one task while idle.
- Dry-run does not mutate fake Notion rows unless explicitly using fake write mode in tests.
- Fake atomic claim loses when row version/session changed between discovery and claim.
- Launcher dry-run logs intended template but executes no Codex process.
- Codex exit without Notion completion evidence does not become completed.
- Completion evidence in fake Notion produces `COMPLETED`.
- Blocked evidence in fake Notion produces `BLOCKED`.
- SIGTERM/SIGINT cancellation exits gracefully.

## Manual Dry-Run Acceptance

```bash
cd agent-runner
go test ./...
go run ./cmd/agent-runner \
  --lane operator \
  --dry-run \
  --once \
  --fake-notion examples/tasks.fake.json \
  --repo HeaInSeo/infra-lab=..
```

Expected output is structured JSON containing `DISCOVERED`, `CLAIMING`, and `STARTING` or a safety skip reason. No real Notion API call and no Codex process execution should occur.
