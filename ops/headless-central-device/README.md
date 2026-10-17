# Headless central supervision device

The existing hosted central supervisor can reach the development server directly through a registered Remote Desktop Commander device. Operator and Guardrail keep their existing services, timers, usage gates and ownership rules. No additional LLM worker is created.

## Install

This directory pins Remote Desktop Commander 0.2.51 and its dependency lock. Run `./install-user.sh` as the development-server user. It installs only a user service and does not start or enable it automatically. It requires Node.js 18+ and an existing systemd user manager. Confirm `loginctl show-user "$USER" -p Linger` reports `Linger=yes` for operation after logout.

Start `systemctl --user start desktop-commander-remote.service`, then read `journalctl _SYSTEMD_USER_UNIT=desktop-commander-remote.service --since -2min --no-pager -o cat`. Use the printed verification URL and code to authenticate this new server device with the same Remote Desktop Commander account as the existing assistant connection. This is the provider's OAuth device flow. The explicit user-unit journal selector also works on the development server's systemd 239, where the journal's --user filter does not return these records. Do not copy the laptop's saved session or create another account.

After the device appears online, enable the unit: `systemctl --user enable desktop-commander-remote.service`. Verify its persistent session remains private (0600). Restart the service once and check that the same device reconnects without another browser login.

## Central cutover

1. Read the actual online server device ID from the connector. Check its user, host and user-service status using a direct device call.
2. Check the original Notion projection guard's approved hashes, policy and journal. No unfinished batch is allowed before changing the central execution route.
3. Preserve the pinned `runner.js` revision and use the new server device ID with the local command `cd /home/heain/.local/share/notion-projection-guard && sha256sum -c approved.sha256 >/dev/null && exec node broker.cjs --stdio --state /home/heain/.local/state/notion-projection-guard --policy /home/heain/.local/share/notion-projection-guard/policy.json`. There is no laptop or SSH hop in this route.
4. Verify a bounded central observation update through the existing guard, including full protected-content readback and durable ACK. Then update the existing hosted central prompt to this verified server device and local command. Preserve its cadence, normal merge gates and the single guard state.
5. Check a subsequent real worker handoff independently. Configuration storage or a service PID is not worker intake or product acceptance.

The guard's allowlist, production policy, state directory, snapshots and raw-write prohibition are unchanged. Never fall back to direct Notion body edits or a second state after an error. The laptop remains optional for inspection and manual intervention.

## Recovery

On a temporary network failure, the official agent reconnects. The user service restarts on process failure with a bounded start rate. A revoked or expired provider authorization requires authenticating this same device again; do not substitute laptop credentials. Disable only this unit to return to the previously recorded central route; do not alter the Operator or Guardrail units.

Official setup: https://github.com/desktop-commander/remote-desktop-commander/blob/main/docs/SETUP.md
Session persistence: https://github.com/wonderwhy-er/DesktopCommanderMCP/blob/main/src/remote-device/README.md
