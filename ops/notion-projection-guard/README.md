# Notion projection guard — U0

## Complete NodeKit/Sori observations

`complete_projection.js` is the Code Mode entry point for these observations.
It requires a fixed manifest: both native central metadata records, the repo
index, both role queues, Router, hub and dashboard. Sori also requires its two
already approved native central body sections. A combined observation therefore
has nine body targets and four metadata targets. Missing or duplicate targets,
wrong headings or different observation IDs reject before the body runner starts.

Evaluate the file as a function expression and call
`createCompleteProjection(tools, {createRunner, runnerConfig}).run(bundle)`.
`createRunner` and `runnerConfig` remain the separately approved pinned runner
and canonical broker command. The bundle contains `observationId` (ASCII letters,
digits, underscore and hyphen), `repositories` (`nodekit` and/or `sori`), ordinary
`edits`, and `native: [{pageId, properties}]`. Each new body literal and each native
`큐 정리 근거` must contain the identical `observation_id=` field with a backtick
quoted ID. Native expectations must include 작업 상태, 다음 담당자, 큐 구획,
차단 대상, 재개 지시 and 큐 정리 근거; only central metadata keys are accepted.

First check the canonical journal and authority, then reconcile native properties
using the existing minimal property-only preservation procedure. The entry point
does not write properties, worker proof or policy. It requires their fresh values
before any body write and verifies all complete bodies/properties after the
ordinary durable body batch. Only this final readback returns `COMPLETE`.
`COMPLETED` from a caller-selected broker batch is insufficient. A concurrent own
event, stale property, unknown read or uncertain write produces no completion
certificate; preserve existing writes and re-observe rather than rolling back.

This is workflow enforcement when the supervisor uses this entry point, not an
access-control claim that it cannot invoke a different connector. Adoption must
be recorded in the actual supervisor prompt and verified on real observations.

## Exact transport pre-prepare closure

`reconcile_transport_preprepare.cjs` handles only the recorded seven-page
`QSYNC-20261019T132702Z-NK138-LANDED-SORI43-RECIRCULATE` incident. It pins the
plan/error/ordered scope/runtime and verifies its first full receipt plus seven
fresh pages. Any later snapshot, pending ticket, changed completed page, already
written remaining literal, altered evidence or policy rejects closure.
It defaults to check-only. Explicit `--apply` privately preserves the full journal,
fresh sources and receipt hashes, marks only this incident `ABORTED_UNWRITTEN`
and clears its active barrier. Its first write remains verified; the remaining
six edits are not replayed. No Notion write, ACK, index advance, runtime replacement
or policy expansion occurs. A new complete observation uses the normal runner.

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
Replayed legacy READY batches validate every remaining edit as well. The same
check runs in prepare and authorize, so a saved PREPARED ticket cannot bypass it.

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
Both recovery paths use the same canonical UUID scope check as initial admission,
while retaining the exact original edit spelling and immutable plan hash.

This entry point is never called automatically by `run` or `resume`. Its hashes
come from a reviewed original incident, not from a replacement batch or a manual
journal reset. A subsequent normal update still uses fresh preflight, the single
canonical state, protected-content verification and durable ACK.

### Explicit recovery of central arrow escaping

A later six-page central batch halted on its first page: the connector returned
the two plain ` -> ` sequences as ` -\> `. New writes reject plain ASCII arrows
outside inline code/links (`MARKDOWN_ESCAPE_RISK`); use `→` or inline code.
The ordinary verifier still requires exact equality.

The separately invoked broker action `reconcile_escaped_arrows` requires that
same six-page, index-zero HALTED plan and its original ticket, full snapshots,
hashes and fresh revision. Only those two arrow escapes inside the intended
central literal are admitted; properties and every protected byte must match.
All remaining edits must pass the current policy and rendering preflight.
The action writes no Notion content. It saves the reviewed first-page receipt
and advances the original batch only to READY at index one. Run the unchanged
original plan to apply its remaining five pages through normal verification.
The audited receipt is also revalidated if a later unwritten transport recovery
occurs, without rewriting the first page. No new batch/state, journal reset,
general Markdown normalization, skipped page or policy expansion is involved.
Calling `resume` with an unsupported identity leaves the current batch unchanged;
in particular it cannot re-halt the READY batch after arrow reconciliation.
Likewise, a rejected `run` admission cannot halt somebody else's existing plan.
Inline code treats backslashes as literal content, including Windows paths.

Run `node --test test_guard.cjs`. The suite exercises the 4,000-character range,
tail loss, native child references, worker evidence, ambiguous scope, concurrent
revision/properties, snapshot tampering/storage failure, lost ACK/restart,
async failure and the actual connector orchestration's stop-before-next-write.

### Exact Sori W02 central anchors

The optional `centralSections` policy enables only the two legacy W02 writer and
paired-review central headings listed in `policy.json`. The broker pins both
page/heading pairs in code and rejects unknown, duplicate, broadened or malformed
entries before opening state. These task pages cannot be canaries. Production
must be enabled; the checked-in default remains disabled.

These two pages receive no general heading-prefix permission, even if their IDs
are added to `productionPages`. Their existing headings stay unchanged as stable
anchors. No worker preflight, event, history, property, append or whole-page
operation is supported. The same unique single-line limit, real following
boundary, durable snapshot, fresh authorization and complete readback apply.
Scope is checked again at prepare and authorize, including a prepared ticket
reopened after the opt-in is removed or production is disabled. A rejected ticket
stays blocked; no journal reset or replacement batch bypass is introduced.

For activation, keep the existing canonical state and all runtime policy values;
add only the two exact `centralSections` entries. Verify the candidate with a
separate non-production test state, inspect that the canonical journal is idle,
and acquire its existing broker lock before replacing approved runtime files.
Keep an exact private backup and hash manifest. Stop on a busy lock or source
change. Re-run file hashes and test rejection paths before any live write, then
verify the two actual complete page readbacks. Test fixtures are not worker
receipts or product acceptance, and full fetched equality is not atomic Notion CAS.

`reconcile_w02_prewrite.cjs` is a separately invoked, single-incident recovery for
the W02 projection batch halted before its first prepare when Notion stored a
queue-cleanup timestamp at minute precision. It pins that batch, plan, error,
zero-progress/no-ticket state, absence of prepared snapshots, the completed
two-source batch and its full durable receipts, and exact fresh native values.
It acknowledges no write, advances no page and emits no Notion request. A private
audit restores only READY on the same plan, after which ordinary fresh prepare,
authorization and full readback still apply. `--check-only` changes no journal.
Any other incident, body/worker-property drift or prepared write remains blocked;
the normal broker verifier admits no timestamp or content normalization.

### Explicit recovery of the 2026-10-19 preflight connection failure

`reconcile_preflight_connection.cjs` is a separately invoked, single-incident
recovery for `QSYNC-20261019T0245Z-SORI40-LANDED-RWC1-ACCEPTED-NK138-CODEX-RUNNING`.
That immutable nine-page batch halted with exactly
`REMOTE_ERROR: McpServerError: Connection failed.` after one verified page and
before preparing page two. The recovery requires next=1, no pending ticket,
no pending snapshot/readback, the pinned plan and unchanged approved runtime.
It validates the first page's complete durable snapshot/receipt and its fresh
body, properties and revision. All nine fresh ordered reads must be complete;
each remaining page must still have the unique original literal and no planned
output inside the exact policy-authorized central section.

The helper emits no Notion request, acknowledges no write and advances no page.
`--check-only` leaves the journal unchanged. Explicit application saves a private
full audit, keeps the original plan/error/next/ticket and restores only READY.
The unchanged approved runner then executes the original batch, starting at
page two, through ordinary prepare, fresh authorization and exact full readback.
The already verified first page is never written again. Other errors, identities,
prepared writes, changed receipts/pages, scope removal and unknown results stay
blocked. No runtime broker, runner, policy or verifier normalization is added.
Live pages and receipt copies remain outside the repository.

### Explicit closure of the 2026-10-19 stale Router preflight

`reconcile_router_stale_prewrite.cjs` is a separately invoked, one-incident closure,
not an ordinary broker action or a general HALTED reset. The final Router edit
in `QSYNC-20261019T082053Z-SORI42-RELEASE-ADMISSION` matched zero old literals:
the actual central field contained additional retained text. No final-page
snapshot, ticket or authorization existed. Its proposed REVIEW_REQUESTED state
was subsequently superseded by the existing paired worker's real RELEASE v12.

The helper requires the exact original five-page plan/error/next=4, all four
complete durable snapshots and verification receipts, the original approved
runtime/policy, five fresh ordered complete page reads, the exact reviewed
Router body/properties/revision and the successor's own release/head. The complete
paired worker body and revision are also pinned: retaining an old RELEASE while
appending a later CLAIM/HOLD or moving its JSON to quoted history rejects closure.
Later worker records on other completed pages are saved and never rewritten. Any prepared
final-page evidence, unknown ACK, changed scope or receipt, concurrent Router
change, absent successor release or production hold rejects the closure.

Default invocation is check-only. Explicit `--apply` first saves the full original
journal and fresh reads privately, then records `ABORTED_UNWRITTEN` on that
original batch and releases its active barrier. The immutable plan/hash,
next=4, original error, null receipt, other batches and earlier evidence remain
unchanged. It performs zero Notion writes, acknowledges zero writes and advances
zero pages; this status is not COMPLETED. Replaying the old plan cannot write.
A fresh, bounded central observation must use the latest worker/GitHub source
and the ordinary broker's preflight/write/full-readback/ACK path after closure.
No runtime broker/runner replacement, policy expansion or new state directory
is involved. A crash after the private audit but before closure remains blocked
and requires separate reconciliation; do not delete that audit to retry.


## Inline field boundaries and whole-batch preflight

The broker now rejects incomplete backtick spans in old and new literals. The
planner also checks each selection against the complete affected line, rejecting
an edge inside an existing span. Escaped ticks outside code and literal backslashes
inside code remain distinct. This is a conservative single-line editing grammar,
not a general Markdown normalizer. The exact verifier remains unchanged.

The runner validates admission without claiming a journal batch, then preflights
all remaining complete target pages through that planner before `begin` or any
Notion mutation. Each ordinary prepare/authorize repeats the check on fresh reads.
A bad later selection therefore produces zero writes and no new HALTED batch.
This is required for both direct guarded updates and the complete-manifest entry.
Deploy the reviewed broker and runner together under verified runtime pins.

`reconcile_v02_partial.cjs` is explicitly restricted to the original nine-page
CENTRAL-20261019T153711Z-NK139-SORI43-GATES incident. It pins all eight complete
snapshot/receipt files, the immutable plan/ticket and the single recorded central
suffix escape, and requires nine fresh full readbacks. It never accepts that
escape as unchanged protected content. Explicit apply saves a private audit and
marks the same batch RECONCILED_PARTIAL, retaining next=3, ticket, error, edits
and original planHash. No Notion write, successful ACK or page advance occurs.
The five untouched old instructions are not replayed. A separately guarded bounded
repair restores the original central V01/V02 fields and valid code boundaries;
a fresh complete observation then consumes the later genuine NodeKit paired PASS
alongside Sori's PASS. Only that full readback can establish current synchronization.
The helper defaults to check-only and cannot be called automatically by run/resume.
