# Existing Claude runtime integrity

The 2026-10-10 Operator outage was caused by a read-only inspection command:
`ssh host grep -nE 'LOG|log|claude.out|tee' /path/to/wrapper.sh`.
The local shell removed the pattern quotes; SSH reconstructed the remote
command as a pipeline whose final `tee` truncated the executable. Earlier
pipeline errors were hidden by the successful `tee` exit status.

Quote the complete remote command as one SSH argument, preserving quoting
inside it. Use `shlex.quote` (or equivalent POSIX quoting), not JSON quoting.
Never pass a shell pattern as an unquoted reconstructed remote command.
Never write diagnostics into a runtime script. Use `set -o pipefail` in shell
diagnostics so a failed earlier command does not look successful.

The original Operator and Guardrail payloads, locks, usage gate, authentication,
tool permissions and timer cadence are retained. Wrappers use mode0555;
approved versioned backups and manifest use0444. This protects against accidental
writes by the ordinary runtime user, not deliberate owner/root tampering.

`claude_runtime_integrity.py check ROLE` runs as ExecStartPre of each existing
service. It checks owned regular files, exact SHA256, size, shebang, Bash syntax
and protection mode. Damage is recovered by complete write, fsync and atomic
replace from the exact validated approved backup. Invalid backup/manifest blocks
worker execution and records an alert. `report ROLE` runs as ExecStopPost and
records service failures including pre-worker EXEC errors. It never starts a
worker or changes the usage/authentication policy.

Use the existing role alert log and `.local/state/claude-runtime-integrity/integrity.log`
for supervisor health checks. A known-backup recovery is an internal runtime
repair; new source, permissions or product decisions still require their owner.
Future approved runtime updates must stage and validate the complete candidate,
retain the previous approved backup, update exact manifest data and atomically
replace under the integrity repair lock. Direct truncating writes are prohibited.

Run `python3 test_runtime_integrity.py` as the actual non-root runtime owner.
Tests reproduce the SSH quote loss on disposable files, check read-only protection,
zero-byte/checksum recovery, invalid-backup blocking, atomic replace failure and
pre-exec alert visibility. No live wrapper is damaged by tests.
