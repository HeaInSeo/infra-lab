# Repository work rules

## One active modifying branch and PR

- Use `main` as the integration branch. Keep at most one ACTIVE modifying branch and PR for this repository, including directly requested operational fixes.
- Before changing code, read the current remote branches and open PRs. Reuse the existing ACTIVE branch and PR for corrections and follow-through. Do not create a second branch or stacked PR for the same work.
- A draft PR is not an exception to this limit. A preserved draft must be explicitly marked `[PARKED]`, with its exact head, remaining blockers and observable resume trigger recorded in its description.
- PARKED work does not occupy the ACTIVE lane. Preserve its code and findings. Resume only after the current lane is released and its scope, ownership and repository state are freshly checked; reuse its original branch and PR.
- Classify branches as ACTIVE, PARKED or superseded/merged before cleanup. Do not delete an UNKNOWN branch or discard unmerged code merely to reduce the branch count.
- For retired branches, verify integration or patch equivalence, preserve the original commit with a remote archive tag when needed, read back that tag, and use an expected-head lease for deletion. Stop if any source head changes.
- Keep `main` landing gates and normal reviews in force. Branch consolidation does not authorize deployment, runtime-state changes, fabricated worker ownership or bypasses.

Dated branch inventories are evidence snapshots, not execution authority. Re-read GitHub before starting the next task.
