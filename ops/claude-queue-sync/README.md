# Queue source synchronization

QSYNC-20261010-v1 fixes stale queue intake at the existing Operator and Guardrail
PAYLOAD boundary. The Router still owns scope and HOLD; authentic task receipts own
execution state. Worker events update body and properties, then read back both.
Central projections carry source keys and observation times and never manufacture
worker proof. Notion updates are not atomic: partial updates are explicit pending
states, and a newer source always invalidates a cached projection.

The patch changes only the PAYLOAD heredoc. Locks, usage/authentication gates,
tools, mode, deadlines, service units and hourly timers remain byte-identical.
The old unconditional SCHEDULED label is replaced by an observed-origin label.

Run `python3 -m unittest -v test_queue_sync.py` here and the existing integrity tests.
Then `python3 apply_queue_sync.py` validates and previews both candidates.
`python3 apply_queue_sync.py --apply` requires both existing services to be
inactive and both role locks plus the existing repair lock to be available.
Candidates pass Bash syntax and are saved as protected versioned backups before
the protected manifest switch and atomic live replacement. Old backups survive.
A failed live swap rolls back the manifest and wrappers. The existing integrity
pre-start hook can recover an interrupted manifest-to-wrapper transition.

No worker is started by deployment. A configured service receipt must record
the contract and actual source events before actual worker adoption is claimed.
Central automation and the read-only queue watcher require the same source rule.
