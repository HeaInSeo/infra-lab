# Notion projection guard — U0

The hosted central supervisor previously called the Notion connector directly with
a model-computed selection. On 2026-10-11T23:34Z it reported overwriting about
4,000 characters across eleven pages. The exact faulty selection code is not
retained in this repository, so an off-by-one or particular parser bug is not
claimed. The observed failure mechanism is an unchecked selection followed by
continued writes, without a durable per-page preservation barrier.

`broker.cjs` now constructs the connector request. `runner.js` calls that request
through the existing authenticated Notion connector. Neither file contains or
extracts a Notion token. The broker runs via the existing Desktop Commander →
`heain@100.92.45.46` route. No Claude wrapper, timer, session, usage policy, or
worker-owned proof is changed.

## Enforced update sequence

1. Bind an observation ID to exact ordered page IDs and literal edits. A prior
   pending/HALTED batch blocks any new batch. A completed ID cannot issue a write.
2. Fetch the entire enhanced-Markdown body and properties. Reject wrong IDs,
   unknown/truncated blocks, unbounded/ambiguous sections and unsafe syntax.
3. Accept a unique, single-line literal of at most 1024 characters, exclusively
   in an explicit central observation/action/incident section. Reject headings,
   nested headings, worker own records, synced blocks and native child blocks.
   No offsets, ellipsis selection, full replacement, append or replace-all exist.
4. Save the full raw preimage, body, properties, planned output and hashes with
   private permissions, fsync, atomic rename, directory fsync and readback.
5. Fetch again and compare body, properties and revision. Save WRITE_PENDING
   before returning the one generated `update_content` request.
6. Await the connector operation and any recognized async task. Fetch again.
   Require the entire expected body and all protected prefix/suffix/properties
   to match. Only a durable verified receipt unlocks the next page.
7. Any discrepancy or unknown result halts the batch. Keep the preimage and
   unexpected readback; do not rollback, blindly retry, or proceed to a new batch.

This is a check-and-detect barrier, not remote atomic CAS. Notion can change
between the final preflight and the write; exact-literal scope limits that write
and full readback detects the change before the next page. Body hashes prove
equality of the fetched enhanced-Markdown representation, not preservation of
invisible native block identities. Omitted native completeness metadata is not
invented as a positive guarantee; explicit unknown/truncation and unknown tags
are rejected.

## Hosted automation integration

Fetch `runner.js` from the approved full GitHub commit, unwrap its `content`, and
evaluate it as a function expression in Code Mode. Supply the verified fixed
remote command and the observed Desktop Commander device ID. The generated
request is the only Notion body mutation used by this execution path:

```js
const createRunner = Function('return ' + runnerSource)();
const runner = createRunner(tools, { deviceId, command: approvedBrokerCommand });
const result = await runner.run(observationId, [
  { pageId, heading: exactFetchedCentralHeading, oldStr, newStr }
]);
```

Create a fresh runner for each batch. One minimal literal per page per batch is
intentional: additional edits require a new bounded batch after verified ACK.
Never use a new batch ID to bypass unresolved state. Do not manually manufacture
ACKs, clear the journal, or use a new state directory for a production retry.

The hosted automation API currently exposes prompt/enabled/schedule updates,
without per-task tool allowlists or routing controls. This integration changes
the real calling path and enforces checks within it; it cannot revoke the
automation's raw connector capability. The task remains paused until restoration
and adoption are verified. Do not describe a prompt rule as capability isolation.

## Deployment and recovery

The approved branch is `fix/notion-projection-guard-20261012` in `infra-lab`.
The runtime is `/home/heain/.local/share/notion-projection-guard` and durable state
is `/home/heain/.local/state/notion-projection-guard`. Approved code/policy hashes
are checked before starting the broker. Runtime code is 0555; policy/manifest
0444; snapshots/journal 0600 within a 0700 directory. These modes protect against
accidental writes, not owner/root tampering. Snapshots stay outside the public
repository.

`productionEnabled:false` is the installed default. Only the two explicit private
NON-EXECUTABLE canary pages are writable. The eleven damaged production pages are
listed separately and stay blocked even if someone enables the hosted task early.
Keep latest NodeKit worker receipts; a whole-page rollback to before 08:34 KST
would erase later real worker evidence. Restoration and reconciliation are a
separate gate, not performed by this guard.

An ordinary close releases the process lock while preserving unresolved state.
A crash can leave `broker.lock`: inspect its recorded PID on heain and prove that
process is absent before removing only the stale process lock. Keep journal and
snapshot files. A pending write can be reconciled read-only against its saved
expected body; a mismatched/HALTED batch needs a reviewed bounded recovery.
No automatic HALTED reset or rollback is implemented.

### Unwritten authorize transport recovery

JSON-line broker calls disable Desktop Commander prompt matching and match only
the exact protocol request ID. Page JSON is never used as a prompt expression.

An explicitly invoked runner.resume(batchId, expectedPlanHash) can reconcile
only the observed DC invalid-prompt-regex failure while awaiting authorize.
It requires the original HALTED batch, unchanged plan/error/ticket and intact
private snapshots. All earlier full protected-content verification receipts are
revalidated against their preimages, planned output, native raw reads and hashes.
Those completed pages receive zero recovery writes, preserving later worker
records without restoring old snapshots. The unwritten pending page must still
match its entire fresh body/properties/revision preimage. An already-applied
output, changed pending page, other transport error, lost Notion ACK, async
failure or readback mismatch remains blocked.

The broker records a durable reconciliation audit and restores only PREPARED.
It keeps the page index, ticket, snapshots and edits; it does not acknowledge a
write or advance the batch. The ordinary fresh authorization, one connector
mutation, full verification and durable receipt still run for each remaining
page. A restarted recovery must recheck all earlier durable receipts from zero.
Use the same state directory and exact canonical batch; no journal editing or
new batch ID is a recovery path.

### Explicit recovery of the 2026-10-18 autolink mismatch

Notion rendered one bare `package-cli.sh` in a central observation as
`[package-cli.sh](http://package-cli.sh)`. The ordinary exact verifier correctly
halted. It continues to reject normalization; no general readback cleanup is
introduced. New batches reject unmarked hostname-shaped filenames and URLs
before claiming the journal or fetching Notion. Use inline code or an explicit
Markdown link so the planned representation is stable.

The separately invoked `runner.reconcileAutolink(batchId, planHash,
snapshotHash, actualHash, actualSnapshotHash)` reads Notion once and performs
zero Notion mutations. It admits only a single-page, index-zero original
`READBACK_MISMATCH` batch. The plan, original ticket, complete private preimage,
complete unexpected readback and their hashes must match. The only permitted
difference is the observed `package-cli.sh` autolink within the intended new
literal; protected prefix, suffix and properties remain byte-for-byte equal.
The fresh full page and revision must still match the saved unexpected readback.
Only then does the broker save a verified receipt and reconciliation audit and
complete that original batch. Other failures, multi-page recovery, changed links,
concurrent worker records, tampered evidence and production holds stay blocked.

This entry point is never called automatically by `run` or `resume`. Its hashes
come from a reviewed original incident, not from a replacement batch or a manual
journal reset. A subsequent normal update still uses fresh preflight, the single
canonical state, protected-content verification and durable ACK.

Run `node --test test_guard.cjs`. The suite exercises the 4,000-character range,
tail loss, native child references, worker evidence, ambiguous scope, concurrent
revision/properties, snapshot tampering/storage failure, lost ACK/restart,
async failure and the actual connector orchestration's stop-before-next-write.
